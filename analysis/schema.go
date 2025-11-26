package analysis

// TypeSchema represents the database schema extracted from user code.
// It is dialect-agnostic and populated by adapters (e.g., neogo) that crawl
// the user's codebase to discover models, fields, and relationships.
//
// The LSP uses this schema to provide completions and type information
// when editing queries. Dialects define what arguments/returns functions
// expect, and the TypeSchema maps those to concrete types from user code.
type TypeSchema struct {
	// Models maps model name (e.g., "Person", "ActedIn") to its definition.
	Models map[string]*Model `json:"models"`
}

// NewTypeSchema creates an empty TypeSchema.
func NewTypeSchema() *TypeSchema {
	return &TypeSchema{
		Models: make(map[string]*Model),
	}
}

// Model represents a database entity (node, relationship, table, etc.).
// In graph databases, both nodes and relationships are models.
// In relational databases, tables are models.
type Model struct {
	// Name is the model identifier (e.g., "Person", "ACTED_IN").
	Name string `json:"name"`

	// Fields are the properties/columns on this model.
	Fields []*Field `json:"fields,omitempty"`

	// Relationships are edges from this model to other models.
	// Only applicable for node-like models in graph databases.
	Relationships []*Relationship `json:"relationships,omitempty"`
}

// Field represents a property/column on a model.
type Field struct {
	// Name is the field name as it appears in queries (e.g., "name", "age").
	Name string `json:"name"`

	// Type is the field's type (e.g., "string", "int", "time.Time").
	// This is a dialect-agnostic representation.
	Type string `json:"type"`

	// Required indicates whether the field must have a value.
	Required bool `json:"required,omitempty"`
}

// Relationship represents an edge from one model to another.
type Relationship struct {
	// Name is the field name on the source model (e.g., "Friends", "ActedIn").
	Name string `json:"name"`

	// RelType is the relationship type in the database (e.g., "FRIENDS", "ACTED_IN").
	RelType string `json:"relType"`

	// Target is the target model name.
	// For shorthand relationships: the target node (e.g., "Person").
	// For relationship structs: the relationship model (e.g., "ActedIn").
	Target string `json:"target"`

	// Many indicates whether this is a one-to-many relationship.
	// true = Many[T], false = One[T]
	Many bool `json:"many,omitempty"`

	// Direction is the relationship direction.
	Direction Direction `json:"direction"`
}

// Direction represents the direction of a relationship.
type Direction string

const (
	// DirectionOutgoing represents an outgoing relationship (->).
	DirectionOutgoing Direction = "outgoing"
	// DirectionIncoming represents an incoming relationship (<-).
	DirectionIncoming Direction = "incoming"
)

// SchemaAdapter is the interface that adapter libraries implement to extract
// type schemas from user codebases.
//
// Adapters run in the user's codebase (not in scaf/LSP), typically as a Go
// command that outputs JSON. The user must explicitly register their types
// with the adapter, which then uses the underlying library (e.g., neogo's
// registry) to extract schema information.
//
// Example usage in user's codebase:
//
//	// cmd/scaf-schema/main.go
//	package main
//
//	import (
//	    "encoding/json"
//	    "os"
//
//	    "github.com/rlch/neogo/scafadapter"
//	    "myapp/models"
//	)
//
//	func main() {
//	    adapter := scafadapter.New()
//	    adapter.Register(
//	        &models.Person{},
//	        &models.Movie{},
//	        &models.ActedIn{},
//	    )
//	    schema, _ := adapter.ExtractSchema()
//	    json.NewEncoder(os.Stdout).Encode(schema)
//	}
//
// The user then runs: go run ./cmd/scaf-schema > .scaf-schema.json
// The LSP reads .scaf-schema.json to provide completions and type info.
type SchemaAdapter interface {
	// Register adds types to be included in the schema.
	// Types should be zero-value pointers (e.g., &Person{}).
	Register(types ...any)

	// ExtractSchema discovers models from registered types and returns
	// a TypeSchema. The adapter is responsible for:
	// - Extracting fields and their types from registered types
	// - Discovering relationships between models
	// - Mapping library-specific metadata to the dialect-agnostic schema
	ExtractSchema() (*TypeSchema, error)
}
