package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	gowiki "github.com/anchoo2kewl/go-wiki"
	"github.com/anchoo2kewl/go-wiki/render"
)

var wiki = gowiki.New(
	gowiki.WithClassConfig(render.ClassConfig{
		ULClass:         "list-disc pl-6 space-y-1",
		OLClass:         "list-decimal pl-6 space-y-1",
		LIClass:         "",
		BlockquoteClass: "border-l-4 border-blue-500/40 pl-4 italic text-gray-300",
	}),
	gowiki.WithDrawBasePath("/draw"),
)

func main() {
	http.HandleFunc("/api/wiki/preview", func(w http.ResponseWriter, r *http.Request) {
		// CORS for browser
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}

		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		var req struct{ Content string `json:"content"` }
		json.Unmarshal(body, &req)

		fmt.Printf("[PREVIEW] len=%d hasNewlines=%v tables_in_content=%v\n",
			len(req.Content), strings.Contains(req.Content, "\n"), strings.Contains(req.Content, "|---"))

		html := wiki.RenderContent(req.Content)
		fmt.Printf("[PREVIEW] has <table>: %v\n", strings.Contains(html, "<table>"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"html": html})
	})

	fmt.Println("Preview server on :8080 with CORS")
	http.ListenAndServe(":8080", nil)
}
