package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/pageversion"
	"taskai/ent/wikiblock"
	"taskai/ent/wikipage"
	"taskai/ent/wikipageversion"
	"taskai/internal/yjs"
)

// StartIndexingWorker starts a background worker that periodically indexes wiki content
func (s *Server) StartIndexingWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	s.logger.Info("Starting wiki indexing worker",
		zap.Duration("interval", 2*time.Minute),
	)

	// Run immediately on startup
	s.indexPages(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Indexing worker shutting down")
			return
		case <-ticker.C:
			s.indexPages(ctx)
		}
	}
}

// indexBatch is how many pages one pass of the worker indexes at most.
const indexBatch = 50

// indexMu keeps the periodic worker and a requested reindex from indexing
// the same page at once, which could interleave two delete-and-insert
// transactions and duplicate its blocks.
var indexMu sync.Mutex

// indexPages indexes every page whose content changed since it was last
// indexed (or that has never been indexed), batch by batch until none are
// left or the pass runs out of time.
func (s *Server) indexPages(parentCtx context.Context) {
	indexMu.Lock()
	defer indexMu.Unlock()
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
	defer cancel()

	successCount, failCount := 0, 0
	failed := map[int64]bool{}
	for ctx.Err() == nil {
		pages, err := s.pagesNeedingIndex(ctx, indexBatch+len(failed))
		if err != nil {
			s.logger.Error("Failed to fetch pages for indexing", zap.Error(err))
			return
		}
		progressed := false
		for _, page := range pages {
			if failed[page.ID] {
				continue
			}
			progressed = true
			if err := s.indexPage(ctx, page); err != nil {
				s.logger.Error("Failed to index page",
					zap.Int64("page_id", page.ID),
					zap.String("page_title", page.Title),
					zap.Error(err),
				)
				failed[page.ID] = true
				failCount++
			} else {
				successCount++
			}
		}
		if !progressed {
			break
		}
	}

	if successCount+failCount > 0 {
		s.logger.Info("Indexing completed",
			zap.Int("success", successCount),
			zap.Int("failed", failCount),
		)
	}
}

// pagesNeedingIndex lists pages edited since they were last indexed,
// most recently edited first.
func (s *Server) pagesNeedingIndex(ctx context.Context, limit int) ([]*ent.WikiPage, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT id FROM wiki_pages
		 WHERE search_indexed_at IS NULL OR updated_at > search_indexed_at
		 ORDER BY updated_at DESC
		 LIMIT ?`), limit)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return s.db.Client.WikiPage.Query().Where(wikipage.IDIn(ids...)).Order(ent.Desc(wikipage.FieldUpdatedAt)).All(ctx)
}

// indexPage replaces a page's search blocks with ones built from its current
// content. The page row is read and locked, its blocks swapped, and the
// edit that was indexed recorded in search_indexed_at, all in one
// transaction: an edit that commits meanwhile waits for it and then shows
// as newer, so the next pass picks it up.
func (s *Server) indexPage(ctx context.Context, page *ent.WikiPage) error {
	// A page with no content falls back to its Yjs snapshot or saved
	// versions, which can be slow; work that out before taking the lock.
	var fallback []yjs.Block
	if strings.TrimSpace(page.Content) == "" {
		var err error
		if fallback, err = s.extractPageBlocks(ctx, page); err != nil {
			// Keep the blocks already there: stale results beat none.
			return fmt.Errorf("extract blocks: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	lock := ""
	if s.db.Driver == "postgres" {
		lock = " FOR UPDATE"
	}
	var content sql.NullString
	if err := tx.QueryRowContext(ctx, s.db.Rebind(`SELECT content FROM wiki_pages WHERE id = ?`+lock), page.ID).Scan(&content); err != nil {
		return err
	}
	blocks := fallback
	if strings.TrimSpace(content.String) != "" {
		// Parse what is in the row now, which may be newer than page.
		blocks = markdownBlocks(content.String)
	}

	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM wiki_blocks WHERE page_id = ?`), page.ID); err != nil {
		return err
	}
	insert := s.db.Rebind(`INSERT INTO wiki_blocks (page_id, block_type, level, headings_path, canonical_json, plain_text, position) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	for _, b := range blocks {
		var level, canonical any
		if b.Level != nil {
			level = *b.Level
		}
		if b.CanonicalJSON != "" {
			canonical = b.CanonicalJSON
		}
		if _, err := tx.ExecContext(ctx, insert, page.ID, b.Type, level, b.HeadingsPath, canonical, b.PlainText, b.Position); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE wiki_pages SET search_indexed_at = updated_at WHERE id = ?`), page.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.logger.Info("Indexed page",
		zap.Int64("page_id", page.ID),
		zap.String("page_title", page.Title),
		zap.Int("block_count", len(blocks)),
	)

	// Select explicitly: the ent schema also knows search_text, a column
	// only the SQLite schema has.
	savedBlocks, err := s.db.Client.WikiBlock.Query().
		Where(wikiblock.PageID(page.ID)).
		Select(wikiblock.FieldID, wikiblock.FieldHeadingsPath, wikiblock.FieldPlainText).
		All(ctx)
	if err != nil {
		s.logger.Warn("Failed to load blocks for embedding (non-fatal)", zap.Int64("page_id", page.ID), zap.Error(err))
		return nil
	}
	// Generate and store embeddings (skip if embedding client is nil)
	if s.embeddingClient != nil && len(savedBlocks) > 0 {
		if err := s.embedBlocks(ctx, savedBlocks); err != nil {
			s.logger.Warn("Failed to embed blocks (non-fatal)",
				zap.Int64("page_id", page.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// extractPageBlocks splits a page's content into search blocks. The page's
// own content column is what readers are served, so it comes first; a Yjs
// snapshot or the latest saved version are used only when it is empty.
func (s *Server) extractPageBlocks(ctx context.Context, page *ent.WikiPage) ([]yjs.Block, error) {
	if strings.TrimSpace(page.Content) != "" {
		return markdownBlocks(page.Content), nil
	}
	if s.yjsClient != nil {
		if blocks, err := s.extractBlocksFromYjs(ctx, page); err == nil && len(blocks) > 0 {
			return blocks, nil
		} else if err != nil && !ent.IsNotFound(err) {
			s.logger.Debug("Yjs extraction failed, trying saved versions", zap.Int64("page_id", page.ID), zap.Error(err))
		}
	}
	version, err := s.db.Client.WikiPageVersion.Query().
		Where(wikipageversion.WikiPageID(page.ID)).
		Order(ent.Desc(wikipageversion.FieldVersionNumber)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil // an empty page has no blocks
	}
	if err != nil {
		return nil, err
	}
	// Versions are stored compressed; read them through the decoder.
	content, err := s.getWikiPageVersionContent(ctx, page.ID, version.VersionNumber)
	if err != nil {
		return nil, err
	}
	return markdownBlocks(content), nil
}

// extractBlocksFromYjs extracts blocks via the Yjs processor (binary state).
func (s *Server) extractBlocksFromYjs(ctx context.Context, page *ent.WikiPage) ([]yjs.Block, error) {
	snapshot, err := s.db.Client.PageVersion.Query().
		Where(pageversion.PageID(page.ID)).
		Order(ent.Desc(pageversion.FieldVersionNumber)).
		First(ctx)
	if err != nil {
		return nil, err
	}
	return s.yjsClient.ExtractBlocks(ctx, base64.StdEncoding.EncodeToString(snapshot.YjsState))
}

// markdownBlocks splits markdown into one block per section. headings_path
// is the chain of enclosing headings ("Deploy > Rollback"), and lines inside
// fenced code are text, never headings.
func markdownBlocks(content string) []yjs.Block {
	var (
		blocks  []yjs.Block
		path    []string // heading text by level, index 0 = level 1
		text    strings.Builder
		inFence bool
		fence   string
	)
	headings := func() string {
		var parts []string
		for _, h := range path {
			if h != "" {
				parts = append(parts, h)
			}
		}
		return strings.Join(parts, " > ")
	}
	flush := func() {
		body := strings.TrimSpace(text.String())
		text.Reset()
		if body == "" {
			return
		}
		blockType := "paragraph"
		if len(path) > 0 {
			blockType = "section"
		}
		blocks = append(blocks, yjs.Block{Type: blockType, HeadingsPath: headings(), PlainText: body, Position: len(blocks)})
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence = false
			}
		}
		if !inFence && strings.HasPrefix(trimmed, "#") {
			level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			rest := strings.TrimSpace(trimmed[level:])
			if level <= 6 && (rest == "" || trimmed[level] == ' ') {
				flush()
				for len(path) < level {
					path = append(path, "")
				}
				path = append(path[:level-1], rest)
				continue
			}
		}
		if trimmed != "" {
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(trimmed)
		}
	}
	flush()
	return blocks
}

// embedBlocks generates and stores vector embeddings for wiki blocks.
func (s *Server) embedBlocks(ctx context.Context, blocks []*ent.WikiBlock) error {
	// Build embedding inputs: headings_path + plain_text for context-enriched vectors
	// Truncate to ~500 chars to stay within model context length (all-minilm max ~256 tokens)
	const maxChars = 500
	texts := make([]string, len(blocks))
	for i, block := range blocks {
		var parts []string
		if block.HeadingsPath != nil && *block.HeadingsPath != "" {
			parts = append(parts, *block.HeadingsPath)
		}
		if block.PlainText != nil && *block.PlainText != "" {
			parts = append(parts, *block.PlainText)
		}
		text := strings.Join(parts, "\n")
		if len(text) > maxChars {
			text = text[:maxChars]
		}
		texts[i] = text
	}

	start := time.Now()
	vectors, err := s.embeddingClient.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed batch: %w", err)
	}

	s.logger.Debug("Generated embeddings",
		zap.Int("count", len(vectors)),
		zap.Duration("latency", time.Since(start)),
	)

	// Store embeddings via raw SQL (Ent doesn't support pgvector natively)
	model := s.embeddingClient.Model()
	now := time.Now()
	for i, block := range blocks {
		if vectors[i] == nil {
			continue
		}
		vectorStr := float32SliceToVectorString(vectors[i])
		_, err := s.db.ExecContext(ctx,
			`UPDATE wiki_blocks SET embedding = $1::vector, embedding_model = $2, embedded_at = $3 WHERE id = $4`,
			vectorStr, model, now, block.ID,
		)
		if err != nil {
			s.logger.Warn("Failed to store embedding for block",
				zap.Int64("block_id", block.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// HandleReindexWiki rebuilds the search index for every wiki page: full-text
// blocks always, and vector embeddings when an embedding model is set up.
// Pages are marked for indexing and the work runs in the background.
func (s *Server) HandleReindexWiki(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	res, err := s.db.ExecContext(ctx, `UPDATE wiki_pages SET search_indexed_at = NULL`)
	if err != nil {
		s.logger.Error("Reindex: failed to mark pages", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to start re-indexing", "internal_error")
		return
	}
	pages, _ := res.RowsAffected()
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		// Each pass is bounded; keep going until every page is indexed.
		for bg.Err() == nil {
			remaining, err := s.pagesNeedingIndex(bg, 1)
			if err != nil || len(remaining) == 0 {
				return
			}
			s.indexPages(bg)
		}
	}()
	msg := "re-indexing started in background"
	if s.embeddingClient == nil {
		msg += " (full-text only: no embedding model is configured)"
	}
	respondJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "message": msg, "pages": pages})
}

// float32SliceToVectorString converts a float32 slice to pgvector string format: "[0.1,0.2,0.3]"
func float32SliceToVectorString(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%g", f))
	}
	b.WriteByte(']')
	return b.String()
}
