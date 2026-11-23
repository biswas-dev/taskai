package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Memories (protocol version memoryProtocolVersion, docs/memory-protocol.md): short lessons (fact, decision,
// gotcha, runbook, handoff) that belong to a user, written and searched by any application or LLM. Repo, issue,
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
	memoryMaxQuota       = int64(1) << 40 // 1 TiB
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
		m.Title = strings.TrimSpace(memorySpaceRe.ReplaceAllString(strings.ReplaceAll(m.Title, "\r", ""), " "))
		if len(m.Title) > memoryMaxTitle {
			m.Title = strings.ToValidUTF8(m.Title[:memoryMaxTitle], "")
		}
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
	sc.Folder = strings.TrimSpace(sc.Folder)
	if strings.Trim(sc.Folder, "/") == "" && sc.Folder != "" {
		sc.Folder = "/"
	} else {
		sc.Folder = strings.TrimRight(sc.Folder, "/")
	}
	switch {
	case sc.Repo != "" && !memoryRepoRe.MatchString(sc.Repo):
		return m, "scope.repo must be owner/name"
	case sc.Folder != "" && !strings.HasPrefix(sc.Folder, "/"):
		return m, "scope.folder must be an absolute path"
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

	for _, f := range []string{sc.Repo, sc.Folder, m.Author.Agent, m.Author.Provider, m.Author.Host, m.Author.Session,
		m.OriginHost, m.OriginID, m.supersedesOriginID} {
		if len(f) > memoryMaxField || !oneLine(f) {
			return m, "scope.repo, scope.folder, author fields and origin ids must be one line under 500 bytes"
		}
	}
	if m.OriginID != "" && m.OriginHost == "" {
		return m, "origin_id needs origin_host"
	}
	if m.supersedesOriginID != "" && m.OriginID == "" {
		return m, "supersedes_origin_id needs origin_host and origin_id"
	}
	if m.supersedesOriginID != "" && m.supersedesOriginID == m.OriginID {
		return m, "a memory cannot supersede itself"
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
		strconv.Itoa(m.Scope.Issue), strconv.Itoa(m.Scope.PR), m.Scope.Folder, strings.Join(m.Scope.Files, "\n")} {
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
	// Bytes count every row; count leaves out tombstones, active_count leaves out expired memories.
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0),
		COALESCE(SUM(CASE WHEN status <> 'compacted' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'active' AND (expires_at IS NULL OR expires_at > $2) THEN 1 ELSE 0 END), 0)
		FROM agent_memories WHERE owner_id = $1`, ownerID, time.Now().UTC()).Scan(&u.UsedBytes, &u.Count, &u.ActiveCount)
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
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('agent_memories'), hashtext($1::text))`, strconv.FormatInt(ownerID, 10))
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

// supersedeMemory marks an active memory as replaced by another and links the successor back to it.
func supersedeMemory(ctx context.Context, q sqlQuerier, oldID, newID int64, now time.Time) error {
	if _, err := q.ExecContext(ctx, `UPDATE agent_memories SET status = 'superseded', superseded_by = $1, updated_at = $2
		WHERE id = $3 AND status = 'active'`, newID, now, oldID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE agent_memories SET supersedes = $1 WHERE id = $2 AND supersedes IS NULL`, oldID, newID)
	return err
}

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
	// Resolve what it supersedes before writing, so a bad id is refused.
	var target int64
	unchangedLink := found && oldSupersedes != nil && (*oldSupersedes == m.supersedes || m.supersedes == 0 && m.supersedesOriginID == oldSupOrigin && oldSupOrigin != "")
	if unchangedLink {
		target = *oldSupersedes // a replay of an accepted link, even if its target was compacted since
	} else if m.supersedes > 0 {
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
		var targetSupOrigin string
		err := tx.QueryRowContext(ctx, `SELECT id, supersedes_origin_id FROM agent_memories WHERE owner_id = $1 AND origin_host = $2 AND origin_id = $3`,
			ownerID, m.OriginHost, m.supersedesOriginID).Scan(&target, &targetSupOrigin)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
		if targetSupOrigin != "" && targetSupOrigin == m.OriginID {
			return Memory{}, "", memoryInputError("supersedes_origin_id: the two memories would supersede each other")
		}
	}
	if found && target == existingID {
		return Memory{}, "", memoryInputError("a memory cannot supersede itself")
	}
	if found && oldStatus == "superseded" && target > 0 && !unchangedLink {
		return Memory{}, "", memoryInputError("a superseded memory cannot supersede another")
	}

	if m.OriginID == "" {
		// A write without an origin (MCP, UI) that repeats an active memory's content and scope keeps the one we
		// have. Writes with an origin always land on their own key; compaction later folds copies from other keys.
		var dupID int64
		var dupExpires *time.Time
		var dupConfidence string
		err := tx.QueryRowContext(ctx, `SELECT id, expires_at, confidence FROM agent_memories WHERE owner_id = $1 AND content_hash = $2
			AND status = 'active' AND (expires_at IS NULL OR expires_at > $3) ORDER BY id LIMIT 1`, ownerID, m.hash, m.UpdatedAt).
			Scan(&dupID, &dupExpires, &dupConfidence)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Memory{}, "", err
		}
		if err == nil {
			if !outlives(dupExpires, m.ExpiresAt) { // the repeat should last longer: extend the one we keep
				if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET expires_at = $1, updated_at = $2 WHERE id = $3`,
					m.ExpiresAt, m.UpdatedAt, dupID); err != nil {
					return Memory{}, "", err
				}
			}
			if memoryConfidenceRank[m.Confidence] > memoryConfidenceRank[dupConfidence] {
				if _, err := tx.ExecContext(ctx, `UPDATE agent_memories SET confidence = $1, updated_at = $2 WHERE id = $3`,
					m.Confidence, m.UpdatedAt, dupID); err != nil {
					return Memory{}, "", err
				}
			}
			if target > 0 && target != dupID {
				if err := supersedeMemory(ctx, tx, target, dupID, m.UpdatedAt); err != nil {
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
		if err := supersedeMemory(ctx, tx, target, id, m.UpdatedAt); err != nil {
			return Memory{}, "", err
		}
	}
	if !found && m.OriginID != "" && m.Status == "active" {
		// A newer memory from the same hub that arrived first already names this one: it is superseded on arrival.
		var newer int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM agent_memories WHERE owner_id = $1 AND origin_host = $2
			AND supersedes_origin_id = $3 AND supersedes IS NULL AND id <> $4 AND id <> $5 ORDER BY id DESC LIMIT 1`,
			ownerID, m.OriginHost, m.OriginID, id, target).Scan(&newer)
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

// removeMemories deletes rows, leaving a tombstone for rows a hub may send again (they carry an origin).
func (s *Server) removeMemories(ctx context.Context, q sqlQuerier, ids []int64, supersededBy int64, now time.Time) error {
	var by any
	if supersededBy > 0 {
		by = supersededBy
	}
	for _, id := range ids {
		var hash, host, origin string
		err := q.QueryRowContext(ctx, `SELECT content_hash, origin_host, origin_id FROM agent_memories WHERE id = $1`, id).
			Scan(&hash, &host, &origin)
		if errors.Is(err, sql.ErrNoRows) {
			continue // removed meanwhile
		}
		if err != nil {
			return err
		}
		if origin == "" {
			if _, err := q.ExecContext(ctx, `DELETE FROM agent_memories WHERE id = $1`, id); err != nil {
				return err
			}
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE agent_memories SET status = 'compacted', title = '', body = '', files = '',
			folder = '', agent = '', provider = '', host = '', session = '',
			superseded_by = COALESCE($1, superseded_by), size_bytes = $2 + LENGTH(supersedes_origin_id), updated_at = $3 WHERE id = $4`,
			by, tombstoneSize(hash, host, origin), now, id); err != nil {
			return err
		}
		if err := s.clearMemoryEmbedding(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}
