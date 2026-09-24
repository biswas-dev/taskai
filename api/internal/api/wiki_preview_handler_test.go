package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWikiPreview_TableRendering(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	tests := []struct {
		name       string
		content    string
		wantTable  bool
		wantMinLen int
	}{
		{
			name: "proper markdown table renders as HTML table",
			content: "## Section\n\n| Header | Value | Inference |\n|---|---|---|\n| `x-powered-by` | `Next.js` | Next.js app |\n| `x-opennext` | `1` | **OpenNext** adapter |\n",
			wantTable:  true,
			wantMinLen: 100,
		},
		{
			name: "collapsed table (all newlines stripped) renders as HTML table",
			content: "## Section| Header | Value | Inference ||---|---|---|| `x-powered-by` | `Next.js` | Next.js app || `x-opennext` | `1` | **OpenNext** adapter |",
			wantTable:  true,
			wantMinLen: 100,
		},
		{
			name: "multiple collapsed tables after headings",
			content: "## Table 1| A | B ||---|---|| 1 | 2 |## Table 2| X | Y | Z ||---|---|---|| a | b | c |",
			wantTable:  true,
			wantMinLen: 50,
		},
		{
			name: "inline code with pipes is not a table",
			content: "Use `a || b` for logical OR.\n",
			wantTable:  false,
			wantMinLen: 10,
		},
		{
			name: "4-column table after heading (collapsed)",
			content: "## Backend| Service | Endpoint | Tech | Host ||---|---|---|---|| API | `api.x.com` | Go | AWS || Chat | `chat.x.com` | Node | Render |",
			wantTable:  true,
			wantMinLen: 50,
		},
		{
			name: "empty content returns empty HTML",
			content:    "",
			wantTable:  false,
			wantMinLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(wikiPreviewRequest{Content: tt.content})
			if err != nil {
				t.Fatalf("failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/wiki/preview", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			ts.HandleWikiPreview(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var resp wikiPreviewResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			hasTable := strings.Contains(resp.HTML, "<table>")
			if hasTable != tt.wantTable {
				t.Errorf("hasTable=%v, want %v\nHTML preview (first 500 chars):\n%s",
					hasTable, tt.wantTable, resp.HTML[:minLen(500, len(resp.HTML))])
			}

			if len(resp.HTML) < tt.wantMinLen {
				t.Errorf("HTML length=%d, want >= %d", len(resp.HTML), tt.wantMinLen)
			}
		})
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRenderWikiHTML_StripsScript(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		mustNotHave []string
		mustHave    []string
	}{
		{"script tag", "<script>alert(1)</script>", []string{"<script"}, nil},
		{"event handler", "<img src=x onerror=alert(1)>", []string{"onerror"}, nil},
		{"javascript link", "[x](javascript:alert(1))", []string{"javascript:"}, nil},
		{"mixed-case javascript link", `<a href="JaVaScRiPt:alert(1)">y</a>`, []string{"JaVaScRiPt", "javascript:"}, nil},
		{"draw embed keeps its data but loses the script", "[draw:abc123]", []string{"<script"}, []string{`class="godraw-embed"`, `data-src="/draw/abc123"`}},
		{"graph link keeps styling hooks", "[[wiki:5|Page]]", nil, []string{`data-graph-type="wiki"`, `data-entity-id="5"`, "style="}},
		{"normal markdown survives", "# Title\n\n![alt](https://res.cloudinary.com/x.png) [in](/app/projects/1)", nil, []string{`<h1 id="title">`, `src="https://res.cloudinary.com/x.png"`, `href="/app/projects/1"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderWikiHTML(tt.input)
			for _, bad := range tt.mustNotHave {
				if strings.Contains(out, bad) {
					t.Errorf("output contains %q: %s", bad, out)
				}
			}
			for _, good := range tt.mustHave {
				if !strings.Contains(out, good) {
					t.Errorf("output missing %q: %s", good, out)
				}
			}
		})
	}
}
