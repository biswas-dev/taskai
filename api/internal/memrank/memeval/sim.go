package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"taskai/internal/memrank"
)

// variant is one ranking under test.
type variant struct {
	name        string
	w           memrank.Weights
	recencyOnly bool // strength from the write time only: no use signal
	supersede   bool // oracle validity: every update supersedes the older memories of its slot
}

// actors is how many distinct credentials give feedback. The server counts one confirmed and one wrong vote per
// credential (memory_feedback), so with one API key each count is 0 or 1. 0 means no cap (an idealised store).
var actors = 1

// replacedBy: a change of the memory's slot after it was written and by t, so a writer that always supersedes
// would have retired it.
func replacedBy(w *world, m *synthMem, t float64) bool {
	for _, c := range w.history[m.slot] {
		if c.at > m.created && c.at <= t {
			return true
		}
	}
	return false
}

type loaded struct {
	w    *world
	byID map[int64]*synthMem
	hits func(text string) []hit
}

type simResult struct {
	all, hot, cold retrieval
	top1           map[string]int // right / stale / wrong / other
	cal            calib          // (confidence, right) for same-slot memories in the top 5
	stated         calib          // the same memories, scored by the stated level only
}

func at(day float64) time.Time { return epoch.Add(time.Duration(day * 24 * float64(time.Hour))) }

// simulate runs the queries in time order; after each, the simulated agent gives feedback on the top result,
// which changes strength and confidence for the next queries. fbSeed makes variants share random draws.
func simulate(l loaded, v variant, fb feedback, fbSeed int) simResult {
	w := l.w
	uses := make([]memrank.Use, len(w.mems))
	ev := make([]memrank.Evidence, len(w.mems))
	for i, m := range w.mems {
		uses[i] = memrank.Use{Created: at(m.created), Uses: 1, LastUsed: at(m.created)}
		ev[i] = memrank.Evidence{Prior: memrank.Prior(m.stated)}
	}
	r := rand.New(rand.NewPCG(uint64(fbSeed), 11))
	res := simResult{top1: map[string]int{}}
	type vote struct {
		idx, actor int
		signal     string
	}
	voted := map[vote]bool{}
	// cast applies the server's rule: one vote per memory, signal and credential.
	cast := func(i int, signal string) bool {
		k := vote{i, 0, signal}
		if actors > 0 {
			k.actor = r.IntN(actors)
		} else {
			k.actor = len(voted) // every vote distinct
		}
		if voted[k] {
			return false
		}
		voted[k] = true
		return true
	}
	use := func(i int, now time.Time) { // "useful" counts at most once an hour, as on the server
		if now.Sub(uses[i].LastUsed) >= time.Hour {
			uses[i].Uses++
			uses[i].LastUsed = now
		}
	}
	for _, q := range w.queries {
		now := at(q.at)
		var cands []memrank.Candidate
		for _, h := range l.hits(q.text) {
			m := l.byID[h.ID]
			if m == nil || m.created > q.at || m.superseded != 0 && m.superseded <= q.at {
				continue
			}
			if v.supersede && m.slot.svc >= 0 && replacedBy(w, m, q.at) {
				continue
			}
			u := uses[m.idx]
			if v.recencyOnly {
				u = memrank.Use{Created: u.Created, Uses: 1, LastUsed: u.Created}
			}
			cands = append(cands, memrank.Candidate{ID: h.ID, Relevance: h.Rank,
				Strength: memrank.Strength(u, now), Confidence: memrank.Confidence(ev[m.idx])})
		}
		out := memrank.Rerank(cands, v.w, 10)
		ids := make([]int64, len(out))
		gold := map[int64]bool{}
		for i, c := range out {
			ids[i] = c.ID
		}
		for _, m := range w.mems {
			if w.correct(m, q.slot, q.at) {
				gold[m.id] = true
			}
		}
		rank := rankOf(ids, gold)
		res.all.add(rank)
		if q.cold {
			res.cold.add(rank)
		} else {
			res.hot.add(rank)
		}
		for i, c := range out {
			if i >= 5 {
				break
			}
			if m := l.byID[c.ID]; m.slot == q.slot {
				res.cal.add(c.Confidence, gold[c.ID])
				res.stated.add(memrank.Prior(m.stated), gold[c.ID])
			}
		}
		if len(out) == 0 {
			res.top1["none"]++
			continue
		}
		top := l.byID[out[0].ID]
		switch {
		case gold[top.id]:
			res.top1["right"]++
		case top.slot != q.slot:
			res.top1["other"]++
			continue // not about the question: the agent ignores it
		case wasTrue(w, top, q.at):
			res.top1["stale"]++
		default:
			res.top1["wrong"]++
		}
		i := top.idx
		if gold[top.id] {
			if r.Float64() < fb.useRight {
				use(i, now)
			}
			if r.Float64() < fb.confirmRight && cast(i, "confirmed") {
				use(i, now)
				ev[i].Confirmed++
			}
			if r.Float64() < fb.falseWrong && cast(i, "wrong") {
				ev[i].Wrong++
			}
		} else if r.Float64() < fb.detectWrong {
			if cast(i, "wrong") {
				ev[i].Wrong++
			}
		} else if r.Float64() < fb.useWrong {
			use(i, now)
		}
	}
	return res
}

// wasTrue: the memory stated the slot's value at some earlier time (stale), not a value it never had (wrong).
func wasTrue(w *world, m *synthMem, t float64) bool {
	for _, c := range w.history[m.slot] {
		if c.value == m.value && c.at <= t {
			return true
		}
	}
	return false
}

func (r simResult) top1Row() string {
	n := 0
	for _, v := range r.top1 {
		n += v
	}
	f := func(k string) float64 { return float64(r.top1[k]) / math.Max(float64(n), 1) }
	return fmt.Sprintf("%.3f | %.3f | %.3f | %.3f", f("right"), f("stale"), f("wrong"), f("other")+f("none"))
}

func merge(rs []simResult) simResult {
	out := simResult{top1: map[string]int{}}
	for _, r := range rs {
		for _, p := range []struct{ dst, src *retrieval }{{&out.all, &r.all}, {&out.hot, &r.hot}, {&out.cold, &r.cold}} {
			p.dst.n += p.src.n
			p.dst.hit1 += p.src.hit1
			p.dst.hit5 += p.src.hit5
			p.dst.hit10 += p.src.hit10
			p.dst.rr += p.src.rr
		}
		for k, v := range r.top1 {
			out.top1[k] += v
		}
		out.cal.p, out.cal.y = append(out.cal.p, r.cal.p...), append(out.cal.y, r.cal.y...)
		out.stated.p, out.stated.y = append(out.stated.p, r.stated.p...), append(out.stated.y, r.stated.y...)
	}
	return out
}

// platt fits p' = sigmoid(a*logit(p) + b) by gradient descent on log loss (Platt 1999).
func platt(c calib) func(float64) float64 {
	logit := func(p float64) float64 { p = min(max(p, 1e-4), 1-1e-4); return math.Log(p / (1 - p)) }
	a, b := 1.0, 0.0
	for range 3000 {
		var ga, gb float64
		for i, p := range c.p {
			x := logit(p)
			q := 1 / (1 + math.Exp(-(a*x + b)))
			ga += (q - c.y[i]) * x
			gb += q - c.y[i]
		}
		n := float64(len(c.p))
		a -= 0.5 * ga / n
		b -= 0.5 * gb / n
	}
	return func(p float64) float64 { return 1 / (1 + math.Exp(-(a*logit(p) + b))) }
}

func mapCalib(c calib, f func(float64) float64) calib {
	out := calib{y: c.y}
	for _, p := range c.p {
		out.p = append(out.p, f(p))
	}
	return out
}

// assisted models "similar memories on write": when a memory is written, the store shows the writer earlier
// memories of the same scope within the cosine threshold, and the writer supersedes a shown memory that states
// another value of the same slot with probability act. A wrong write can retire a right memory: that is kept.
func assisted(l loaded, vecs map[int64][]float32, threshold, act float64, seed int) loaded {
	r := rand.New(rand.NewPCG(uint64(seed), 13))
	w := *l.w
	w.mems = make([]*synthMem, len(l.w.mems))
	byID := map[int64]*synthMem{}
	for i, m := range l.w.mems {
		c := *m
		w.mems[i] = &c
		byID[c.id] = &c
	}
	order := append([]*synthMem(nil), w.mems...)
	sort.Slice(order, func(i, j int) bool { return order[i].created < order[j].created })
	for i, m := range order {
		if m.slot.svc < 0 {
			continue
		}
		for _, o := range order[:i] {
			if o.slot != m.slot || o.value == m.value || o.superseded != 0 && o.superseded <= m.created {
				continue
			}
			if cosine(vecs[m.id], vecs[o.id]) >= threshold && r.Float64() < act {
				o.superseded = m.created
			}
		}
	}
	return loaded{w: &w, byID: byID, hits: l.hits}
}
