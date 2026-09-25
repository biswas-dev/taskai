package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/wikipage"
)

// ── PDF Job Store ───────────────────────────────────────────────────

// pdfJobStatus represents the lifecycle of an async PDF generation job.
type pdfJobStatus string

const (
	pdfJobPending  pdfJobStatus = "pending"
	pdfJobDone     pdfJobStatus = "done"
	pdfJobFailed   pdfJobStatus = "failed"
)

// pdfJob holds the state of a single PDF generation request.
type pdfJob struct {
	ID        string       `json:"id"`
	Status    pdfJobStatus `json:"status"`
	Filename  string       `json:"filename,omitempty"`
	Error     string       `json:"error,omitempty"`
	CreatedAt time.Time    `json:"created_at"`

	data   []byte // PDF bytes — not serialized to JSON
	userID int64  // only the requester may poll or download the job
	pageID int64
}

// pdfJobStore is a simple in-memory store for PDF generation jobs.
// Jobs are automatically cleaned up after 10 minutes.
type pdfJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*pdfJob
}

func newPDFJobStore() *pdfJobStore {
	s := &pdfJobStore{jobs: make(map[string]*pdfJob)}
	go s.cleanupLoop()
	return s
}

func (s *pdfJobStore) create(filename string, userID, pageID int64) *pdfJob {
	j := &pdfJob{
		ID:        uuid.New().String(),
		Status:    pdfJobPending,
		Filename:  filename,
		CreatedAt: time.Now(),
		userID:    userID,
		pageID:    pageID,
	}
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()
	return j
}

func (s *pdfJobStore) get(id string) *pdfJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id]
}

func (s *pdfJobStore) complete(id string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = pdfJobDone
		j.data = data
	}
}

func (s *pdfJobStore) fail(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = pdfJobFailed
		j.Error = errMsg
	}
}

func (s *pdfJobStore) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// cleanupLoop removes jobs older than 10 minutes.
func (s *pdfJobStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for id, j := range s.jobs {
			if j.CreatedAt.Before(cutoff) {
				delete(s.jobs, id)
			}
		}
		s.mu.Unlock()
	}
}

// pdfJobs is the singleton job store, initialized once.
var pdfJobs = newPDFJobStore()

// ── Regexes ─────────────────────────────────────────────────────────

// drawEmbedRe matches godraw-embed divs in rendered HTML and captures the draw ID.
var drawEmbedRe = regexp.MustCompile(`<div class="godraw-embed"[^>]*data-src="[^"]*?/([a-zA-Z0-9_-]+?)(?:/edit)?"[^>]*></div>`)

// drawScriptRe matches the go-draw embed.js script tags injected by go-wiki.
var drawScriptRe = regexp.MustCompile(`<script[^>]*src="[^"]*embed\.js"[^>]*></script>`)

// ── CSS ─────────────────────────────────────────────────────────────

const pdfCSS = `
body {
  font-family: Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Helvetica Neue', Arial, sans-serif;
  font-size: 16px;
  line-height: 1.75;
  color: #374151;
  -webkit-font-smoothing: antialiased;
}

.prose { max-width: none; }

.prose h1 { font-size: 2.25em; margin-top: 0; margin-bottom: 0.8888889em; line-height: 1.1111111; font-weight: 800; color: #111827; }
.prose h2 { font-size: 1.5em; margin-top: 2em; margin-bottom: 1em; line-height: 1.3333333; font-weight: 700; color: #111827; }
.prose h3 { font-size: 1.25em; margin-top: 1.6em; margin-bottom: 0.6em; line-height: 1.6; font-weight: 600; color: #111827; }
.prose h4 { margin-top: 1.5em; margin-bottom: 0.5em; line-height: 1.5; font-weight: 600; color: #111827; }

.prose p { margin-top: 1.25em; margin-bottom: 1.25em; }
.prose a { color: #5e6ad2; text-decoration: underline; font-weight: 500; }
.prose strong { color: #111827; font-weight: 600; }
.prose em { font-style: italic; }

.prose ul, ul.list-disc { list-style-type: disc; padding-left: 1.5rem; }
.prose ol, ol.list-decimal { list-style-type: decimal; padding-left: 1.5rem; }
.prose li { margin-top: 0.5em; margin-bottom: 0.5em; }
.space-y-1 > * + * { margin-top: 0.25rem; }

.prose blockquote, blockquote.border-l-4 {
  border-left: 4px solid rgba(59, 130, 246, 0.4);
  padding-left: 1rem; font-style: italic; color: #6b7280;
  margin-top: 1.6em; margin-bottom: 1.6em;
}

.prose code { color: #111827; font-weight: 600; font-size: 0.875em; font-family: 'Source Code Pro', Menlo, Monaco, Consolas, monospace; }
.prose code::before, .prose code::after { content: none; }
.prose pre { background-color: #1f2937; color: #e5e7eb; border-radius: 0.375rem; padding: 0.875rem 1.125rem; overflow-x: auto; font-size: 0.875em; line-height: 1.7142857; margin-top: 1.7142857em; margin-bottom: 1.7142857em; }
.prose pre code { background: transparent; color: inherit; font-weight: 400; font-size: inherit; padding: 0; border-radius: 0; }

.prose hr { border-color: #e5e7eb; border-top-width: 1px; margin-top: 3em; margin-bottom: 3em; }

.prose table { border-collapse: collapse; width: 100%; font-size: 0.875em; line-height: 1.7142857; }
.prose thead { border-bottom: 2px solid #d1d5db; }
.prose thead th { padding: 0.625rem 0.75rem; font-weight: 600; text-align: left; color: #111827; }
.prose tbody td { padding: 0.5rem 0.75rem; }
.prose tbody tr { border-bottom: 1px solid #e5e7eb; }

.prose img { margin-top: 2em; margin-bottom: 2em; border-radius: 0.375rem; max-width: 100%; }
.prose sup { font-size: 0.75em; }

a[data-graph-type] {
  display: inline-flex; align-items: center; gap: 4px; padding: 1px 8px;
  border-radius: 9999px; font-size: 11px; font-weight: 500;
  text-decoration: none; vertical-align: middle;
}

.draw-svg-inline { margin: 1.5em 0; max-width: 100%; }
.draw-svg-inline svg { max-width: 100%; height: auto; }

@media print {
  body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  .prose pre { break-inside: avoid; }
  .prose table { break-inside: auto; }
  .prose tr { break-inside: avoid; }
  .prose h2, .prose h3, .prose h4 { break-after: avoid; }
  .prose img { break-inside: avoid; }
  .draw-svg-inline { break-inside: avoid; }
}

.godraw-embed { display: none; }
.figma-embed { display: none; }
`

// wikiPDFDocument wraps rendered wiki HTML in the standalone page chromedp prints.
func wikiPDFDocument(title, renderedHTML string) string {
	return fmt.Sprintf(pdfHTMLTemplate, title, pdfCSS, title, renderedHTML)
}

const pdfHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>%s</style>
</head>
<body>
<h1 style="font-size:2em;font-weight:800;color:#111827;margin-bottom:0.5em;padding-bottom:0.5em;border-bottom:2px solid #e5e7eb;">%s</h1>
<div class="prose">%s</div>
</body>
</html>`

// ── Handlers ────────────────────────────────────────────────────────

// HandleStartWikiPagePDF kicks off async PDF generation and returns a job ID.
// POST /wiki/pages/{pageId}/pdf
func (s *Server) HandleStartWikiPagePDF(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	pg, err := s.fetchWikiPageWithAccess(ctx, userID, pageID)
	if err != nil {
		s.handleWikiPageError(w, err, pageID)
		return
	}

	job := pdfJobs.create(pg.Slug+".pdf", userID, pg.ID)

	// Run generation in the background — fully detached from the HTTP request.
	go s.generatePDF(job.ID, pg.Title, pg.Slug, pg.Content)

	respondJSON(w, http.StatusAccepted, map[string]string{
		"job_id": job.ID,
		"status": string(pdfJobPending),
	})
}

// HandleGetWikiPagePDFJob checks job status or downloads the finished PDF.
// GET /wiki/pages/{pageId}/pdf/{jobId}
func (s *Server) HandleGetWikiPagePDFJob(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(int64)
	jobID := chi.URLParam(r, "jobId")
	job := pdfJobs.get(jobID)
	if job == nil || job.userID != userID || chi.URLParam(r, "pageId") != strconv.FormatInt(job.pageID, 10) {
		respondError(w, http.StatusNotFound, "job not found or expired", "not_found")
		return
	}

	switch job.Status {
	case pdfJobPending:
		respondJSON(w, http.StatusOK, map[string]string{
			"job_id": job.ID,
			"status": string(pdfJobPending),
		})
	case pdfJobFailed:
		respondJSON(w, http.StatusOK, map[string]string{
			"job_id": job.ID,
			"status": string(pdfJobFailed),
			"error":  job.Error,
		})
		pdfJobs.remove(jobID)
	case pdfJobDone:
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.Filename))
		w.Header().Set("Content-Length", strconv.Itoa(len(job.data)))
		w.WriteHeader(http.StatusOK)
		w.Write(job.data) //nolint:errcheck
		// Clean up after successful download.
		pdfJobs.remove(jobID)
	}
}

// HandleWikiPageMarkdown serves the raw markdown content as a downloadable .md file.
// GET /wiki/pages/{pageId}/markdown
func (s *Server) HandleWikiPageMarkdown(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	pg, err := s.fetchWikiPageWithAccess(ctx, userID, pageID)
	if err != nil {
		s.handleWikiPageError(w, err, pageID)
		return
	}

	filename := pg.Slug + ".md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pg.Content)))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(pg.Content)) //nolint:errcheck
}

// ── Background PDF generation ───────────────────────────────────────

// generatePDF runs in a goroutine. It renders the wiki content to HTML,
// inlines draw SVGs, and uses chromedp to print to PDF.
func (s *Server) generatePDF(jobID, title, slug, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Render markdown → HTML.
	renderedHTML := renderWikiHTML(content)

	// Inline draw diagrams as SVGs (fast — HTTP call to local server).
	renderedHTML = s.inlineDrawSVGs(ctx, renderedHTML)

	htmlDoc := wikiPDFDocument(title, renderedHTML)

	pdfBytes, err := renderHTMLToPDF(ctx, htmlDoc)
	if err != nil {
		s.logger.Error("PDF generation failed",
			zap.String("job_id", jobID),
			zap.Error(err),
		)
		pdfJobs.fail(jobID, "PDF generation failed")
		return
	}

	s.logger.Info("PDF generated",
		zap.String("job_id", jobID),
		zap.Int("size_bytes", len(pdfBytes)),
	)
	pdfJobs.complete(jobID, pdfBytes)
}

// ── Draw SVG inlining ───────────────────────────────────────────────

// inlineDrawSVGs replaces godraw-embed divs with inline SVG content
// fetched from the go-draw export endpoint (/draw/{id}/export.svg).
func (s *Server) inlineDrawSVGs(ctx context.Context, htmlContent string) string {
	htmlContent = drawScriptRe.ReplaceAllString(htmlContent, "")

	htmlContent = drawEmbedRe.ReplaceAllStringFunc(htmlContent, func(match string) string {
		m := drawEmbedRe.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		drawID := m[1]

		svgContent, err := s.fetchDrawSVG(ctx, drawID)
		if err != nil {
			s.logger.Warn("Failed to fetch draw SVG for PDF",
				zap.String("draw_id", drawID),
				zap.Error(err),
			)
			return ""
		}
		return fmt.Sprintf(`<div class="draw-svg-inline">%s</div>`, svgContent)
	})

	return htmlContent
}

func (s *Server) fetchDrawSVG(ctx context.Context, drawID string) (string, error) {
	u := fmt.Sprintf("http://localhost:%s/draw/%s/export.svg?bg=transparent", s.config.Port, drawID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch SVG: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SVG export returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read SVG body: %w", err)
	}

	svg := strings.TrimSpace(string(body))
	if strings.HasPrefix(svg, "<?xml") {
		if idx := strings.Index(svg, "?>"); idx != -1 {
			svg = strings.TrimSpace(svg[idx+2:])
		}
	}
	return svg, nil
}

// ── Chromedp PDF rendering ──────────────────────────────────────────

func renderHTMLToPDF(ctx context.Context, htmlContent string) ([]byte, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var pdfBuf []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("get frame tree: %w", err)
			}
			return page.SetDocumentContent(frameTree.Frame.ID, htmlContent).Do(ctx)
		}),
		// Wait for images to load (Cloudinary etc.).
		chromedp.ActionFunc(func(ctx context.Context) error {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				var allLoaded bool
				if err := chromedp.Evaluate(`
					(function() {
						var imgs = document.querySelectorAll('img');
						if (imgs.length === 0) return true;
						for (var i = 0; i < imgs.length; i++) {
							if (!imgs[i].complete) return false;
						}
						return true;
					})()
				`, &allLoaded).Do(ctx); err != nil {
					return nil
				}
				if allLoaded {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		}),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithMarginTop(0.6).
				WithMarginBottom(0.6).
				WithMarginLeft(0.5).
				WithMarginRight(0.5).
				WithPreferCSSPageSize(true).
				Do(ctx)
			return err
		}),
	); err != nil {
		return nil, fmt.Errorf("chromedp run: %w", err)
	}

	return pdfBuf, nil
}

// ── Shared helpers ──────────────────────────────────────────────────

func (s *Server) fetchWikiPageWithAccess(ctx context.Context, userID, pageID int64) (*ent.WikiPage, error) {
	pg, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	hasAccess, err := s.canViewWikiPage(ctx, userID, pg)
	if err != nil {
		return nil, fmt.Errorf("access check: %w", err)
	}
	if !hasAccess {
		return nil, errForbidden
	}
	return pg, nil
}

func (s *Server) handleWikiPageError(w http.ResponseWriter, err error, pageID int64) {
	if ent.IsNotFound(err) {
		respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
		return
	}
	if err == errForbidden {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}
	s.logger.Error("Wiki page error", zap.Int64("page_id", pageID), zap.Error(err))
	respondError(w, http.StatusInternalServerError, "internal error", "internal_error")
}

var errForbidden = fmt.Errorf("forbidden")

// respondJSON is a helper that is already defined elsewhere in the api package,
// but we use json.NewEncoder here for the inline PDF job responses.
func respondPDFJob(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
