package neogo

import (
	"fmt"
	"strings"

	"github.com/rlch/scaf/language/go"
)

// Binding implements golang.Binding for neogo.
// It generates function bodies that execute Cypher queries using neogo's driver.
type Binding struct{}

// NewBinding creates a new neogo binding.
func NewBinding() *Binding {
	return &Binding{}
}

// Name returns the binding identifier.
func (b *Binding) Name() string {
	return "neogo"
}

// Imports returns the import paths needed by generated code.
func (b *Binding) Imports() []string {
	return []string{
		"context",
		"github.com/rlch/neogo",
	}
}

// PrependParams returns ctx and db as the first params for all functions.
func (b *Binding) PrependParams() []golang.BindingParam {
	return []golang.BindingParam{
		{Name: "ctx", Type: "context.Context"},
		{Name: "db", Type: "neogo.Driver"},
	}
}

// ReturnsError returns true - neogo functions always return error.
func (b *Binding) ReturnsError() bool {
	return true
}

// ReceiverType returns empty - neogo generates standalone functions, not methods.
func (b *Binding) ReceiverType() string {
	return ""
}

// GenerateBody generates the function body for executing a Cypher query.
//
// Generated code pattern (with params):
//
//	var name string
//	var age int
//	err := db.Exec().
//		Cypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name, u.age AS age`).
//		RunWithParams(ctx, map[string]any{"userId": userId}, "name", &name, "age", &age)
//	if err != nil {
//		return "", 0, err
//	}
//	return name, age, nil
//
// Generated code pattern (no params):
//
//	var count int64
//	err := db.Exec().
//		Cypher(`MATCH (u:User) RETURN count(u) AS count`).
//		Run(ctx, "count", &count)
//	if err != nil {
//		return 0, err
//	}
//	return count, nil
func (b *Binding) GenerateBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	if sig == nil {
		return "panic(\"no signature\")", nil
	}

	var sb strings.Builder

	// db is passed as a parameter (from PrependParams)

	// Use QueryParams for the database call (excludes binding params like ctx)
	queryParams := ctx.QueryParams

	// Declare result variables
	for _, ret := range sig.Returns {
		varName := toLocalName(ret.Name)
		fmt.Fprintf(&sb, "var %s %s\n", varName, ret.Type)
	}

	// Build the Cypher query execution
	// err := db.Exec().Cypher(`...`).Run[WithParams](ctx, ...)
	if sig.ReturnsError {
		sb.WriteString("err := ")
	}

	fmt.Fprintf(&sb, "db.Exec().\n\tCypher(`%s`).\n\t", strings.TrimSpace(ctx.Query))

	// Use RunWithParams if we have query params, otherwise Run
	if len(queryParams) > 0 {
		sb.WriteString("RunWithParams(ctx, map[string]any{")
		for i, param := range queryParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", param.Name, param.Name)
		}
		sb.WriteString("}")
	} else {
		sb.WriteString("Run(ctx")
	}

	// Add return bindings (use ColumnName for the db column, Name for the Go variable)
	for _, ret := range sig.Returns {
		varName := toLocalName(ret.Name)
		columnName := ret.ColumnName
		if columnName == "" {
			columnName = ret.Name
		}
		fmt.Fprintf(&sb, ", %q, &%s", columnName, varName)
	}
	sb.WriteString(")")

	// Handle error if the signature returns error
	if sig.ReturnsError {
		sb.WriteString("\nif err != nil {\n\treturn ")
		b.writeZeroReturns(&sb, sig.Returns)
		if len(sig.Returns) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("err\n}")
	}

	// Return statement
	sb.WriteString("\nreturn ")
	for i, ret := range sig.Returns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(toLocalName(ret.Name))
	}
	if sig.ReturnsError {
		if len(sig.Returns) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("nil")
	}

	return sb.String(), nil
}

// writeZeroReturns writes zero values for all return types.
func (b *Binding) writeZeroReturns(sb *strings.Builder, returns []golang.BindingReturn) {
	for i, ret := range returns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(zeroValue(ret.Type))
	}
}

// zeroValue returns the zero value literal for a Go type.
func zeroValue(typ string) string {
	switch typ {
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune":
		return "0"
	case "bool":
		return "false"
	default:
		// Pointers, slices, maps, interfaces, any
		if strings.HasPrefix(typ, "*") ||
			strings.HasPrefix(typ, "[]") ||
			strings.HasPrefix(typ, "map[") ||
			typ == "any" || typ == "error" {
			return "nil"
		}
		// Struct or unknown - use zero value syntax
		return typ + "{}"
	}
}

// toLocalName converts a return name to a local variable name.
func toLocalName(name string) string {
	if name == "" {
		return "result"
	}
	return name
}

// Register the binding on package init.
//
//nolint:gochecknoinits // Registration pattern requires init.
func init() {
	golang.RegisterBinding(NewBinding())
}
