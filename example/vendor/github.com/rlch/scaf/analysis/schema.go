package analysis

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/rlch/scaf"
	"gopkg.in/yaml.v3"
)

// TypeSchema represents the database schema extracted from user code.
// It is dialect-agnostic and populated by adapters (e.g., neogo) that crawl
// the user's codebase to discover models, fields, and relationships.
//
// The LSP uses this schema to provide completions and type information
// when editing queries. Dialects define what arguments/returns functions
// expect, and the TypeSchema maps those to concrete types from user code.
type TypeSchema struct {
	// Models maps model name (e.g., "Person", "ActedIn") to its definition.
	Models map[string]*Model
}

// NewTypeSchema creates an empty TypeSchema.
func NewTypeSchema() *TypeSchema {
	return &TypeSchema{
		Models: make(map[string]*Model),
	}
}

// yamlSchema is the YAML representation of TypeSchema.
type yamlSchema struct {
	Models map[string]*yamlModel `yaml:"models"`
}

// yamlModel is the YAML representation of Model.
type yamlModel struct {
	Fields        map[string]*yamlField        `yaml:"fields,omitempty"`
	Relationships map[string]*yamlRelationship `yaml:"relationships,omitempty"`
}

// yamlField is the YAML representation of Field.
type yamlField struct {
	Type     string `yaml:"type"`
	Required bool   `yaml:"required,omitempty"`
	Unique   bool   `yaml:"unique,omitempty"`
}

// yamlRelationship is the YAML representation of Relationship.
type yamlRelationship struct {
	RelType   string `yaml:"rel_type"`
	Target    string `yaml:"target"`
	Many      bool   `yaml:"many,omitempty"`
	Direction string `yaml:"direction"`
}

// LoadSchema loads a TypeSchema from a YAML file.
// The path can be absolute or relative to baseDir.
func LoadSchema(path, baseDir string) (*TypeSchema, error) {
	if path == "" {
		return nil, nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}

	var ys yamlSchema
	if err := yaml.Unmarshal(data, &ys); err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}

	return yamlSchemaToTypeSchema(&ys)
}

// yamlSchemaToTypeSchema converts the YAML representation to TypeSchema.
func yamlSchemaToTypeSchema(ys *yamlSchema) (*TypeSchema, error) {
	schema := NewTypeSchema()

	for modelName, ym := range ys.Models {
		model := &Model{
			Name:          modelName,
			Fields:        make([]*Field, 0),
			Relationships: make([]*Relationship, 0),
		}

		// Convert fields
		if ym.Fields != nil {
			for fieldName, yf := range ym.Fields {
				typ, err := ParseTypeString(yf.Type)
				if err != nil {
					return nil, fmt.Errorf("model %s, field %s: %w", modelName, fieldName, err)
				}

				model.Fields = append(model.Fields, &Field{
					Name:     fieldName,
					Type:     typ,
					Required: yf.Required,
					Unique:   yf.Unique,
				})
			}
		}

		// Convert relationships
		if ym.Relationships != nil {
			for relName, yr := range ym.Relationships {
				model.Relationships = append(model.Relationships, &Relationship{
					Name:      relName,
					RelType:   yr.RelType,
					Target:    yr.Target,
					Many:      yr.Many,
					Direction: Direction(yr.Direction),
				})
			}
		}

		schema.Models[model.Name] = model
	}

	return schema, nil
}

// WriteSchema writes a TypeSchema as YAML to the given writer.
// The output includes a yaml-language-server schema comment for editor validation.
func WriteSchema(w io.Writer, schema *TypeSchema) error {
	// Write schema comment for editor validation
	if _, err := fmt.Fprintln(w, "# yaml-language-server: $schema=https://raw.githubusercontent.com/rlch/scaf/main/.scaf-type.schema.json"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Convert to YAML representation
	ys := &yamlSchema{
		Models: make(map[string]*yamlModel),
	}

	// Sort model names for deterministic output
	modelNames := make([]string, 0, len(schema.Models))
	for name := range schema.Models {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	for _, name := range modelNames {
		model := schema.Models[name]

		ym := &yamlModel{}

		// Convert fields
		if len(model.Fields) > 0 {
			ym.Fields = make(map[string]*yamlField)
			for _, field := range model.Fields {
				ym.Fields[field.Name] = &yamlField{
					Type:     field.Type.String(),
					Required: field.Required,
					Unique:   field.Unique,
				}
			}
		}

		// Convert relationships
		if len(model.Relationships) > 0 {
			ym.Relationships = make(map[string]*yamlRelationship)
			for _, rel := range model.Relationships {
				ym.Relationships[rel.Name] = &yamlRelationship{
					RelType:   rel.RelType,
					Target:    rel.Target,
					Many:      rel.Many,
					Direction: string(rel.Direction),
				}
			}
		}

		ys.Models[name] = ym
	}

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer encoder.Close()

	return encoder.Encode(ys)
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
	Kind TypeKind

	// Name is the type name.
	// For primitives: "string", "int", "bool", "float64", etc.
	// For named types: "Time", "UUID", etc.
	Name string

	// Package is the package path for named types (e.g., "time", "github.com/google/uuid").
	// Empty for primitives.
	Package string

	// Elem is the element type for slices, arrays, pointers, and map values.
	Elem *Type

	// Key is the key type for maps.
	Key *Type

	// ArrayLen is the length for array types.
	ArrayLen int
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

// ParseTypeString parses a Go-style type string into a Type.
// Supports: string, int, int64, float64, bool, []T, [N]T, *T, map[K]V, pkg.Name
//
// Examples:
//
//	"string"           -> TypeKindPrimitive, Name="string"
//	"[]string"         -> TypeKindSlice, Elem=string
//	"[5]int"           -> TypeKindArray, ArrayLen=5, Elem=int
//	"*int"             -> TypeKindPointer, Elem=int
//	"map[string]int"   -> TypeKindMap, Key=string, Elem=int
//	"time.Time"        -> TypeKindNamed, Package="time", Name="Time"
//	"[]map[string]*int" -> nested types
func ParseTypeString(s string) (*Type, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty type string")
	}

	return parseType(s)
}

// parseType recursively parses a type string.
func parseType(s string) (*Type, error) {
	// Check for slice: []T
	if strings.HasPrefix(s, "[]") {
		elem, err := parseType(s[2:])
		if err != nil {
			return nil, err
		}
		return SliceOf(elem), nil
	}

	// Check for array: [N]T
	if strings.HasPrefix(s, "[") {
		closeIdx := strings.Index(s, "]")
		if closeIdx == -1 {
			return nil, fmt.Errorf("invalid array type: %s", s)
		}

		lenStr := s[1:closeIdx]
		if lenStr == "" {
			return nil, fmt.Errorf("invalid array type: %s", s)
		}

		arrayLen, err := strconv.Atoi(lenStr)
		if err != nil {
			return nil, fmt.Errorf("invalid array length %q: %w", lenStr, err)
		}

		elem, err := parseType(s[closeIdx+1:])
		if err != nil {
			return nil, err
		}

		return &Type{Kind: TypeKindArray, ArrayLen: arrayLen, Elem: elem}, nil
	}

	// Check for pointer: *T
	if strings.HasPrefix(s, "*") {
		elem, err := parseType(s[1:])
		if err != nil {
			return nil, err
		}
		return PointerTo(elem), nil
	}

	// Check for map: map[K]V
	if strings.HasPrefix(s, "map[") {
		// Find the closing bracket for the key type
		depth := 0
		keyEnd := -1

		for i := 4; i < len(s); i++ {
			switch s[i] {
			case '[':
				depth++
			case ']':
				if depth == 0 {
					keyEnd = i
				} else {
					depth--
				}
			}

			if keyEnd != -1 {
				break
			}
		}

		if keyEnd == -1 {
			return nil, fmt.Errorf("invalid map type: %s", s)
		}

		keyStr := s[4:keyEnd]
		valueStr := s[keyEnd+1:]

		key, err := parseType(keyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid map key type: %w", err)
		}

		value, err := parseType(valueStr)
		if err != nil {
			return nil, fmt.Errorf("invalid map value type: %w", err)
		}

		return MapOf(key, value), nil
	}

	// Check for named type: pkg.Name
	if idx := strings.LastIndex(s, "."); idx > 0 {
		pkg := s[:idx]
		name := s[idx+1:]

		if isValidIdentifier(pkg) && isValidIdentifier(name) {
			return NamedType(pkg, name), nil
		}
	}

	// Check for primitive types
	if isPrimitiveType(s) {
		return &Type{Kind: TypeKindPrimitive, Name: s}, nil
	}

	// Treat as named type without package (user-defined type in same package)
	if isValidIdentifier(s) {
		return NamedType("", s), nil
	}

	return nil, fmt.Errorf("unrecognized type: %s", s)
}

// isPrimitiveType returns true if s is a Go primitive type name.
func isPrimitiveType(s string) bool {
	switch s {
	case "bool", "string",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune",
		"float32", "float64",
		"complex64", "complex128",
		"any", "error":
		return true
	}
	return false
}

// isValidIdentifier returns true if s is a valid Go identifier.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}

	return true
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
	Name string

	// Fields are the properties/columns on this model.
	Fields []*Field

	// Relationships are edges from this model to other models.
	// Only applicable for node-like models in graph databases.
	Relationships []*Relationship
}

// Field represents a property/column on a model.
type Field struct {
	// Name is the field name as it appears in queries (e.g., "name", "age").
	Name string

	// Type is the field's type.
	Type *Type

	// Required indicates whether the field must have a value.
	Required bool

	// Unique indicates whether this field has a uniqueness constraint.
	// When a query filters on a unique field with equality, it returns at most one row.
	Unique bool
}

// Relationship represents an edge from one model to another.
type Relationship struct {
	// Name is the field name on the source model (e.g., "Friends", "ActedIn").
	Name string

	// RelType is the relationship type in the database (e.g., "FRIENDS", "ACTED_IN").
	RelType string

	// Target is the target model name.
	// For shorthand relationships: the target node (e.g., "Person").
	// For relationship structs: the relationship model (e.g., "ActedIn").
	Target string

	// Many indicates whether this is a one-to-many relationship.
	// true = Many[T], false = One[T]
	Many bool

	// Direction is the relationship direction.
	Direction Direction
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
//	    "log"
//	    "os"
//
//	    "github.com/rlch/scaf/adapters/neogo"
//	    "github.com/rlch/scaf/analysis"
//	    "myapp/models"
//	)
//
//	func main() {
//	    adapter := neogo.NewAdapter(
//	        &models.Person{},
//	        &models.Movie{},
//	        &models.ActedIn{},
//	    )
//	    schema, err := adapter.ExtractSchema()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    if err := analysis.WriteSchema(os.Stdout, schema); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// The user then runs: go run ./cmd/scaf-schema > .scaf-schema.yaml
// The LSP reads .scaf-schema.yaml to provide completions and type info.
type SchemaAdapter interface {
	// ExtractSchema discovers models from registered types and returns
	// a TypeSchema. The adapter is responsible for:
	// - Extracting fields and their types from registered types
	// - Discovering relationships between models
	// - Mapping library-specific metadata to the dialect-agnostic schema
	ExtractSchema() (*TypeSchema, error)
}

// SchemaAwareAnalyzer extends scaf.QueryAnalyzer with schema-aware analysis.
// When schema is provided, the analyzer can determine cardinality (ReturnsOne)
// by checking if the query filters on unique fields.
//
// Dialect analyzers that support cardinality inference should implement this interface.
// The Go code generator checks for this interface and uses it when available.
type SchemaAwareAnalyzer interface {
	// AnalyzeQueryWithSchema extracts metadata using schema for cardinality inference.
	// If schema is nil, behaves like AnalyzeQuery with ReturnsOne = false.
	AnalyzeQueryWithSchema(query string, schema *TypeSchema) (*scaf.QueryMetadata, error)
}
