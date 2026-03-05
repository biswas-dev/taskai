package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("project_id"),
		field.Int("task_number").Optional().Nillable(),
		field.String("title").NotEmpty(),
		field.String("description").Optional().Nillable(),
		field.String("status").Default("todo"),
		field.Int64("swim_lane_id").Optional().Nillable(),
		field.Int64("sprint_id").Optional().Nillable(),
		field.Int64("assignee_id").Optional().Nillable(),
		field.String("priority").Default("medium"),
		field.Float("estimated_hours").Optional().Nillable(),
		field.Float("actual_hours").Optional().Nillable(),
		field.Time("start_date").Optional().Nillable(),
		field.Time("due_date").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the Task.
func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("tasks").Unique().Required().Field("project_id"),
		edge.From("swim_lane", SwimLane.Type).Ref("tasks").Unique().Field("swim_lane_id"),
		edge.From("sprint", Sprint.Type).Ref("tasks").Unique().Field("sprint_id"),
		edge.From("assignee", User.Type).Ref("tasks_assigned").Unique().Field("assignee_id"),
		edge.To("comments", TaskComment.Type),
		edge.To("attachments", TaskAttachment.Type),
		edge.To("task_tags", TaskTag.Type),
		edge.To("task_assignees", TaskAssignee.Type),
	}
}

// Indexes of the Task.
func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("status"),
		index.Fields("swim_lane_id"),
		index.Fields("sprint_id"),
		index.Fields("priority"),
		index.Fields("assignee_id"),
		index.Fields("project_id", "task_number").Unique(),
	}
}
