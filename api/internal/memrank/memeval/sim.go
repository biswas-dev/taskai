package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"taskai/internal/memrank"
)

// variant is one ranking under test.
type variant struct {
	name        string
	w           memrank.Weights
	recencyOnly bool // strength from the write time only: no use signal
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
	for _, q := range w.queries {
		now := at(q.at)
		var cands []memrank.Candidate
		for _, h := range l.hits(q.text) {
			m := l.byID[h.ID]
			if m == nil || m.created > q.at || m.superseded != 0 && m.superseded <= q.at {
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
				uses[i].Uses++
				uses[i].LastUsed = now
			}
			if r.Float64() < fb.confirmRight {
				uses[i].Uses++
				uses[i].LastUsed = now
				ev[i].Confirmed++
			}
			if r.Float64() < fb.falseWrong {
				ev[i].Wrong++
			}
		} else if r.Float64() < fb.detectWrong {
			ev[i].Wrong++
		} else if r.Float64() < fb.useWrong {
			uses[i].Uses++
			uses[i].LastUsed = now
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
