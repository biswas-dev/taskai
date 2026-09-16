package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"taskai/ent"
	"taskai/ent/wikipage"
)

// wikiMaxDepth is the maximum nesting depth for wiki pages (root page = depth 1).
const wikiMaxDepth = 6

// OptionalInt64 distinguishes an omitted JSON field from an explicit null,
// so PATCH clients can clear a value ("parent_id": null) without ambiguity.
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// MarshalJSON implements json.Marshaler. Callers should tag the field with
// omitzero so an unset value is omitted rather than sent as null.
func (o OptionalInt64) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.Value)
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *OptionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// wikiHierarchyError is a validation failure that maps to a 4xx response.
type wikiHierarchyError struct {
	status  int
	message string
	code    string
}

func (e *wikiHierarchyError) Error() string { return e.message }

// wikiTree is a lightweight in-memory view of a project's page hierarchy.
type wikiTree struct {
	parent   map[int64]*int64
	children map[int64][]int64
}

// loadWikiTree fetches id/parent_id for every page in the project.
func (s *Server) loadWikiTree(ctx context.Context, projectID int64) (*wikiTree, error) {
	pages, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ProjectID(projectID)).
		Select(wikipage.FieldID, wikipage.FieldParentID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load wiki tree: %w", err)
	}
	t := &wikiTree{
		parent:   make(map[int64]*int64, len(pages)),
		children: make(map[int64][]int64),
	}
	for _, p := range pages {
		t.parent[p.ID] = p.ParentID
		if p.ParentID != nil {
			t.children[*p.ParentID] = append(t.children[*p.ParentID], p.ID)
		}
	}
	return t, nil
}

// depth returns the 1-based depth of a page (root = 1). A missing page returns 0.
// Cycles (which should never exist) are bounded to avoid infinite loops.
func (t *wikiTree) depth(id int64) int {
	d := 0
	cur := &id
	for i := 0; cur != nil && i <= len(t.parent)+1; i++ {
		if _, ok := t.parent[*cur]; !ok {
			break
		}
		d++
		cur = t.parent[*cur]
	}
	return d
}

// height returns the number of levels in the subtree rooted at id (leaf = 1).
func (t *wikiTree) height(id int64) int {
	h := 1
	for _, c := range t.children[id] {
		if ch := t.height(c) + 1; ch > h {
			h = ch
		}
	}
	return h
}

// isDescendant reports whether candidate is id itself or somewhere beneath it.
func (t *wikiTree) isDescendant(id, candidate int64) bool {
	if id == candidate {
		return true
	}
	for _, c := range t.children[id] {
		if t.isDescendant(c, candidate) {
			return true
		}
	}
	return false
}

// validateWikiParent checks that parentID (if non-nil) is a valid parent for a page
// in projectID. pageID is nil when creating a new page. It enforces:
//   - the parent exists and belongs to the same project
//   - the page is not moved under itself or one of its descendants
//   - the resulting deepest page stays within wikiMaxDepth levels
func (s *Server) validateWikiParent(ctx context.Context, projectID int64, pageID, parentID *int64) error {
	tree, err := s.loadWikiTree(ctx, projectID)
	if err != nil {
		return err
	}

	// Height of the subtree being placed (a new page is a single leaf).
	subtreeHeight := 1
	if pageID != nil {
		subtreeHeight = tree.height(*pageID)
	}

	if parentID == nil {
		// Moving to root can only reduce depth; nothing more to check.
		return nil
	}

	if _, ok := tree.parent[*parentID]; !ok {
		return &wikiHierarchyError{400, "parent page not found in this project", "invalid_parent"}
	}
	if pageID != nil && tree.isDescendant(*pageID, *parentID) {
		return &wikiHierarchyError{400, "a page cannot be nested under itself or its own descendants", "invalid_parent"}
	}
	if tree.depth(*parentID)+subtreeHeight > wikiMaxDepth {
		return &wikiHierarchyError{
			400,
			fmt.Sprintf("wiki pages can be nested at most %d levels deep", wikiMaxDepth),
			"max_depth_exceeded",
		}
	}
	return nil
}

// nextWikiSiblingPosition returns one past the highest position among the
// pages sharing the given parent (nil = root) in the project.
func (s *Server) nextWikiSiblingPosition(ctx context.Context, projectID int64, parentID *int64) (int, error) {
	q := s.db.Client.WikiPage.Query().Where(wikipage.ProjectID(projectID))
	if parentID == nil {
		q = q.Where(wikipage.ParentIDIsNil())
	} else {
		q = q.Where(wikipage.ParentID(*parentID))
	}
	last, err := q.Order(ent.Desc(wikipage.FieldPosition)).Select(wikipage.FieldPosition).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return last.Position + 1, nil
}

// respondWikiHierarchyError writes the appropriate error response for a
// failure returned by validateWikiParent.
func respondWikiHierarchyError(w http.ResponseWriter, err error) {
	var he *wikiHierarchyError
	if errors.As(err, &he) {
		respondError(w, he.status, he.message, he.code)
		return
	}
	respondError(w, http.StatusInternalServerError, "failed to validate page hierarchy", "internal_error")
}
