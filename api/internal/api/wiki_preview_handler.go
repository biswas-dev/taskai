package api

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"

	gowiki "github.com/anchoo2kewl/go-wiki"
	"github.com/anchoo2kewl/go-wiki/render"
)

// wiki is the shared go-wiki instance for server-side markdown rendering.
var wiki = gowiki.New(
	gowiki.WithClassConfig(render.ClassConfig{
		ULClass:         "list-disc pl-6 space-y-1",
		OLClass:         "list-decimal pl-6 space-y-1",
		LIClass:         "",
		BlockquoteClass: "border-l-4 border-blue-500/40 pl-4 italic text-gray-300",
	}),
	gowiki.WithDrawBasePath("/draw"),
)

// drawEditSrcRe matches data-src attributes ending in /edit inside godraw-embed divs.
var drawEditSrcRe = regexp.MustCompile(`(data-src="[^"]+)/edit"`)

// graphLinkPreRe matches [[wiki:ID|Label]] and [[task:ID|Label]] for preview rendering.
var graphLinkPreRe = regexp.MustCompile(`\[\[(wiki|task):(\d+)(?:\|([^\]]*))?\]\]`)

// figmaShortcodeRe matches [figma:URL] or [figma:URL:s/m/l] shortcodes.
var figmaShortcodeRe = regexp.MustCompile(`\[figma:([^\]]+?)(?::([sml]))?\]`)

// preprocessFigmaShortcodes replaces [figma:URL:size] shortcodes with placeholder divs
// before markdown rendering, so the URL is not mangled by the auto-linker.
func preprocessFigmaShortcodes(content string) string {
	return figmaShortcodeRe.ReplaceAllStringFunc(content, func(match string) string {
		m := figmaShortcodeRe.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		u := html.EscapeString(m[1])
		size := "m"
		if len(m) >= 3 && m[2] != "" {
			size = m[2]
		}
		return fmt.Sprintf(`<div class="figma-embed" data-url="%s" data-size="%s"></div>`, u, size)
	})
}

// preprocessGraphLinksForPreview converts [[wiki:ID|Label]] / [[task:ID|Label]] syntax
// into styled inline HTML elements before markdown rendering.
func preprocessGraphLinksForPreview(content string) string {
	return graphLinkPreRe.ReplaceAllStringFunc(content, func(match string) string {
		m := graphLinkPreRe.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		entityType, entityID, label := m[1], m[2], m[3]
		if label == "" {
			if entityType == "wiki" {
				label = "Wiki #" + entityID
			} else {
				label = "Task #" + entityID
			}
		}
		icon := "📄"
		baseColor := "#3b82f6"
		bgColor := "rgba(59,130,246,0.15)"
		borderColor := "rgba(59,130,246,0.3)"
		if entityType == "task" {
			icon = "✅"
			baseColor = "#f97316"
			bgColor = "rgba(249,115,22,0.15)"
			borderColor = "rgba(249,115,22,0.3)"
		}
		style := fmt.Sprintf(
			"display:inline-flex;align-items:center;gap:4px;padding:1px 8px;border-radius:9999px;font-size:11px;font-weight:500;background:%s;color:%s;border:1px solid %s;text-decoration:none;cursor:pointer;vertical-align:middle",
			bgColor, baseColor, borderColor,
		)
		return fmt.Sprintf(
			`<a href="#" data-graph-type="%s" data-entity-id="%s" style="%s">%s %s</a>`,
			entityType, entityID, style, icon, label,
		)
	})
}

// stripDrawEditMode removes /edit from go-draw embed URLs so that
// rendered content always shows a read-only canvas viewer.
func stripDrawEditMode(html string) string {
	return drawEditSrcRe.ReplaceAllString(html, `$1"`)
}

// wikiPreviewRequest is the JSON body for the preview endpoint.
type wikiPreviewRequest struct {
	Content string `json:"content"`
}

// wikiPreviewResponse is the JSON response from the preview endpoint.
type wikiPreviewResponse struct {
	HTML string `json:"html"`
}

// HandleWikiPreview renders markdown content to HTML using the go-wiki renderer.
func (s *Server) HandleWikiPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed", "method_not_allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read request body", "invalid_input")
		return
	}
	defer r.Body.Close()

	var req wikiPreviewRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body", "invalid_input")
		return
	}

	html := stripDrawEditMode(wiki.RenderContent(preprocessFigmaShortcodes(preprocessGraphLinksForPreview(req.Content))))

	respondJSON(w, http.StatusOK, wikiPreviewResponse{HTML: html})
}
