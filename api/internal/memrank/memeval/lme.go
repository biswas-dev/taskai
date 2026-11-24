package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"taskai/internal/memrank"
)

// LongMemEval_S (Wu et al., ICLR 2025, MIT licence): each question has its own haystack of ~50 chat sessions. A
// session becomes one or more memories (bodies are capped at 8000 bytes), written at the session's date, scoped
// to its question by repo. Retrieval is scored at session level against answer_session_ids; abstention
// questions (_abs) are skipped, as in the paper. There is no use signal here, so strength is creation recency.

type lmeTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type lmeQuestion struct {
	ID         string      `json:"question_id"`
	Type       string      `json:"question_type"`
	Question   string      `json:"question"`
	Date       string      `json:"question_date"`
	Dates      []string    `json:"haystack_dates"`
	SessionIDs []string    `json:"haystack_session_ids"`
	Sessions   [][]lmeTurn `json:"haystack_sessions"`
	AnswerIDs  []string    `json:"answer_session_ids"`
}

const lmeLayout = "2006/01/02 (Mon) 15:04"

var repoUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func lmeRepo(q lmeQuestion) string { return "lme/" + repoUnsafe.ReplaceAllString(q.ID, "_") }

func lmeWrites(q lmeQuestion) []memWrite {
	var out []memWrite
	for i, sess := range q.Sessions {
		created, err := time.Parse(lmeLayout, q.Dates[i])
		if err != nil {
			continue
		}
		var b strings.Builder
		title := ""
		for _, t := range sess {
			if title == "" && t.Role == "user" {
				title = cut(strings.Join(strings.Fields(t.Content), " "), 150)
			}
			b.WriteString(t.Role + ": " + t.Content + "\n")
		}
		text := b.String()
		for part := 0; len(text) > 0; part++ {
			body := cut(text, 7900)
			text = text[len(body):]
			out = append(out, memWrite{Kind: "fact", Title: title, Body: body, Scope: memScope{Repo: lmeRepo(q)},
				OriginHost: "memeval-lme", OriginID: fmt.Sprintf("%s/%s/%d", q.ID, q.SessionIDs[i], part), CreatedAt: &created})
		}
	}
	return out
}

func runLME(ctx context.Context) error {
	raw, err := os.ReadFile(*lmePath)
	if err != nil {
		return err
	}
	var qs []lmeQuestion
	if err := json.Unmarshal(raw, &qs); err != nil {
		return err
	}
	var keep []lmeQuestion
	for _, q := range qs {
		if !strings.HasSuffix(q.ID, "_abs") {
			keep = append(keep, q)
		}
	}
	if *limitQ > 0 { // all knowledge-update questions, and the first limitQ of each other type
		perType := map[string]int{}
		var sub []lmeQuestion
		for _, q := range keep {
			if q.Type == "knowledge-update" || perType[q.Type] < *limitQ {
				perType[q.Type]++
				sub = append(sub, q)
			}
		}
		keep = sub
	}
	// Ten owners: a write locks its owner and sums the owner's usage, so one owner with 40k memories is slow.
	const owners = 10
	clients := make([]*client, owners)
	for k := range clients {
		email := fmt.Sprintf("lme%d@memeval.local", k+1)
		if clients[k], err = login(*apiURL, email, *password); err != nil {
			return err
		}
	}
	clientOf := func(i int) *client { return clients[i%owners] }
	// session of each memory id, written with 8 workers (writes are idempotent by origin: a rerun is safe)
	session := map[int64]string{}
	created := map[int64]time.Time{}
	var mu sync.Mutex
	type job struct {
		c *client
		m memWrite
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var writeErr error
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				m := j.m
				id, err := j.c.write(m)
				mu.Lock()
				if err != nil && writeErr == nil {
					writeErr = err
				}
				parts := strings.Split(m.OriginID, "/")
				session[id], created[id] = parts[1], *m.CreatedAt
				mu.Unlock()
			}
		}()
	}
	for i, q := range keep {
		for _, m := range lmeWrites(q) {
			jobs <- job{clientOf(i), m}
		}
		if i%50 == 0 {
			logf("lme: wrote haystacks of %d/%d questions", i, len(keep))
		}
	}
	close(jobs)
	wg.Wait()
	if writeErr != nil {
		return writeErr
	}
	for k := range owners {
		owner, err := ownerID(ctx, *dsn, fmt.Sprintf("lme%d@memeval.local", k+1))
		if err != nil {
			return err
		}
		if err := waitEmbedded(ctx, *dsn, owner); err != nil {
			return err
		}
	}

	vs := []variant{{name: "flat"}}
	for _, x := range []float64{0.25, 0.5, 1, 2} {
		vs = append(vs, variant{name: fmt.Sprintf("recency w=%g", x), w: memrank.Weights{Strength: x}})
	}
	type agg struct {
		byType map[string]*retrieval
		all    retrieval
		kuNew  retrieval // knowledge-update: rank of the NEWEST answer session
		kuOrd  [2]int    // newest ranked above the older one / pairs with both present
	}
	res := make([]agg, len(vs))
	for i := range res {
		res[i].byType = map[string]*retrieval{}
	}
	var lat []time.Duration
	modes := map[string]int{}
	for qi, q := range keep {
		hits, mode, d, err := clientOf(qi).search(q.Question, lmeRepo(q), 50)
		if err != nil {
			return err
		}
		lat, modes[mode] = append(lat, d), modes[mode]+1
		now, err := time.Parse(lmeLayout, q.Date)
		if err != nil {
			return err
		}
		gold := map[string]bool{}
		for _, a := range q.AnswerIDs {
			gold[a] = true
		}
		newest := ""
		var newestAt time.Time
		for i, sid := range q.SessionIDs {
			if t, err := time.Parse(lmeLayout, q.Dates[i]); err == nil && gold[sid] && !t.Before(newestAt) {
				newest, newestAt = sid, t
			}
		}
		for vi, v := range vs {
			var cands []memrank.Candidate
			for _, h := range hits {
				cr := created[h.ID]
				cands = append(cands, memrank.Candidate{ID: h.ID, Relevance: h.Rank,
					Strength: memrank.Strength(memrank.Use{Created: cr, Uses: 1, LastUsed: cr}, now)})
			}
			out := memrank.Rerank(cands, v.w, 50)
			var order []string // sessions, first occurrence
			seen := map[string]bool{}
			for _, c := range out {
				if s := session[c.ID]; !seen[s] {
					seen[s] = true
					order = append(order, s)
				}
			}
			rank := 0
			pos := map[string]int{}
			for i, s := range order {
				pos[s] = i + 1
				if gold[s] && rank == 0 {
					rank = i + 1
				}
			}
			r := &res[vi]
			r.all.add(rank)
			if r.byType[q.Type] == nil {
				r.byType[q.Type] = &retrieval{}
			}
			r.byType[q.Type].add(rank)
			if q.Type == "knowledge-update" {
				r.kuNew.add(pos[newest])
				older := ""
				for _, a := range q.AnswerIDs {
					if a != newest {
						older = a
					}
				}
				if pos[newest] > 0 && pos[older] > 0 {
					r.kuOrd[1]++
					if pos[newest] < pos[older] {
						r.kuOrd[0]++
					}
				}
			}
		}
	}
	fmt.Printf("## LongMemEval_S, %d questions (no abstention), session-level retrieval through the shipped search\n\n", len(keep))
	fmt.Printf("Search modes: %v. Search latency p50 %v, p95 %v (local API, %d memories).\n\n", modes, percentile(lat, 0.5).Round(time.Millisecond),
		percentile(lat, 0.95).Round(time.Millisecond), len(session))
	fmt.Println("| variant | n | hit@1 | recall@5 | recall@10 | MRR |\n|---|---|---|---|---|---|")
	for vi, v := range vs {
		fmt.Printf("| %s | %s |\n", v.name, res[vi].all.row())
	}
	var types []string
	for t := range res[0].byType {
		types = append(types, t)
	}
	sort.Strings(types)
	fmt.Println("\nrecall@5 by question type:\n\n| variant | " + strings.Join(types, " | ") + " |\n|---|" + strings.Repeat("---|", len(types)))
	for vi, v := range vs {
		row := "| " + v.name
		for _, t := range types {
			r := res[vi].byType[t]
			row += fmt.Sprintf(" | %.3f (n=%d)", r.frac(r.hit5), r.n)
		}
		fmt.Println(row + " |")
	}
	fmt.Println("\nKnowledge update: the newest answer session.\n\n| variant | newest hit@1 | newest recall@5 | MRR newest | newest above older |\n|---|---|---|---|---|")
	for vi, v := range vs {
		r := res[vi]
		fmt.Printf("| %s | %.3f | %.3f | %.3f | %d/%d = %.3f |\n", v.name, r.kuNew.frac(r.kuNew.hit1), r.kuNew.frac(r.kuNew.hit5),
			r.kuNew.rr/max(float64(r.kuNew.n), 1), r.kuOrd[0], r.kuOrd[1], float64(r.kuOrd[0])/max(float64(r.kuOrd[1]), 1))
	}
	return nil
}
