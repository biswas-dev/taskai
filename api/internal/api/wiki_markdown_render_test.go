package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wikiMarkdownCase is one GFM construct that every wiki render path — the
// editor preview, public share links and the PDF export — must agree on.
type wikiMarkdownCase struct {
	name        string
	content     string
	mustHave    []string
	mustNotHave []string
}

var wikiMarkdownCases = []wikiMarkdownCase{
	{
		name:        "table whose first header cell is #",
		content:     "**Facts.**\n\n| # | Fact | Question |\n| --- | --- | --- |\n| T1 | a | b |\n",
		mustHave:    []string{"<table>", "<th>#</th>", "<td>T1</td>"},
		mustNotHave: []string{"<h1 id", "| Fact"},
	},
	{
		name:        "table whose first header cell is empty",
		content:     "| | A | B |\n| --- | --- | --- |\n| x | | 2 |\n\n| | C |\n| --- | --- |\n| y | 3 |\n",
		mustHave:    []string{"<table>", "<th></th>", "<td>y</td>"},
		mustNotHave: []string{"| A", "| C"},
	},
	{
		name:        "table whose first header cell is inline code",
		content:     "| `id` | Value |\n| --- | --- |\n| `a \\|\\| b` | 1 |\n",
		mustHave:    []string{"<table>", "<th><code>id</code></th>", "<code>a || b</code>"},
		mustNotHave: []string{"| Value"},
	},
	{
		name:        "table inside a list item",
		content:     "- **Decision table**:\n\n  | State | Verdict |\n  | --- | --- |\n  | ok | yes |\n",
		mustHave:    []string{"<table>", "<td>ok</td>"},
		mustNotHave: []string{"| State"},
	},
	{
		name: "named footnotes",
		content: "Novelty[^usc102] and Europe[^epc54], again[^usc102], plus[^1].\n\n" +
			"[^usc102]: 35 U.S.C. 102, see [law](https://example.com/102)\n[^epc54]: EPC Article 54, `Art. 54(2)`\n[^1]: Numeric still works\n",
		mustHave: []string{
			`href="#gw-ref-usc102"`, `id="gw-cite-usc102"`, `id="gw-ref-usc102"`, ">[1]</a>",
			`href="#gw-ref-epc54"`, `id="gw-ref-epc54"`, ">[2]</a>",
			`href="#gw-ref-1"`, ">[3]</a>", "Numeric still works",
			`href="https://example.com/102"`, "<code>Art. 54(2)</code>",
		},
		mustNotHave: []string{"[^usc102]", "[^epc54]", "[^1]"},
	},
	{
		name:        "inline code after bold keeps no backticks in the markup",
		content:     "**Keep private:** this page, the `example-agent` repository and the design.\n",
		mustHave:    []string{"<strong>Keep private:</strong>", "<code>example-agent</code>"},
		mustNotHave: []string{"`"},
	},
	{
		name:        "numbered headings and strikethrough",
		content:     "## 1. Confidentiality\n\n### 3.1 The problem\n\n~~old~~ new\n",
		mustHave:    []string{`<h2 id="1-confidentiality">1. Confidentiality</h2>`, `<h3 id="3-1-the-problem">3.1 The problem</h3>`, "<del>old</del>"},
		mustNotHave: []string{"<ol"},
	},
}

func checkWikiHTML(t *testing.T, path string, tc wikiMarkdownCase, html string) {
	t.Helper()
	for _, want := range tc.mustHave {
		if !strings.Contains(html, want) {
			t.Errorf("%s: missing %q in:\n%s", path, want, html)
		}
	}
	for _, bad := range tc.mustNotHave {
		if strings.Contains(html, bad) {
			t.Errorf("%s: unexpected %q in:\n%s", path, bad, html)
		}
	}
}

func TestWikiMarkdown_PreviewEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	for _, tc := range wikiMarkdownCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(wikiPreviewRequest{Content: tc.content})
			req := httptest.NewRequest(http.MethodPost, "/api/wiki/preview", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			ts.HandleWikiPreview(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var resp wikiPreviewResponse
			DecodeJSON(t, rec, &resp)
			checkWikiHTML(t, "preview", tc, resp.HTML)
		})
	}
}

func TestWikiMarkdown_PublicLink(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	owner := ts.CreateTestUser(t, "owner@example.com", "password123")
	projectID := ts.CreateTestProject(t, owner, "Docs")

	for i, tc := range wikiMarkdownCases {
		t.Run(tc.name, func(t *testing.T) {
			token := "public-token-for-markdown-case-" + string(rune('a'+i))
			if _, err := ts.DB.ExecContext(context.Background(),
				`INSERT INTO wiki_pages (project_id, title, slug, created_by, content, public_token) VALUES (?, ?, ?, ?, ?, ?)`,
				projectID, tc.name, "md-case-"+string(rune('a'+i)), owner, tc.content, token); err != nil {
				t.Fatal(err)
			}
			rec, req := MakeRequest(t, http.MethodGet, "/", nil, nil)
			req = withURLParams(req, map[string]string{"token": token})
			ts.HandleGetPublicWikiPage(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var pub PublicWikiPageResponse
			DecodeJSON(t, rec, &pub)
			checkWikiHTML(t, "public link", tc, pub.HTML)
		})
	}
}

func TestWikiMarkdown_PDFDocument(t *testing.T) {
	for _, tc := range wikiMarkdownCases {
		t.Run(tc.name, func(t *testing.T) {
			checkWikiHTML(t, "pdf", tc, wikiPDFDocument("Title", renderWikiHTML(tc.content)))
		})
	}
	t.Run("pdf stylesheet drops typography backticks around inline code", func(t *testing.T) {
		if !strings.Contains(pdfCSS, ".prose code::before, .prose code::after { content: none; }") {
			t.Error("pdfCSS no longer disables the code::before/::after backticks")
		}
	})
}
