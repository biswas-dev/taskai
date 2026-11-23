package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Import of the memories llm-brain hubs wrote as wiki pages (one page per memory in an "Agent memory" project).
// It copies; the pages stay until someone deletes them. The page footer names the hub's memory id, which becomes
// origin_id under the origin_host the caller gives (the hub's name), so the hub's later upserts land on the same
// rows and a second import is a no-op.

// MemoryImportRequest names the project holding memory pages and the hub that wrote them.
type MemoryImportRequest struct {
	ProjectID  int64  `json:"project_id"`
	OriginHost string `json:"origin_host"`
}

// MemoryImportReport says what an import did.
type MemoryImportReport struct {
	Pages             int      `json:"pages"`
	Imported          int      `json:"imported"`
	Duplicates        int      `json:"duplicates"`
	SkippedSuperseded int      `json:"skipped_superseded"`
	Failed            int      `json:"failed"`
	Errors            []string `json:"errors,omitempty"`
	QuotaExceeded     bool     `json:"quota_exceeded,omitempty"`
}

var (
	memoryPageTitleRe = regexp.MustCompile(`^(\[superseded\] )?\[(fact|decision|gotcha|runbook|handoff)\] (.+?): (.+)$`)
	memoryBacktickRe  = regexp.MustCompile("^`(.*)`$")
)

// parseMemoryPage reads a page written by llm-brain's PageTitle and PageMarkdown back into a write.
func parseMemoryPage(title, content string) (req MemoryWriteRequest, superseded bool, err error) {
	t := memoryPageTitleRe.FindStringSubmatch(strings.TrimSpace(title))
	if t == nil {
		return req, false, errors.New("title is not [kind] scope: title")
	}
	superseded, req.Kind, req.Title = t[1] != "", t[2], t[4]

	content = strings.ReplaceAll(strings.TrimSpace(content), "\r\n", "\n")
	if strings.HasPrefix(content, "> Superseded") {
		superseded = true
		_, content, _ = strings.Cut(content, "\n\n")
	}
	meta, rest, ok := strings.Cut(content, "\n\n")
	if !ok || !strings.HasPrefix(meta, "**") {
		return req, superseded, errors.New("no **kind** line")
	}
	cut := strings.LastIndex(rest, "\n---\n")
	if cut < 0 {
		return req, superseded, errors.New("no footer")
	}
	req.Body, rest = strings.TrimSpace(rest[:cut]), strings.TrimSpace(rest[cut+len("\n---\n"):])

	for _, part := range strings.Split(meta, " · ")[1:] {
		v := memoryBacktickRe.FindStringSubmatch(strings.TrimPrefix(strings.TrimPrefix(part, "repo "), "folder "))
		switch {
		case strings.HasPrefix(part, "repo ") && v != nil:
			req.Scope.Repo = v[1]
		case strings.HasPrefix(part, "folder ") && v != nil:
			req.Scope.Folder = v[1]
		case v != nil:
			req.Scope.Files = append(req.Scope.Files, v[1])
		case strings.HasPrefix(part, "issue #"):
			req.Scope.Issue, _ = strconv.Atoi(strings.TrimPrefix(part, "issue #"))
		case strings.HasPrefix(part, "PR #"):
			req.Scope.PR, _ = strconv.Atoi(strings.TrimPrefix(part, "PR #"))
		case strings.HasPrefix(part, "confidence "):
			req.Confidence = strings.TrimPrefix(part, "confidence ")
		case strings.HasPrefix(part, "expires "):
			if at, err := time.Parse("2006-01-02", strings.TrimPrefix(part, "expires ")); err == nil {
				req.ExpiresAt = &at
			}
		}
	}

	footer := strings.Trim(rest, "_\n ")
	for i, part := range strings.Split(footer, " · ") {
		if i == 0 { // "2026-01-02" or "2026-01-02 on host"
			date, host, _ := strings.Cut(part, " on ")
			if at, err := time.Parse("2006-01-02", date); err == nil {
				req.CreatedAt = &at
			}
			req.Author.Host = host
			continue
		}
		switch {
		case strings.HasPrefix(part, "by "):
			by, host, _ := strings.Cut(strings.TrimPrefix(part, "by "), " on ")
			req.Author.Host = host
			req.Author.Agent, req.Author.Provider, _ = strings.Cut(by, " / ")
		case strings.HasPrefix(part, "session "):
			req.Author.Session = strings.Trim(strings.TrimPrefix(part, "session "), "`")
		case strings.HasPrefix(part, "replaces memory "):
			req.SupersedesOriginID = strings.TrimPrefix(part, "replaces memory ")
		case strings.HasPrefix(part, "llm-brain memory "):
			req.OriginID = strings.TrimPrefix(part, "llm-brain memory ")
		}
	}
	return req, superseded, nil
}

// HandleImportWikiMemories copies memory pages from a project into the caller's memories (POST /api/memories/import-wiki).
func (s *Server) HandleImportWikiMemories(w http.ResponseWriter, r *http.Request) {
	var req MemoryImportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	req.OriginHost = strings.TrimSpace(req.OriginHost)
	if req.ProjectID <= 0 || req.OriginHost == "" {
		respondError(w, http.StatusBadRequest, "project_id and origin_host (the hub that wrote the pages) are required", "validation_error")
		return
	}
	userID, ok := s.requireProjectMember(w, r, req.ProjectID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `SELECT wp.id, wp.title, wp.content FROM wiki_pages wp
		WHERE wp.project_id = $1 AND `+wikiVisibleSQL("wp", "$2")+` ORDER BY wp.id`, req.ProjectID, userID)
	if err != nil {
		s.logger.Error("Failed to list memory pages", zap.Int64("project_id", req.ProjectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to read pages", "internal_error")
		return
	}
	type page struct {
		id             int64
		title, content string
	}
	var pages []page
	for rows.Next() {
		var p page
		if err := rows.Scan(&p.id, &p.title, &p.content); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, "failed to read pages", "internal_error")
			return
		}
		pages = append(pages, p)
	}
	rows.Close()

	rep := MemoryImportReport{Pages: len(pages)}
	fail := func(p page, why string) {
		rep.Failed++
		if len(rep.Errors) < 20 {
			rep.Errors = append(rep.Errors, fmt.Sprintf("page %d %q: %s", p.id, p.title, why))
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, p := range pages {
		wreq, superseded, err := parseMemoryPage(p.title, p.content)
		if err != nil {
			fail(p, err.Error())
			continue
		}
		if superseded {
			rep.SkippedSuperseded++
			continue
		}
		if wreq.OriginID != "" {
			wreq.OriginHost = req.OriginHost
		} else {
			wreq.SupersedesOriginID = ""
		}
		m, msg := normalizeMemory(wreq, now)
		if msg != "" {
			fail(p, msg)
			continue
		}
		_, outcome, err := s.upsertMemory(ctx, userID, m)
		switch {
		case errors.Is(err, errMemoryQuota):
			rep.QuotaExceeded = true
		case err != nil:
			s.logger.Warn("Failed to import memory page", zap.Int64("page_id", p.id), zap.Error(err))
			fail(p, "could not be stored")
		case outcome == "created":
			rep.Imported++
		default:
			rep.Duplicates++
		}
		if rep.QuotaExceeded {
			break
		}
	}
	respondJSON(w, http.StatusOK, rep)
}
