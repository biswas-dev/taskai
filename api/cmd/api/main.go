package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	godraw "github.com/anchoo2kewl/go-draw"
	godrawstore "github.com/anchoo2kewl/go-draw/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	"taskai/internal/api"
	"taskai/internal/auth"
	"taskai/internal/collab"
	"taskai/internal/config"
	"taskai/internal/db"
	"taskai/internal/yjs"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	logger := config.MustInitLogger(cfg.Env, cfg.LogLevel)
	defer logger.Sync() // Flush any buffered log entries

	logger.Info("Starting TaskAI API",
		zap.String("env", cfg.Env),
		zap.String("port", cfg.Port),
	)

	// Initialize database with auto-migrations
	dbCfg := db.Config{
		Driver:         cfg.DBDriver,
		DBPath:         cfg.DBPath,
		DSN:            cfg.DBDSN,
		MigrationsPath: cfg.MigrationsPath,
		EnableSQLLog:   cfg.EnableSQLLog,
	}

	database, err := db.New(dbCfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	// Create background context with cancel for graceful shutdown
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Initialize auth service
	authService := auth.NewService(cfg.JWTSecret, time.Duration(cfg.JWTExpiryHours)*time.Hour)

	// Initialize collaboration manager for WebSocket connections
	collabManager := collab.NewManager(bgCtx, logger)

	// Initialize Yjs processor client
	yjsClient := yjs.NewClient(cfg.YJSProcessorURL, logger)

	// Initialize package-level logger for response helpers
	api.SetLogger(logger)

	// Create server with logger
	server := api.NewServer(database, cfg, logger)
	server.SetAuthService(authService)
	server.SetCollabManager(collabManager)
	server.SetYjsClient(yjsClient)

	// Setup router
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Compress(5)) // gzip response compression
	r.Use(api.ZapLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Request size limit (1MB)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
			next.ServeHTTP(w, r)
		})
	})

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"TaskAI API","version":"0.1.0"}`)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := database.HealthCheck(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"error","message":"database unavailable"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","database":"connected"}`)
	})

	// pprof profiling endpoints (non-production or explicitly enabled)
	if cfg.Env != "production" || cfg.EnablePprof {
		r.Route("/debug/pprof", func(r chi.Router) {
			r.HandleFunc("/", pprof.Index)
			r.HandleFunc("/cmdline", pprof.Cmdline)
			r.HandleFunc("/profile", pprof.Profile)
			r.HandleFunc("/symbol", pprof.Symbol)
			r.HandleFunc("/trace", pprof.Trace)
			r.Handle("/allocs", pprof.Handler("allocs"))
			r.Handle("/block", pprof.Handler("block"))
			r.Handle("/goroutine", pprof.Handler("goroutine"))
			r.Handle("/heap", pprof.Handler("heap"))
			r.Handle("/mutex", pprof.Handler("mutex"))
			r.Handle("/threadcreate", pprof.Handler("threadcreate"))
		})
		logger.Info("pprof endpoints enabled at /debug/pprof/")
	}

	// go-draw canvas editor — use /data/draw-data for writable storage in container
	drawStore, err := godrawstore.NewFileStore("/data/draw-data")
	if err != nil {
		logger.Fatal("could not initialize go-draw store", zap.Error(err))
	}
	drawHandler, err := godraw.New(godraw.WithBasePath("/draw"), godraw.WithStore(drawStore), godraw.WithUploadDir("/data/draw-uploads"))
	if err != nil {
		logger.Fatal("could not initialize go-draw", zap.Error(err))
	}
	r.Handle("/draw/*", drawHandler.Handler())

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Legacy health endpoint
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok"}`)
		})

		// Version endpoint (public)
		r.Get("/version", server.HandleVersion)

		// OpenAPI specification (public)
		r.Get("/openapi", server.HandleOpenAPI)

		// Swagger UI (public)
		r.Get("/docs", server.HandleSwaggerUI)

		// Auth routes (public) with rate limiting
		r.Route("/auth", func(r chi.Router) {
			// Apply stricter rate limiting to auth endpoints (20 req/min)
			r.Use(api.RateLimitMiddleware(20))
			r.Post("/signup", server.HandleSignup)
			r.Post("/login", server.HandleLogin)
		})

		// Invite validation (public, rate limited)
		r.Group(func(r chi.Router) {
			r.Use(api.RateLimitMiddleware(30))
			r.Get("/invites/validate", server.HandleValidateInvite)
			r.Get("/team/invitations/by-token", server.HandleGetInvitationByToken)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(server.JWTAuth)
			// Apply general rate limiting (100 req/min)
			r.Use(api.RateLimitMiddleware(cfg.RateLimitRequests))

			r.Get("/me", server.HandleMe)
			r.Patch("/me", server.HandleUpdateProfile)

			// Project routes
			r.Get("/projects", server.HandleListProjects)
			r.Post("/projects", server.HandleCreateProject)
			r.Get("/projects/{id}", server.HandleGetProject)
			r.Patch("/projects/{id}", server.HandleUpdateProject)
			r.Delete("/projects/{id}", server.HandleDeleteProject)

			// Task routes
			r.Get("/projects/{projectId}/tasks", server.HandleListTasks)
			r.Post("/projects/{projectId}/tasks", server.HandleCreateTask)
			r.Get("/projects/{projectId}/tasks/{taskNumber}", server.HandleGetTaskByNumber)
			r.Patch("/tasks/{id}", server.HandleUpdateTask)
			r.Delete("/tasks/{id}", server.HandleDeleteTask)

			// Swim lane routes
			r.Get("/projects/{projectId}/swim-lanes", server.HandleListSwimLanes)
			r.Post("/projects/{projectId}/swim-lanes", server.HandleCreateSwimLane)
			r.Patch("/swim-lanes/{id}", server.HandleUpdateSwimLane)
			r.Delete("/swim-lanes/{id}", server.HandleDeleteSwimLane)

			// Wiki routes
			r.Get("/projects/{projectId}/wiki/pages", server.HandleListWikiPages)
			r.Post("/projects/{projectId}/wiki/pages", server.HandleCreateWikiPage)
			r.Get("/wiki/pages/{pageId}", server.HandleGetWikiPage)
			r.Patch("/wiki/pages/{pageId}", server.HandleUpdateWikiPage)
			r.Delete("/wiki/pages/{pageId}", server.HandleDeleteWikiPage)
			r.Get("/wiki/pages/{pageId}/content", server.HandleGetWikiPageContent)
			r.Put("/wiki/pages/{pageId}/content", server.HandleUpdateWikiPageContent)
			r.Post("/wiki/preview", server.HandleWikiPreview)

			// Wiki WebSocket route for real-time collaboration
			r.Get("/wiki/collab", server.HandleWikiWebSocket)

			// Wiki page attachment routes
			r.Get("/wiki/pages/{pageId}/attachments", server.HandleListWikiPageAttachments)
			r.Post("/wiki/pages/{pageId}/attachments", server.HandleCreateWikiPageAttachment)
			r.Delete("/wiki/attachments/{attachmentId}", server.HandleDeleteWikiPageAttachment)

			// Wiki search routes
			r.Post("/wiki/search", server.HandleSearchWiki)
			r.Get("/wiki/autocomplete", server.HandleAutocompletePages)

			// Global search
			r.Post("/search", server.HandleGlobalSearch)

			// Task comment routes
			r.Get("/tasks/{taskId}/comments", server.HandleListTaskComments)
			r.Post("/tasks/{taskId}/comments", server.HandleCreateTaskComment)

			// Sprint routes
			r.Get("/sprints", server.HandleListSprints)
			r.Post("/sprints", server.HandleCreateSprint)
			r.Patch("/sprints/{id}", server.HandleUpdateSprint)
			r.Delete("/sprints/{id}", server.HandleDeleteSprint)

			// Tag routes
			r.Get("/tags", server.HandleListTags)
			r.Post("/tags", server.HandleCreateTag)
			r.Patch("/tags/{id}", server.HandleUpdateTag)
			r.Delete("/tags/{id}", server.HandleDeleteTag)

			// Project settings routes
			r.Get("/projects/{id}/members", server.HandleGetProjectMembers)
			r.Post("/projects/{id}/members", server.HandleAddProjectMember)
			r.Patch("/projects/{id}/members/{memberId}", server.HandleUpdateProjectMember)
			r.Delete("/projects/{id}/members/{memberId}", server.HandleRemoveProjectMember)
			r.Get("/projects/{id}/github", server.HandleGetProjectGitHubSettings)
			r.Patch("/projects/{id}/github", server.HandleUpdateProjectGitHubSettings)

			// Security/Settings routes
			r.Post("/settings/password", server.HandleChangePassword)
			r.Get("/settings/2fa/status", server.Handle2FAStatus)
			r.Post("/settings/2fa/setup", server.Handle2FASetup)
			r.Post("/settings/2fa/enable", server.Handle2FAEnable)
			r.Post("/settings/2fa/disable", server.Handle2FADisable)

			// API key routes
			r.Get("/api-keys", server.HandleListAPIKeys)
			r.Post("/api-keys", server.HandleCreateAPIKey)
			r.Delete("/api-keys/{id}", server.HandleDeleteAPIKey)

			// OpenAPI download with authentication
			r.Get("/openapi.yaml", server.HandleOpenAPIYAML)

			// Team routes
			r.Get("/team", server.HandleGetMyTeam)
			r.Patch("/team", server.HandleUpdateTeam)
			r.Get("/team/members", server.HandleGetTeamMembers)
			r.Post("/team/members", server.HandleAddTeamMember)
			r.Post("/team/invite", server.HandleInviteTeamMember)
			r.Delete("/team/members/{memberId}", server.HandleRemoveTeamMember)
			r.Get("/team/users/search", server.HandleSearchUsers)

			// Team invitations
			r.Get("/team/invitations", server.HandleGetMyInvitations)
			r.Get("/team/invitations/sent", server.HandleGetTeamSentInvitations)
			r.Post("/team/invitations/{id}/accept", server.HandleAcceptInvitation)
			r.Post("/team/invitations/{id}/reject", server.HandleRejectInvitation)
			r.Post("/team/invitations/accept-by-token", server.HandleAcceptInvitationByToken)

			// Cloudinary routes
			r.Get("/settings/cloudinary", server.HandleGetCloudinaryCredential)
			r.Post("/settings/cloudinary", server.HandleSaveCloudinaryCredential)
			r.Delete("/settings/cloudinary", server.HandleDeleteCloudinaryCredential)
			r.Post("/settings/cloudinary/test", server.HandleTestCloudinaryConnection)
			r.Get("/settings/cloudinary/signature", server.HandleGetUploadSignature)

			// Task attachment routes
			r.Get("/tasks/{taskId}/attachments", server.HandleListTaskAttachments)
			r.Post("/tasks/{taskId}/attachments", server.HandleCreateTaskAttachment)
			r.Delete("/tasks/{taskId}/attachments/{attachmentId}", server.HandleDeleteTaskAttachment)

			// Storage usage
			r.Get("/projects/{id}/storage", server.HandleGetStorageUsage)

			// Image library
			r.Get("/images", server.HandleListImages)

			// Asset management
			r.Get("/assets", server.HandleListAssets)

			// Attachment update and delete
			r.Patch("/attachments/{id}", server.HandleUpdateAttachment)
			r.Delete("/attachments/{id}", server.HandleDeleteAttachment)

			// Invite routes
			r.Get("/invites", server.HandleListInvites)
			r.Post("/invites", server.HandleCreateInvite)

			// Admin routes (requires admin role)
			r.Get("/admin/users", server.HandleGetUsers)
			r.Get("/admin/users/{id}/activity", server.HandleGetUserActivity)
			r.Patch("/admin/users/{id}/admin", server.HandleUpdateUserAdmin)
			r.Patch("/admin/users/{id}/invites", server.HandleAdminBoostInvites)

			// Admin email provider routes
			r.Get("/admin/settings/email", server.HandleGetEmailProvider)
			r.Post("/admin/settings/email", server.HandleSaveEmailProvider)
			r.Delete("/admin/settings/email", server.HandleDeleteEmailProvider)
			r.Post("/admin/settings/email/test", server.HandleTestEmailProvider)

			// Admin backup/restore routes
			r.Get("/admin/backup/export", server.HandleExportData)
			r.Post("/admin/backup/import", server.HandleImportData)
		})
	})

	// Start background workers
	server.StartBrevoHealthCheck(bgCtx)
	go server.StartSnapshotWorker(bgCtx)
	go server.StartIndexingWorker(bgCtx)

	// Create HTTP server
	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to listen for server errors
	serverErrors := make(chan error, 1)

	// Start server in goroutine
	go func() {
		logger.Info("Server listening", zap.String("addr", addr))
		serverErrors <- srv.ListenAndServe()
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive shutdown signal or server error
	select {
	case err := <-serverErrors:
		logger.Fatal("Server error", zap.Error(err))
	case sig := <-shutdown:
		logger.Info("Received shutdown signal, starting graceful shutdown", zap.String("signal", sig.String()))

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Gracefully shutdown server
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown failed", zap.Error(err))
			if err := srv.Close(); err != nil {
				logger.Fatal("Failed to close server", zap.Error(err))
			}
		}

		logger.Info("Server stopped gracefully")
	}
}
