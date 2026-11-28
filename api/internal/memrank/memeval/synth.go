package main

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// The synthetic world: services with settings (slots) whose values change over 120 days. Agents of different
// reliability write memories about them, sometimes wrong, sometimes forgetting to supersede the old value.
// Queries ask for a slot's current value. Everything is generated: no private data.

var (
	epoch    = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	horizon  = 120.0 // days
	services = []string{"billing", "auth", "search", "ledger", "notify", "upload", "report", "gateway", "catalog", "audit",
		"scheduler", "export", "invoice", "profile", "session", "metrics", "pricing", "inventory", "checkout", "webhook"}
	suffixes = []string{"api", "worker"}
	attrs    = []struct {
		name   string
		values []string
	}{
		{"database port", []string{"5432", "5433", "6432", "15432", "25432", "5434"}},
		{"deploy region", []string{"us-east-1", "us-west-2", "eu-central-1", "ap-south-1", "ca-central-1", "eu-west-1"}},
		{"owning team", []string{"platform", "payments", "growth", "infra", "identity", "data"}},
		{"test runner", []string{"jest", "vitest", "go test", "pytest", "mocha", "cargo test"}},
		{"node version", []string{"18", "20", "22", "24", "16", "21"}},
		{"release day", []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Sunday"}},
		{"ci provider", []string{"GitHub Actions", "Buildkite", "CircleCI", "Jenkins", "Travis", "Drone"}},
		{"cache backend", []string{"redis", "memcached", "valkey", "dragonfly", "keydb", "hazelcast"}},
	}
	bodyTpl = []string{
		"The %[2]s for %[1]s is %[3]s.",
		"%[1]s uses %[3]s as its %[2]s.",
		"Set the %[1]s %[2]s to %[3]s; other values break it.",
		"Use %[3]s for the %[2]s of %[1]s.",
		"Checked today: %[1]s %[2]s = %[3]s.",
	}
	titleTpl = []string{"%[1]s %[2]s", "%[2]s for %[1]s", "%[1]s: %[2]s is %[3]s"}
	queryTpl = []string{"what is the %[2]s of %[1]s", "%[1]s %[2]s", "which %[2]s does %[1]s use"}
	noise    = []string{"incident review notes", "flaky test seen in CI", "on-call handover", "dashboard link moved",
		"slow query investigated", "dependency bump merged", "alert threshold tuned", "load test results"}
)

// sources write memories; err is how often a source states a wrong value.
var sources = []struct {
	name   string
	weight float64
	err    float64
}{{"careful-agent", 0.4, 0.03}, {"fast-agent", 0.3, 0.08}, {"batch-import", 0.2, 0.2}, {"guessing-agent", 0.1, 0.4}}

type slot struct{ svc, attr int }

type change struct {
	at    float64
	value int
}

type synthMem struct {
	idx        int
	slot       slot // svc = -1 for a distractor
	value      int
	created    float64
	superseded float64 // 0: never
	stated     string
	source     int
	write      memWrite
	id         int64 // TaskAI id after loading
}

type synthQuery struct {
	at   float64
	slot slot
	text string
	cold bool // the slot was never asked before
}

type world struct {
	seed    int
	repo    string
	history map[slot][]change
	mems    []*synthMem
	queries []synthQuery
}

func (w *world) svcName(i int) string {
	return services[i/len(suffixes)] + "-" + suffixes[i%len(suffixes)]
}

// truth is a slot's value at time t, or -1 before it is first set.
func (w *world) truth(s slot, t float64) int {
	v := -1
	for _, c := range w.history[s] {
		if c.at <= t {
			v = c.value
		}
	}
	return v
}

// correct: the memory exists at t, is not superseded at t, and states the slot's value at t.
func (w *world) correct(m *synthMem, s slot, t float64) bool {
	return m.slot == s && m.created <= t && (m.superseded == 0 || m.superseded > t) && m.value == w.truth(s, t)
}

func pickSource(r *rand.Rand) int {
	x := r.Float64()
	for i, s := range sources {
		if x < s.weight {
			return i
		}
		x -= s.weight
	}
	return len(sources) - 1
}

// newWorld generates one world. rho is the share of queries that revisit a recently asked slot (bursty need,
// Anderson and Schooler 1991); the rest ask a uniformly random slot.
func newWorld(seed int, rho float64, nQueries int) *world {
	r := rand.New(rand.NewPCG(uint64(seed), 7))
	w := &world{seed: seed, repo: fmt.Sprintf("synth%d/world", seed), history: map[slot][]change{}}
	nSvc := len(services) * len(suffixes)
	var slots []slot
	for s := range nSvc {
		for a := range attrs {
			if r.Float64() < 0.6 {
				slots = append(slots, slot{s, a})
			}
		}
	}
	add := func(m *synthMem) { m.idx = len(w.mems); w.mems = append(w.mems, m) }
	text := func(s slot, v int, tpl int) (string, string) {
		svc, an, val := w.svcName(s.svc), attrs[s.attr].name, attrs[s.attr].values[v]
		return fmt.Sprintf(titleTpl[tpl%len(titleTpl)], svc, an, val), fmt.Sprintf(bodyTpl[r.IntN(len(bodyTpl))], svc, an, val)
	}
	write := func(s slot, trueV int, at float64) *synthMem {
		src := pickSource(r)
		v := trueV
		if r.Float64() < sources[src].err {
			v = (trueV + 1 + r.IntN(len(attrs[s.attr].values)-1)) % len(attrs[s.attr].values)
		}
		stated := "high"
		if r.Float64() < 0.05 {
			stated = "medium"
		}
		m := &synthMem{slot: s, value: v, created: at, stated: stated, source: src}
		add(m)
		return m
	}
	for _, s := range slots {
		t := r.Float64() * 30
		v := r.IntN(len(attrs[s.attr].values))
		w.history[s] = []change{{t, v}}
		var last *synthMem
		for {
			m := write(s, v, t)
			if last != nil && r.Float64() < 0.4 { // the writer remembered to supersede the old value
				last.superseded = t
			}
			last = m
			// Restatements by other agents until the next change.
			next := t + r.ExpFloat64()*90
			for rt := t + r.ExpFloat64()*60; rt < min(next, horizon); rt += r.ExpFloat64() * 60 {
				write(s, v, rt)
			}
			if next >= horizon {
				break
			}
			t = next
			v = (v + 1 + r.IntN(len(attrs[s.attr].values)-1)) % len(attrs[s.attr].values)
			w.history[s] = append(w.history[s], change{t, v})
		}
	}
	for range 300 {
		s := r.IntN(nSvc)
		add(&synthMem{slot: slot{-1, -1}, created: r.Float64() * horizon, stated: "high", source: pickSource(r),
			write: memWrite{Kind: "fact", Title: w.svcName(s) + " " + noise[r.IntN(len(noise))],
				Body: fmt.Sprintf("%s: %s on day %d; see the runbook.", w.svcName(s), noise[r.IntN(len(noise))], r.IntN(120))}})
	}
	for i, m := range w.mems {
		if m.slot.svc >= 0 {
			title, body := text(m.slot, m.value, r.IntN(len(titleTpl)))
			m.write = memWrite{Kind: "fact", Title: title, Body: body}
		}
		at := epoch.Add(time.Duration(m.created * 24 * float64(time.Hour)))
		m.write.Scope = memScope{Repo: w.repo}
		m.write.Confidence = m.stated
		m.write.OriginHost = "memeval-" + w.repo
		m.write.OriginID = fmt.Sprint(i)
		m.write.CreatedAt = &at
	}

	asked := map[slot]bool{}
	var recent []slot
	times := make([]float64, nQueries)
	for i := range times {
		times[i] = 30 + r.Float64()*(horizon-30)
	}
	sort.Float64s(times)
	for _, t := range times {
		var s slot
		if len(recent) > 0 && r.Float64() < rho {
			s = recent[max(0, len(recent)-50)+r.IntN(min(50, len(recent)))]
		} else {
			for {
				s = slots[r.IntN(len(slots))]
				if w.truth(s, t) >= 0 {
					break
				}
			}
		}
		if w.truth(s, t) < 0 {
			continue
		}
		q := fmt.Sprintf(queryTpl[r.IntN(len(queryTpl))], w.svcName(s.svc), attrs[s.attr].name)
		w.queries = append(w.queries, synthQuery{at: t, slot: s, text: strings.TrimSpace(q), cold: !asked[s]})
		asked[s] = true
		recent = append(recent, s)
	}
	return w
}

// feedback is how a simulated agent reacts to the top result.
type feedback struct {
	useRight, confirmRight, detectWrong, useWrong, falseWrong float64
}

var (
	feedbackTrain = feedback{useRight: 0.5, confirmRight: 0.1, detectWrong: 0.5, useWrong: 0.3, falseWrong: 0.03}
	// feedbackShift is a mis-specified agent: it confirms less, catches fewer errors and cites more wrong memories.
	feedbackShift = feedback{useRight: 0.3, confirmRight: 0.05, detectWrong: 0.25, useWrong: 0.5, falseWrong: 0.1}
)
