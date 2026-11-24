package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"taskai/internal/memrank"
)

// Memory strength and belief (MEMORY_STRENGTH=on; docs/research/memory-strength.md). Strength rises when a
// memory is useful or confirmed and decays with time (ACT-R base-level activation). Belief is the
// Beta-Bernoulli posterior of confirmations and wrong votes over a prior from the stated confidence level. Search re-ranks
// its relevance candidates with both, and never drops the most relevant one.

// MemorySignals is what a client sees of a memory's use and evidence.
type MemorySignals struct {
	Strength float64 `json:"strength"` // ACT-R activation: higher is stronger; ~1.6 when just written
	// Belief is an evidence score in (0,1): confirmed and wrong votes over a prior from the stated confidence level.
	// It is NOT calibrated until feedback labels exist to fit it (docs/research/memory-strength.md).
	Belief     float64    `json:"belief"`
	Uses       int        `json:"uses"` // the write counts as the first use
	Confirmed  int        `json:"confirmed"`
	Wrong      int        `json:"wrong"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// memoryWeights are the re-rank weights the offline eval chose (docs/research/memory-strength.md).
var memoryWeights = memrank.Weights{Strength: 0.5, Confidence: 0.25}

// memoryUseSpacing: a "useful" signal counts at most once an hour per memory, since massed uses add little
// strength (Pavlik and Anderson 2005) and a loop must not inflate it.
const memoryUseSpacing = time.Hour

// memoryCollect: feedback, signals and similar-on-write are on (MEMORY_STRENGTH=collect or on).
func (s *Server) memoryCollect() bool {
	return s.config.MemoryStrength == "collect" || s.config.MemoryStrength == "on"
}

// memoryRerank: search is re-ranked by strength and belief (MEMORY_STRENGTH=on).
func (s *Server) memoryRerank() bool { return s.config.MemoryStrength == "on" }

func (m Memory) signals(now time.Time) *MemorySignals {
	last := m.CreatedAt
	if m.use.lastUsed != nil {
		last = *m.use.lastUsed
	}
	return &MemorySignals{
		Strength: memrank.Strength(memrank.Use{Created: m.CreatedAt, Uses: m.use.count, LastUsed: last}, now),
		Belief:   memrank.Confidence(memrank.Evidence{Prior: memrank.Prior(m.Confidence), Confirmed: float64(m.use.confirmed), Wrong: float64(m.use.wrong)}),
		Uses:     m.use.count, Confirmed: m.use.confirmed, Wrong: m.use.wrong, LastUsedAt: m.use.lastUsed,
	}
}

// withSignals sets Signals on each memory when the feature is on.
func (s *Server) withSignals(ms []Memory) {
	if !s.memoryCollect() {
		return
	}
	now := time.Now().UTC()
	for i := range ms {
		if ms[i].ID > 0 && ms[i].Status != "compacted" { // a tombstone (an ignored write) has no evidence
			ms[i].Signals = ms[i].signals(now)
		}
	}
}

// rerankMemories orders relevance candidates by relevance, strength and confidence, and keeps limit.
func rerankMemories(cands []Memory, limit int, now time.Time) []Memory {
	byID := make(map[int64]Memory, len(cands))
	rc := make([]memrank.Candidate, len(cands))
	for i, m := range cands {
		m.Signals = m.signals(now)
		byID[m.ID] = m
		rc[i] = memrank.Candidate{ID: m.ID, Relevance: m.Rank, Strength: m.Signals.Strength, Confidence: m.Signals.Belief}
	}
	ranked := memrank.Rerank(rc, memoryWeights, limit)
	out := make([]Memory, len(ranked))
	for i, c := range ranked {
		out[i] = byID[c.ID]
	}
	return out
}

// resetMemoryVotes: votes judged the old text, so new text starts over (uses stay: it is the same lesson). A replay
// with the same content never gets here, so evidence survives hub retries.
func resetMemoryVotes(ctx context.Context, q sqlQuerier, id int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE agent_memories SET confirmed_count = 0, wrong_count = 0 WHERE id = $1`, id); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `DELETE FROM memory_feedback WHERE memory_id = $1 AND signal <> 'useful'`, id)
	return err
}

// MemorySimilar is an older memory close to a new one.
type MemorySimilar struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Similarity float64 `json:"similarity"` // cosine of the two embeddings
}

const (
	// memorySimilarMin: in the eval, showing same-scope memories above this cosine to the writer, who then
	// supersedes the outdated ones, cut stale answers the most of the thresholds tried (0.80 to 0.90).
	memorySimilarMin     = 0.80
	memorySimilarTimeout = time.Second
)

// similarMemories finds up to 3 active memories of the same owner and repo whose text is close to mem's, so the
// writer can supersede what mem replaces. It is best effort: without embeddings, or past the timeout, it is empty.
func (s *Server) similarMemories(ctx context.Context, mem Memory) []MemorySimilar {
	if !s.memoryCollect() || s.db.Driver != "postgres" || s.embeddingClient == nil || mem.Status != "active" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, memorySimilarTimeout)
	defer cancel()
	text := mem.Title + "\n" + mem.Body
	if len(text) > memoryEmbedInputChars {
		text = strings.ToValidUTF8(text[:memoryEmbedInputChars], "")
	}
	vec, err := s.embeddingClient.Embed(ctx, text)
	if err != nil || vec == nil {
		return nil
	}
	// Keep the vector: the next write sees this memory at once, and the worker need not embed it again.
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_memories SET embedding = $1::vector, embedded_at = $2
		WHERE id = $3 AND content_hash = $4 AND embedding IS NULL`, float32SliceToVectorString(vec), time.Now().UTC(), mem.ID, memoryHash(mem)); err != nil {
		s.logger.Warn("Failed to store memory embedding", zap.Int64("memory_id", mem.ID), zap.Error(err))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, 1 - (embedding <=> $1::vector) AS sim FROM agent_memories
		WHERE owner_id = $2 AND LOWER(repo) = LOWER($3) AND status = 'active' AND id <> $4 AND content_hash <> $5 AND embedding IS NOT NULL
		AND (expires_at IS NULL OR expires_at > $6)
		ORDER BY sim DESC, id DESC LIMIT 3`, // exact over the owner's rows: an HNSW scan filters after its ~40 neighbours
		float32SliceToVectorString(vec), mem.OwnerID, mem.Scope.Repo, mem.ID, memoryHash(mem), time.Now().UTC())
	if err != nil {
		s.logger.Warn("Failed to find similar memories", zap.Int64("memory_id", mem.ID), zap.Error(err))
		return nil
	}
	defer rows.Close()
	var out []MemorySimilar
	for rows.Next() {
		var m MemorySimilar
		if err := rows.Scan(&m.ID, &m.Title, &m.Similarity); err != nil {
			return out
		}
		if m.Similarity >= memorySimilarMin {
			out = append(out, m)
		}
	}
	return out
}

// MemoryFeedbackRequest is one signal about a memory.
type MemoryFeedbackRequest struct {
	Signal string `json:"signal"` // useful | confirmed | wrong
	// UpdatedAt, if set, is the updated_at of the memory the caller judged; a changed memory answers 409.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// MemoryFeedbackResponse says whether the signal counted.
type MemoryFeedbackResponse struct {
	Memory  Memory `json:"memory"`
	Outcome string `json:"outcome"` // recorded | duplicate (this credential already voted) | spaced (used within the hour)
}

// HandleMemoryFeedback records a signal on one of the caller's memories (POST /api/memories/{id}/feedback).
func (s *Server) HandleMemoryFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.memoryCollect() {
		respondError(w, http.StatusNotFound, "memory feedback is not enabled", "not_found")
		return
	}
	mem, userID, owner, ok := s.memoryFromPath(w, r)
	if !ok {
		return
	}
	var req MemoryFeedbackRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if req.Signal != "useful" && req.Signal != "confirmed" && req.Signal != "wrong" {
		respondError(w, http.StatusBadRequest, "signal must be useful, confirmed or wrong", "validation_error")
		return
	}
	if !owner { // sharees read; only the owner's credentials shape the owner's store
		respondError(w, http.StatusForbidden, "only the owner can give feedback on a memory", "forbidden")
		return
	}
	if mem.Status != "active" {
		respondError(w, http.StatusBadRequest, "only an active memory takes feedback", "validation_error")
		return
	}
	actor := "session"
	if key := GetApiKeyID(r); key > 0 {
		actor = fmt.Sprintf("key:%d", key)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	outcome, saved, err := s.recordMemoryFeedback(ctx, mem.ID, req.UpdatedAt, userID, req.Signal, actor, time.Now().UTC().Truncate(time.Second))
	switch {
	case err != nil:
		s.logger.Error("Failed to record memory feedback", zap.Int64("memory_id", mem.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to record feedback", "internal_error")
		return
	case outcome == "inactive":
		respondError(w, http.StatusBadRequest, "only an active memory takes feedback", "validation_error")
		return
	case outcome == "changed":
		respondError(w, http.StatusConflict, "the memory changed after the updated_at you sent; read it again", "conflict")
		return
	}
	mem = saved
	mem.Signals = mem.signals(time.Now().UTC())
	respondJSON(w, http.StatusOK, MemoryFeedbackResponse{Memory: mem, Outcome: outcome})
}

// recordMemoryFeedback stores the signal and updates the counters in one transaction, under the owner lock that
// writes, deletes and compaction also take. "confirmed" and "wrong" are votes: one per credential, so a retry or a
// loop cannot stack them. "useful" is spaced by memoryUseSpacing. The memory must still be active and unexpired
// ("inactive") and, if the caller sent the updated_at it judged, unchanged since ("changed"). It returns the memory
// as committed.
func (s *Server) recordMemoryFeedback(ctx context.Context, id int64, judged *time.Time, ownerID int64, signal, actor string, now time.Time) (string, Memory, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", Memory{}, err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := s.lockMemoryOwner(ctx, tx, ownerID); err != nil {
		return "", Memory{}, err
	}
	cur, err := s.getMemory(ctx, tx, id)
	switch {
	case errors.Is(err, errMemoryAccess) || err == nil && (cur.Status != "active" || cur.ExpiresAt != nil && !cur.ExpiresAt.After(now)):
		return "inactive", Memory{}, nil
	case err != nil:
		return "", Memory{}, err
	case judged != nil && !judged.Equal(cur.UpdatedAt):
		// shortcut: updated_at has 1 s resolution, so an edit in the same second as the read goes unseen; add a
		// revision column if clients need a strict precondition.
		return "changed", Memory{}, nil // an upsert replaced what the caller judged
	}
	outcome := "recorded"
	if signal == "useful" {
		res, err := tx.ExecContext(ctx, `UPDATE agent_memories SET use_count = use_count + 1, last_used_at = $2
			WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $3)`, id, now, now.Add(-memoryUseSpacing))
		if err != nil {
			return "", Memory{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return "", Memory{}, err
		} else if n == 0 {
			outcome = "spaced"
		} else if _, err := tx.ExecContext(ctx, `INSERT INTO memory_feedback (memory_id, owner_id, signal, actor, created_at)
			VALUES ($1, $2, $3, $4, $5)`, id, ownerID, signal, actor, now); err != nil {
			return "", Memory{}, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `INSERT INTO memory_feedback (memory_id, owner_id, signal, actor, created_at)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (memory_id, signal, actor) WHERE signal <> 'useful' DO NOTHING`,
			id, ownerID, signal, actor, now)
		if err != nil {
			return "", Memory{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return "", Memory{}, err
		} else if n == 0 {
			outcome = "duplicate"
		} else {
			set, args := `wrong_count = wrong_count + 1`, []any{id}
			if signal == "confirmed" {
				set, args = `use_count = use_count + 1, last_used_at = $2, confirmed_count = confirmed_count + 1`, append(args, now)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET `+set+` WHERE id = $1`, args...); err != nil {
				return "", Memory{}, err
			}
		}
	}
	mem, err := s.getMemory(ctx, tx, id)
	if err != nil {
		return "", Memory{}, err
	}
	return outcome, mem, tx.Commit()
}
