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
//  3. copies (same kind, title, body and scope, ignoring case and spacing) fold into the one that dominates them;
//  4. old low-confidence memories of one kind in one scope, without an expiry, are merged into one memory with
//     their bodies kept verbatim.
// Rows with an origin become tombstones, so a hub re-sending them is ignored.

const (
	memorySummariseAfter  = 90 * 24 * time.Hour
	memoryMinCluster      = 3
	memoryEmbedBatch      = 32
	memoryCompactRecent   = 5 * time.Minute // automatic compaction looks at owners written to this recently
	memoryEmbedInputChars = 500             // the embedding model's context, as wiki blocks
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

// memoryScopeKey: the same repo, issue, PR and folder, so a merge never moves a note to another scope.
func memoryScopeKey(sc MemoryScope) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", strings.ToLower(sc.Repo), sc.Issue, sc.PR, sc.Folder)
}

var memoryConfidenceRank = map[string]int{"low": 0, "medium": 1, "high": 2}

// dominatesAll: c lasts at least as long and is at least as confident as every memory in group.
func dominatesAll(c Memory, group []Memory) bool {
	for _, o := range group {
		if !outlives(c.ExpiresAt, o.ExpiresAt) || memoryConfidenceRank[c.Confidence] < memoryConfidenceRank[o.Confidence] {
			return false
		}
	}
	return true
}

// outlives: a memory expiring at a lasts at least as long as one expiring at b (nil = never expires).
func outlives(a, b *time.Time) bool {
	return a == nil || b != nil && !a.Before(*b)
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

	// Copies: the same content hash (kind, title, body and scope, ignoring case and spacing). The kept copy must
	// dominate the rest (lasts at least as long, confidence at least as high; the newest wins ties), so nothing it
	// replaces is worth more and a replay from its own hub cannot undo the fold. Without such a copy the group stays.
	// Anything less than a copy is left alone: overlapping text can say the opposite ("Use X" / "Do not use X").
	groups := map[string][]Memory{}
	var hashes []string
	for _, m := range active {
		h := memoryHash(m)
		if groups[h] == nil {
			hashes = append(hashes, h)
		}
		groups[h] = append(groups[h], m)
	}
	var survivors []Memory
	for _, h := range hashes {
		group := groups[h]
		keep := -1
		for i, c := range group { // newest first, so the first dominating copy is the newest one
			if dominatesAll(c, group) {
				keep = i
				break
			}
		}
		if keep < 0 || len(group) == 1 {
			survivors = append(survivors, group...)
			continue
		}
		for i, m := range group {
			if i == keep {
				continue
			}
			if err := foldMemoryUse(ctx, tx, group[keep].ID, m); err != nil {
				return rep, err
			}
			group[keep].use = group[keep].use.fold(m.use) // the summary step below must see the folded evidence
			if err := s.removeMemories(ctx, tx, []int64{m.ID}, group[keep].ID, now); err != nil {
				return rep, err
			}
			rep.DuplicatesMerged++
		}
		survivors = append(survivors, group[keep])
	}

	// Old low-confidence clusters (one scope, one kind) merge into memories of at most memoryMaxBody.
	clusters := map[string][]Memory{}
	var order []string
	for _, m := range survivors {
		if m.Confidence != "low" || m.ExpiresAt != nil || m.CreatedAt.After(now.Add(-memorySummariseAfter)) || m.inUse(now) {
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

// inUse: a memory someone confirmed, or found useful within memorySummariseAfter, is not "old and low": keep it whole.
func (m Memory) inUse(now time.Time) bool {
	return m.use.confirmed > 0 || m.use.lastUsed != nil && m.use.lastUsed.After(now.Add(-memorySummariseAfter))
}

// fold is the in-memory twin of foldMemoryUse.
func (u memoryUse) fold(g memoryUse) memoryUse {
	u.count += g.count - 1
	u.confirmed += g.confirmed
	u.wrong += g.wrong
	if g.lastUsed != nil && (u.lastUsed == nil || g.lastUsed.After(*u.lastUsed)) {
		u.lastUsed = g.lastUsed
	}
	return u
}

// foldMemoryUse moves a removed copy's feedback to the copy that is kept, so folding loses no evidence. Votes stay
// one per credential: a vote the kept copy already has from the same credential is dropped, and the vote
// counters are recounted from the rows.
func foldMemoryUse(ctx context.Context, q sqlQuerier, keepID int64, gone Memory) error {
	if gone.use.count <= 1 && gone.use.confirmed == 0 && gone.use.wrong == 0 {
		return nil
	}
	if _, err := q.ExecContext(ctx, `UPDATE memory_feedback SET memory_id = $1 WHERE memory_id = $2 AND (signal = 'useful'
		OR NOT EXISTS (SELECT 1 FROM memory_feedback k WHERE k.memory_id = $1 AND k.signal = memory_feedback.signal AND k.actor = memory_feedback.actor))`,
		keepID, gone.ID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE agent_memories SET use_count = use_count + $1,
		confirmed_count = (SELECT COUNT(*) FROM memory_feedback WHERE memory_id = $3 AND signal = 'confirmed'),
		wrong_count = (SELECT COUNT(*) FROM memory_feedback WHERE memory_id = $3 AND signal = 'wrong'),
		last_used_at = CASE WHEN last_used_at IS NULL OR last_used_at < $2 THEN $2 ELSE last_used_at END
		WHERE id = $3`, gone.use.count-1, gone.use.lastUsed, keepID)
	return err
}

const memorySummaryHeader = "Merged by compaction from %d low-confidence notes, kept verbatim.\n\n"

func memorySummaryEntry(m Memory) string {
	return fmt.Sprintf("### %s\n_%s_\n\n%s\n\n", m.Title, m.CreatedAt.Format("2006-01-02"), m.Body)
}

// chunkMemories packs memories, in order, into groups whose merged body fits memoryMaxBody and whose files fit
// memoryMaxFiles, so a merge never drops a file a search could filter by.
func chunkMemories(ms []Memory) [][]Memory {
	budget := memoryMaxBody - len(memorySummaryHeader) - 8
	var out [][]Memory
	var cur []Memory
	size := 0
	files := map[string]bool{}
	for _, m := range ms {
		n := len(memorySummaryEntry(m))
		added := 0
		for _, f := range m.Scope.Files {
			if !files[f] {
				added++
			}
		}
		if len(cur) > 0 && (size+n > budget || len(files)+added > memoryMaxFiles) {
			out = append(out, cur)
			cur, size, files = nil, 0, map[string]bool{}
		}
		cur = append(cur, m)
		size += n
		for _, f := range m.Scope.Files {
			files[f] = true
		}
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
	embedCtx, cancelEmbed := context.WithTimeout(parent, time.Minute) // a backlog must not starve compaction
	s.embedPendingMemories(embedCtx)
	cancelEmbed()
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()

	// Only owners past the high-water mark with something compaction can act on: a recent write, or rows that are
	// superseded or expired. Otherwise a full owner who cannot shrink would be rescanned on every tick.
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT m.owner_id, SUM(m.size_bytes), COALESCE(MAX(q.quota_bytes), $1)
		FROM agent_memories m LEFT JOIN memory_quotas q ON q.user_id = m.owner_id
		GROUP BY m.owner_id
		HAVING MAX(m.updated_at) >= $2
		    OR SUM(CASE WHEN m.status = 'superseded' OR (m.status = 'active' AND m.expires_at <= $3) THEN 1 ELSE 0 END) > 0`,
		memoryDefaultQuota, now.Add(-memoryCompactRecent), now)
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
		if float64(used)*100 >= float64(quota)*memoryHighWater {
			over = append(over, owner)
		}
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("Failed to read memory usage for compaction", zap.Error(err))
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
	var cursor int64 // a row that fails is retried on the next pass, not again in this one
	for ctx.Err() == nil {
		rows, err := s.db.QueryContext(ctx, `SELECT id, title, body, content_hash FROM agent_memories
			WHERE embedding IS NULL AND status <> 'compacted' AND id > $1 ORDER BY id LIMIT `+strconv.Itoa(memoryEmbedBatch), cursor)
		if err != nil {
			s.logger.Error("Failed to list memories to embed", zap.Error(err))
			return
		}
		var ids []int64
		var texts, hashes []string
		for rows.Next() {
			var id int64
			var title, body, hash string
			if err := rows.Scan(&id, &title, &body, &hash); err != nil {
				rows.Close()
				s.logger.Error("Failed to scan memory to embed", zap.Error(err))
				return
			}
			text := title + "\n" + body
			if len(text) > memoryEmbedInputChars {
				text = strings.ToValidUTF8(text[:memoryEmbedInputChars], "")
			}
			ids, texts, hashes = append(ids, id), append(texts, text), append(hashes, hash)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			s.logger.Error("Failed to list memories to embed", zap.Error(err))
			return
		}
		if len(ids) == 0 {
			return
		}
		cursor = ids[len(ids)-1]
		vectors, err := s.embeddingClient.EmbedBatch(ctx, texts)
		if err != nil {
			s.logger.Warn("Failed to embed memories (will retry)", zap.Error(err))
			return
		}
		for i, id := range ids {
			if i >= len(vectors) || vectors[i] == nil {
				continue
			}
			// Only if the text is still what was embedded: an edit meanwhile clears the vector for the next pass.
			if _, err := s.db.ExecContext(ctx, `UPDATE agent_memories SET embedding = $1::vector, embedded_at = $2
				WHERE id = $3 AND content_hash = $4 AND status <> 'compacted'`,
				float32SliceToVectorString(vectors[i]), time.Now().UTC(), id, hashes[i]); err != nil {
				s.logger.Warn("Failed to store memory embedding", zap.Int64("memory_id", id), zap.Error(err))
			}
		}
		if len(ids) < memoryEmbedBatch {
			return
		}
	}
}
