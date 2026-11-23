package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Agent memories: short lessons (fact, decision, gotcha, runbook, handoff) that belong to a user. Repo, issue,
// PR, folder and files are filterable scope, not a container. The owner and their API keys write; people the
// owner shares with read and search. Writes from llm-brain hubs carry origin_host + origin_id and are
// idempotent upserts. Compaction leaves a 'compacted' tombstone for rows with an origin, so a hub re-sending
// one does not bring it back.

const (
	memoryMaxTitle       = 200
	memoryMaxBody        = 8000
	memoryMaxFiles       = 50
	memoryMaxField       = 500
	memoryDefaultQuota   = int64(1) << 30 // 1 GiB
	memoryHighWater      = 80             // percent: warn, and compact automatically
	memoryEmbeddingBytes = 384 * 4        // vector(384) of float4, charged up front
	memoryRowOverhead    = 64             // ids, numbers, timestamps
	memoryMaxShares      = 100
	memoryCols           = `id, owner_id, kind, title, body, repo, issue, pr, folder, files, agent, provider, host, session,
		confidence, supersedes, superseded_by, status, expires_at, origin_host, origin_id, size_bytes, created_at, updated_at`
)

var (
	memoryKinds     = []string{"fact", "decision", "gotcha", "runbook", "handoff"}
	memoryRepoRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	memorySpaceRe   = regexp.MustCompile(`[ \t]+`)
	errMemoryQuota  = errors.New("memory quota exceeded")
	errMemoryAccess = errors.New("memory not found")
	likeEscaper     = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

// MemoryScope says what a memory is about.
type MemoryScope struct {
	Repo   string   `json:"repo,omitempty"`
	Issue  int      `json:"issue,omitempty"`
	PR     int      `json:"pr,omitempty"`
	Folder string   `json:"folder,omitempty"`
	Files  []string `json:"files,omitempty"`
}

// MemoryAuthor is who wrote a memory.
type MemoryAuthor struct {
	Agent    string `json:"agent,omitempty"`
	Provider string `json:"provider,omitempty"`
	Host     string `json:"host,omitempty"`
	Session  string `json:"session,omitempty"`
}

// Memory is one stored lesson.
type Memory struct {
	ID           int64        `json:"id"`
	OwnerID      int64        `json:"owner_id"`
	Kind         string       `json:"kind"`
	Title        string       `json:"title"`
	Body         string       `json:"body"`
	Scope        MemoryScope  `json:"scope"`
	Author       MemoryAuthor `json:"author"`
	Confidence   string       `json:"confidence"`
	Supersedes   *int64       `json:"supersedes,omitempty"`
	SupersededBy *int64       `json:"superseded_by,omitempty"`
	Status       string       `json:"status"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	OriginHost   string       `json:"origin_host,omitempty"`
	OriginID     string       `json:"origin_id,omitempty"`
	SizeBytes    int64        `json:"size_bytes"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Rank         float64      `json:"rank,omitempty"`
}

// MemoryWriteRequest creates or updates (by origin) a memory.
type MemoryWriteRequest struct {
	Kind       string       `json:"kind"`
	Title      string       `json:"title"`
	Body       string       `json:"body"`
	Scope      MemoryScope  `json:"scope"`
	Author     MemoryAuthor `json:"author"`
	Confidence string       `json:"confidence,omitempty"`
	// Status is active (default) or superseded, for a hub replicating a memory it already replaced.
	Status    string     `json:"status,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Supersedes is a TaskAI memory id; SupersedesOriginID names one by origin_id under the same origin_host
	// and may arrive before that memory does.
	Supersedes         int64      `json:"supersedes,omitempty"`
	SupersedesOriginID string     `json:"supersedes_origin_id,omitempty"`
	OriginHost         string     `json:"origin_host,omitempty"`
	OriginID           string     `json:"origin_id,omitempty"`
	CreatedAt          *time.Time `json:"created_at,omitempty"` // keep the original time when replicating
}

// MemoryUsage is a user's storage use.
type MemoryUsage struct {
	UsedBytes        int64   `json:"used_bytes"`
	QuotaBytes       int64   `json:"quota_bytes"`
	Percent          float64 `json:"percent"`
	Count            int     `json:"count"`
	ActiveCount      int     `json:"active_count"`
	HighWaterPercent int     `json:"high_water_percent"`
	Warning          string  `json:"warning,omitempty"`
}

// MemoryWriteResponse says what a write did: created, updated, duplicate (already known) or ignored (compacted).
type MemoryWriteResponse struct {
	Memory  Memory      `json:"memory"`
	Outcome string      `json:"outcome"`
	Usage   MemoryUsage `json:"usage"`
}

// memoryRow is a validated write, ready to store.
type memoryRow struct {
	Memory
	files              string
	hash               string
	supersedes         int64
	supersedesOriginID string
}

type sqlQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func oneLine(s string) bool { return !strings.ContainsAny(s, "\r\n") }

// normalizeMemory validates a write. It returns a message safe to show the writer when the write is refused.
func normalizeMemory(req MemoryWriteRequest, now time.Time) (memoryRow, string) {
	m := memoryRow{Memory: Memory{
		Kind:       strings.ToLower(strings.TrimSpace(req.Kind)),
		Title:      strings.TrimSpace(memorySpaceRe.ReplaceAllString(req.Title, " ")),
		Body:       strings.TrimSpace(req.Body),
		Scope:      req.Scope,
		Author:     req.Author,
		Confidence: strings.ToLower(strings.TrimSpace(req.Confidence)),
		Status:     strings.ToLower(strings.TrimSpace(req.Status)),
		OriginHost: strings.TrimSpace(req.OriginHost),
		OriginID:   strings.TrimSpace(req.OriginID),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, supersedes: req.Supersedes, supersedesOriginID: strings.TrimSpace(req.SupersedesOriginID)}

	known := false
	for _, k := range memoryKinds {
		known = known || m.Kind == k
	}
	if !known {
		return m, "kind must be one of " + strings.Join(memoryKinds, ", ")
	}
	if m.Title == "" {
		m.Title, _, _ = strings.Cut(m.Body, "\n")
		m.Title = strings.TrimSpace(m.Title)
	}
	switch {
	case m.Body == "":
		return m, "body is required"
	case !oneLine(m.Title):
		return m, "title must be one line"
	case len(m.Title) > memoryMaxTitle:
		return m, fmt.Sprintf("title is %d bytes; keep it under %d", len(m.Title), memoryMaxTitle)
	case len(m.Body) > memoryMaxBody:
		return m, fmt.Sprintf("body is %d bytes; keep it under %d", len(m.Body), memoryMaxBody)
	}
	switch m.Confidence {
	case "":
		m.Confidence = "medium"
	case "low", "medium", "high":
	default:
		return m, "confidence must be low, medium or high"
	}
	switch m.Status {
	case "":
		m.Status = "active"
	case "active", "superseded":
	default:
		return m, "status must be active or superseded"
	}

	sc := &m.Scope
	sc.Repo = strings.TrimSuffix(strings.TrimSpace(sc.Repo), ".git")
	sc.Folder = strings.TrimRight(strings.TrimSpace(sc.Folder), "/")
	switch {
	case sc.Repo != "" && !memoryRepoRe.MatchString(sc.Repo):
		return m, "scope.repo must be owner/name"
	case sc.Folder != "" && !strings.HasPrefix(sc.Folder, "/"):
		return m, "scope.folder must be an absolute path"
	case sc.Repo == "" && sc.Folder == "":
		return m, "scope needs a repo or a folder"
	case (sc.Issue != 0 || sc.PR != 0) && sc.Repo == "":
		return m, "scope.issue and scope.pr need scope.repo"
	case sc.Issue < 0 || sc.PR < 0:
		return m, "scope.issue and scope.pr are positive numbers"
	case len(sc.Files) > memoryMaxFiles:
		return m, fmt.Sprintf("at most %d files", memoryMaxFiles)
	}
	var files []string
	for _, f := range sc.Files {
		f = strings.TrimPrefix(strings.TrimSpace(f), "./")
		if !oneLine(f) || len(f) > memoryMaxField {
			return m, "each file must be one line under 500 bytes"
		}
		if f != "" {
			files = append(files, f)
		}
	}
	sc.Files = files
	if len(files) > 0 {
		m.files = "\n" + strings.Join(files, "\n") + "\n"
	}

	for _, f := range []string{sc.Folder, m.Author.Agent, m.Author.Provider, m.Author.Host, m.Author.Session,
		m.OriginHost, m.OriginID, m.supersedesOriginID} {
		if len(f) > memoryMaxField || !oneLine(f) {
			return m, "scope.folder, author fields and origin ids must be one line under 500 bytes"
		}
	}
	if m.OriginID != "" && m.OriginHost == "" {
		return m, "origin_id needs origin_host"
	}
	if m.supersedesOriginID != "" && m.OriginID == "" {
		return m, "supersedes_origin_id needs origin_host and origin_id"
	}
	if m.supersedes < 0 {
		return m, "supersedes must be a memory id"
	}
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.UTC().Truncate(time.Second)
		m.ExpiresAt = &t
	}
	if req.CreatedAt != nil && !req.CreatedAt.IsZero() && req.CreatedAt.Before(now) {
		m.CreatedAt = req.CreatedAt.UTC().Truncate(time.Second)
	}
	m.hash = memoryHash(m.Memory)
	m.SizeBytes = memorySize(m)
	return m, ""
}

// memoryHash identifies a memory's content and scope, ignoring case and spacing (the llm-brain hub's rule).
func memoryHash(m Memory) string {
	h := sha256.New()
	for _, s := range []string{m.Kind, normWords(m.Title), normWords(m.Body), strings.ToLower(m.Scope.Repo),
		strconv.Itoa(m.Scope.Issue), strconv.Itoa(m.Scope.PR), m.Scope.Folder, strings.Join(m.Scope.Files, ",")} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func normWords(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// memorySize is what a row costs against the quota: text, metadata and its embedding.
func memorySize(m memoryRow) int64 {
	n := memoryRowOverhead + memoryEmbeddingBytes
	for _, s := range []string{m.Title, m.Body, m.Scope.Repo, m.Scope.Folder, m.files, m.Author.Agent, m.Author.Provider,
		m.Author.Host, m.Author.Session, m.hash, m.OriginHost, m.OriginID, m.supersedesOriginID} {
		n += len(s)
	}
	return int64(n)
}

// tombstoneSize is what a compacted row still costs: its keys.
func tombstoneSize(hash, originHost, originID string) int64 {
	return int64(memoryRowOverhead + len(hash) + len(originHost) + len(originID))
}

func splitMemoryFiles(s string) []string {
	var out []string
	for _, f := range strings.Split(s, "\n") {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func scanMemories(rows *sql.Rows, withRank bool) ([]Memory, error) {
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		var files string
		dest := []any{&m.ID, &m.OwnerID, &m.Kind, &m.Title, &m.Body, &m.Scope.Repo, &m.Scope.Issue, &m.Scope.PR,
			&m.Scope.Folder, &files, &m.Author.Agent, &m.Author.Provider, &m.Author.Host, &m.Author.Session,
			&m.Confidence, &m.Supersedes, &m.SupersededBy, &m.Status, &m.ExpiresAt, &m.OriginHost, &m.OriginID,
			&m.SizeBytes, &m.CreatedAt, &m.UpdatedAt}
		if withRank {
			dest = append(dest, &m.Rank)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		m.Scope.Files = splitMemoryFiles(files)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Server) getMemory(ctx context.Context, q sqlQuerier, id int64) (Memory, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+memoryCols+` FROM agent_memories WHERE id = $1`, id)
	if err != nil {
		return Memory{}, err
	}
	ms, err := scanMemories(rows, false)
	if err != nil {
		return Memory{}, err
	}
	if len(ms) == 0 {
		return Memory{}, errMemoryAccess
	}
	return ms[0], nil
}

// readableMemoryOwners is the caller plus everyone who shared their memories with the caller.
func (s *Server) readableMemoryOwners(ctx context.Context, userID int64) ([]int64, error) {
	owners := []int64{userID}
	rows, err := s.db.QueryContext(ctx, `SELECT owner_id FROM memory_shares WHERE user_id = $1 ORDER BY owner_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		owners = append(owners, id)
	}
	return owners, rows.Err()
}

func (s *Server) canReadMemories(ctx context.Context, userID, ownerID int64) (bool, error) {
	if userID == ownerID {
		return true, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_shares WHERE owner_id = $1 AND user_id = $2`, ownerID, userID).Scan(&n)
	return n > 0, err
}

func (s *Server) memoryQuota(ctx context.Context, q sqlQuerier, ownerID int64) (int64, error) {
	var quota int64
	err := q.QueryRowContext(ctx, `SELECT quota_bytes FROM memory_quotas WHERE user_id = $1`, ownerID).Scan(&quota)
	if errors.Is(err, sql.ErrNoRows) {
		return memoryDefaultQuota, nil
	}
	return quota, err
}

func (s *Server) memoryUsage(ctx context.Context, q sqlQuerier, ownerID int64) (MemoryUsage, error) {
	u := MemoryUsage{HighWaterPercent: memoryHighWater}
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0), COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0)
		FROM agent_memories WHERE owner_id = $1`, ownerID).Scan(&u.UsedBytes, &u.Count, &u.ActiveCount)
	if err != nil {
		return u, err
	}
	if u.QuotaBytes, err = s.memoryQuota(ctx, q, ownerID); err != nil {
		return u, err
	}
	u.Percent = float64(u.UsedBytes) * 100 / float64(u.QuotaBytes)
	if u.Percent >= memoryHighWater {
		u.Warning = fmt.Sprintf("memory storage is %.0f%% full (%d of %d bytes); compact or delete memories", u.Percent, u.UsedBytes, u.QuotaBytes)
	}
	return u, nil
}

// lockMemoryOwner serialises writes to one owner's memories for the rest of tx (SQLite serialises writes itself).
func (s *Server) lockMemoryOwner(ctx context.Context, tx *sql.Tx, ownerID int64) error {
	if s.db.Driver != "postgres" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('agent_memories'), $1::int)`, ownerID)
	return err
}

// clearMemoryEmbedding drops a stale vector so the worker embeds the row again (Postgres only).
func (s *Server) clearMemoryEmbedding(ctx context.Context, q sqlQuerier, id int64) error {
	if s.db.Driver != "postgres" {
		return nil
	}
	_, err := q.ExecContext(ctx, `UPDATE agent_memories SET embedding = NULL, embedded_at = NULL WHERE id = $1`, id)
	return err
}

type memoryInputError string

func (e memoryInputError) Error() string { return string(e) }

// insertMemoryRow stores a validated memory and returns its id.
func insertMemoryRow(ctx context.Context, q sqlQuerier, ownerID int64, m memoryRow, supersedes any) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `INSERT INTO agent_memories (owner_id, kind, title, body, repo, issue, pr, folder, files,
		agent, provider, host, session, confidence, supersedes, supersedes_origin_id, status, expires_at, content_hash,
		origin_host, origin_id, size_bytes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		RETURNING id`,
		ownerID, m.Kind, m.Title, m.Body, m.Scope.Repo, m.Scope.Issue, m.Scope.PR, m.Scope.Folder, m.files, m.Author.Agent,
		m.Author.Provider, m.Author.Host, m.Author.Session, m.Confidence, supersedes, m.supersedesOriginID, m.Status,
		m.ExpiresAt, m.hash, m.OriginHost, m.OriginID, m.SizeBytes, m.CreatedAt, m.UpdatedAt).Scan(&id)
	return id, err
}

// upsertMemory stores m for owner: an existing origin is updated (or ignored when compacted), an exact repeat
// returns the memory already kept, anything else is inserted if it fits the quota.
func (s *Server) upsertMemory(ctx context.Context, ownerID int64, m memoryRow) (Memory, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Memory{}, "", err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := s.lockMemoryOwner(ctx, tx, ownerID); err != nil {
		return Memory{}, "", err
	}

	var existingID, oldSize int64
	var oldStatus, oldHash, oldConfidence, oldSupOrigin string
	var oldExpires *time.Time
	var oldSupersedes *int64
	found := false
	if m.OriginID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id, status, content_hash, size_bytes, confidence, expires_at, supersedes_origin_id, supersedes
			FROM agent_memories WHERE owner_id = $1 AND origin_host = $2 AND origin_id = $3`, ownerID, m.OriginHost, m.OriginID).
			Scan(&existingID, &oldStatus, &oldHash, &oldSize, &oldConfidence, &oldExpires, &oldSupOrigin, &oldSupersedes)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
		found = err == nil
	}
	if found && oldStatus == "compacted" {
		mem, err := s.getMemory(ctx, tx, existingID)
		return mem, "ignored", err
	}
	if found && oldStatus == "superseded" {
		m.Status = "superseded" // a replay of the original must not bring back what replaced it
	}
	if !found {
		var dupID int64
		var dupOrigin string
		err := tx.QueryRowContext(ctx, `SELECT id, origin_id FROM agent_memories
			WHERE owner_id = $1 AND content_hash = $2 AND status = 'active' ORDER BY id LIMIT 1`, ownerID, m.hash).Scan(&dupID, &dupOrigin)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
		if err == nil {
			switch {
			case dupOrigin == "" && m.OriginID != "": // adopt the origin so later updates from that hub land here
				if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET origin_host = $1, origin_id = $2, size_bytes = size_bytes + $3 WHERE id = $4`,
					m.OriginHost, m.OriginID, len(m.OriginHost)+len(m.OriginID), dupID); err != nil {
					return Memory{}, "", err
				}
			case m.OriginID != "":
				// Known under another origin: record this one as an alias tombstone, so deleting or compacting the
				// memory is not undone when the second hub sends it again.
				alias := memoryRow{Memory: Memory{Kind: m.Kind, Confidence: m.Confidence, Status: "compacted",
					OriginHost: m.OriginHost, OriginID: m.OriginID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}, hash: m.hash}
				alias.SizeBytes = tombstoneSize(m.hash, m.OriginHost, m.OriginID)
				aliasID, err := insertMemoryRow(ctx, tx, ownerID, alias, nil)
				if err == nil {
					_, err = tx.ExecContext(ctx, `UPDATE agent_memories SET superseded_by = $1 WHERE id = $2`, dupID, aliasID)
				}
				if err != nil {
					return Memory{}, "", err
				}
			}
			mem, err := s.getMemory(ctx, tx, dupID)
			if err != nil {
				return Memory{}, "", err
			}
			return mem, "duplicate", tx.Commit()
		}
	}

	// Resolve what it supersedes before writing, so a bad id is refused.
	var target int64
	if m.supersedes > 0 {
		var st string
		err := tx.QueryRowContext(ctx, `SELECT status FROM agent_memories WHERE id = $1 AND owner_id = $2`, m.supersedes, ownerID).Scan(&st)
		if errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", memoryInputError(fmt.Sprintf("supersedes: no memory %d", m.supersedes))
		}
		if err != nil {
			return Memory{}, "", err
		}
		if st == "compacted" {
			return Memory{}, "", memoryInputError(fmt.Sprintf("supersedes: memory %d was removed", m.supersedes))
		}
		target = m.supersedes
	} else if m.supersedesOriginID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id FROM agent_memories WHERE owner_id = $1 AND origin_host = $2 AND origin_id = $3`,
			ownerID, m.OriginHost, m.supersedesOriginID).Scan(&target)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
	}

	sameExpiry := (oldExpires == nil) == (m.ExpiresAt == nil) && (oldExpires == nil || oldExpires.Equal(*m.ExpiresAt))
	sameLink := oldSupOrigin == m.supersedesOriginID && (target == 0 || oldSupersedes != nil && *oldSupersedes == target)
	if found && oldHash == m.hash && oldStatus == m.Status && oldConfidence == m.Confidence && sameExpiry && sameLink {
		mem, err := s.getMemory(ctx, tx, existingID)
		return mem, "duplicate", err
	}

	usage, err := s.memoryUsage(ctx, tx, ownerID)
	if err != nil {
		return Memory{}, "", err
	}
	if grow := m.SizeBytes - oldSize; grow > 0 && usage.UsedBytes+grow > usage.QuotaBytes {
		return Memory{}, "", errMemoryQuota
	}

	var supersedes any
	if target > 0 {
		supersedes = target
	}
	outcome := "created"
	id := existingID
	if found {
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `UPDATE agent_memories SET kind = $1, title = $2, body = $3, repo = $4, issue = $5, pr = $6,
			folder = $7, files = $8, agent = $9, provider = $10, host = $11, session = $12, confidence = $13, status = $14,
			expires_at = $15, content_hash = $16, size_bytes = $17, supersedes = COALESCE($18, supersedes),
			supersedes_origin_id = $19, updated_at = $20 WHERE id = $21`,
			m.Kind, m.Title, m.Body, m.Scope.Repo, m.Scope.Issue, m.Scope.PR, m.Scope.Folder, m.files, m.Author.Agent,
			m.Author.Provider, m.Author.Host, m.Author.Session, m.Confidence, m.Status, m.ExpiresAt, m.hash, m.SizeBytes,
			supersedes, m.supersedesOriginID, m.UpdatedAt, id)
		if err == nil && oldHash != m.hash {
			err = s.clearMemoryEmbedding(ctx, tx, id)
		}
	} else {
		id, err = insertMemoryRow(ctx, tx, ownerID, m, supersedes)
	}
	if err != nil {
		return Memory{}, "", err
	}

	if target > 0 && target != id {
		if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET status = 'superseded', superseded_by = $1, updated_at = $2
			WHERE id = $3 AND status = 'active'`, id, m.UpdatedAt, target); err != nil {
			return Memory{}, "", err
		}
	}
	if !found && m.OriginID != "" && m.Status == "active" {
		// A newer memory from the same hub that arrived first already names this one: it is superseded on arrival.
		var newer int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM agent_memories WHERE owner_id = $1 AND origin_host = $2
			AND supersedes_origin_id = $3 AND supersedes IS NULL AND id <> $4 ORDER BY id DESC LIMIT 1`,
			ownerID, m.OriginHost, m.OriginID, id).Scan(&newer)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
		if err == nil {
			if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET status = 'superseded', superseded_by = $1 WHERE id = $2`, newer, id); err != nil {
				return Memory{}, "", err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET supersedes = $1 WHERE id = $2`, id, newer); err != nil {
				return Memory{}, "", err
			}
		}
	}

	mem, err := s.getMemory(ctx, tx, id)
	if err != nil {
		return Memory{}, "", err
	}
	return mem, outcome, tx.Commit()
}

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

// removeMemories deletes rows, leaving a tombstone for rows a hub may send again (they carry an origin).
func (s *Server) removeMemories(ctx context.Context, q sqlQuerier, ids []int64, supersededBy int64, now time.Time) error {
	var by any
	if supersededBy > 0 {
		by = supersededBy
	}
	for _, id := range ids {
		var hash, host, origin string
		if err := q.QueryRowContext(ctx, `SELECT content_hash, origin_host, origin_id FROM agent_memories WHERE id = $1`, id).
			Scan(&hash, &host, &origin); err != nil {
			return err
		}
		if origin == "" {
			if _, err := q.ExecContext(ctx, `DELETE FROM agent_memories WHERE id = $1`, id); err != nil {
				return err
			}
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE agent_memories SET status = 'compacted', title = '', body = '', files = '',
			folder = '', agent = '', provider = '', host = '', session = '', supersedes_origin_id = '',
			superseded_by = COALESCE($1, superseded_by), size_bytes = $2, updated_at = $3 WHERE id = $4`,
			by, tombstoneSize(hash, host, origin), now, id); err != nil {
			return err
		}
		if err := s.clearMemoryEmbedding(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
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

// MemoryShareUser is someone on either side of a share.
type MemoryShareUser struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
}

func (s *Server) memoryShareUsers(ctx context.Context, query string, userID int64) ([]MemoryShareUser, error) {
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryShareUser{}
	for rows.Next() {
		var u MemoryShareUser
		if err := rows.Scan(&u.UserID, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) respondMemoryShares(w http.ResponseWriter, ctx context.Context, userID int64) {
	with, err := s.memoryShareUsers(ctx, `SELECT u.id, u.email FROM memory_shares ms JOIN users u ON u.id = ms.user_id
		WHERE ms.owner_id = $1 ORDER BY u.email`, userID)
	var me []MemoryShareUser
	if err == nil {
		me, err = s.memoryShareUsers(ctx, `SELECT u.id, u.email FROM memory_shares ms JOIN users u ON u.id = ms.owner_id
			WHERE ms.user_id = $1 ORDER BY u.email`, userID)
	}
	if err != nil {
		s.logger.Error("Failed to list memory shares", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list shares", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"shared_with": with, "shared_with_me": me})
}

// HandleGetMemoryShares lists who the caller shares memories with, and who shares with them (GET /api/memories/shares).
func (s *Server) HandleGetMemoryShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	s.respondMemoryShares(w, r.Context(), userID)
}

// HandleSetMemoryShares replaces the list of people who can read the caller's memories (PUT /api/memories/shares).
func (s *Server) HandleSetMemoryShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if len(req.UserIDs) > memoryMaxShares {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("share with at most %d people", memoryMaxShares), "validation_error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `DELETE FROM memory_shares WHERE owner_id = $1`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	seen := map[int64]bool{userID: true}
	for _, id := range req.UserIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		// Only people the owner already works with (a team or a project in common), so ids cannot be probed for emails.
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u WHERE u.id = $2 AND u.deleted_at IS NULL AND (
			EXISTS (SELECT 1 FROM team_members a JOIN team_members b ON a.team_id = b.team_id
				WHERE a.user_id = $1 AND b.user_id = u.id AND a.status = 'active' AND b.status = 'active')
			OR EXISTS (SELECT 1 FROM project_members a JOIN project_members b ON a.project_id = b.project_id
				WHERE a.user_id = $1 AND b.user_id = u.id))`, userID, id).Scan(&n); err != nil {
			s.logger.Error("Failed to check memory share target", zap.Int64("user_id", userID), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
			return
		}
		if n == 0 {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("user %d is not in a team or project with you", id), "validation_error")
			return
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO memory_shares (owner_id, user_id, granted_by) VALUES ($1, $2, $1)`, userID, id); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to update memory shares", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	s.respondMemoryShares(w, ctx, userID)
}

// HandleAdminSetMemoryQuota sets a user's memory quota; null restores the default (PUT /api/admin/users/{id}/memory-quota).
func (s *Server) HandleAdminSetMemoryQuota(w http.ResponseWriter, r *http.Request) {
	callerID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), callerID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}
	target, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}
	var req struct {
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if req.QuotaBytes != nil && *req.QuotaBytes <= 0 {
		respondError(w, http.StatusBadRequest, "quota_bytes must be positive, or null for the default", "validation_error")
		return
	}
	ctx := r.Context()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, target).Scan(&n); err != nil {
		s.logger.Error("Failed to look up user for memory quota", zap.Int64("user_id", target), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set quota", "internal_error")
		return
	}
	if n == 0 {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}
	if req.QuotaBytes == nil {
		_, err = s.db.ExecContext(ctx, `DELETE FROM memory_quotas WHERE user_id = $1`, target)
	} else {
		_, err = s.db.ExecContext(ctx, `INSERT INTO memory_quotas (user_id, quota_bytes, updated_at) VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE SET quota_bytes = excluded.quota_bytes, updated_at = excluded.updated_at`,
			target, *req.QuotaBytes, time.Now().UTC())
	}
	if err != nil {
		s.logger.Error("Failed to set memory quota", zap.Int64("user_id", target), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set quota", "internal_error")
		return
	}
	usage, err := s.memoryUsage(ctx, s.db, target)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read usage", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, usage)
}
