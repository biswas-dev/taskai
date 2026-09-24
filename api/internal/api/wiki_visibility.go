package api

import (
	"context"
	"fmt"
	"net/http"

	entsql "entgo.io/ent/dialect/sql"

	"taskai/ent"
	"taskai/ent/predicate"
	"taskai/ent/wikipage"
)

// Wiki page visibility values stored in wiki_pages.visibility.
const (
	wikiVisibilityProject    = "project"
	wikiVisibilityRestricted = "restricted"
)

// wikiVisibleSQL returns a SQL condition that keeps only the wiki pages
// (aliased as alias) the user bound to userParam (e.g. "$3") may see, on top
// of project membership which callers check separately:
//   - project-visible pages: everyone in the project
//   - restricted pages: the creator, the project owner and users the page is
//     shared with
func wikiVisibleSQL(alias, userParam string) string {
	return fmt.Sprintf(`(%[1]s.visibility = 'project'
		OR %[1]s.created_by = %[2]s
		OR EXISTS (SELECT 1 FROM projects wv_p WHERE wv_p.id = %[1]s.project_id AND wv_p.owner_id = %[2]s)
		OR EXISTS (SELECT 1 FROM wiki_page_shares wv_s WHERE wv_s.page_id = %[1]s.id AND wv_s.user_id = %[2]s))`,
		alias, userParam)
}

// wikiVisiblePredicate is wikiVisibleSQL for Ent queries on wiki pages. It
// is built with the SQL builder so placeholders match the dialect.
func wikiVisiblePredicate(userID int64) predicate.WikiPage {
	return func(sel *entsql.Selector) {
		b := entsql.Dialect(sel.Dialect())
		projects := entsql.Table("projects").As("wv_p")
		shares := entsql.Table("wiki_page_shares").As("wv_s")
		ownsProject := b.Select(projects.C("id")).From(projects).Where(entsql.And(
			entsql.ColumnsEQ(projects.C("id"), sel.C(wikipage.FieldProjectID)),
			entsql.EQ(projects.C("owner_id"), userID),
		))
		sharedWith := b.Select(shares.C("page_id")).From(shares).Where(entsql.And(
			entsql.ColumnsEQ(shares.C("page_id"), sel.C(wikipage.FieldID)),
			entsql.EQ(shares.C("user_id"), userID),
		))
		sel.Where(entsql.Or(
			entsql.EQ(sel.C(wikipage.FieldVisibility), wikiVisibilityProject),
			entsql.EQ(sel.C(wikipage.FieldCreatedBy), userID),
			entsql.Exists(ownsProject),
			entsql.Exists(sharedWith),
		))
	}
}

// canViewWikiPage reports whether the user may read (and edit) the page: they
// must be a project member and, for restricted pages, the creator, the
// project owner or someone the page is shared with.
func (s *Server) canViewWikiPage(ctx context.Context, userID int64, page *ent.WikiPage) (bool, error) {
	member, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil || !member {
		return false, err
	}
	if page.Visibility != wikiVisibilityRestricted {
		return true, nil
	}
	var visible bool
	err = s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM wiki_pages wp WHERE wp.id = $1 AND `+wikiVisibleSQL("wp", "$2")+`)`,
		page.ID, userID,
	).Scan(&visible)
	return visible, err
}

// canManageWikiSharing reports whether the user may change a page's
// visibility, sharing list or public link: the page creator or the project
// owner.
func (s *Server) canManageWikiSharing(ctx context.Context, userID int64, page *ent.WikiPage) (bool, error) {
	if page.CreatedBy == userID {
		member, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
		return member, err
	}
	var isOwner bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM projects p WHERE p.id = $1 AND p.owner_id = $2
		) OR EXISTS (
			SELECT 1 FROM project_members pm WHERE pm.project_id = $1 AND pm.user_id = $2 AND pm.role = 'owner'
		)`, page.ProjectID, userID,
	).Scan(&isOwner)
	return isOwner, err
}

// hiddenWikiPageIDs returns, for a project, the set of restricted page IDs
// the user may NOT see. List endpoints drop these.
func (s *Server) hiddenWikiPageIDs(ctx context.Context, userID, projectID int64) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT wp.id FROM wiki_pages wp
		 WHERE wp.project_id = $1 AND NOT `+wikiVisibleSQL("wp", "$2"),
		projectID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hidden := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		hidden[id] = true
	}
	return hidden, rows.Err()
}

// visibleWikiParent loads the parent a page is being created under or moved
// to, treating parents the caller cannot see as missing. On failure the
// error response has been written.
func (s *Server) visibleWikiParent(ctx context.Context, w http.ResponseWriter, userID int64, parentID *int64) (*ent.WikiPage, bool) {
	if parentID == nil {
		return nil, true
	}
	parent, err := s.db.Client.WikiPage.Get(ctx, *parentID)
	if err == nil {
		var visible bool
		visible, err = s.canViewWikiPage(ctx, userID, parent)
		if err == nil && visible {
			return parent, true
		}
	}
	if err != nil && !ent.IsNotFound(err) {
		respondError(w, http.StatusInternalServerError, "failed to verify parent page", "internal_error")
		return nil, false
	}
	respondError(w, http.StatusBadRequest, "parent page not found", "invalid_parent")
	return nil, false
}

// inheritWikiRestriction makes a page created under a restricted parent
// restricted too, shared with the same people plus the parent's creator, so
// nesting a page never widens who can read it.
func (s *Server) inheritWikiRestriction(ctx context.Context, page, parent *ent.WikiPage) (*ent.WikiPage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return page, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `UPDATE wiki_pages SET visibility = $1 WHERE id = $2`, wikiVisibilityRestricted, page.ID); err != nil {
		return page, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wiki_page_shares (page_id, user_id, granted_by)
		SELECT CAST($1 AS BIGINT), user_id, granted_by FROM wiki_page_shares WHERE page_id = $2 AND user_id <> $3`,
		page.ID, parent.ID, page.CreatedBy,
	); err != nil {
		return page, err
	}
	if parent.CreatedBy != page.CreatedBy {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wiki_page_shares (page_id, user_id, granted_by) VALUES ($1, $2, $3)
			ON CONFLICT (page_id, user_id) DO NOTHING`,
			page.ID, parent.CreatedBy, page.CreatedBy,
		); err != nil {
			return page, err
		}
	}
	if err := tx.Commit(); err != nil {
		return page, err
	}
	page.Visibility = wikiVisibilityRestricted
	return page, nil
}
