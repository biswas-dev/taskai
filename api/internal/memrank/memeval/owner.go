package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"taskai/internal/memrank"
)

// The owner suite reads a PRIVATE export of real memories that stays on this machine. It prints aggregates only:
// never a title, body or query. With no use log and no correctness labels, it can only check that the ranking
// does no harm (known-item retrieval), measure latency and storage, and count conflict candidates.

type ownerMem struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Repo       string `json:"repo"`
	Folder     string `json:"folder"`
	Files      string `json:"files"`
	Confidence string `json:"confidence"`
	Status     string `json:"status"`
	Supersedes int64  `json:"supersedes"`
	Hash       string `json:"hash"`
	CreatedAt  string `json:"created_at"`
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`the a an and or of to in on for is are was be with it this that as at by from not no
		use do does did when then than but if into out up via can must should will you your we our they them its has
		have had all any each one two new old run make see also only more most same other such these those there here`) {
		stopwords[w] = true
	}
}

func contentWords(s string) []string {
	var out []string
	for _, w := range memoryWords(s) {
		if len(w) >= 4 && !stopwords[w] {
			out = append(out, w)
		}
	}
	return out
}

func memoryWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	})
}

func runOwner(ctx context.Context) error {
	raw, err := os.ReadFile(*ownerIn)
	if err != nil {
		return err
	}
	var ms []ownerMem
	if err := json.Unmarshal(raw, &ms); err != nil {
		return err
	}
	c, err := login(*apiURL, "owner@memeval.local", *password)
	if err != nil {
		return err
	}
	owner, err := ownerID(ctx, *dsn, "owner@memeval.local")
	if err != nil {
		return err
	}
	byHub := map[int64]int64{}
	created := map[int64]time.Time{}
	hashOf := map[int64]string{}
	var newest time.Time
	failed := 0
	for _, m := range ms {
		t, err := time.Parse(time.RFC3339, m.CreatedAt)
		if err != nil {
			return err
		}
		var files []string
		_ = json.Unmarshal([]byte(m.Files), &files)
		w := memWrite{Kind: m.Kind, Title: m.Title, Body: m.Body, Scope: memScope{Repo: m.Repo, Folder: m.Folder, Files: files},
			Confidence: m.Confidence, OriginHost: "memeval-owner", OriginID: fmt.Sprint(m.ID), CreatedAt: &t}
		id, err := c.write(w)
		if err != nil { // a scope the protocol refuses: keep the text, drop the scope
			w.Scope = memScope{}
			if id, err = c.write(w); err != nil {
				failed++
				continue
			}
		}
		byHub[m.ID], created[id], hashOf[id] = id, t, m.Hash
		if t.After(newest) {
			newest = t
		}
	}
	if err := waitEmbedded(ctx, *dsn, owner); err != nil {
		return err
	}
	now := newest.Add(24 * time.Hour)

	vs := []variant{{name: "flat"}}
	for _, x := range []float64{0.5, 1, 2} {
		vs = append(vs, variant{name: fmt.Sprintf("recency w=%g", x), w: memrank.Weights{Strength: x}})
	}
	res := make([]retrieval, len(vs))
	r := rand.New(rand.NewPCG(42, 1))
	var lat []time.Duration
	modes := map[string]int{}
	queries := 0
	for _, m := range ms {
		id, ok := byHub[m.ID]
		if !ok || m.Status != "active" {
			continue
		}
		words := contentWords(m.Body)
		if len(words) < 6 {
			continue
		}
		r.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
		hits, mode, d, err := c.search(strings.Join(words[:4], " "), "", 50)
		if err != nil {
			return err
		}
		queries++
		lat, modes[mode] = append(lat, d), modes[mode]+1
		gold := map[int64]bool{id: true}
		for other, h := range hashOf {
			if h == m.Hash {
				gold[other] = true
			}
		}
		for vi, v := range vs {
			var cands []memrank.Candidate
			for _, h := range hits {
				cr := created[h.ID]
				cands = append(cands, memrank.Candidate{ID: h.ID, Relevance: h.Rank,
					Strength: memrank.Strength(memrank.Use{Created: cr, Uses: 1, LastUsed: cr}, now)})
			}
			out := memrank.Rerank(cands, v.w, 10)
			ids := make([]int64, len(out))
			for i, c := range out {
				ids[i] = c.ID
			}
			res[vi].add(rankOf(ids, gold))
		}
	}
	stated := map[string]int{}
	for _, m := range ms {
		stated[m.Confidence]++
	}
	fmt.Printf("## Owner store (private, aggregates only): %d memories loaded, %d refused; stated confidence %v\n\n", len(byHub), failed, stated)
	fmt.Printf("Known-item queries (4 random content words of a memory's body): %d. Modes %v. Latency p50 %v, p95 %v.\n\n",
		queries, modes, percentile(lat, 0.5).Round(time.Millisecond), percentile(lat, 0.95).Round(time.Millisecond))
	fmt.Println("| variant | n | hit@1 | recall@5 | recall@10 | MRR |\n|---|---|---|---|---|---|")
	for vi, v := range vs {
		fmt.Printf("| %s | %s |\n", v.name, res[vi].row())
	}

	// Conflict candidates: pairs in the same repo scope above a cosine threshold, and the known supersede pairs.
	vecs, err := embeddings(ctx, *dsn, owner)
	if err != nil {
		return err
	}
	repoOf := map[int64]string{}
	for _, m := range ms {
		if id, ok := byHub[m.ID]; ok {
			repoOf[id] = m.Repo
		}
	}
	var pairs [][2]int64
	for _, m := range ms {
		if m.Supersedes > 0 && byHub[m.Supersedes] > 0 && byHub[m.ID] > 0 {
			pairs = append(pairs, [2]int64{byHub[m.Supersedes], byHub[m.ID]})
		}
	}
	ids := make([]int64, 0, len(vecs))
	for id := range vecs {
		ids = append(ids, id)
	}
	fmt.Printf("\nConflict candidates (same repo scope, different text): %d supersede pairs known.\n\n", len(pairs))
	fmt.Println("| threshold | flagged pairs | flagged per memory | supersede pairs found |\n|---|---|---|---|")
	for _, th := range []float64{0.8, 0.85, 0.9, 0.95} {
		n := 0
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				if repoOf[a] == repoOf[b] && hashOf[a] != hashOf[b] && cosine(vecs[a], vecs[b]) >= th {
					n++
				}
			}
		}
		found := 0
		for _, p := range pairs {
			if cosine(vecs[p[0]], vecs[p[1]]) >= th {
				found++
			}
		}
		fmt.Printf("| %.2f | %d | %.3f | %d/%d |\n", th, n, 2*float64(n)/float64(len(ids)), found, len(pairs))
	}
	return nil
}
