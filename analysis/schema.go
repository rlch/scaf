package analysis

import "strconv"

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

// TypeKind represents the kind of a type.
type TypeKind string

// Type kind constants.
const (
	TypeKindPrimitive TypeKind = "primitive" // string, int, bool, float64, etc.
	TypeKindSlice     TypeKind = "slice"     // []T
	TypeKindArray     TypeKind = "array"     // [N]T
	TypeKindMap       TypeKind = "map"       // map[K]V
	TypeKindPointer   TypeKind = "pointer"   // *T
	TypeKindNamed     TypeKind = "named"     // time.Time, uuid.UUID, etc.
)

// Type represents a type in the schema.
// This is a recursive structure that can represent complex types like []map[string]*Person.
type Type struct {
	// Kind is the category of this type.
	Kind TypeKind `json:"kind"`

	// Name is the type name.
	// For primitives: "string", "int", "bool", "float64", etc.
	// For named types: "Time", "UUID", etc.
	Name string `json:"name,omitempty"`

	// Package is the package path for named types (e.g., "time", "github.com/google/uuid").
	// Empty for primitives.
	Package string `json:"package,omitempty"`

	// Elem is the element type for slices, arrays, pointers, and map values.
	Elem *Type `json:"elem,omitempty"`

	// Key is the key type for maps.
	Key *Type `json:"key,omitempty"`

	// ArrayLen is the length for array types.
	ArrayLen int `json:"arrayLen,omitempty"`
}

// String returns a Go-style string representation of the type.
func (t *Type) String() string {
	if t == nil {
		return ""
	}

	switch t.Kind {
	case TypeKindPrimitive:
		return t.Name
	case TypeKindSlice:
		return "[]" + t.Elem.String()
	case TypeKindArray:
		return "[" + strconv.Itoa(t.ArrayLen) + "]" + t.Elem.String()
	case TypeKindMap:
		return "map[" + t.Key.String() + "]" + t.Elem.String()
	case TypeKindPointer:
		return "*" + t.Elem.String()
	case TypeKindNamed:
		if t.Package != "" {
			return t.Package + "." + t.Name
		}

		return t.Name
	default:
		return t.Name
	}
}

// Primitive type constructors for convenience.
var (
	TypeString  = &Type{Kind: TypeKindPrimitive, Name: "string"}
	TypeInt     = &Type{Kind: TypeKindPrimitive, Name: "int"}
	TypeInt64   = &Type{Kind: TypeKindPrimitive, Name: "int64"}
	TypeFloat64 = &Type{Kind: TypeKindPrimitive, Name: "float64"}
	TypeBool    = &Type{Kind: TypeKindPrimitive, Name: "bool"}
)

// SliceOf creates a slice type.
func SliceOf(elem *Type) *Type {
	return &Type{Kind: TypeKindSlice, Elem: elem}
}

// PointerTo creates a pointer type.
func PointerTo(elem *Type) *Type {
	return &Type{Kind: TypeKindPointer, Elem: elem}
}

// MapOf creates a map type.
func MapOf(key, value *Type) *Type {
	return &Type{Kind: TypeKindMap, Key: key, Elem: value}
}

// NamedType creates a named type.
func NamedType(pkg, name string) *Type {
	return &Type{Kind: TypeKindNamed, Package: pkg, Name: name}
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

	// Type is the field's type.
	Type *Type `json:"type"`

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
// Adapters are created with the types to be registered, then ExtractSchema
// is called to generate the schema. Each adapter's constructor takes the
// types to register.
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
//	    "github.com/rlch/scaf/adapters/neogo"
//	    "myapp/models"
//	)
//
//	func main() {
//	    adapter := neogo.NewAdapter(
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
	// ExtractSchema discovers models from registered types and returns
	// a TypeSchema. The adapter is responsible for:
	// - Extracting fields and their types from registered types
	// - Discovering relationships between models
	// - Mapping library-specific metadata to the dialect-agnostic schema
	ExtractSchema() (*TypeSchema, error)
}
