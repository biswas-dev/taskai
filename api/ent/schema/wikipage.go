package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WikiPage holds the schema definition for the WikiPage entity.
type WikiPage struct {
	ent.Schema
}

// Fields of the WikiPage.
func (WikiPage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("project_id"),
		field.String("title").NotEmpty().MaxLen(500),
		field.String("slug").NotEmpty().MaxLen(500),
		field.Int64("created_by"),
		field.Text("content").Optional().Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the WikiPage.
func (WikiPage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("wiki_pages").Unique().Required().Field("project_id"),
		edge.From("creator", User.Type).Ref("wiki_pages_created").Unique().Required().Field("created_by"),
		edge.To("yjs_updates", YjsUpdate.Type),
		edge.To("versions", PageVersion.Type),
		edge.To("blocks", WikiBlock.Type),
	}
}

// Indexes of the WikiPage.
func (WikiPage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("slug"),
		index.Fields("project_id", "slug").Unique(),
	}
}
