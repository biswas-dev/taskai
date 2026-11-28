package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Sync objects: opaque bytes per user, addressed by key, for Brain Hub encrypted sync. The server never parses a
// body. The ETag is the quoted SHA-256 hex of the body. If-None-Match: * creates only, If-Match replaces or deletes
// only that version.

const (
	syncMaxObject    = 8 << 20        // 8 MiB per object
	syncDefaultQuota = int64(1) << 30 // 1 GiB per user
	syncMaxKey       = 256
	syncMaxList      = 1000
)

var (
	syncKeyRe    = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
	syncPrefixRe = regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
)

// SyncObject is the metadata of one stored object.
type SyncObject struct {
	Key       string    `json:"key"`
	Size      int64     `json:"size"`
	ETag      string    `json:"etag"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncObjectList is one page of a listing, ordered by key. Next is the after= value for the next page, or empty.
type SyncObjectList struct {
	Objects []SyncObject `json:"objects"`
	Next    string       `json:"next"`
}

func validSyncKey(key string) bool {
	if len(key) > syncMaxKey || !syncKeyRe.MatchString(key) {
		return false
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// syncIfMatch reports whether an If-Match header names the current etag (quoted or bare, weak or strong, or *).
// A missing object (current == "") never matches.
func syncIfMatch(header, current string) bool {
	if current == "" {
		return false
	}
	for _, v := range strings.Split(header, ",") {
		v = strings.TrimPrefix(strings.TrimSpace(v), "W/")
		if v == "*" || strings.Trim(v, `"`) == current {
			return true
		}
	}
	return false
}

func (s *Server) syncQuota() int64 {
	if s.config != nil && s.config.SyncQuotaBytes > 0 {
		return s.config.SyncQuotaBytes
	}
	return syncDefaultQuota
}

// syncObjectKey reads and checks the key from the route; it writes the 400 itself.
func syncObjectKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := chi.URLParam(r, "*")
	if !validSyncKey(key) {
		respondError(w, http.StatusBadRequest, "key must match ^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$, be at most 256 characters and have no . or .. segment", "invalid_key")
		return "", false
	}
	return key, true
}

// beginSyncTx opens a transaction that serialises writes to one owner's objects (SQLite serialises writes itself).
func (s *Server) beginSyncTx(ctx context.Context, ownerID int64) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if s.db.Driver == "postgres" {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('sync_objects'), hashtext($1::text))`, strconv.FormatInt(ownerID, 10)); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return tx, nil
}

// currentSyncETag returns the stored etag and size of key, or "" when it does not exist.
func currentSyncETag(ctx context.Context, q sqlQuerier, ownerID int64, key string) (string, int64, error) {
	var etag string
	var size int64
	err := q.QueryRowContext(ctx, `SELECT etag, size FROM sync_objects WHERE owner_id = $1 AND key = $2`, ownerID, key).Scan(&etag, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return etag, size, err
}

// HandlePutSyncObject stores the raw body under key (PUT /api/sync/objects/{key}).
func (s *Server) HandlePutSyncObject(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	key, ok := syncObjectKey(w, r)
	if !ok {
		return
	}
	ifNoneMatch, ifMatch := strings.TrimSpace(r.Header.Get("If-None-Match")), r.Header.Get("If-Match")
	if ifNoneMatch != "" && ifNoneMatch != "*" || ifNoneMatch != "" && ifMatch != "" {
		respondError(w, http.StatusBadRequest, "use If-None-Match: * or If-Match: <etag>, not both", "invalid_precondition")
		return
	}
	if r.ContentLength > syncMaxObject {
		respondError(w, http.StatusRequestEntityTooLarge, "object is larger than 8 MiB", "object_too_large")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, syncMaxObject))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		respondError(w, http.StatusRequestEntityTooLarge, "object is larger than 8 MiB", "object_too_large")
		return
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, "could not read body", "invalid_request")
		return
	}
	sum := sha256.Sum256(body)
	obj := SyncObject{Key: key, Size: int64(len(body)), ETag: hex.EncodeToString(sum[:]), UpdatedAt: time.Now().UTC().Truncate(time.Millisecond)}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tx, err := s.beginSyncTx(ctx, userID)
	if err != nil {
		s.syncFail(w, "begin", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	current, oldSize, err := currentSyncETag(ctx, tx, userID, key)
	if err != nil {
		s.syncFail(w, "read", err)
		return
	}
	if ifNoneMatch == "*" && current != "" || ifMatch != "" && !syncIfMatch(ifMatch, current) {
		respondError(w, http.StatusPreconditionFailed, "precondition failed", "precondition_failed")
		return
	}
	var used int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM sync_objects WHERE owner_id = $1`, userID).Scan(&used); err != nil {
		s.syncFail(w, "usage", err)
		return
	}
	if used-oldSize+obj.Size > s.syncQuota() {
		respondError(w, http.StatusRequestEntityTooLarge, "sync storage quota exceeded", "sync_quota_exceeded")
		return
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sync_objects (owner_id, key, body, size, etag, updated_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (owner_id, key) DO UPDATE SET body = excluded.body, size = excluded.size, etag = excluded.etag, updated_at = excluded.updated_at`,
		userID, key, body, obj.Size, obj.ETag, obj.UpdatedAt)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.syncFail(w, "write", err)
		return
	}
	status := http.StatusOK
	if current == "" {
		status = http.StatusCreated
	}
	w.Header().Set("ETag", `"`+obj.ETag+`"`)
	respondJSON(w, status, obj)
}

// HandleGetSyncObject returns the exact stored bytes (GET /api/sync/objects/{key}).
func (s *Server) HandleGetSyncObject(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	key, ok := syncObjectKey(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var body []byte
	var etag string
	err := s.db.QueryRowContext(ctx, `SELECT body, etag FROM sync_objects WHERE owner_id = $1 AND key = $2`, userID, key).Scan(&body, &etag)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "object not found", "not_found")
		return
	}
	if err != nil {
		s.syncFail(w, "get", err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("ETag", `"`+etag+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// HandleDeleteSyncObject removes key (DELETE /api/sync/objects/{key}). A missing key is 204, unless If-Match is
// set: then only the named version is removed, and anything else is 412.
func (s *Server) HandleDeleteSyncObject(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	key, ok := syncObjectKey(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tx, err := s.beginSyncTx(ctx, userID)
	if err != nil {
		s.syncFail(w, "begin", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	current, _, err := currentSyncETag(ctx, tx, userID, key)
	if err != nil {
		s.syncFail(w, "read", err)
		return
	}
	if ifMatch := r.Header.Get("If-Match"); ifMatch != "" && !syncIfMatch(ifMatch, current) {
		respondError(w, http.StatusPreconditionFailed, "precondition failed", "precondition_failed")
		return
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM sync_objects WHERE owner_id = $1 AND key = $2`, userID, key)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.syncFail(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleListSyncObjects lists the caller's objects by key (GET /api/sync/objects?prefix=&after=&limit=).
func (s *Server) HandleListSyncObjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	q := r.URL.Query()
	prefix, after := q.Get("prefix"), q.Get("after")
	if len(prefix) > syncMaxKey || len(after) > syncMaxKey || !syncPrefixRe.MatchString(prefix) || !syncPrefixRe.MatchString(after) {
		respondError(w, http.StatusBadRequest, "prefix and after use key characters only, at most 256", "invalid_request")
		return
	}
	limit := syncMaxList
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > syncMaxList {
			respondError(w, http.StatusBadRequest, "limit must be 1 to 1000", "invalid_request")
			return
		}
		limit = n
	}
	// Keys use only characters below '~', so every key that starts with prefix sorts in [prefix, prefix+"~").
	// A range compare is exact and byte-ordered on both drivers; LIKE is case-insensitive on SQLite.
	upper := "\x7f"
	if prefix != "" {
		upper = prefix + "~"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT key, size, etag, updated_at FROM sync_objects
		WHERE owner_id = $1 AND key >= $2 AND key < $3 AND key > $4 ORDER BY key LIMIT $5`,
		userID, prefix, upper, after, limit+1)
	if err != nil {
		s.syncFail(w, "list", err)
		return
	}
	defer rows.Close()
	out := SyncObjectList{Objects: []SyncObject{}}
	for rows.Next() {
		var o SyncObject
		if err := rows.Scan(&o.Key, &o.Size, &o.ETag, &o.UpdatedAt); err != nil {
			s.syncFail(w, "scan", err)
			return
		}
		out.Objects = append(out.Objects, o)
	}
	if err := rows.Err(); err != nil {
		s.syncFail(w, "list", err)
		return
	}
	if len(out.Objects) > limit {
		out.Objects = out.Objects[:limit]
		out.Next = out.Objects[limit-1].Key
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) syncFail(w http.ResponseWriter, step string, err error) {
	s.logger.Error("sync object request failed", zap.String("step", step), zap.Error(err))
	respondError(w, http.StatusInternalServerError, "internal server error", "internal_error")
}
