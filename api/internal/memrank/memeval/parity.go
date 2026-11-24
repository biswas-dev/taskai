package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"taskai/internal/memrank"
)

// runParity checks the server against the eval: with MEMORY_STRENGTH on and no feedback yet, the server's top 10
// must equal memrank.Rerank over the flat top 50 cached by the synth suite (run with the flag off). It also
// times flag-on search.
func runParity(ctx context.Context) error {
	c, err := login(*apiURL, "synth@memeval.local", *password)
	if err != nil {
		return err
	}
	var same, total int
	var lat []time.Duration
	for _, seed := range []int{4, 5, 6, 7, 8} {
		w := newWorld(seed, 0.5, 1500)
		path := filepath.Join(*cacheDir, fmt.Sprintf("hits-%s.json", strings.ReplaceAll(w.repo, "/", "_")))
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("run the synth suite first: %w", err)
		}
		flat := map[string][]hit{}
		if err := json.Unmarshal(raw, &flat); err != nil {
			return err
		}
		byOrigin := map[int64]*synthMem{}
		for _, m := range w.mems {
			id, err := c.write(m.write) // idempotent: returns the stored id
			if err != nil {
				return err
			}
			byOrigin[id] = m
		}
		for text, hits := range flat {
			got, mode, d, err := c.search(text, w.repo, 10)
			if err != nil {
				return err
			}
			if mode != "hybrid" {
				return fmt.Errorf("search fell back to %s", mode)
			}
			lat = append(lat, d)
			now := time.Now().UTC()
			var cands []memrank.Candidate
			for _, h := range hits {
				m := byOrigin[h.ID]
				cr := at(m.created).Truncate(time.Second)
				cands = append(cands, memrank.Candidate{ID: h.ID, Relevance: h.Rank,
					Strength:   memrank.Strength(memrank.Use{Created: cr, Uses: 1, LastUsed: cr}, now),
					Confidence: memrank.Confidence(memrank.Evidence{Prior: memrank.Prior(m.stated)})})
			}
			want := memrank.Rerank(cands, memrank.Weights{Strength: 0.5, Confidence: 0.25}, 10)
			total++
			if len(want) == len(got) {
				ok := true
				for i := range want {
					ok = ok && want[i].ID == got[i].ID
				}
				if ok {
					same++
				}
			}
		}
	}
	fmt.Printf("## Server parity (MEMORY_STRENGTH on, no feedback)\n\nTop-10 lists equal to the eval's re-rank: %d/%d. Flag-on search latency p50 %v, p95 %v.\n",
		same, total, percentile(lat, 0.5).Round(time.Millisecond), percentile(lat, 0.95).Round(time.Millisecond))
	return nil
}
