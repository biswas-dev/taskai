package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("email").Unique().NotEmpty(),
		field.String("password_hash").NotEmpty().Sensitive(),
		field.String("name").Optional().Nillable(),
		field.String("first_name").Optional().Nillable(),
		field.String("last_name").Optional().Nillable(),
		field.Bool("is_admin").Default(false),
		field.String("totp_secret").Optional().Nillable().Sensitive(),
		field.Bool("totp_enabled").Default(false),
		field.String("backup_codes").Optional().Nillable().Sensitive(),
		field.Time("password_changed_at").Optional().Nillable(),
		field.Int("invite_count").Default(3),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("owned_projects", Project.Type),
		edge.To("owned_teams", Team.Type),
		edge.To("team_memberships", TeamMember.Type),
		edge.To("project_memberships", ProjectMember.Type),
		edge.To("project_memberships_granted", ProjectMember.Type),
		edge.To("tasks_assigned", Task.Type),
		edge.To("task_assignees", TaskAssignee.Type),
		edge.To("sprints", Sprint.Type),
		edge.To("tags", Tag.Type),
		edge.To("api_keys", APIKey.Type),
		edge.To("user_activities", UserActivity.Type),
		edge.To("task_comments", TaskComment.Type),
		edge.To("invites_sent", Invite.Type),
		edge.To("invites_received", Invite.Type),
		edge.To("team_invitations_sent", TeamInvitation.Type),
		edge.To("team_invitations_received", TeamInvitation.Type),
		edge.To("cloudinary_credentials", CloudinaryCredential.Type),
		edge.To("task_attachments", TaskAttachment.Type),
		edge.To("wiki_pages_created", WikiPage.Type),
		edge.To("yjs_updates", YjsUpdate.Type),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email"),
	}
}
