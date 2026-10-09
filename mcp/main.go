// Command taskai-mcp serves TaskAI to agents over the Model Context
// Protocol. It is a stateless gateway: each request carries the person's
// TaskAI API key (X-API-Key), and every tool calls the TaskAI REST API as
// them, so permissions and audit trails are the API's own.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gomcp "github.com/anchoo2kewl/go-mcp"
)

var version = "dev"

const instructions = "TaskAI is a project tracker with tasks, swim lanes, milestones, comments and a project wiki. " +
	"Start with list_projects, then list_tasks or search_wiki. Task numbers are per project (get_task); " +
	"task IDs are global (update_task, comments). Send X-Project-ID to scope wiki tools to default projects."

func newHandler(g *Gateway, log *slog.Logger) http.Handler {
	srv := gomcp.New(gomcp.Options{
		Name: "taskai", Version: version, Instructions: instructions,
		Authenticate: g.Authenticate, Logger: log,
	})
	registerTools(srv)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "taskai-mcp", "version": version})
	})
	mux.Handle("/mcp", srv.Handler())
	return mux
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	apiURL := env("TASKAI_API_URL", "https://taskai.cc")
	g := &Gateway{
		APIURL:     apiURL,
		HTTP:       &http.Client{Timeout: 3 * time.Minute},
		CacheTTL:   5 * time.Minute,
		AgentsFile: env("AGENTS_FILE", "/tmp/taskai-mcp-agents.json"),
	}
	server := &http.Server{
		Addr:              ":" + env("PORT", "3000"),
		Handler:           newHandler(g, log),
		ReadHeaderTimeout: 10 * time.Second,
		// A PDF export may poll for up to two minutes.
		WriteTimeout: 3 * time.Minute,
	}
	go func() {
		log.Info("taskai mcp listening", "addr", server.Addr, "api", apiURL, "version", version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
