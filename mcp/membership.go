package main

// Wiki sharing and project membership.
//
// TaskAI's access model is team → project → wiki page:
//   - every project belongs to one team, and only that team's active members
//     can be added to the project;
//   - a restricted wiki page can only be shared with members of its project.
//
// These tools resolve emails to user IDs against those lists and turn the
// API's plain errors into messages that say what to do next.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	gomcp "github.com/anchoo2kewl/go-mcp"
)

type member struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Name   *string `json:"name"`
	Role   string  `json:"role"`
}

type teamMember struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
}

type sharing struct {
	Visibility  string `json:"visibility"`
	CanManage   bool   `json:"can_manage"`
	CreatedBy   int64  `json:"created_by"`
	PublicToken string `json:"public_token,omitempty"`
	SharedWith  []struct {
		UserID   int64  `json:"user_id"`
		Email    string `json:"email"`
		UserName string `json:"user_name,omitempty"`
	} `json:"shared_with"`
}

// minimal hides the public link token, reporting only whether one is on.
func (s sharing) minimal() map[string]any {
	with := make([]map[string]any, 0, len(s.SharedWith))
	for _, u := range s.SharedWith {
		var name any
		if u.UserName != "" {
			name = u.UserName
		}
		with = append(with, map[string]any{"user_id": u.UserID, "email": u.Email, "name": name})
	}
	return map[string]any{"visibility": s.Visibility, "can_manage": s.CanManage, "created_by": s.CreatedBy, "shared_with": with, "public_link": s.PublicToken != ""}
}

func normalEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// resolveMembers maps requested user IDs and emails to project members,
// reporting anything that is not a member rather than dropping it.
func resolveMembers(members []member, userIDs []int64, emails []string) (ids []int64, unknownIDs []int64, unknownEmails []string) {
	byID := map[int64]bool{}
	byEmail := map[string]int64{}
	for _, m := range members {
		byID[m.UserID] = true
		byEmail[normalEmail(m.Email)] = m.UserID
	}
	seen := map[int64]bool{}
	add := func(id int64) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range userIDs {
		if byID[id] {
			add(id)
		} else {
			unknownIDs = append(unknownIDs, id)
		}
	}
	for _, e := range emails {
		if id, ok := byEmail[normalEmail(e)]; ok {
			add(id)
		} else {
			unknownEmails = append(unknownEmails, strings.TrimSpace(e))
		}
	}
	return ids, unknownIDs, unknownEmails
}

// nextShareList is the full list to send; the API replaces the whole list.
func nextShareList(current, requested []int64, mode string) []int64 {
	out := []int64{}
	seen := map[int64]bool{}
	add := func(list []int64) {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	if mode != "replace" {
		add(current)
	}
	add(requested)
	return out
}

type projectMembersIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"Return full details including membership id and granted_at (default: false)"`
}

type addProjectMemberIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	Email     string `json:"email,omitempty" jsonschema:"Email of the person to add (give this or user_id)"`
	UserID    int64  `json:"user_id,omitempty" jsonschema:"User ID of the person to add (give this or email)"`
	Role      string `json:"role,omitempty" jsonschema:"viewer, member (default), editor or owner"`
	AddToTeam bool   `json:"add_to_team,omitempty" jsonschema:"If they are not in the project's team, add them to it first (default: false). With an email that has no TaskAI account, this sends them a signup invitation instead."`
}

type wikiSharingIn struct {
	PageID  string `json:"page_id" jsonschema:"Wiki page ID"`
	Verbose bool   `json:"verbose,omitempty" jsonschema:"Return the raw API response, including the public link token if you can manage the page (default: false)"`
}

type updateSharingIn struct {
	PageID     string   `json:"page_id" jsonschema:"Wiki page ID"`
	Visibility string   `json:"visibility" jsonschema:"'restricted' = private to the people it is shared with; 'project' = every project member"`
	Emails     []string `json:"emails,omitempty" jsonschema:"Emails of project members to share with (max 200)"`
	UserIDs    []int64  `json:"user_ids,omitempty" jsonschema:"User IDs of project members to share with (max 200)"`
	Mode       string   `json:"mode,omitempty" jsonschema:"'add' (default) adds to the current share list; 'replace' sets the whole list"`
	Verbose    bool     `json:"verbose,omitempty" jsonschema:"Return the raw API response (default: false)"`
}

func (c *Client) projectMembers(ctx context.Context, projectID string) ([]member, error) {
	var out []member
	err := c.call(ctx, http.MethodGet, "/api/projects/"+esc(projectID)+"/members", nil, &out)
	return out, err
}

func (c *Client) teamMembers(ctx context.Context, teamID int64) ([]teamMember, error) {
	var out []teamMember
	err := c.call(ctx, http.MethodGet, fmt.Sprintf("/api/teams/%d/members", teamID), nil, &out)
	return out, err
}

// updateWikiSharing sets a page's visibility and share list. Mode "add" adds
// the requested people to whoever already has access; "replace" makes them
// the whole list. Naming no one keeps the current list, unless replacing.
func updateWikiSharing(ctx context.Context, c *Client, in updateSharingIn) (sharing, any, error) {
	var current sharing
	if err := c.call(ctx, http.MethodGet, "/api/wiki/pages/"+esc(in.PageID)+"/sharing", nil, &current); err != nil {
		return sharing{}, nil, err
	}
	if !current.CanManage {
		return sharing{}, nil, errors.New("Only the page's creator or the project owner can change its sharing; you can view it but not change it.")
	}
	currentIDs := make([]int64, 0, len(current.SharedWith))
	for _, u := range current.SharedWith {
		currentIDs = append(currentIDs, u.UserID)
	}
	list := currentIDs
	if len(in.UserIDs) == 0 && len(in.Emails) == 0 {
		if in.Mode == "replace" {
			list = []int64{}
		}
	} else {
		var page struct {
			ProjectID int64 `json:"project_id"`
		}
		if err := c.call(ctx, http.MethodGet, "/api/wiki/pages/"+esc(in.PageID), nil, &page); err != nil {
			return sharing{}, nil, err
		}
		projectID := strconv.FormatInt(page.ProjectID, 10)
		members, err := c.projectMembers(ctx, projectID)
		if err != nil {
			return sharing{}, nil, err
		}
		ids, unknownIDs, unknownEmails := resolveMembers(members, in.UserIDs, in.Emails)
		if len(unknownIDs)+len(unknownEmails) > 0 {
			who := append([]string{}, unknownEmails...)
			for _, id := range unknownIDs {
				who = append(who, fmt.Sprintf("user %d", id))
			}
			return sharing{}, nil, fmt.Errorf("Not members of project %s: %s. A wiki page can only be shared with members of its project. "+
				"Check spelling with list_project_members, or add them with add_project_member first. Nothing was changed.", projectID, strings.Join(who, ", "))
		}
		list = nextShareList(currentIDs, ids, in.Mode)
	}
	var raw any
	if err := c.call(ctx, http.MethodPut, "/api/wiki/pages/"+esc(in.PageID)+"/sharing", map[string]any{"visibility": in.Visibility, "user_ids": list}, &raw); err != nil {
		return sharing{}, nil, err
	}
	var updated sharing
	_ = remarshal(raw, &updated)
	return updated, raw, nil
}

// addProjectMember adds a user to a project directly (no acceptance step).
// They must be an active member of the project's team; with AddToTeam they
// are added to the team first. Someone without a TaskAI account can only be
// invited to the team by email and must sign up before joining the project.
func addProjectMember(ctx context.Context, c *Client, in addProjectMemberIn) (map[string]any, error) {
	email := strings.TrimSpace(in.Email)
	if (email == "") == (in.UserID == 0) {
		return nil, errors.New("Pass exactly one of email or user_id.")
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	if err := oneOf("role", role, "viewer", "member", "editor", "owner"); err != nil {
		return nil, err
	}
	matches := func(e string, id int64) bool {
		if email != "" {
			return normalEmail(e) == normalEmail(email)
		}
		return id == in.UserID
	}
	summary := func(m member) map[string]any {
		var name any
		if m.Name != nil {
			name = *m.Name
		}
		return map[string]any{"user_id": m.UserID, "email": m.Email, "name": name, "role": m.Role}
	}
	findInProject := func() (*member, error) {
		members, err := c.projectMembers(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		for i := range members {
			if matches(members[i].Email, members[i].UserID) {
				return &members[i], nil
			}
		}
		return nil, nil
	}

	existing, err := findInProject()
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return map[string]any{"status": "already_member", "member": summary(*existing),
			"message": fmt.Sprintf("Already a member of project %s with role '%s'. Role was not changed.", in.ProjectID, existing.Role)}, nil
	}

	var project struct {
		TeamID *int64 `json:"team_id"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/projects/"+esc(in.ProjectID), nil, &project); err != nil {
		return nil, err
	}
	addedToTeam := false
	projectEmail := email
	if project.TeamID != nil {
		teamID := *project.TeamID
		findInTeam := func() (*teamMember, error) {
			list, err := c.teamMembers(ctx, teamID)
			if err != nil {
				return nil, err
			}
			for i := range list {
				if matches(list[i].Email, list[i].UserID) {
					return &list[i], nil
				}
			}
			return nil, nil
		}
		tm, err := findInTeam()
		if err != nil {
			return nil, err
		}
		if tm == nil {
			who := email
			if who == "" {
				who = fmt.Sprintf("user %d", in.UserID)
			}
			if !in.AddToTeam {
				return nil, fmt.Errorf("%s is not in team %d, which project %s belongs to. Only team members can join a project. "+
					"Call again with add_to_team=true to add them to the team first (this also gives them access to join the team's other projects). Nothing was changed.", who, teamID, in.ProjectID)
			}
			if email != "" {
				var invitation struct {
					Status string `json:"status"`
				}
				if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/api/teams/%d/invite", teamID), map[string]string{"email": email}, &invitation); err != nil {
					return nil, err
				}
				if invitation.Status != "accepted" {
					return map[string]any{"status": "team_invitation_pending",
						"message": fmt.Sprintf("%s has no TaskAI account, so they were emailed an invitation to sign up and join team %d. "+
							"They are NOT in project %s yet: once they have signed up, call add_project_member again.", email, teamID, in.ProjectID)}, nil
				}
			} else if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/api/teams/%d/members", teamID), map[string]int64{"user_id": in.UserID}, nil); err != nil {
				return nil, err
			}
			addedToTeam = true
			if tm, err = findInTeam(); err != nil {
				return nil, err
			}
			if tm == nil {
				return nil, fmt.Errorf("added %s to team %d but could not find them in its member list", who, teamID)
			}
		}
		projectEmail = tm.Email
	} else if projectEmail == "" {
		return nil, fmt.Errorf("Project %s has no team, so the user can only be added by email.", in.ProjectID)
	}

	if err := c.call(ctx, http.MethodPost, "/api/projects/"+esc(in.ProjectID)+"/members", map[string]string{"email": projectEmail, "role": role}, nil); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Added to project %s as '%s'", in.ProjectID, role)
	if addedToTeam {
		msg += fmt.Sprintf(" (and added to team %d first)", *project.TeamID)
	}
	out := map[string]any{"status": "added", "added_to_team": addedToTeam, "message": msg + ". Restricted wiki pages in the project can now be shared with them."}
	if added, err := findInProject(); err == nil && added != nil {
		out["member"] = summary(*added)
	}
	return out, nil
}

func registerMembershipTools(srv *gomcp.Server) {
	gomcp.Register(srv, gomcp.Tool{Name: "list_project_members", Description: "List the members of a project: user_id, email, name and role. Use it to find someone's user_id or exact email before sharing a wiki page with them (pages can only be shared with project members) or to check who can be added. Pass user_id, not the membership id, to other tools."},
		func(ctx context.Context, in projectMembersIn) (any, error) {
			_, c := sess(ctx)
			if in.Verbose {
				return c.get(ctx, "/api/projects/"+esc(in.ProjectID)+"/members")
			}
			members, err := c.projectMembers(ctx, in.ProjectID)
			out := make([]map[string]any, 0, len(members))
			for _, m := range members {
				var name any
				if m.Name != nil {
					name = *m.Name
				}
				out = append(out, map[string]any{"user_id": m.UserID, "email": m.Email, "name": name, "role": m.Role})
			}
			return out, err
		})

	gomcp.Register(srv, gomcp.Tool{Name: "add_project_member", Write: true, Idempotent: true, Description: "Add a person to a project so they can see its tasks and wiki, and so restricted wiki pages can be shared with them. They are added immediately (no invitation to accept). Only the project owner or a project admin can do this. Every project belongs to one team and only that team's members can join it: if the person is not in the team, the call fails unless add_to_team=true, which adds them to the team first. Someone with no TaskAI account can only be invited to the team by email; they must sign up before they can be added. Existing members are left unchanged."},
		func(ctx context.Context, in addProjectMemberIn) (map[string]any, error) {
			_, c := sess(ctx)
			return addProjectMember(ctx, c, in)
		})

	gomcp.Register(srv, gomcp.Tool{Name: "get_wiki_sharing", Description: "Get who can see a wiki page. visibility 'project' means every project member; 'restricted' means only the page creator, the project owner and the people in shared_with (the creator and owner always have access and are not listed). can_manage says whether you may change it. public_link is true when a read-only public link is on."},
		func(ctx context.Context, in wikiSharingIn) (any, error) {
			_, c := sess(ctx)
			var raw any
			if err := c.call(ctx, http.MethodGet, "/api/wiki/pages/"+esc(in.PageID)+"/sharing", nil, &raw); err != nil {
				return nil, err
			}
			if in.Verbose {
				return raw, nil
			}
			var s sharing
			if err := remarshal(raw, &s); err != nil {
				return nil, err
			}
			return s.minimal(), nil
		})

	gomcp.Register(srv, gomcp.Tool{Name: "update_wiki_sharing", Write: true, Destructive: true, Idempotent: true, Description: "Make a wiki page private or project-wide, and choose who it is shared with. visibility 'restricted' limits the page to its creator, the project owner and the people you share it with; 'project' opens it to every project member. Name people by email or user_id; they must be members of the page's project (see list_project_members, add_project_member), otherwise nothing is changed. mode 'add' (default) keeps everyone already shared and adds these people; 'replace' makes these people the whole list (replace with no one: only the creator and project owner keep access). Omitting user_ids and emails keeps the current list. Only the page creator or the project owner can change sharing. Returns the resulting sharing."},
		func(ctx context.Context, in updateSharingIn) (any, error) {
			if err := oneOf("visibility", in.Visibility, "project", "restricted"); err != nil || in.Visibility == "" {
				return nil, errors.New("visibility must be one of project, restricted")
			}
			if err := oneOf("mode", in.Mode, "add", "replace"); err != nil {
				return nil, err
			}
			if len(in.Emails) > 200 || len(in.UserIDs) > 200 {
				return nil, errors.New("share with at most 200 people at a time")
			}
			_, c := sess(ctx)
			s, raw, err := updateWikiSharing(ctx, c, in)
			if err != nil {
				return nil, err
			}
			if in.Verbose {
				return raw, nil
			}
			return s.minimal(), nil
		})
}
