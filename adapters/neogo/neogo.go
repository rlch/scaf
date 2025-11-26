// Package neogo provides a scaf adapter for the neogo Neo4j ORM.
//
// This package provides schema extraction from neogo models for LSP completions
// and type information in .scaf files.
//
// # Usage
//
// Create a command in your project to generate the schema:
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
//	    schema, err := adapter.ExtractSchema()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    json.NewEncoder(os.Stdout).Encode(schema)
//	}
//
// Then run: go run ./cmd/scaf-schema > .scaf-schema.json
//
// Configure .scaf.yaml to point to the schema file:
//
//	dialect: cypher
//	schema: .scaf-schema.json
package neogo

import (
	"reflect"
	"strings"

	"github.com/rlch/neogo"
	"github.com/rlch/scaf/analysis"
)

// Adapter extracts schema information from neogo models.
// It implements analysis.SchemaAdapter.
type Adapter struct {
	registry *neogo.Registry
}

// NewAdapter creates a new adapter with the given types registered.
// Types should be zero-value pointers (e.g., &Person{}, &Movie{}).
func NewAdapter(types ...any) *Adapter {
	reg := neogo.NewRegistry()
	reg.RegisterTypes(types...)

	return &Adapter{
		registry: reg,
	}
}

// ExtractSchema discovers models from registered types and returns a TypeSchema.
func (a *Adapter) ExtractSchema() (*analysis.TypeSchema, error) {
	schema := analysis.NewTypeSchema()

	// Extract nodes
	for _, node := range a.registry.Nodes() {
		model := a.extractNodeModel(node)
		schema.Models[model.Name] = model
	}

	// Extract relationships
	for _, rel := range a.registry.Relationships() {
		model := a.extractRelationshipModel(rel)
		if model != nil {
			schema.Models[model.Name] = model
		}
	}

	return schema, nil
}

// extractNodeModel converts a RegisteredNode to a Model.
func (a *Adapter) extractNodeModel(node *neogo.RegisteredNode) *analysis.Model {
	model := &analysis.Model{
		Name:   node.Name(),
		Fields: make([]*analysis.Field, 0),
	}

	// Extract fields from FieldsToProps
	fieldsToProps := node.FieldsToProps()
	if fieldsToProps != nil {
		nodeType := node.Type()
		if nodeType != nil {
			model.Fields = a.extractFields(nodeType, fieldsToProps)
		}
	}

	// Extract relationships
	relationships := node.Relationships()
	if relationships != nil {
		model.Relationships = make([]*analysis.Relationship, 0, len(relationships))
		for fieldName, relTarget := range relationships {
			rel := a.extractRelationship(fieldName, relTarget, model.Name)
			model.Relationships = append(model.Relationships, rel)
		}
	}

	return model
}

// extractRelationshipModel converts a RegisteredRelationship to a Model.
// Returns nil for shorthand relationships (they don't have their own model).
func (a *Adapter) extractRelationshipModel(rel *neogo.RegisteredRelationship) *analysis.Model {
	// Skip empty/shorthand relationships (they don't have their own model)
	if rel.Name() == "" {
		return nil
	}

	model := &analysis.Model{
		Name:   rel.Name(),
		Fields: make([]*analysis.Field, 0),
	}

	// Extract fields
	fieldsToProps := rel.FieldsToProps()
	if fieldsToProps != nil {
		relType := rel.Type()
		if relType != nil {
			model.Fields = a.extractFields(relType, fieldsToProps)
		}
	}

	return model
}

// extractFields extracts Field definitions from a struct type.
func (a *Adapter) extractFields(typ reflect.Type, fieldsToProps map[string]string) []*analysis.Field {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil
	}

	fields := make([]*analysis.Field, 0)

	for goFieldName, dbFieldName := range fieldsToProps {
		// Skip startNode/endNode - these are relationship navigation, not stored properties
		if dbFieldName == "startNode" || dbFieldName == "endNode" {
			continue
		}

		// Find the struct field
		field, ok := typ.FieldByName(goFieldName)
		if !ok {
			continue
		}

		// Skip embedded Node/Relationship base types
		if field.Anonymous {
			continue
		}

		// Convert reflect.Type to analysis.Type
		fieldType := reflectTypeToType(field.Type)

		// Check if required (pointer types are optional)
		required := field.Type.Kind() != reflect.Ptr

		fields = append(fields, &analysis.Field{
			Name:     dbFieldName,
			Type:     fieldType,
			Required: required,
		})
	}

	return fields
}

// extractRelationship converts a RelationshipTarget to a Relationship.
func (a *Adapter) extractRelationship(fieldName string, target *neogo.RelationshipTarget, sourceNodeName string) *analysis.Relationship {
	rel := &analysis.Relationship{
		Name:    fieldName,
		RelType: target.RelType(),
		Many:    target.Many(),
	}

	// Direction: Dir=true means outgoing (->), Dir=false means incoming (<-)
	if target.Dir() {
		rel.Direction = analysis.DirectionOutgoing
	} else {
		rel.Direction = analysis.DirectionIncoming
	}

	// Determine target based on relationship type
	relName := target.RelName()
	if relName != "" {
		// This is a relationship struct - try to find the actual end/start node
		rel.Target = a.resolveRelationshipTarget(relName, target.Dir())
	} else {
		// Shorthand relationship - find the target node
		rel.Target = a.resolveShorthandTarget(target, sourceNodeName)
	}

	return rel
}

// resolveRelationshipTarget finds the target model name for a relationship struct.
func (a *Adapter) resolveRelationshipTarget(relName string, outgoing bool) string {
	relMeta := a.registry.GetRelMeta(relName)
	if relMeta == nil {
		return relName
	}

	switch {
	case outgoing && relMeta.EndNode() != nil:
		return relMeta.EndNode().NodeType().Elem().Name()
	case !outgoing && relMeta.StartNode() != nil:
		return relMeta.StartNode().NodeType().Elem().Name()
	default:
		return relName
	}
}

// resolveShorthandTarget finds the target model name for a shorthand relationship.
func (a *Adapter) resolveShorthandTarget(target *neogo.RelationshipTarget, sourceNodeName string) string {
	startNode := target.StartNode()
	endNode := target.EndNode()

	switch {
	case startNode != nil && startNode.Name() != sourceNodeName:
		return startNode.Name()
	case endNode != nil && endNode.Name() != sourceNodeName:
		return endNode.Name()
	case startNode != nil:
		// Self-referential
		return startNode.Name()
	default:
		return ""
	}
}

// reflectTypeToType converts a reflect.Type to an analysis.Type.
func reflectTypeToType(typ reflect.Type) *analysis.Type {
	switch typ.Kind() { //nolint:exhaustive // We only care about composite types
	case reflect.Ptr:
		return analysis.PointerTo(reflectTypeToType(typ.Elem()))
	case reflect.Slice:
		return analysis.SliceOf(reflectTypeToType(typ.Elem()))
	case reflect.Array:
		return &analysis.Type{
			Kind:     analysis.TypeKindArray,
			Elem:     reflectTypeToType(typ.Elem()),
			ArrayLen: typ.Len(),
		}
	case reflect.Map:
		return analysis.MapOf(reflectTypeToType(typ.Key()), reflectTypeToType(typ.Elem()))
	default:
		// Check if it's a named type from another package
		if typ.PkgPath() != "" && !isBuiltinType(typ.Name()) {
			// Use short package name for display
			pkgPath := typ.PkgPath()
			parts := strings.Split(pkgPath, "/")
			pkgName := parts[len(parts)-1]

			return analysis.NamedType(pkgName, typ.Name())
		}

		// Primitive type
		return &analysis.Type{Kind: analysis.TypeKindPrimitive, Name: typ.Name()}
	}
}

// isBuiltinType returns true if the type name is a Go builtin.
func isBuiltinType(name string) bool {
	switch name {
	case "bool", "string",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune",
		"float32", "float64",
		"complex64", "complex128",
		"error":
		return true
	}

	return false
}
