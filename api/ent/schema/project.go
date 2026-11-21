package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Project holds the schema definition for the Project entity.
type Project struct {
	ent.Schema
}

// Fields of the Project.
func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("owner_id"),
		field.Int64("team_id").Optional().Nillable(),
		field.String("name").NotEmpty(),
		// Shared projects are open to every active member of their team.
		field.Bool("shared_with_team").Default(false),
		field.String("description").Optional().Nillable(),
		field.String("github_repo_url").Optional().Nillable(),
		field.String("github_owner").Optional().Nillable(),
		field.String("github_repo_name").Optional().Nillable(),
		field.String("github_branch").Default("main"),
		field.Bool("github_sync_enabled").Default(false),
		field.Time("github_last_sync").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the Project.
func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("owned_projects").Unique().Required().Field("owner_id"),
		edge.From("team", Team.Type).Ref("projects").Unique().Field("team_id"),
		edge.To("members", ProjectMember.Type),
		edge.To("tasks", Task.Type),
		edge.To("swim_lanes", SwimLane.Type),
		edge.To("attachments", TaskAttachment.Type),
		edge.To("wiki_pages", WikiPage.Type),
		edge.To("milestones", Milestone.Type),
	}
}

// Indexes of the Project.
func (Project) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_id"),
		index.Fields("team_id"),
	}
}
