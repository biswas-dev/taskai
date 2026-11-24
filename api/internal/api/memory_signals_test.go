package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"taskai/internal/memrank"
)

// memoryKit carries runMemoryTests' helpers, so the signal tests run on SQLite and Postgres alike.
type memoryKit struct {
	user             func(name string) int64
	write            func(u int64, req MemoryWriteRequest) (int, MemoryWriteResponse, string)
	mem              func(title, body string) MemoryWriteRequest
	get              func(u, id int64) (int, Memory)
	search           func(u int64, req MemorySearchRequest) MemorySearchResponse
	compact          func(u int64) MemoryCompactReport
	friend, outsider int64
}

// testMemorySignals covers feedback, signals, similar-on-write, the re-rank and compaction with evidence.
func testMemorySignals(t *testing.T, ts *TestServer, k memoryKit) {
	ctx := context.Background()
	user, write, mem, get, search, compact, friend, outsider := k.user, k.write, k.mem, k.get, k.search, k.compact, k.friend, k.outsider
	u := user("signals")
	feedbackAs := func(who, id, apiKey int64, signal string) (int, MemoryFeedbackResponse) {
		t.Helper()
		sid := fmt.Sprint(id)
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/"+sid+"/feedback", MemoryFeedbackRequest{Signal: signal}, who, map[string]string{"id": sid})
		if apiKey > 0 {
			r = r.WithContext(context.WithValue(r.Context(), ApiKeyIDKey, apiKey))
		}
		ts.HandleMemoryFeedback(rec, r)
		var resp MemoryFeedbackResponse
		if rec.Code == http.StatusOK {
			DecodeJSON(t, rec, &resp)
		}
		return rec.Code, resp
	}
	feedback := func(who, id int64, signal string) (int, MemoryFeedbackResponse) {
		t.Helper()
		return feedbackAs(who, id, 0, signal)
	}
	req := mem("Deploy port", "The staging deploy listens on port 8443")
	req.OriginHost, req.OriginID = "hub-s", "1"
	_, created, _ := write(u, req)
	id := created.Memory.ID

	if code, _ := feedback(u, id, "useful"); code != http.StatusNotFound {
		t.Fatalf("feedback with the flag off: %d, want 404", code)
	}
	if _, m := get(u, id); m.Signals != nil {
		t.Fatalf("signals with the flag off: %+v", m.Signals)
	}

	ts.config.MemoryStrength = "on"
	defer func() { ts.config.MemoryStrength = "off" }()
	steps := []struct{ signal, want string }{
		{"useful", "recorded"}, {"useful", "spaced"}, {"confirmed", "recorded"}, {"confirmed", "duplicate"}, {"wrong", "recorded"},
	}
	for _, st := range steps {
		if code, resp := feedback(u, id, st.signal); code != http.StatusOK || resp.Outcome != st.want {
			t.Fatalf("%s: %d %q, want %q", st.signal, code, resp.Outcome, st.want)
		}
	}
	// Each credential has its own vote; "useful" counts again once the hour has passed.
	if _, resp := feedbackAs(u, id, 7, "wrong"); resp.Outcome != "recorded" {
		t.Fatalf("a vote from an API key: %q", resp.Outcome)
	}
	if _, resp := feedbackAs(u, id, 7, "wrong"); resp.Outcome != "duplicate" {
		t.Fatalf("a repeat from the same API key: %q", resp.Outcome)
	}
	if _, err := ts.DB.ExecContext(ctx, `UPDATE agent_memories SET last_used_at = $1 WHERE id = $2`, time.Now().UTC().Add(-2*time.Hour), id); err != nil {
		t.Fatal(err)
	}
	if _, resp := feedback(u, id, "useful"); resp.Outcome != "recorded" || resp.Memory.Signals.Uses != 4 || resp.Memory.Signals.Wrong != 2 {
		t.Fatalf("useful after the spacing: %q %+v", resp.Outcome, resp.Memory.Signals)
	}
	if code, _ := feedback(u, id, "liked"); code != http.StatusBadRequest {
		t.Fatalf("bad signal: %d", code)
	}
	if code, _ := feedback(outsider, id, "wrong"); code != http.StatusNotFound {
		t.Fatalf("outsider feedback: %d", code)
	}
	// A replay keeps the evidence; new text resets the votes (they judged the old text) but keeps the uses.
	req.Author.Session = "replay"
	if _, resp, _ := write(u, req); resp.Outcome != "updated" {
		t.Fatalf("replay: %+v", resp)
	}
	_, m := get(u, id)
	sig := m.Signals
	if sig == nil || sig.Uses != 4 || sig.Confirmed != 1 || sig.Wrong != 2 || sig.LastUsedAt == nil {
		t.Fatalf("signals after feedback and replay: %+v", sig)
	}
	if want := (1 + 0.7*2) / 5.0; sig.Belief < want-1e-9 || sig.Belief > want+1e-9 {
		t.Fatalf("belief %v, want %v", sig.Belief, want)
	}
	req.Body = "The staging deploy listens on port 9443"
	if _, resp, _ := write(u, req); resp.Outcome != "updated" {
		t.Fatalf("new text: %+v", resp)
	}
	if _, m := get(u, id); m.Signals.Uses != 4 || m.Signals.Confirmed != 0 || m.Signals.Wrong != 0 {
		t.Fatalf("signals after new text: %+v", m.Signals)
	}
	if code, resp := feedback(u, id, "confirmed"); code != http.StatusOK || resp.Outcome != "recorded" {
		t.Fatalf("a vote on the new text: %d %q", code, resp.Outcome)
	}

	// A write shows close older memories of the same repo, so the writer can supersede them (Postgres only).
	ts.embedPendingMemories(ctx)
	near := mem("Deploy port changed", "The staging deploy listens on port 9443 now")
	_, nr, _ := write(u, near)
	if ts.DB.Driver == "postgres" && ts.embeddingClient != nil {
		if len(nr.Similar) == 0 || nr.Similar[0].ID != id || nr.Similar[0].Similarity < memorySimilarMin {
			t.Fatalf("similar on write: %+v", nr.Similar)
		}
		// A replay of the same text shows nothing new; changed text looks again.
		rp := mem("Deploy port note", "The staging deploy port is 9443 per the runbook")
		rp.OriginHost, rp.OriginID = "hub-r", "1"
		write(u, rp)
		ts.embedPendingMemories(ctx)
		rp.Author.Session = "replay"
		if _, again, _ := write(u, rp); again.Outcome != "updated" || again.Similar != nil {
			t.Fatalf("a replay must not look for similar memories: %q %+v", again.Outcome, again.Similar)
		}
		rp.Title, rp.Body = "Deploy port changed again", "The staging deploy listens on port 9443 now"
		if _, changed, _ := write(u, rp); len(changed.Similar) == 0 {
			t.Fatalf("changed text must look for similar memories: %+v", changed)
		}
	} else if nr.Similar != nil {
		t.Fatalf("similar needs Postgres and embeddings: %+v", nr.Similar)
	}

	// Acting on similar: send the same write again with supersedes, with or without an origin.
	for _, origin := range []string{"", "act-1"} {
		oldReq := mem("Queue name "+origin, "Jobs go to the queue named alpha "+origin)
		_, older, _ := write(u, oldReq)
		newReq := mem("Queue name "+origin, "Jobs go to the queue named beta "+origin)
		newReq.OriginHost, newReq.OriginID = map[bool]string{true: "hub-act"}[origin != ""], origin
		_, newer, _ := write(u, newReq)
		newReq.Supersedes = older.Memory.ID
		if _, again, _ := write(u, newReq); again.Memory.ID != newer.Memory.ID {
			t.Fatalf("origin %q: the repeat must land on the same memory: %+v", origin, again)
		}
		if _, m := get(u, older.Memory.ID); m.Status != "superseded" {
			t.Fatalf("origin %q: the shown memory must be superseded, got %s", origin, m.Status)
		}
	}

	// Compaction folds a copy's evidence into the copy it keeps, and keeps a confirmed old note whole.
	c := user("signals-compact")
	copyReq := mem("Cache TTL", "The cache TTL is 300 seconds")
	copyReq.OriginHost, copyReq.OriginID = "hub-c", "old"
	_, older, _ := write(c, copyReq)
	copyReq.OriginID = "new"
	_, newer, _ := write(c, copyReq)
	feedback(c, older.Memory.ID, "useful")
	feedback(c, older.Memory.ID, "confirmed")
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	var lows []int64
	for i := range 5 {
		lr := mem(fmt.Sprintf("Old note %d", i), fmt.Sprintf("An old low note number %d", i))
		lr.Confidence, lr.CreatedAt = "low", &old
		_, lw, _ := write(c, lr)
		lows = append(lows, lw.Memory.ID)
	}
	feedback(c, lows[0], "confirmed")
	feedback(c, lows[1], "useful") // used recently: in use, so kept whole too
	if rep := compact(c); rep.DuplicatesMerged != 1 || rep.LowConfidenceSummarised != 3 {
		t.Fatalf("compaction: %+v", rep)
	}
	if _, m := get(c, newer.Memory.ID); m.Status != "active" || m.Signals.Uses != 3 || m.Signals.Confirmed != 1 {
		t.Fatalf("kept copy %s: %+v", m.Status, m.Signals)
	}
	if _, resp := feedback(c, newer.Memory.ID, "confirmed"); resp.Outcome != "duplicate" || resp.Memory.Signals.Confirmed != 1 {
		t.Fatalf("the vote moved with the fold, so the same credential cannot vote again: %q %+v", resp.Outcome, resp.Memory.Signals)
	}
	for _, keep := range lows[:2] {
		if _, m := get(c, keep); m.Status != "active" {
			t.Fatalf("a confirmed or recently used old note must not be merged: %s", m.Status)
		}
	}

	// The evidence of a folded copy protects the kept copy in the same compaction run.
	d := user("signals-fold")
	var kept int64
	for i, origin := range []string{"a", "b"} {
		dr := mem("Old copy", "An old low note kept twice")
		dr.Confidence, dr.CreatedAt, dr.OriginHost, dr.OriginID = "low", &old, "hub-d", origin
		_, dw, _ := write(d, dr)
		if i == 0 {
			feedback(d, dw.Memory.ID, "confirmed") // the older copy, which the fold removes
		}
		kept = dw.Memory.ID
	}
	for i := range 2 {
		lr := mem(fmt.Sprintf("Other old note %d", i), fmt.Sprintf("Another old low note %d", i))
		lr.Confidence, lr.CreatedAt = "low", &old
		write(d, lr)
	}
	if rep := compact(d); rep.DuplicatesMerged != 1 || rep.LowConfidenceSummarised != 0 {
		t.Fatalf("fold then summary: %+v", rep)
	}
	if _, m := get(d, kept); m.Status != "active" || m.Signals.Confirmed != 1 {
		t.Fatalf("kept copy after fold: %s %+v", m.Status, m.Signals)
	}
	res := search(u, MemorySearchRequest{Query: "staging deploy port", Limit: 1})
	if len(res.Results) != 1 || res.Results[0].Signals == nil {
		t.Fatalf("re-ranked search: %+v", res.Results)
	}

	// Two copies of one text rank by id when flat; evidence reorders them when re-ranked.
	e := user("signals-rerank")
	twin := mem("Feature flag name", "The rollout flag is called fast-path")
	twin.OriginHost, twin.OriginID = "hub-e", "1"
	_, first, _ := write(e, twin)
	twin.OriginID = "2"
	_, second, _ := write(e, twin)
	feedback(e, first.Memory.ID, "useful")
	feedback(e, first.Memory.ID, "confirmed")
	feedback(e, second.Memory.ID, "wrong")
	q := MemorySearchRequest{Query: "rollout flag fast-path", Limit: 2}
	ranked := search(e, q)
	ts.config.MemoryStrength = "collect" // signals and feedback, but flat search
	flat := search(e, q)
	if len(flat.Results) != 2 || flat.Results[0].ID != second.Memory.ID || flat.Results[0].Signals == nil {
		t.Fatalf("collect keeps the flat order with signals: %+v", flat.Results)
	}
	var cands []memrank.Candidate
	for _, m := range flat.Results {
		cands = append(cands, memrank.Candidate{ID: m.ID, Relevance: m.Rank, Strength: m.Signals.Strength, Confidence: m.Signals.Belief})
	}
	want := memrank.Rerank(cands, memoryWeights, 2)
	if len(ranked.Results) != 2 || ranked.Results[0].ID != want[0].ID || ranked.Results[1].ID != want[1].ID {
		t.Fatalf("re-ranked order %+v, want %+v", ranked.Results, want)
	}
	rec, lr := ts.MakeAuthRequest(t, http.MethodGet, "/api/memories", nil, e, nil)
	ts.HandleListMemories(rec, lr)
	var list struct{ Memories []Memory }
	DecodeJSON(t, rec, &list)
	if len(list.Memories) != 2 || list.Memories[0].Signals == nil {
		t.Fatalf("list signals: %+v", list.Memories)
	}
	// A sharee reads but cannot vote; a superseded memory takes no feedback.
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO memory_shares (owner_id, user_id) VALUES ($1, $2)`, e, friend); err != nil {
		t.Fatal(err)
	}
	if code, _ := feedback(friend, first.Memory.ID, "useful"); code != http.StatusForbidden {
		t.Fatalf("sharee feedback: %d", code)
	}
	repl := mem("Feature flag name", "The rollout flag is now called turbo")
	repl.Supersedes = first.Memory.ID
	write(e, repl)
	if code, _ := feedback(e, first.Memory.ID, "confirmed"); code != http.StatusBadRequest {
		t.Fatalf("feedback on a superseded memory: %d", code)
	}

	// A fold keeps one vote per credential, moves the other copy's votes, and keeps the newer last use.
	g := user("signals-fold-votes")
	twinReq := mem("Retry limit", "Retries stop after 5 attempts")
	twinReq.OriginHost, twinReq.OriginID = "hub-g", "old"
	_, gOld, _ := write(g, twinReq)
	twinReq.OriginID = "new"
	_, gNew, _ := write(g, twinReq)
	feedback(g, gNew.Memory.ID, "useful")
	feedback(g, gNew.Memory.ID, "wrong")
	feedback(g, gOld.Memory.ID, "wrong")          // the same credential: folds into one vote
	feedbackAs(g, gOld.Memory.ID, 9, "confirmed") // another credential: moves to the kept copy
	if rep := compact(g); rep.DuplicatesMerged != 1 {
		t.Fatalf("fold: %+v", rep)
	}
	if _, m := get(g, gNew.Memory.ID); m.Signals.Wrong != 1 || m.Signals.Confirmed != 1 || m.Signals.Uses != 3 || m.Signals.LastUsedAt == nil {
		t.Fatalf("kept copy after a fold with shared votes: %+v", m.Signals)
	}
	if _, resp := feedbackAs(g, gNew.Memory.ID, 9, "confirmed"); resp.Outcome != "duplicate" {
		t.Fatalf("the moved vote still counts once: %q", resp.Outcome)
	}

	// Feedback that judged text an upsert has since replaced is refused.
	ch := mem("Region", "Deploys go to us-east-1")
	ch.OriginHost, ch.OriginID = "hub-h", "1"
	_, chw, _ := write(g, ch)
	_, read := get(g, chw.Memory.ID)
	ch.Body = "Deploys go to eu-west-1"
	write(g, ch)
	judge := func(at time.Time) int {
		sid := fmt.Sprint(chw.Memory.ID)
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/"+sid+"/feedback",
			MemoryFeedbackRequest{Signal: "wrong", UpdatedAt: &at}, g, map[string]string{"id": sid})
		ts.HandleMemoryFeedback(rec, r)
		return rec.Code
	}
	if code := judge(read.UpdatedAt.Add(-time.Hour)); code != http.StatusConflict {
		t.Fatalf("feedback with a stale updated_at: %d, want 409", code)
	}
	_, now := get(g, chw.Memory.ID)
	if code := judge(now.UpdatedAt); code != http.StatusOK {
		t.Fatalf("feedback with the current updated_at: %d", code)
	}

	// Old low notes with a wrong vote stay whole (a summary would reset the vote); expired memories take no feedback.
	old = time.Now().UTC().Add(-100 * 24 * time.Hour)
	o := user("signals-wrong-old")
	var olds []int64
	for i := range 3 {
		lr := mem(fmt.Sprintf("Stale note %d", i), fmt.Sprintf("A stale low note %d", i))
		lr.Confidence, lr.CreatedAt = "low", &old
		_, lw, _ := write(o, lr)
		olds = append(olds, lw.Memory.ID)
	}
	feedback(o, olds[0], "wrong")
	if rep := compact(o); rep.LowConfidenceSummarised != 0 {
		t.Fatalf("a wrong-voted note left 2 others, below a cluster: %+v", rep)
	}
	soon := time.Now().UTC().Add(time.Hour)
	ex := mem("Temp token rule", "Tokens rotate daily this week")
	ex.ExpiresAt = &soon
	_, exw, _ := write(o, ex)
	if _, err := ts.DB.ExecContext(ctx, `UPDATE agent_memories SET expires_at = $1 WHERE id = $2`, time.Now().UTC().Add(-time.Minute), exw.Memory.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := feedback(o, exw.Memory.ID, "useful"); code != http.StatusBadRequest {
		t.Fatalf("feedback on an expired memory: %d", code)
	}

	// A wrong-only copy folds into a kept copy that was used: the last use stays, the vote moves.
	f := user("signals-fold-wrong")
	fr := mem("Cron window", "Backups run at 02:00 UTC")
	fr.OriginHost, fr.OriginID = "hub-f", "old"
	_, fOld, _ := write(f, fr)
	fr.OriginID = "new"
	_, fNew, _ := write(f, fr)
	feedback(f, fNew.Memory.ID, "useful")
	feedback(f, fOld.Memory.ID, "wrong")
	compact(f)
	if _, m := get(f, fNew.Memory.ID); m.Signals.Wrong != 1 || m.Signals.Uses != 2 || m.Signals.LastUsedAt == nil {
		t.Fatalf("wrong-only fold: %+v", m.Signals)
	}

	// An ignored write (a compacted origin) carries no signals.
	ig := mem("Gone soon", "This memory is deleted and its origin remembered")
	ig.OriginHost, ig.OriginID = "hub-i", "1"
	_, igw, _ := write(f, ig)
	srec, sr := ts.MakeAuthRequest(t, http.MethodDelete, "/api/memories/"+fmt.Sprint(igw.Memory.ID), nil, f, map[string]string{"id": fmt.Sprint(igw.Memory.ID)})
	ts.HandleDeleteMemory(srec, sr)
	if _, again, _ := write(f, ig); again.Outcome != "ignored" || again.Memory.Signals != nil {
		t.Fatalf("ignored write: %q %+v", again.Outcome, again.Memory.Signals)
	}
}
