// Command memeval is the offline eval for memory strength and confidence (docs/research/memory-strength.md).
// It is not shipped. It needs a LOCAL TaskAI API on Postgres + pgvector with the production embedding model,
// and never talks to production. Suites: synth (generated world), lme (LongMemEval_S, public), owner (a private
// export that stays on this machine; only aggregates are printed).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"taskai/internal/memrank"
)

var (
	apiURL   = flag.String("api", "http://localhost:18080", "local TaskAI API")
	dsn      = flag.String("dsn", "postgres://postgres:memeval@localhost:55433/taskai?sslmode=disable", "local Postgres of that API")
	password = flag.String("password", "Memeval-pass-123", "password of the local eval users")
	suite    = flag.String("suite", "synth", "synth | lme | owner | parity | similar")
	lmePath  = flag.String("lme", "", "path to longmemeval_s_cleaned.json")
	ownerIn  = flag.String("owner", "", "path to a private JSON export of memories (owner suite)")
	limitQ   = flag.Int("max", 0, "lme: all knowledge-update questions and at most this many of each other type (0: all)")
	cacheDir = flag.String("cache", ".", "directory for cached search results (synth)")
	forceS   = flag.Float64("both-s", 0, "synth: use this strength weight for the combined variant instead of the train choice")
	forceC   = flag.Float64("both-c", 0, "synth: use this belief weight for the combined variant instead of the train choice")
)

func main() {
	flag.Parse()
	// Local only: the eval writes test data, and the owner suite reads private memories.
	if u, err := url.Parse(*apiURL); err != nil || !isLoopback(u.Hostname()) {
		logf("refusing -api %q: the eval runs against a local API only", *apiURL)
		os.Exit(2)
	}
	if u, err := url.Parse(*dsn); err != nil || !isLoopback(u.Hostname()) {
		logf("refusing -dsn: the eval reads a local database only")
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch *suite {
	case "synth":
		err = runSynth(ctx)
	case "lme":
		err = runLME(ctx)
	case "owner":
		err = runOwner(ctx)
	case "parity":
		err = runParity(ctx)
	case "similar":
		err = runSimilar(ctx)
	default:
		err = fmt.Errorf("unknown suite %q", *suite)
	}
	if err != nil {
		logf("memeval: %v", err)
		os.Exit(1)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadWorld(ctx context.Context, c *client, owner int64, w *world) (loaded, error) {
	byID := map[int64]*synthMem{}
	for _, m := range w.mems {
		id, err := c.write(m.write) // idempotent by origin: a rerun gets the same ids back
		if err != nil {
			return loaded{}, err
		}
		m.id = id
		byID[id] = m
	}
	if err := waitEmbedded(ctx, *dsn, owner); err != nil {
		return loaded{}, err
	}
	// Search results are cached on disk, so a rerun of the simulation does not depend on the API's load.
	path := filepath.Join(*cacheDir, fmt.Sprintf("hits-%s.json", strings.ReplaceAll(w.repo, "/", "_")))
	cache := map[string][]hit{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &cache)
	}
	return loaded{w: w, byID: byID, hits: func(text string) []hit {
		if h, ok := cache[text]; ok {
			return h
		}
		h, mode, _, err := c.search(text, w.repo, 50)
		if err != nil || mode != "hybrid" { // a fallback would silently change the baseline
			logf("search %q: mode %s err %v", text, mode, err)
			os.Exit(1)
		}
		cache[text] = h
		if raw, err := json.Marshal(cache); err == nil {
			_ = os.WriteFile(path, raw, 0o600)
		}
		return h
	}}, nil
}

var grid = []float64{0.1, 0.25, 0.5, 1}

func synthVariants() []variant {
	vs := []variant{{name: "flat"}, {name: "recency only", w: memrank.Weights{Strength: 1}, recencyOnly: true}}
	for _, x := range grid {
		vs = append(vs, variant{name: fmt.Sprintf("strength w=%g", x), w: memrank.Weights{Strength: x}})
	}
	for _, x := range grid {
		vs = append(vs, variant{name: fmt.Sprintf("confidence w=%g", x), w: memrank.Weights{Confidence: x}})
	}
	for _, s := range grid {
		for _, c := range grid {
			vs = append(vs, variant{name: fmt.Sprintf("both s=%g c=%g", s, c), w: memrank.Weights{Strength: s, Confidence: c}})
		}
	}
	return vs
}

func runSynth(ctx context.Context) error {
	c, err := login(*apiURL, "synth@memeval.local", *password)
	if err != nil {
		return err
	}
	owner, err := ownerID(ctx, *dsn, "synth@memeval.local")
	if err != nil {
		return err
	}
	train, test := []int{1, 2, 3}, []int{4, 5, 6, 7, 8}
	worlds := map[int]loaded{}
	load := func(seed int, rho float64) (loaded, error) {
		w := newWorld(seed, rho, 1500)
		if base, ok := worlds[seed]; ok { // same memories (they are drawn first); only the queries differ
			for i, m := range base.w.mems {
				w.mems[i].id = m.id
			}
			byID := map[int64]*synthMem{}
			for _, m := range w.mems {
				byID[m.id] = m
			}
			return loaded{w: w, byID: byID, hits: base.hits}, nil
		}
		logf("loading world %d (%d memories)", seed, len(w.mems))
		l, err := loadWorld(ctx, c, owner, w)
		worlds[seed] = l
		return l, err
	}
	perSeed := func(seeds []int, rho float64, v variant, fb feedback) ([]simResult, error) {
		var rs []simResult
		for _, s := range seeds {
			l, err := load(s, rho)
			if err != nil {
				return nil, err
			}
			rs = append(rs, simulate(l, v, fb, s))
		}
		return rs, nil
	}
	run := func(seeds []int, rho float64, v variant, fb feedback) (simResult, error) {
		rs, err := perSeed(seeds, rho, v, fb)
		return merge(rs), err
	}

	vs := synthVariants()
	fmt.Printf("## Synthetic world: weight selection on train seeds 1-3 (rho 0.5, %d feedback credential(s))\n", actors)
	fmt.Println("\n| variant | n | hit@1 | recall@5 | recall@10 | MRR |\n|---|---|---|---|---|---|")
	best := map[string]variant{}
	bestHit := map[string]float64{}
	for _, v := range vs {
		r, err := run(train, 0.5, v, feedbackTrain)
		if err != nil {
			return err
		}
		fmt.Printf("| %s | %s |\n", v.name, r.all.row())
		fam := strings.Fields(v.name)[0]
		if h := r.all.frac(r.all.hit1); h > bestHit[fam] || best[fam].name == "" {
			best[fam], bestHit[fam] = v, h
		}
	}
	both := best["both"]
	if *forceS > 0 || *forceC > 0 {
		both = variant{name: fmt.Sprintf("both s=%g c=%g (fixed)", *forceS, *forceC), w: memrank.Weights{Strength: *forceS, Confidence: *forceC}}
	}
	withSup := both
	withSup.name, withSup.supersede = "supersede + "+both.name, true
	chosen := []variant{vs[0], vs[1], {name: "supersede (oracle)", supersede: true}, best["strength"], best["confidence"], both, withSup}

	fmt.Println("\n## Synthetic world: held-out test seeds 4-8, rho 0.5, same feedback model")
	if err := synthTable(run, test, 0.5, chosen, feedbackTrain); err != nil {
		return err
	}
	fmt.Println("\n## Synthetic world: held-out seeds, mis-specified agent feedback")
	if err := synthTable(run, test, 0.5, chosen, feedbackShift); err != nil {
		return err
	}

	fmt.Println("\n## Paired hit@1 change against flat, per test seed (same feedback model)")
	fmt.Println("\n| variant | mean | min | max | seeds better |\n|---|---|---|---|---|")
	flatRs, err := perSeed(test, 0.5, vs[0], feedbackTrain)
	if err != nil {
		return err
	}
	for _, v := range chosen[1:] {
		rs, err := perSeed(test, 0.5, v, feedbackTrain)
		if err != nil {
			return err
		}
		mean, lo, hi, better := 0.0, 1.0, -1.0, 0
		for i, r := range rs {
			d := r.all.frac(r.all.hit1) - flatRs[i].all.frac(flatRs[i].all.hit1)
			mean += d / float64(len(rs))
			lo, hi = min(lo, d), max(hi, d)
			if d > 0 {
				better++
			}
		}
		fmt.Printf("| %s | %+.3f | %+.3f | %+.3f | %d/%d |\n", v.name, mean, lo, hi, better, len(rs))
	}

	fmt.Println("\n## Feedback credentials (hit@1 on test seeds; the server counts one vote per credential)")
	fmt.Println("\n| variant | 1 credential | 3 credentials | no cap |\n|---|---|---|---|")
	saved := actors
	for _, v := range []variant{best["confidence"], both} {
		row := "| " + v.name
		for _, a := range []int{1, 3, 0} {
			actors = a
			r, err := run(test, 0.5, v, feedbackTrain)
			if err != nil {
				return err
			}
			row += fmt.Sprintf(" | %.3f", r.all.frac(r.all.hit1))
		}
		fmt.Println(row + " |")
	}
	actors = saved

	fmt.Println("\n## Need-distribution sweep (hit@1 on test seeds; rho = share of queries that revisit a recent slot)")
	fmt.Println("\n| variant | rho 0 | rho 0.25 | rho 0.5 | rho 0.75 | rho 0.9 |\n|---|---|---|---|---|---|")
	for _, v := range chosen {
		row := "| " + v.name
		for _, rho := range []float64{0, 0.25, 0.5, 0.75, 0.9} {
			r, err := run(test, rho, v, feedbackTrain)
			if err != nil {
				return err
			}
			row += fmt.Sprintf(" | %.3f", r.all.frac(r.all.hit1))
		}
		fmt.Println(row + " |")
	}

	// Calibration: fit Platt on train seeds, evaluate on test seeds (and on the shifted feedback).
	trainR, err := run(train, 0.5, both, feedbackTrain)
	if err != nil {
		return err
	}
	cal := platt(trainR.cal)
	fmt.Println("\n## Calibration of the evidence score (same-slot memories in the top 5; variant " + both.name + ")")
	fmt.Println("\n| scorer | feedback | n | ECE (15 bins) | Brier | AUC |\n|---|---|---|---|---|---|")
	for _, f := range []struct {
		name string
		fb   feedback
	}{{"train model", feedbackTrain}, {"mis-specified", feedbackShift}} {
		r, err := run(test, 0.5, both, f.fb)
		if err != nil {
			return err
		}
		for _, s := range []struct {
			name string
			c    calib
		}{{"stated level only", r.stated}, {"Beta posterior", r.cal}, {"Beta + Platt (fit on train)", mapCalib(r.cal, cal)}} {
			fmt.Printf("| %s | %s | %d | %.3f | %.3f | %.3f |\n", s.name, f.name, len(s.c.p), s.c.ece(15), s.c.brier(), s.c.auc())
		}
	}
	r, err := run(test, 0.5, both, feedbackTrain)
	if err != nil {
		return err
	}
	fmt.Println("\nReliability on test seeds, Beta posterior (as the server returns it) and Beta + Platt:\n\n" + r.cal.reliability() + "\n" + mapCalib(r.cal, cal).reliability())
	if err := conflictExperiment(ctx, owner, worlds, test); err != nil {
		return err
	}

	vecs, err := embeddings(ctx, *dsn, owner)
	if err != nil {
		return err
	}
	fmt.Println("\n## Similar memories on write (the writer supersedes a shown older value with probability act; test seeds)")
	fmt.Println("\n| threshold | act | flat hit@1 | flat stale | both hit@1 | both stale |\n|---|---|---|---|---|---|")
	for _, th := range []float64{0.8, 0.85, 0.9} {
		for _, act := range []float64{0.5, 0.9} {
			var fr, br []simResult
			for _, sd := range test {
				l, err := load(sd, 0.5)
				if err != nil {
					return err
				}
				a := assisted(l, vecs, th, act, sd)
				fr = append(fr, simulate(a, vs[0], feedbackTrain, sd))
				br = append(br, simulate(a, both, feedbackTrain, sd))
			}
			f, b := merge(fr), merge(br)
			stale := func(r simResult) float64 {
				n := 0
				for _, v := range r.top1 {
					n += v
				}
				return float64(r.top1["stale"]) / float64(max(n, 1))
			}
			fmt.Printf("| %.2f | %.1f | %.3f | %.3f | %.3f | %.3f |\n", th, act, f.all.frac(f.all.hit1), stale(f), b.all.frac(b.all.hit1), stale(b))
		}
	}
	return nil
}

func synthTable(run func([]int, float64, variant, feedback) (simResult, error), seeds []int, rho float64, vs []variant, fb feedback) error {
	fmt.Println("\n| variant | n | hit@1 | recall@5 | recall@10 | MRR | hit@1 hot | hit@1 cold | top-1 right | stale | wrong | other |\n|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, v := range vs {
		r, err := run(seeds, rho, v, fb)
		if err != nil {
			return err
		}
		fmt.Printf("| %s | %s | %.3f | %.3f | %s |\n", v.name, r.all.row(), r.hot.frac(r.hot.hit1), r.cold.frac(r.cold.hit1), r.top1Row())
	}
	return nil
}

// conflictExperiment: when a memory is written, flag earlier memories of the same scope whose vectors are within
// a cosine threshold. A same-slot pair with a different value is a conflict (an update or an error); with the
// same value it is a restatement. Both deserve review; anything else is a false alarm.
func conflictExperiment(ctx context.Context, owner int64, worlds map[int]loaded, seeds []int) error {
	vecs, err := embeddings(ctx, *dsn, owner)
	if err != nil {
		return err
	}
	thresholds := []float64{0.75, 0.8, 0.85, 0.9, 0.95}
	type counts struct{ conflict, restate, other int }
	flagged := make([]counts, len(thresholds))
	var totalConflict, writes int
	start := time.Now()
	for _, s := range seeds {
		ms := append([]*synthMem(nil), worlds[s].w.mems...)
		sort.Slice(ms, func(i, j int) bool { return ms[i].created < ms[j].created })
		for i, m := range ms {
			if m.slot.svc < 0 {
				continue
			}
			writes++
			for _, o := range ms[:i] {
				same := o.slot == m.slot
				if same && o.value != m.value {
					totalConflict++
				}
				cs := cosine(vecs[m.id], vecs[o.id])
				for k, th := range thresholds {
					if cs < th {
						continue
					}
					switch {
					case same && o.value != m.value:
						flagged[k].conflict++
					case same:
						flagged[k].restate++
					default:
						flagged[k].other++
					}
				}
			}
		}
	}
	fmt.Printf("\n## Conflict candidates on write (cosine of all-minilm vectors, same scope; %d writes, %d conflicting pairs)\n", writes, totalConflict)
	fmt.Println("\n| threshold | flagged per write | precision (conflict or restatement) | precision (conflict only) | recall of conflicts |\n|---|---|---|---|---|")
	for k, th := range thresholds {
		f := flagged[k]
		all := float64(f.conflict + f.restate + f.other)
		fmt.Printf("| %.2f | %.2f | %.3f | %.3f | %.3f |\n", th, all/float64(writes), float64(f.conflict+f.restate)/max(all, 1),
			float64(f.conflict)/max(all, 1), float64(f.conflict)/max(float64(totalConflict), 1))
	}
	logf("conflict scan took %v", time.Since(start))
	return nil
}
