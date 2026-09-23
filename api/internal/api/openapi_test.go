package api

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gopkg.in/yaml.v3"
)

// openAPISpec represents the minimal structure needed to extract paths
type openAPISpec struct {
	Paths map[string]map[string]interface{} `yaml:"paths"`
}

// excludedRoutes are infrastructure routes that don't need spec coverage
var excludedRoutes = map[string]bool{
	"GET /":                 true,
	"GET /healthz":          true,
	"GET /api/health":       true,
	"GET /api/docs":         true,
	"GET /api/openapi":      true,
	"GET /api/openapi.yaml": true,
}

// buildTestRouter creates a chi router with the same routes as main.go.
// This mirrors the route registration in cmd/api/main.go.
func buildTestRouter(server *Server) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"message":"TaskAI API","version":"0.1.0"}`)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"status":"ok"}`)
		})
		r.Get("/openapi", server.HandleOpenAPI)
		r.Get("/docs", server.HandleSwaggerUI)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", server.HandleSignup)
			r.Post("/login", server.HandleLogin)
		})

		r.Group(func(r chi.Router) {
			r.Use(server.JWTAuth)

			r.Get("/me", server.HandleMe)
			r.Patch("/me", server.HandleUpdateProfile)

			r.Get("/projects", server.HandleListProjects)
			r.Post("/projects", server.HandleCreateProject)
			r.Get("/projects/{id}", server.HandleGetProject)
			r.Patch("/projects/{id}", server.HandleUpdateProject)
			r.Delete("/projects/{id}", server.HandleDeleteProject)

			r.Get("/projects/{projectId}/tasks", server.HandleListTasks)
			r.Post("/projects/{projectId}/tasks", server.HandleCreateTask)
			r.Get("/projects/{projectId}/tasks/{taskNumber}", server.HandleGetTaskByNumber)
			r.Patch("/tasks/{id}", server.HandleUpdateTask)
			r.Delete("/tasks/{id}", server.HandleDeleteTask)

			r.Get("/projects/{projectId}/swim-lanes", server.HandleListSwimLanes)
			r.Post("/projects/{projectId}/swim-lanes", server.HandleCreateSwimLane)
			r.Patch("/swim-lanes/{id}", server.HandleUpdateSwimLane)
			r.Delete("/swim-lanes/{id}", server.HandleDeleteSwimLane)

			r.Get("/tasks/{taskId}/comments", server.HandleListTaskComments)
			r.Post("/tasks/{taskId}/comments", server.HandleCreateTaskComment)

			r.Get("/tasks/{taskId}/activity", server.HandleListTaskActivity)

			r.Get("/sprints", server.HandleListSprints)
			r.Post("/sprints", server.HandleCreateSprint)
			r.Patch("/sprints/{id}", server.HandleUpdateSprint)
			r.Delete("/sprints/{id}", server.HandleDeleteSprint)

			r.Get("/tags", server.HandleListTags)
			r.Post("/tags", server.HandleCreateTag)
			r.Patch("/tags/{id}", server.HandleUpdateTag)
			r.Delete("/tags/{id}", server.HandleDeleteTag)

			r.Get("/projects/{id}/members", server.HandleGetProjectMembers)
			r.Post("/projects/{id}/members", server.HandleAddProjectMember)
			r.Patch("/projects/{id}/members/{memberId}", server.HandleUpdateProjectMember)
			r.Delete("/projects/{id}/members/{memberId}", server.HandleRemoveProjectMember)
			r.Get("/projects/{id}/github", server.HandleGetProjectGitHubSettings)
			r.Patch("/projects/{id}/github", server.HandleUpdateProjectGitHubSettings)

			r.Post("/settings/password", server.HandleChangePassword)
			r.Get("/settings/2fa/status", server.Handle2FAStatus)
			r.Post("/settings/2fa/setup", server.Handle2FASetup)
			r.Post("/settings/2fa/enable", server.Handle2FAEnable)
			r.Post("/settings/2fa/disable", server.Handle2FADisable)

			r.Get("/api-keys", server.HandleListAPIKeys)
			r.Post("/api-keys", server.HandleCreateAPIKey)
			r.Delete("/api-keys/{id}", server.HandleDeleteAPIKey)

			r.Get("/openapi.yaml", server.HandleOpenAPIYAML)

			r.Get("/team", server.HandleGetMyTeam)
			r.Get("/team/members", server.HandleGetTeamMembers)
			r.Post("/team/invite", server.HandleInviteTeamMember)
			r.Delete("/team/members/{memberId}", server.HandleRemoveTeamMember)

			r.Get("/teams", server.HandleListMyTeams)
			r.Post("/teams", server.HandleCreateTeam)
			r.Patch("/teams/{teamId}", server.HandleUpdateTeam)
			r.Delete("/teams/{teamId}", server.HandleDeleteTeam)
			r.Get("/teams/{teamId}/members", server.HandleGetTeamMembers)
			r.Post("/teams/{teamId}/members", server.HandleAddTeamMember)
			r.Post("/teams/{teamId}/invite", server.HandleInviteTeamMember)
			r.Delete("/teams/{teamId}/members/{memberId}", server.HandleRemoveTeamMember)
			r.Post("/teams/{teamId}/members/{memberId}/move", server.HandleMoveTeamMember)
			r.Get("/teams/{teamId}/users/search", server.HandleSearchUsers)
			r.Get("/teams/{teamId}/invitations/sent", server.HandleGetTeamSentInvitations)
			r.Post("/teams/{teamId}/leave", server.HandleLeaveTeam)

			r.Get("/team/invitations", server.HandleGetMyInvitations)
			r.Post("/team/invitations/{id}/accept", server.HandleAcceptInvitation)
			r.Post("/team/invitations/{id}/reject", server.HandleRejectInvitation)

			r.Get("/admin/users", server.HandleGetUsers)
			r.Get("/admin/users/{id}/activity", server.HandleGetUserActivity)
			r.Patch("/admin/users/{id}/admin", server.HandleUpdateUserAdmin)
		})
	})

	return r
}

// collectRouterRoutes walks the chi router and returns all METHOD /path pairs
func collectRouterRoutes(router chi.Router) []string {
	var routes []string
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	chi.Walk(router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if !validMethods[method] {
			return nil
		}
		// Normalize: remove trailing slash
		route = strings.TrimRight(route, "/")
		if route == "" {
			route = "/"
		}
		key := method + " " + route
		if !excludedRoutes[key] {
			routes = append(routes, key)
		}
		return nil
	})

	sort.Strings(routes)
	return routes
}

// parseOpenAPIRoutes reads the OpenAPI spec and extracts all METHOD /path pairs
func parseOpenAPIRoutes(specPath string) ([]string, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAPI spec: %w", err)
	}

	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	var routes []string
	for path, methods := range spec.Paths {
		for method := range methods {
			upper := strings.ToUpper(method)
			if upper == "GET" || upper == "POST" || upper == "PUT" || upper == "PATCH" || upper == "DELETE" {
				key := upper + " " + path
				if !excludedRoutes[key] {
					routes = append(routes, key)
				}
			}
		}
	}

	sort.Strings(routes)
	return routes, nil
}

func TestOpenAPIRoutesCoverage(t *testing.T) {
	// Build test server and router
	ts := NewTestServer(t)
	defer ts.Close()

	router := buildTestRouter(ts.Server)

	// Collect routes from the router
	routerRoutes := collectRouterRoutes(router)

	// Parse routes from OpenAPI spec
	specPath := "../../openapi.yaml"
	specRoutes, err := parseOpenAPIRoutes(specPath)
	if err != nil {
		t.Fatalf("Failed to parse OpenAPI spec: %v", err)
	}

	// Build lookup maps
	routerSet := make(map[string]bool)
	for _, r := range routerRoutes {
		routerSet[r] = true
	}

	specSet := make(map[string]bool)
	for _, r := range specRoutes {
		specSet[r] = true
	}

	// Check for routes in router but missing from spec
	var missingFromSpec []string
	for _, r := range routerRoutes {
		if !specSet[r] {
			missingFromSpec = append(missingFromSpec, r)
		}
	}

	// Check for routes in spec but missing from router
	var missingFromRouter []string
	for _, r := range specRoutes {
		if !routerSet[r] {
			missingFromRouter = append(missingFromRouter, r)
		}
	}

	if len(missingFromSpec) > 0 {
		t.Errorf("Routes registered in Go code but missing from OpenAPI spec:\n")
		for _, r := range missingFromSpec {
			t.Errorf("  - %s\n", r)
		}
		t.Error("Add these routes to api/openapi.yaml")
	}

	if len(missingFromRouter) > 0 {
		t.Errorf("Routes in OpenAPI spec but not registered in Go code:\n")
		for _, r := range missingFromRouter {
			t.Errorf("  - %s\n", r)
		}
		t.Error("Remove these routes from api/openapi.yaml or register them in the router")
	}

	if len(missingFromSpec) == 0 && len(missingFromRouter) == 0 {
		t.Logf("All %d routes match between Go code and OpenAPI spec", len(routerRoutes))
	}
}
