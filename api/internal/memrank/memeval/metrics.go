package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"
)

func logf(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

// rankOf is the 1-based position of the first result in gold, or 0 when none is in the list.
func rankOf(results []int64, gold map[int64]bool) int {
	for i, id := range results {
		if gold[id] {
			return i + 1
		}
	}
	return 0
}

// retrieval accumulates hit@1, recall@k and MRR over queries.
type retrieval struct {
	n, hit1, hit5, hit10 int
	rr                   float64
}

func (r *retrieval) add(rank int) {
	r.n++
	if rank == 1 {
		r.hit1++
	}
	if rank >= 1 && rank <= 5 {
		r.hit5++
	}
	if rank >= 1 && rank <= 10 {
		r.hit10++
	}
	if rank > 0 {
		r.rr += 1 / float64(rank)
	}
}

func (r retrieval) frac(x int) float64 { return float64(x) / math.Max(float64(r.n), 1) }

func (r retrieval) row() string {
	return fmt.Sprintf("%5d | %.3f | %.3f | %.3f | %.3f", r.n, r.frac(r.hit1), r.frac(r.hit5), r.frac(r.hit10), r.rr/math.Max(float64(r.n), 1))
}

// calib collects (confidence, correct) pairs.
type calib struct{ p, y []float64 }

func (c *calib) add(p float64, correct bool) {
	c.p = append(c.p, p)
	y := 0.0
	if correct {
		y = 1
	}
	c.y = append(c.y, y)
}

// ece is the expected calibration error with equal-width bins (Guo et al. 2017, M = 15 by default).
func (c calib) ece(bins int) float64 {
	n := float64(len(c.p))
	if n == 0 {
		return 0
	}
	sumP, sumY, cnt := make([]float64, bins), make([]float64, bins), make([]float64, bins)
	for i, p := range c.p {
		b := min(int(p*float64(bins)), bins-1)
		sumP[b] += p
		sumY[b] += c.y[i]
		cnt[b]++
	}
	e := 0.0
	for b := range bins {
		if cnt[b] > 0 {
			e += cnt[b] / n * math.Abs(sumY[b]/cnt[b]-sumP[b]/cnt[b])
		}
	}
	return e
}

func (c calib) brier() float64 {
	s := 0.0
	for i, p := range c.p {
		s += (p - c.y[i]) * (p - c.y[i])
	}
	return s / math.Max(float64(len(c.p)), 1)
}

// auc is the probability that a random correct item has a higher confidence than a random wrong one.
func (c calib) auc() float64 {
	type pair struct{ p, y float64 }
	ps := make([]pair, len(c.p))
	for i := range c.p {
		ps[i] = pair{c.p[i], c.y[i]}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].p < ps[j].p })
	var pos, neg, rankSum float64
	for i := 0; i < len(ps); {
		j := i
		for j < len(ps) && ps[j].p == ps[i].p {
			j++
		}
		avg := float64(i+j+1) / 2 // average 1-based rank of the tie group
		for k := i; k < j; k++ {
			if ps[k].y == 1 {
				pos++
				rankSum += avg
			} else {
				neg++
			}
		}
		i = j
	}
	if pos == 0 || neg == 0 {
		return math.NaN()
	}
	return (rankSum - pos*(pos+1)/2) / (pos * neg)
}

// reliability prints the reliability diagram as a table of 5 bins.
func (c calib) reliability() string {
	const bins = 5
	sumP, sumY, cnt := make([]float64, bins), make([]float64, bins), make([]float64, bins)
	for i, p := range c.p {
		b := min(int(p*bins), bins-1)
		sumP[b] += p
		sumY[b] += c.y[i]
		cnt[b]++
	}
	s := "| bin | n | mean c | observed right |\n|---|---|---|---|\n"
	for b := range bins {
		if cnt[b] > 0 {
			s += fmt.Sprintf("| %.1f-%.1f | %d | %.3f | %.3f |\n", float64(b)/bins, float64(b+1)/bins, int(cnt[b]), sumP[b]/cnt[b], sumY[b]/cnt[b])
		}
	}
	return s
}

func percentile(ds []time.Duration, q float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[min(int(q*float64(len(s))), len(s)-1)]
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / math.Sqrt(na*nb)
}
