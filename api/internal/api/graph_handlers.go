package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent/task"
	"taskai/ent/wikipage"
)

// GraphNode represents a node in the knowledge graph.
type GraphNode struct {
	ID           int64     `json:"id"`
	ProjectID    int64     `json:"project_id"`
	EntityType   string    `json:"entity_type"` // "wiki" or "task"
	EntityID     int64     `json:"entity_id"`
	EntityNumber *int64    `json:"entity_number,omitempty"` // task_number for tasks
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GraphEdge represents a directed edge in the knowledge graph.
type GraphEdge struct {
	ID           int64     `json:"id"`
	SourceNodeID int64     `json:"source_node_id"`
	TargetNodeID int64     `json:"target_node_id"`
	RelationType string    `json:"relation_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// GraphData holds nodes and edges for the graph response.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// graphLinkRef holds a parsed entity reference found in content.
type graphLinkRef struct {
	EntityType string
	EntityID   int64
}

// graphLinkPattern matches [[wiki:123]], [[wiki:123|Label]], [[task:456]], [[task:456|Label]]
var graphLinkPattern = regexp.MustCompile(`\[\[(wiki|task):(\d+)(?:\|[^\]]*)?]]`)

// parseGraphLinks extracts all [[wiki:ID]] and [[task:ID]] references from text content.
func parseGraphLinks(content string) []graphLinkRef {
	matches := graphLinkPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	refs := make([]graphLinkRef, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		entityType := m[1]
		entityID, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			continue
		}
		key := entityType + ":" + strconv.FormatInt(entityID, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, graphLinkRef{EntityType: entityType, EntityID: entityID})
	}
	return refs
}

// upsertGraphNode creates or updates a graph node and returns its ID.
func (s *Server) upsertGraphNode(ctx context.Context, projectID int64, entityType string, entityID int64, entityNumber *int64, title string) (int64, error) {
	var nodeID int64
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`
		INSERT INTO graph_nodes (project_id, entity_type, entity_id, entity_number, title, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(project_id, entity_type, entity_id) DO UPDATE SET
			entity_number = excluded.entity_number,
			title = excluded.title,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`), projectID, entityType, entityID, entityNumber, title).Scan(&nodeID)
	return nodeID, err
}

// syncGraphLinks parses [[wiki:ID]] / [[task:ID]] links from content and updates the
// graph_nodes and graph_edges tables. Designed to be called in a goroutine (best-effort).
func (s *Server) syncGraphLinks(ctx context.Context, projectID int64, sourceType string, sourceID int64, sourceEntityNumber *int64, sourceTitle string, content string) {
	sourceNodeID, err := s.upsertGraphNode(ctx, projectID, sourceType, sourceID, sourceEntityNumber, sourceTitle)
	if err != nil {
		s.logger.Warn("Failed to upsert source graph node",
			zap.String("entity_type", sourceType),
			zap.Int64("entity_id", sourceID),
			zap.Error(err),
		)
		return
	}

	refs := parseGraphLinks(content)

	// Collect target node IDs for the current content.
	targetNodeIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		var targetTitle string
		var targetProjectID int64
		var targetEntityNumber *int64

		switch ref.EntityType {
		case "wiki":
			page, err := s.db.Client.WikiPage.Query().
				Where(wikipage.ID(ref.EntityID)).
				Only(ctx)
			if err != nil {
				continue
			}
			targetTitle = page.Title
			targetProjectID = page.ProjectID

		case "task":
			t, err := s.db.Client.Task.Query().
				Where(task.ID(ref.EntityID)).
				Only(ctx)
			if err != nil {
				continue
			}
			targetTitle = t.Title
			targetProjectID = t.ProjectID
			if t.TaskNumber != nil {
				n := int64(*t.TaskNumber)
				targetEntityNumber = &n
			}

		default:
			continue
		}

		targetNodeID, err := s.upsertGraphNode(ctx, targetProjectID, ref.EntityType, ref.EntityID, targetEntityNumber, targetTitle)
		if err != nil {
			s.logger.Warn("Failed to upsert target graph node",
				zap.String("entity_type", ref.EntityType),
				zap.Int64("entity_id", ref.EntityID),
				zap.Error(err),
			)
			continue
		}
		targetNodeIDs = append(targetNodeIDs, targetNodeID)
	}

	// Delete all outgoing edges from source, then re-insert current ones.
	if _, err = s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM graph_edges WHERE source_node_id = ?`,
	), sourceNodeID); err != nil {
		s.logger.Warn("Failed to delete stale graph edges",
			zap.Int64("source_node_id", sourceNodeID),
			zap.Error(err),
		)
	}

	for _, targetNodeID := range targetNodeIDs {
		if _, err = s.db.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO graph_edges (source_node_id, target_node_id, relation_type)
			VALUES (?, ?, 'reference')
			ON CONFLICT(source_node_id, target_node_id) DO NOTHING
		`), sourceNodeID, targetNodeID); err != nil {
			s.logger.Warn("Failed to insert graph edge",
				zap.Int64("source", sourceNodeID),
				zap.Int64("target", targetNodeID),
				zap.Error(err),
			)
		}
	}
}

// HandleGetProjectGraph returns all graph nodes and edges for a project.
func (s *Server) HandleGetProjectGraph(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch nodes for this project (limit 200 for performance).
	nodeRows, err := s.db.QueryContext(ctx, s.db.Rebind(`
		SELECT id, project_id, entity_type, entity_id, entity_number, title, created_at, updated_at
		FROM graph_nodes
		WHERE project_id = ?
		  AND (entity_type <> 'wiki' OR entity_id IN (
		    SELECT wp.id FROM wiki_pages wp WHERE wp.project_id = ? AND `+wikiVisibleSQL("wp", "?")+`
		  ))
		ORDER BY created_at DESC
		LIMIT 200
	`), projectID, projectID, userID, userID, userID)
	if err != nil {
		s.logger.Error("Failed to fetch graph nodes", zap.Int64("project_id", projectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch graph", "internal_error")
		return
	}
	defer nodeRows.Close()

	nodes := make([]GraphNode, 0)
	nodeIDs := make([]int64, 0)
	for nodeRows.Next() {
		var n GraphNode
		if err := nodeRows.Scan(&n.ID, &n.ProjectID, &n.EntityType, &n.EntityID, &n.EntityNumber, &n.Title, &n.CreatedAt, &n.UpdatedAt); err != nil {
			s.logger.Warn("Failed to scan graph node", zap.Error(err))
			continue
		}
		nodes = append(nodes, n)
		nodeIDs = append(nodeIDs, n.ID)
	}
	if err := nodeRows.Err(); err != nil {
		s.logger.Error("Error iterating graph nodes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch graph", "internal_error")
		return
	}

	if len(nodeIDs) == 0 {
		respondJSON(w, http.StatusOK, GraphData{Nodes: nodes, Edges: []GraphEdge{}})
		return
	}

	// Build placeholder list for IN clause.
	placeholders := make([]string, len(nodeIDs))
	for i := range nodeIDs {
		placeholders[i] = "?"
	}
	ph := strings.Join(placeholders, ",")

	// Args: nodeIDs twice (for source and target IN clauses).
	args := make([]interface{}, len(nodeIDs)*2)
	for i, id := range nodeIDs {
		args[i] = id
		args[len(nodeIDs)+i] = id
	}

	edgeQuery := fmt.Sprintf(`
		SELECT id, source_node_id, target_node_id, relation_type, created_at
		FROM graph_edges
		WHERE source_node_id IN (%s)
		  AND target_node_id IN (%s)
	`, ph, ph)

	edgeRows, err := s.db.QueryContext(ctx, s.db.Rebind(edgeQuery), args...)
	if err != nil {
		s.logger.Error("Failed to fetch graph edges", zap.Int64("project_id", projectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch graph", "internal_error")
		return
	}
	defer edgeRows.Close()

	edges := make([]GraphEdge, 0)
	for edgeRows.Next() {
		var e GraphEdge
		if err := edgeRows.Scan(&e.ID, &e.SourceNodeID, &e.TargetNodeID, &e.RelationType, &e.CreatedAt); err != nil {
			s.logger.Warn("Failed to scan graph edge", zap.Error(err))
			continue
		}
		edges = append(edges, e)
	}
	if err := edgeRows.Err(); err != nil {
		s.logger.Error("Error iterating graph edges", zap.Error(err))
	}

	respondJSON(w, http.StatusOK, GraphData{Nodes: nodes, Edges: edges})
}
