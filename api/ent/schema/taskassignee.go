package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaskAssignee holds the schema definition for the TaskAssignee entity (junction table).
type TaskAssignee struct {
	ent.Schema
}

// Fields of the TaskAssignee.
func (TaskAssignee) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("task_id"),
		field.Int64("user_id"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Edges of the TaskAssignee.
func (TaskAssignee) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", Task.Type).Ref("task_assignees").Unique().Required().Field("task_id"),
		edge.From("user", User.Type).Ref("task_assignees").Unique().Required().Field("user_id"),
	}
}

// Indexes of the TaskAssignee.
func (TaskAssignee) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("user_id"),
		index.Fields("task_id", "user_id").Unique(),
	}
}
