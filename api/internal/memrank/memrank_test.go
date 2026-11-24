package memrank

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestStrength(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name         string
		strong, weak Use
		now          time.Time
	}{
		{"more uses are stronger", Use{t0, 5, t0.Add(9 * day)}, Use{t0, 1, t0.Add(9 * day)}, t0.Add(10 * day)},
		{"a recent use is stronger", Use{t0, 2, t0.Add(9 * day)}, Use{t0, 2, t0.Add(1 * day)}, t0.Add(10 * day)},
		{"a newer memory is stronger", Use{t0.Add(8 * day), 1, time.Time{}}, Use{t0, 1, time.Time{}}, t0.Add(10 * day)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s, w := Strength(tt.strong, tt.now), Strength(tt.weak, tt.now); !(s > w) {
				t.Fatalf("strong %v <= weak %v", s, w)
			}
		})
	}
	// One use: B = -d ln(t). Uses at one instant: B = ln(n) - d ln(t).
	if got, want := Strength(Use{t0, 1, t0}, t0.Add(4*day)), -0.5*math.Log(4); math.Abs(got-want) > 1e-9 {
		t.Fatalf("one use: got %v want %v", got, want)
	}
	if got, want := Strength(Use{t0, 3, t0}, t0.Add(4*day)), math.Log(3)-0.5*math.Log(4); math.Abs(got-want) > 1e-9 {
		t.Fatalf("massed uses: got %v want %v", got, want)
	}
	if s := Strength(Use{t0, 1, t0}, t0); math.IsInf(s, 0) || math.IsNaN(s) {
		t.Fatalf("a memory used now must be finite, got %v", s)
	}
}

func TestConfidence(t *testing.T) {
	tests := []struct {
		name string
		e    Evidence
		want float64
	}{
		{"prior only", Evidence{Prior: 0.8}, 0.8},
		{"one wrong", Evidence{Prior: 0.8, Wrong: 1}, 1.6 / 3},
		{"two confirmed", Evidence{Prior: 0.5, Confirmed: 2}, 3.0 / 4},
		{"prior is clamped", Evidence{Prior: 1}, 0.99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Confidence(tt.e); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func ids(cs []Candidate) []int64 {
	out := make([]int64, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestRerank(t *testing.T) {
	cands := []Candidate{
		{ID: 1, Relevance: 0.049, Strength: -3, Confidence: 0.2},
		{ID: 2, Relevance: 0.048, Strength: 2, Confidence: 0.9},
		{ID: 3, Relevance: 0.047, Strength: 1, Confidence: 0.8},
		{ID: 4, Relevance: 0.0164, Strength: 3, Confidence: 0.95},
	}
	if got := ids(Rerank(cands, Weights{}, 10)); len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("zero weights must keep relevance order, got %v", got)
	}
	got := ids(Rerank(cands, Weights{Strength: 1, Confidence: 1}, 4))
	if got[0] != 2 {
		t.Fatalf("a strong, confirmed, relevant memory should lead, got %v", got)
	}
	// The weakest, most doubted memory is the most relevant: it is demoted but must stay in a short list.
	got = ids(Rerank(cands, Weights{Strength: 5, Confidence: 5}, 2))
	if len(got) != 2 || got[1] != 1 {
		t.Fatalf("the most relevant memory must never be hidden, got %v", got)
	}
	if Rerank(nil, Weights{Strength: 1}, 5) != nil {
		t.Fatal("no candidates, no results")
	}
}
