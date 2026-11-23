package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MemorySearchRequest searches the memories the caller can read (their own and those shared with them).
type MemorySearchRequest struct {
	Query           string   `json:"query"`
	OwnerID         int64    `json:"owner_id,omitempty"`
	Repo            string   `json:"repo,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Issue           int      `json:"issue,omitempty"`
	PR              int      `json:"pr,omitempty"`
	Files           []string `json:"files,omitempty"` // matches a memory naming any of them
	IncludeInactive bool     `json:"include_inactive,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Mode            string   `json:"mode,omitempty"` // hybrid (default with embeddings) | fts | keyword
}

// MemorySearchResponse is the ranked result.
type MemorySearchResponse struct {
	Results []Memory `json:"results"`
	Total   int      `json:"total"`
	Mode    string   `json:"mode"`
}

// memoryQueryEmbedTimeout bounds the query embedding so a slow model cannot eat an agent's context budget;
// past it, search falls back to full text.
const (
	memoryQueryEmbedTimeout = 1500 * time.Millisecond
	memoryMaxQuery          = memoryEmbedInputChars // longer queries are cut: the model reads no more
)

var memoryWordRe = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// memoryQueryWords are the distinct words of a query (at most 12), lower case.
func memoryQueryWords(q string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range memoryWordRe.FindAllString(strings.ToLower(q), -1) {
		if !seen[w] && len(out) < 12 {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// HandleSearchMemories ranks memories by full text and vector similarity merged with RRF (POST /api/memories/search).
func (s *Server) HandleSearchMemories(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req MemorySearchRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if len(req.Files) > memoryMaxFiles {
		respondError(w, http.StatusBadRequest, "at most 50 files", "validation_error")
		return
	}
	if len(req.Query) > memoryMaxQuery {
		req.Query = strings.ToValidUTF8(req.Query[:memoryMaxQuery], "")
	}
	if strings.TrimSpace(req.Query) == "" {
		respondError(w, http.StatusBadRequest, "query is required", "invalid_request")
		return
	}
	if req.Issue < 0 || req.PR < 0 || req.OwnerID < 0 {
		respondError(w, http.StatusBadRequest, "issue, pr and owner_id are positive numbers", "validation_error")
		return
	}
	words := memoryQueryWords(req.Query)
	if len(words) == 0 { // only punctuation: nothing can match
		respondJSON(w, http.StatusOK, MemorySearchResponse{Results: []Memory{}, Mode: "keyword"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	req.Limit = min(req.Limit, 50)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	owners, err := s.resolveMemoryOwners(ctx, userID, req.OwnerID)
	if errors.Is(err, errMemoryAccess) {
		respondError(w, http.StatusForbidden, "those memories are not shared with you", "forbidden")
		return
	}
	if err != nil {
		s.logger.Error("Failed to resolve memory owners", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to search memories", "internal_error")
		return
	}
	f := memoryFilter{Owners: owners, Repo: req.Repo, Kind: req.Kind, Issue: req.Issue, PR: req.PR, Files: req.Files,
		IncludeInactive: req.IncludeInactive}

	mode := req.Mode
	if mode != "" && mode != "hybrid" && mode != "fts" && mode != "keyword" {
		respondError(w, http.StatusBadRequest, "mode must be hybrid, fts or keyword", "invalid_request")
		return
	}
	if s.db.Driver != "postgres" {
		mode = "keyword"
	} else if mode == "" {
		mode = "hybrid"
	}
	var results []Memory
	if mode == "hybrid" {
		results, err = s.searchMemoriesHybrid(ctx, f, req.Query, words, req.Limit)
		if errors.Is(err, errNoQueryVector) {
			mode = "fts"
		}
	}
	if mode == "fts" {
		results, err = s.searchMemoriesFTS(ctx, f, words, req.Limit)
		if err == nil && len(results) == 0 {
			mode = "keyword" // partial words
		}
	}
	if mode == "keyword" {
		results, err = s.searchMemoriesKeyword(ctx, f, words, req.Limit)
	}
	if err != nil {
		s.logger.Error("Failed to search memories", zap.Int64("user_id", userID), zap.String("mode", mode), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to search memories", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, MemorySearchResponse{Results: results, Total: len(results), Mode: mode})
}

var errNoQueryVector = errors.New("no query vector")

// memoryTSQuery matches any of the words (ranked by how many match), via websearch syntax, which is safe for any input.
func memoryTSQuery(words []string) string { return strings.Join(words, " or ") }

func (s *Server) searchMemoriesFTS(ctx context.Context, f memoryFilter, words []string, limit int) ([]Memory, error) {
	where, args := f.sql("m", time.Now().UTC(), nil)
	args = append(args, memoryTSQuery(words))
	q := "$" + strconv.Itoa(len(args))
	rows, err := s.db.QueryContext(ctx, `SELECT `+prefixCols("m")+`, ts_rank(m.search_vector, websearch_to_tsquery('english', `+q+`)) AS rank
		FROM agent_memories m WHERE m.search_vector @@ websearch_to_tsquery('english', `+q+`) AND `+where+`
		ORDER BY rank DESC, m.id DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows, true)
}

func (s *Server) searchMemoriesHybrid(ctx context.Context, f memoryFilter, query string, words []string, limit int) ([]Memory, error) {
	if s.embeddingClient == nil {
		return nil, errNoQueryVector
	}
	ectx, cancel := context.WithTimeout(ctx, memoryQueryEmbedTimeout)
	vec, err := s.embeddingClient.Embed(ectx, query)
	cancel()
	if err != nil {
		s.logger.Warn("Memory query embedding failed, using full text", zap.Error(err))
		return nil, errNoQueryVector
	}
	where, args := f.sql("m", time.Now().UTC(), nil)
	match, score, args := memoryKeywordMatch("m", words, args)
	args = append(args, memoryTSQuery(words), float32SliceToVectorString(vec))
	tsq, vq := "$"+strconv.Itoa(len(args)-1), "$"+strconv.Itoa(len(args))
	pool := strconv.Itoa(limit * 3)
	// Reciprocal Rank Fusion of full text, vectors and keywords: the keyword leg finds partial words,
	// including in fresh memories the worker has not embedded yet.
	rows, err := s.db.QueryContext(ctx, `
		WITH fts AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY r DESC, id DESC) AS rn FROM (
				SELECT m.id, ts_rank(m.search_vector, websearch_to_tsquery('english', `+tsq+`)) AS r
				FROM agent_memories m
				WHERE m.search_vector @@ websearch_to_tsquery('english', `+tsq+`) AND `+where+`
				ORDER BY r DESC, m.id DESC LIMIT `+pool+`) x
		),
		vec AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY d, id DESC) AS rn FROM (
				SELECT m.id, m.embedding <=> `+vq+`::vector AS d
				FROM agent_memories m
				WHERE m.embedding IS NOT NULL AND `+where+`
				ORDER BY d LIMIT `+pool+`) y
		),
		kw AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY k DESC, u DESC, id DESC) AS rn FROM (
				SELECT m.id, `+score+` AS k, m.updated_at AS u
				FROM agent_memories m
				WHERE `+match+` AND `+where+`
				ORDER BY k DESC, m.updated_at DESC, m.id DESC LIMIT `+pool+`) z
		),
		combined AS (
			SELECT id, SUM(1.0/(60+rn)) AS score FROM (
				SELECT id, rn FROM fts UNION ALL SELECT id, rn FROM vec UNION ALL SELECT id, rn FROM kw) legs
			GROUP BY id
		)
		SELECT `+prefixCols("m")+`, c.score FROM combined c JOIN agent_memories m ON m.id = c.id
		ORDER BY c.score DESC, m.id DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows, true)
}

// memoryKeywordMatch matches memories whose title or body contains any query word (served by the trigram index on
// Postgres) and scores them by how many of the words they contain.
func memoryKeywordMatch(alias string, words []string, args []any) (match, score string, out []any) {
	text := "LOWER(" + alias + ".title || ' ' || " + alias + ".body)"
	ors := make([]string, len(words))
	hits := make([]string, len(words))
	for i, w := range words {
		args = append(args, "%"+likeEscaper.Replace(w)+"%")
		ors[i] = text + " LIKE $" + strconv.Itoa(len(args)) + ` ESCAPE '\'`
		hits[i] = "(CASE WHEN " + ors[i] + " THEN 1 ELSE 0 END)"
	}
	return "(" + strings.Join(ors, " OR ") + ")", strings.Join(hits, " + "), args
}

// searchMemoriesKeyword ranks by how many query words appear in the title or body (SQLite, and partial words).
func (s *Server) searchMemoriesKeyword(ctx context.Context, f memoryFilter, words []string, limit int) ([]Memory, error) {
	where, args := f.sql("m", time.Now().UTC(), nil)
	match, score, args := memoryKeywordMatch("m", words, args)
	rows, err := s.db.QueryContext(ctx, `SELECT `+prefixCols("m")+`, CAST(`+score+` AS REAL) AS rank
		FROM agent_memories m WHERE `+where+` AND `+match+`
		ORDER BY rank DESC, m.updated_at DESC, m.id DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows, true)
}

// prefixCols qualifies memoryCols with a table alias.
func prefixCols(alias string) string {
	cols := strings.Split(memoryCols, ",")
	for i, c := range cols {
		cols[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(cols, ", ")
}
