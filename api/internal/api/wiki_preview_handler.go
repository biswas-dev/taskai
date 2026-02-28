package api

import (
	"encoding/json"
	"io"
	"net/http"

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
)

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

	html := wiki.RenderContent(req.Content)

	respondJSON(w, http.StatusOK, wikiPreviewResponse{HTML: html})
}
