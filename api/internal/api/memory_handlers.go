package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// HandleWriteMemory creates or upserts a memory for the caller (POST /api/memories).
func (s *Server) HandleWriteMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req MemoryWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	m, msg := normalizeMemory(req, time.Now().UTC().Truncate(time.Second))
	if msg != "" {
		respondError(w, http.StatusBadRequest, msg, "validation_error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	mem, outcome, err := s.upsertMemory(ctx, userID, m)
	if errors.Is(err, errMemoryQuota) {
		// Compaction may free enough; try once before refusing.
		if _, cerr := s.compactMemories(ctx, userID); cerr != nil {
			s.logger.Warn("Memory compaction before refusing a write failed", zap.Int64("user_id", userID), zap.Error(cerr))
		}
		mem, outcome, err = s.upsertMemory(ctx, userID, m)
	}
	var inputErr memoryInputError
	switch {
	case errors.Is(err, errMemoryQuota):
		usage, _ := s.memoryUsage(ctx, s.db, userID)
		respondError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"memory quota exceeded: %d of %d bytes used and this memory needs %d; compact or delete memories, or ask an admin to raise the quota",
			usage.UsedBytes, usage.QuotaBytes, m.SizeBytes), "memory_quota_exceeded")
		return
	case errors.As(err, &inputErr):
		respondError(w, http.StatusBadRequest, inputErr.Error(), "validation_error")
		return
	case err != nil:
		s.logger.Error("Failed to write memory", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to write memory", "internal_error")
		return
	}
	usage, err := s.memoryUsage(ctx, s.db, userID)
	if err != nil {
		s.logger.Warn("Failed to read memory usage", zap.Int64("user_id", userID), zap.Error(err))
	}
	status := http.StatusOK
	if outcome == "created" {
		status = http.StatusCreated
	}
	respondJSON(w, status, MemoryWriteResponse{Memory: mem, Outcome: outcome, Usage: usage})
}

// memoryFilter builds the WHERE clause shared by list and search. The caller's readable owners are always applied.
type memoryFilter struct {
	Owners          []int64
	Repo            string
	Kind            string
	Issue, PR       int
	Files           []string
	IncludeInactive bool
}

func (f memoryFilter) sql(alias string, now time.Time, args []any) (string, []any) {
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	ph := make([]string, len(f.Owners))
	for i, o := range f.Owners {
		ph[i] = arg(o)
	}
	conds := []string{alias + ".owner_id IN (" + strings.Join(ph, ",") + ")"}
	if f.IncludeInactive {
		conds = append(conds, alias+".status <> 'compacted'")
	} else {
		conds = append(conds, alias+".status = 'active'", "("+alias+".expires_at IS NULL OR "+alias+".expires_at > "+arg(now)+")")
	}
	if f.Repo != "" {
		conds = append(conds, "LOWER("+alias+".repo) = "+arg(strings.ToLower(f.Repo)))
	}
	if f.Kind != "" {
		conds = append(conds, alias+".kind = "+arg(f.Kind))
	}
	if f.Issue > 0 {
		conds = append(conds, alias+".issue = "+arg(f.Issue))
	}
	if f.PR > 0 {
		conds = append(conds, alias+".pr = "+arg(f.PR))
	}
	if len(f.Files) > 0 {
		var ors []string
		for _, file := range f.Files {
			esc := likeEscaper.Replace(strings.TrimPrefix(file, "./"))
			ors = append(ors, alias+".files LIKE "+arg("%\n"+esc+"\n%")+` ESCAPE '\'`)
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	return strings.Join(conds, " AND "), args
}

// resolveMemoryOwners narrows the readable owners to one when owner_id is given (403 if not readable).
func (s *Server) resolveMemoryOwners(ctx context.Context, userID, ownerID int64) ([]int64, error) {
	owners, err := s.readableMemoryOwners(ctx, userID)
	if err != nil || ownerID == 0 {
		return owners, err
	}
	for _, o := range owners {
		if o == ownerID {
			return []int64{ownerID}, nil
		}
	}
	return nil, errMemoryAccess
}

// HandleListMemories lists memories the caller can read, newest first (GET /api/memories).
func (s *Server) HandleListMemories(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	q := r.URL.Query()
	atoi := func(k string) int { n, _ := strconv.Atoi(q.Get(k)); return n }
	var ownerID int64
	if v := q.Get("owner_id"); v != "" {
		var err error
		if ownerID, err = strconv.ParseInt(v, 10, 64); err != nil {
			respondError(w, http.StatusBadRequest, "owner_id must be a user id", "validation_error")
			return
		}
	}
	owners, err := s.resolveMemoryOwners(ctx, userID, ownerID)
	if errors.Is(err, errMemoryAccess) {
		respondError(w, http.StatusForbidden, "those memories are not shared with you", "forbidden")
		return
	}
	if err != nil {
		s.logger.Error("Failed to resolve memory owners", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list memories", "internal_error")
		return
	}
	if len(q["file"]) > memoryMaxFiles {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("at most %d files", memoryMaxFiles), "validation_error")
		return
	}
	f := memoryFilter{Owners: owners, Repo: q.Get("repo"), Kind: q.Get("kind"), Issue: atoi("issue"), PR: atoi("pr"),
		Files: q["file"], IncludeInactive: q.Get("status") == "all"}
	limit, offset := atoi("limit"), max(atoi("offset"), 0)
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	where, args := f.sql("m", time.Now().UTC(), nil)
	rows, err := s.db.QueryContext(ctx, `SELECT `+memoryCols+` FROM agent_memories m WHERE `+where+
		` ORDER BY m.updated_at DESC, m.id DESC LIMIT `+strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(offset), args...)
	if err == nil {
		var ms []Memory
		if ms, err = scanMemories(rows, false); err == nil {
			respondJSON(w, http.StatusOK, map[string]any{"memories": ms, "limit": limit, "offset": offset})
			return
		}
	}
	s.logger.Error("Failed to list memories", zap.Int64("user_id", userID), zap.Error(err))
	respondError(w, http.StatusInternalServerError, "failed to list memories", "internal_error")
}

// memoryFromPath loads {id} and checks the caller may read it; owner says whether the caller owns it.
func (s *Server) memoryFromPath(w http.ResponseWriter, r *http.Request) (mem Memory, userID int64, owner, ok bool) {
	userID, ok = GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return mem, 0, false, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid memory ID", "bad_request")
		return mem, 0, false, false
	}
	mem, err = s.getMemory(r.Context(), s.db, id)
	if err == nil && mem.Status == "compacted" {
		err = errMemoryAccess
	}
	if err == nil && mem.OwnerID != userID {
		var can bool
		if can, err = s.canReadMemories(r.Context(), userID, mem.OwnerID); err == nil && !can {
			err = errMemoryAccess
		}
	}
	if errors.Is(err, errMemoryAccess) {
		respondError(w, http.StatusNotFound, "memory not found", "not_found")
		return mem, 0, false, false
	}
	if err != nil {
		s.logger.Error("Failed to load memory", zap.Int64("memory_id", id), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to load memory", "internal_error")
		return mem, 0, false, false
	}
	return mem, userID, mem.OwnerID == userID, true
}

// HandleGetMemory returns one memory (GET /api/memories/{id}).
func (s *Server) HandleGetMemory(w http.ResponseWriter, r *http.Request) {
	if mem, _, _, ok := s.memoryFromPath(w, r); ok {
		respondJSON(w, http.StatusOK, mem)
	}
}

// HandleDeleteMemory removes one of the caller's memories (DELETE /api/memories/{id}).
func (s *Server) HandleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	mem, userID, owner, ok := s.memoryFromPath(w, r)
	if !ok {
		return
	}
	if !owner {
		respondError(w, http.StatusForbidden, "only the owner can delete a memory", "forbidden")
		return
	}
	err := func() error {
		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		if err := s.lockMemoryOwner(r.Context(), tx, userID); err != nil {
			return err
		}
		if err := s.removeMemories(r.Context(), tx, []int64{mem.ID}, 0, time.Now().UTC().Truncate(time.Second)); err != nil {
			return err
		}
		return tx.Commit()
	}()
	if err != nil {
		s.logger.Error("Failed to delete memory", zap.Int64("memory_id", mem.ID), zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete memory", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleMemoryUsage reports the caller's storage use (GET /api/memories/usage).
func (s *Server) HandleMemoryUsage(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	usage, err := s.memoryUsage(r.Context(), s.db, userID)
	if err != nil {
		s.logger.Error("Failed to read memory usage", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to read usage", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, usage)
}
