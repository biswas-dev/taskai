package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Compaction shrinks a user's memory storage without losing knowledge. It is deterministic (TaskAI has no LLM
// path) and idempotent: a second run finds nothing to do.
//  1. superseded memories go (their successor holds the lesson);
//  2. expired memories go;
//  3. a near-duplicate in the same scope (the hub's rule: 70% of words shared) gives way to the newest one;
//  4. old low-confidence memories of one kind in one scope are merged, bodies kept verbatim, into one memory.
// Rows with an origin become tombstones, so a hub re-sending them is ignored.

const (
	memoryNearDuplicate   = 0.7
	memorySummariseAfter  = 90 * 24 * time.Hour
	memoryMinCluster      = 3
	memoryNearDupWindow   = 200 // shortcut: compare with the newest 200 kept per scope, not all; widen if dupes slip by
	memoryEmbedBatch      = 32
	memoryEmbedInputChars = 500 // the embedding model's context, as wiki blocks
)

// MemoryCompactReport says what a compaction freed.
type MemoryCompactReport struct {
	SupersededRemoved       int   `json:"superseded_removed"`
	ExpiredRemoved          int   `json:"expired_removed"`
	DuplicatesMerged        int   `json:"duplicates_merged"`
	LowConfidenceSummarised int   `json:"low_confidence_summarised"`
	SummariesCreated        int   `json:"summaries_created"`
	BytesBefore             int64 `json:"bytes_before"`
	BytesAfter              int64 `json:"bytes_after"`
	FreedBytes              int64 `json:"freed_bytes"`
}

func memoryWordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range memoryWordRe.FindAllString(strings.ToLower(s), -1) {
		if len(w) > 1 {
			out[w] = true
		}
	}
	return out
}

func memoryJaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return float64(n) / float64(len(a)+len(b)-n)
}

// memoryScopeKey: the same repo (with issue and PR), or the same folder when there is no repo (the hub's sameScope).
func memoryScopeKey(sc MemoryScope) string {
	if sc.Repo != "" {
		return fmt.Sprintf("r\x00%s\x00%d\x00%d", strings.ToLower(sc.Repo), sc.Issue, sc.PR)
	}
	return "f\x00" + sc.Folder
}

func queryIDs(ctx context.Context, q sqlQuerier, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// compactMemories runs compaction for one owner in one transaction.
func (s *Server) compactMemories(ctx context.Context, ownerID int64) (MemoryCompactReport, error) {
	var rep MemoryCompactReport
	now := time.Now().UTC().Truncate(time.Second)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := s.lockMemoryOwner(ctx, tx, ownerID); err != nil {
		return rep, err
	}
	before, err := s.memoryUsage(ctx, tx, ownerID)
	if err != nil {
		return rep, err
	}

	superseded, err := queryIDs(ctx, tx, `SELECT id FROM agent_memories WHERE owner_id = $1 AND status = 'superseded'`, ownerID)
	if err != nil {
		return rep, err
	}
	expired, err := queryIDs(ctx, tx, `SELECT id FROM agent_memories WHERE owner_id = $1 AND status = 'active'
		AND expires_at IS NOT NULL AND expires_at <= $2`, ownerID, now)
	if err != nil {
		return rep, err
	}
	if err := s.removeMemories(ctx, tx, append(superseded, expired...), 0, now); err != nil {
		return rep, err
	}
	rep.SupersededRemoved, rep.ExpiredRemoved = len(superseded), len(expired)

	rows, err := tx.QueryContext(ctx, `SELECT `+memoryCols+` FROM agent_memories WHERE owner_id = $1 AND status = 'active'
		ORDER BY created_at DESC, id DESC`, ownerID)
	if err != nil {
		return rep, err
	}
	active, err := scanMemories(rows, false)
	if err != nil {
		return rep, err
	}

	// Near-duplicates: newest first, so the newest of a group is kept.
	type kept struct {
		id    int64
		words map[string]bool
	}
	byScope := map[string][]kept{}
	var survivors []Memory
	for _, m := range active {
		key := memoryScopeKey(m.Scope)
		words := memoryWordSet(m.Title + " " + m.Body)
		var dupOf int64
		group := byScope[key]
		for i := len(group) - 1; i >= 0 && i >= len(group)-memoryNearDupWindow; i-- {
			if memoryJaccard(words, group[i].words) >= memoryNearDuplicate {
				dupOf = group[i].id
				break
			}
		}
		if dupOf > 0 {
			if err := s.removeMemories(ctx, tx, []int64{m.ID}, dupOf, now); err != nil {
				return rep, err
			}
			rep.DuplicatesMerged++
			continue
		}
		byScope[key] = append(group, kept{m.ID, words})
		survivors = append(survivors, m)
	}

	// Old low-confidence clusters (one scope, one kind) merge into memories of at most memoryMaxBody.
	clusters := map[string][]Memory{}
	var order []string
	for _, m := range survivors {
		if m.Confidence != "low" || m.CreatedAt.After(now.Add(-memorySummariseAfter)) {
			continue
		}
		key := memoryScopeKey(m.Scope) + "\x00" + m.Kind
		if clusters[key] == nil {
			order = append(order, key)
		}
		clusters[key] = append(clusters[key], m)
	}
	for _, key := range order {
		group := clusters[key]
		if len(group) < memoryMinCluster {
			continue
		}
		for _, chunk := range chunkMemories(group) {
			if len(chunk) < 2 {
				continue
			}
			if err := s.summariseMemories(ctx, tx, ownerID, chunk, now); err != nil {
				return rep, err
			}
			rep.LowConfidenceSummarised += len(chunk)
			rep.SummariesCreated++
		}
	}

	after, err := s.memoryUsage(ctx, tx, ownerID)
	if err != nil {
		return rep, err
	}
	rep.BytesBefore, rep.BytesAfter, rep.FreedBytes = before.UsedBytes, after.UsedBytes, before.UsedBytes-after.UsedBytes
	return rep, tx.Commit()
}

const memorySummaryHeader = "Merged by compaction from %d low-confidence notes, kept verbatim.\n\n"

func memorySummaryEntry(m Memory) string {
	return fmt.Sprintf("### %s\n_%s_\n\n%s\n\n", m.Title, m.CreatedAt.Format("2006-01-02"), m.Body)
}

// chunkMemories packs memories, in order, into groups whose merged body fits memoryMaxBody.
func chunkMemories(ms []Memory) [][]Memory {
	budget := memoryMaxBody - len(memorySummaryHeader) - 8
	var out [][]Memory
	var cur []Memory
	size := 0
	for _, m := range ms {
		n := len(memorySummaryEntry(m))
		if len(cur) > 0 && size+n > budget {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, m)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// summariseMemories replaces a chunk with one memory holding every body verbatim.
func (s *Server) summariseMemories(ctx context.Context, q sqlQuerier, ownerID int64, chunk []Memory, now time.Time) error {
	var b strings.Builder
	fmt.Fprintf(&b, memorySummaryHeader, len(chunk))
	files := map[string]bool{}
	var fileList []string
	ids := make([]int64, len(chunk))
	for i, m := range chunk {
		b.WriteString(memorySummaryEntry(m))
		ids[i] = m.ID
		for _, f := range m.Scope.Files {
			if !files[f] && len(fileList) < memoryMaxFiles {
				files[f] = true
				fileList = append(fileList, f)
			}
		}
	}
	first := chunk[0]
	row, msg := normalizeMemory(MemoryWriteRequest{
		Kind:       first.Kind,
		Title:      fmt.Sprintf("%d low-confidence %s notes (compacted)", len(chunk), first.Kind),
		Body:       b.String(),
		Scope:      MemoryScope{Repo: first.Scope.Repo, Issue: first.Scope.Issue, PR: first.Scope.PR, Folder: first.Scope.Folder, Files: fileList},
		Author:     MemoryAuthor{Agent: "taskai-compaction"},
		Confidence: "low",
	}, now)
	if msg != "" {
		return fmt.Errorf("summary memory: %s", msg)
	}
	id, err := insertMemoryRow(ctx, q, ownerID, row, nil)
	if err != nil {
		return err
	}
	return s.removeMemories(ctx, q, ids, id, now)
}

// HandleCompactMemories compacts the caller's memories and reports what it freed (POST /api/memories/compact).
func (s *Server) HandleCompactMemories(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	rep, err := s.compactMemories(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to compact memories", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to compact memories", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, rep)
}

// maintainMemories runs with the indexing worker: it embeds new memories and compacts owners past the high-water mark.
func (s *Server) maintainMemories(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	s.embedPendingMemories(ctx)

	rows, err := s.db.QueryContext(ctx, `SELECT m.owner_id, SUM(m.size_bytes), COALESCE(MAX(q.quota_bytes), $1)
		FROM agent_memories m LEFT JOIN memory_quotas q ON q.user_id = m.owner_id GROUP BY m.owner_id`, memoryDefaultQuota)
	if err != nil {
		s.logger.Error("Failed to read memory usage for compaction", zap.Error(err))
		return
	}
	var over []int64
	for rows.Next() {
		var owner, used, quota int64
		if err := rows.Scan(&owner, &used, &quota); err != nil {
			s.logger.Error("Failed to scan memory usage", zap.Error(err))
			break
		}
		if used*100 >= quota*memoryHighWater {
			over = append(over, owner)
		}
	}
	rows.Close()
	for _, owner := range over {
		rep, err := s.compactMemories(ctx, owner)
		if err != nil {
			s.logger.Error("Automatic memory compaction failed", zap.Int64("user_id", owner), zap.Error(err))
			continue
		}
		s.logger.Info("Compacted memories past the high-water mark", zap.Int64("user_id", owner), zap.Int64("freed_bytes", rep.FreedBytes))
	}
}

// embedPendingMemories stores vectors for memories that have none (Postgres with an embedding model only).
func (s *Server) embedPendingMemories(ctx context.Context) {
	if s.db.Driver != "postgres" || s.embeddingClient == nil {
		return
	}
	for ctx.Err() == nil {
		rows, err := s.db.QueryContext(ctx, `SELECT id, title, body FROM agent_memories
			WHERE embedding IS NULL AND status <> 'compacted' ORDER BY id LIMIT `+strconv.Itoa(memoryEmbedBatch))
		if err != nil {
			s.logger.Error("Failed to list memories to embed", zap.Error(err))
			return
		}
		var ids []int64
		var texts []string
		for rows.Next() {
			var id int64
			var title, body string
			if err := rows.Scan(&id, &title, &body); err != nil {
				rows.Close()
				s.logger.Error("Failed to scan memory to embed", zap.Error(err))
				return
			}
			text := title + "\n" + body
			if len(text) > memoryEmbedInputChars {
				text = text[:memoryEmbedInputChars]
			}
			ids, texts = append(ids, id), append(texts, text)
		}
		rows.Close()
		if len(ids) == 0 {
			return
		}
		vectors, err := s.embeddingClient.EmbedBatch(ctx, texts)
		if err != nil {
			s.logger.Warn("Failed to embed memories (will retry)", zap.Error(err))
			return
		}
		stored := 0
		for i, id := range ids {
			if i >= len(vectors) || vectors[i] == nil {
				continue
			}
			stored++
			if _, err := s.db.ExecContext(ctx, `UPDATE agent_memories SET embedding = $1::vector, embedded_at = $2 WHERE id = $3`,
				float32SliceToVectorString(vectors[i]), time.Now().UTC(), id); err != nil {
				s.logger.Warn("Failed to store memory embedding", zap.Int64("memory_id", id), zap.Error(err))
			}
		}
		if len(ids) < memoryEmbedBatch || stored == 0 {
			return
		}
	}
}
