package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

func dropProbes(ctx context.Context, owner int64) error {
	conn, err := pgx.Connect(ctx, *dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, `DELETE FROM agent_memories WHERE owner_id = $1 AND origin_id LIKE 'similar-check-%'`, owner)
	return err
}

// runSimilar checks similar-on-write on the real server path (MEMORY_STRENGTH collect or on): it writes a new
// value for existing settings of test world 4, then compares the returned candidates with the exact top 3 by
// cosine over the stored vectors (same owner and repo, active, other text, cosine >= 0.80). It also times writes.
func runSimilar(ctx context.Context) error {
	c, err := login(*apiURL, "synth@memeval.local", *password)
	if err != nil {
		return err
	}
	owner, err := ownerID(ctx, *dsn, "synth@memeval.local")
	if err != nil {
		return err
	}
	if err := dropProbes(ctx, owner); err != nil { // a rerun starts from the world alone
		return err
	}
	w := newWorld(4, 0.5, 1500)
	byID := map[int64]*synthMem{}
	for _, m := range w.mems {
		id, err := c.write(m.write)
		if err != nil {
			return err
		}
		m.id = id
		byID[id] = m
	}
	if err := waitEmbedded(ctx, *dsn, owner); err != nil { // the world must be embedded before probing
		return err
	}
	type probe struct {
		id      int64
		similar []int64
		took    time.Duration
	}
	var probes []probe
	for i, m := range w.mems {
		if m.slot.svc < 0 || len(probes) >= 40 || i%7 != 0 {
			continue
		}
		nv := (m.value + 1) % len(attrs[m.slot.attr].values)
		mw := m.write
		mw.Title = fmt.Sprintf("%s %s", w.svcName(m.slot.svc), attrs[m.slot.attr].name)
		mw.Body = fmt.Sprintf("Changed: the %s for %s is now %s.", attrs[m.slot.attr].name, w.svcName(m.slot.svc), attrs[m.slot.attr].values[nv])
		mw.OriginID = fmt.Sprintf("similar-check-%d", i)
		mw.CreatedAt = nil
		var out struct {
			Memory struct {
				ID int64 `json:"id"`
			} `json:"memory"`
			Similar []struct {
				ID int64 `json:"id"`
			} `json:"similar"`
		}
		start := time.Now()
		if err := c.do(http.MethodPost, "/api/memories", mw, &out); err != nil {
			return err
		}
		p := probe{id: out.Memory.ID, took: time.Since(start)}
		for _, s := range out.Similar {
			p.similar = append(p.similar, s.ID)
		}
		probes = append(probes, p)
	}
	if err := waitEmbedded(ctx, *dsn, owner); err != nil {
		return err
	}
	vecs, err := embeddings(ctx, *dsn, owner)
	if err != nil {
		return err
	}
	var exact, fewer, nonEmpty int
	var lat []time.Duration
	for pi, p := range probes {
		lat = append(lat, p.took)
		type cand struct {
			id  int64
			sim float64
		}
		var cs []cand
		for id := range byID { // the world's memories, plus the probes written before this one
			cs = append(cs, cand{id, cosine(vecs[p.id], vecs[id])})
		}
		for _, q := range probes[:pi] {
			cs = append(cs, cand{q.id, cosine(vecs[p.id], vecs[q.id])})
		}
		sort.Slice(cs, func(a, b int) bool { // as the server: similarity, then the newer id
			if cs[a].sim != cs[b].sim {
				return cs[a].sim > cs[b].sim
			}
			return cs[a].id > cs[b].id
		})
		var want []int64
		for _, c := range cs[:min(3, len(cs))] {
			if c.sim >= 0.80 {
				want = append(want, c.id)
			}
		}
		if len(want) > 0 {
			nonEmpty++
		}
		if fmt.Sprint(want) == fmt.Sprint(p.similar) {
			exact++
		} else if len(p.similar) < len(want) {
			fewer++
		}
	}
	fmt.Printf("## Similar on write, real server path (world 4, %d writes)\n\nExact match with the true top 3: %d/%d (%d with candidates). Server returned fewer than the truth: %d. Write latency p50 %v, p95 %v.\n",
		len(probes), exact, len(probes), nonEmpty, fewer, percentile(lat, 0.5).Round(time.Millisecond), percentile(lat, 0.95).Round(time.Millisecond))
	return nil
}
