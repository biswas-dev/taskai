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
