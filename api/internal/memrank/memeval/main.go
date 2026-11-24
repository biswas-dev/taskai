// Command memeval is the offline eval for memory strength and confidence (docs/research/memory-strength.md).
// It is not shipped. It needs a LOCAL TaskAI API on Postgres + pgvector with the production embedding model,
// and never talks to production. Suites: synth (generated world), lme (LongMemEval_S, public), owner (a private
// export that stays on this machine; only aggregates are printed).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"taskai/internal/memrank"
)

var (
	apiURL   = flag.String("api", "http://localhost:18080", "local TaskAI API")
	dsn      = flag.String("dsn", "postgres://postgres:memeval@localhost:55433/taskai?sslmode=disable", "local Postgres of that API")
	password = flag.String("password", "Memeval-pass-123", "password of the local eval users")
	suite    = flag.String("suite", "synth", "synth | lme | owner")
	lmePath  = flag.String("lme", "", "path to longmemeval_s_cleaned.json")
	ownerIn  = flag.String("owner", "", "path to a private JSON export of memories (owner suite)")
	limitQ   = flag.Int("max", 0, "at most this many lme questions (0: all)")
)

func main() {
	flag.Parse()
	if strings.Contains(*apiURL, "taskai.cc") {
		logf("refusing to run against production")
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
	default:
		err = fmt.Errorf("unknown suite %q", *suite)
	}
	if err != nil {
		logf("memeval: %v", err)
		os.Exit(1)
	}
}

func loadWorld(ctx context.Context, c *client, owner int64, w *world) (loaded, error) {
	byID := map[int64]*synthMem{}
	for _, m := range w.mems {
		id, err := c.write(m.write)
		if err != nil {
			return loaded{}, err
		}
		m.id = id
		byID[id] = m
	}
	if err := waitEmbedded(ctx, *dsn, owner); err != nil {
		return loaded{}, err
	}
	cache := map[string][]hit{}
	return loaded{w: w, byID: byID, hits: func(text string) []hit {
		if h, ok := cache[text]; ok {
			return h
		}
		h, mode, _, err := c.search(text, w.repo, 50)
		if err != nil || mode != "hybrid" {
			logf("search %q: mode %s err %v", text, mode, err)
		}
		cache[text] = h
		return h
	}}, nil
}

func synthVariants() []variant {
	vs := []variant{{name: "flat"}, {name: "recency only", w: memrank.Weights{Strength: 1}, recencyOnly: true}}
	for _, x := range []float64{0.5, 1, 2} {
		vs = append(vs, variant{name: fmt.Sprintf("strength w=%g", x), w: memrank.Weights{Strength: x}})
	}
	for _, x := range []float64{0.5, 1, 2} {
		vs = append(vs, variant{name: fmt.Sprintf("confidence w=%g", x), w: memrank.Weights{Confidence: x}})
	}
	for _, s := range []float64{0.5, 1, 2} {
		for _, c := range []float64{0.5, 1, 2} {
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
	run := func(seeds []int, rho float64, v variant, fb feedback) (simResult, error) {
		var rs []simResult
		for _, s := range seeds {
			l, err := load(s, rho)
			if err != nil {
				return simResult{}, err
			}
			rs = append(rs, simulate(l, v, fb, s))
		}
		return merge(rs), nil
	}

	vs := synthVariants()
	fmt.Println("## Synthetic world: weight selection on train seeds 1-3 (rho 0.5)")
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
	chosen := []variant{vs[0], vs[1], best["strength"], best["confidence"], best["both"]}

	fmt.Println("\n## Synthetic world: held-out test seeds 4-8, rho 0.5, same feedback model")
	if err := synthTable(run, test, 0.5, chosen, feedbackTrain); err != nil {
		return err
	}
	fmt.Println("\n## Synthetic world: held-out seeds, mis-specified agent feedback")
	if err := synthTable(run, test, 0.5, chosen, feedbackShift); err != nil {
		return err
	}
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
	trainR, err := run(train, 0.5, best["both"], feedbackTrain)
	if err != nil {
		return err
	}
	cal := platt(trainR.cal)
	fmt.Println("\n## Calibration of confidence (same-slot memories in the top 5; variant " + best["both"].name + ")")
	fmt.Println("\n| scorer | feedback | n | ECE (15 bins) | Brier | AUC |\n|---|---|---|---|---|---|")
	for _, f := range []struct {
		name string
		fb   feedback
	}{{"train model", feedbackTrain}, {"mis-specified", feedbackShift}} {
		r, err := run(test, 0.5, best["both"], f.fb)
		if err != nil {
			return err
		}
		for _, s := range []struct {
			name string
			c    calib
		}{{"stated level only", r.stated}, {"Beta posterior", r.cal}, {"Beta + Platt (fit on train)", mapCalib(r.cal, cal)}} {
			fmt.Printf("| %s | %s | %d | %.3f | %.3f | %.3f |\n", s.name, f.name, len(s.c.p), s.c.ece(15), s.c.brier(), s.c.auc())
		}
		if f.name == "train model" {
			fmt.Println("\nReliability, Beta + Platt, test seeds:\n\n" + mapCalib(r.cal, cal).reliability())
			fmt.Println("| scorer | feedback | n | ECE (15 bins) | Brier | AUC |\n|---|---|---|---|---|---|")
		}
	}
	return conflictExperiment(ctx, owner, worlds, test)
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
