// Package memrank scores memories by use (strength) and evidence (confidence) and re-ranks search results with
// them. It is pure: the API server and the offline eval (docs/research/memory-strength.md) call the same code.
package memrank

import (
	"math"
	"sort"
	"time"
)

// Decay is the ACT-R base-level decay d (Anderson and Schooler 1991; ACT-R default 0.5).
const Decay = 0.5

// minAge keeps a just-used memory finite: ACT-R's t^-d is infinite at t = 0.
const minAge = time.Hour

// Use is what strength needs about a memory: when it was written, how often it was used (the write counts as
// the first use) and when it was last used.
type Use struct {
	Created  time.Time
	Uses     int
	LastUsed time.Time
}

func days(d time.Duration) float64 { return max(d, minAge).Hours() / 24 }

// Strength is ACT-R base-level activation B = ln(sum_j t_j^-d) with Petrov's (2006) hybrid approximation (k = 1): the
// latest use is exact and the n-1 older uses are assumed evenly spread between the write and the latest use.
// Time is in days.
// Higher is stronger; a memory written now and never used again has B = -d*ln(1/24) ~ 1.6.
func Strength(u Use, now time.Time) float64 {
	n := max(u.Uses, 1)
	last := u.LastUsed
	if last.IsZero() || last.Before(u.Created) {
		last = u.Created
	}
	tn, life := days(now.Sub(last)), days(now.Sub(u.Created))
	sum := math.Pow(tn, -Decay)
	if n > 1 {
		if life-tn < 1e-9 { // all uses at once
			sum *= float64(n)
		} else {
			sum += float64(n-1) * (math.Pow(life, 1-Decay) - math.Pow(tn, 1-Decay)) / ((1 - Decay) * (life - tn))
		}
	}
	return math.Log(sum)
}

// Evidence is what confidence needs: a prior belief that the memory is right, and the weighted confirmations and
// contradictions it has had since.
type Evidence struct {
	Prior     float64 // in (0,1): from the memory's kind and stated confidence, see Prior
	Confirmed float64
	Wrong     float64
}

// PriorWeight is how many observations the prior is worth (Beta-Bernoulli pseudo-counts, W in Josang's
// subjective logic). Two keeps one "wrong" from erasing a memory and lets a few votes dominate the prior.
const PriorWeight = 2.0

// Confidence is the Beta-Bernoulli posterior mean (Confirmed + Prior*W) / (Confirmed + Wrong + W).
func Confidence(e Evidence) float64 {
	p := min(max(e.Prior, 0.01), 0.99)
	return (e.Confirmed + p*PriorWeight) / (e.Confirmed + e.Wrong + PriorWeight)
}

// Prior maps the protocol's stated confidence (low, medium, high) to a starting probability. Writers mark almost
// everything "high" (95% of one real store), so the levels are close: the evidence must do the work.
func Prior(stated string) float64 {
	switch stated {
	case "low":
		return 0.6
	case "high":
		return 0.8
	default:
		return 0.7
	}
}

// Weights say how much each signal adds to relevance. Each is the weight of one more Reciprocal Rank Fusion leg
// over the candidates the relevance legs found, so a signal can reorder relevant memories but never add one.
type Weights struct {
	Strength   float64
	Confidence float64
}

// Candidate is one memory that relevance search found, with its fused relevance score.
type Candidate struct {
	ID         int64
	Relevance  float64
	Strength   float64
	Confidence float64
}

// rrfK is the Reciprocal Rank Fusion constant of the relevance legs (Cormack et al. 2009).
const rrfK = 60

// Rerank orders candidates by relevance plus the weighted strength and confidence legs, and keeps the first
// limit. With room for two or more, the most relevant candidate is always kept, so a weak or doubted memory is
// demoted, never hidden. A single slot goes to the best combined score.
func Rerank(cands []Candidate, w Weights, limit int) []Candidate {
	if len(cands) == 0 || limit <= 0 {
		return nil
	}
	score := make(map[int64]float64, len(cands))
	for _, c := range cands {
		score[c.ID] = c.Relevance
	}
	addLeg(cands, score, w.Strength, func(c Candidate) float64 { return c.Strength })
	addLeg(cands, score, w.Confidence, func(c Candidate) float64 { return c.Confidence })

	top := cands[0]
	for _, c := range cands[1:] {
		if c.Relevance > top.Relevance || c.Relevance == top.Relevance && score[c.ID] > score[top.ID] {
			top = c
		}
	}
	out := append([]Candidate(nil), cands...)
	sort.SliceStable(out, func(i, j int) bool {
		if score[out[i].ID] != score[out[j].ID] {
			return score[out[i].ID] > score[out[j].ID]
		}
		return out[i].ID > out[j].ID
	})
	out = out[:min(limit, len(out))]
	if len(out) < 2 {
		return out
	}
	for _, c := range out {
		if c.ID == top.ID {
			return out
		}
	}
	out[len(out)-1] = top
	return out
}

// addLeg adds weight/(rrfK + rank) by the signal, with equal signals sharing a rank.
func addLeg(cands []Candidate, score map[int64]float64, weight float64, signal func(Candidate) float64) {
	if weight == 0 {
		return
	}
	vals := make([]float64, len(cands))
	for i, c := range cands {
		vals[i] = signal(c)
	}
	sorted := append([]float64(nil), vals...)
	sort.Sort(sort.Reverse(sort.Float64Slice(sorted)))
	for i, c := range cands {
		rank := sort.Search(len(sorted), func(j int) bool { return sorted[j] <= vals[i] }) + 1
		score[c.ID] += weight / float64(rrfK+rank)
	}
}
