package golang

import "github.com/rlch/scaf/analysis"

// Binding generates database-specific Go code for query execution.
//
// A binding knows how to generate code that executes queries against a specific
// database driver (e.g., neogo for Neo4j, pgx for Postgres). Bindings are used
// at code-generation time only - the generated code directly uses the database
// driver without any scaf runtime dependency.
//
// The language generator assembles the full function signature from:
//   - PrependParams() - binding-specific params added at the start (e.g., ctx)
//   - Query params - extracted from the query ($userId → userId)
//   - Query returns - extracted from the query (RETURN name → name)
//   - ReturnsError() - whether to add error as final return
type Binding interface {
	// Name returns the binding identifier (e.g., "neogo", "pgx").
	Name() string

	// Imports returns the import paths needed by generated code.
	// These are added to the generated file's import block.
	Imports() []string

	// ReceiverType returns the receiver type for generated methods.
	// Returns empty string if functions should be standalone (not methods).
	// Example: "*Queries" for method generation.
	ReceiverType() string

	// PrependParams returns parameters to add at the start of every function.
	// For example, [{Name: "ctx", Type: "context.Context"}] for context-aware bindings.
	PrependParams() []BindingParam

	// ReturnsError returns true if generated functions should return error.
	// When true, error is added as the final return value.
	ReturnsError() bool

	// GenerateBody generates the function body for a query.
	// The body should execute the query and return the results.
	// It should NOT include the function signature or braces.
	GenerateBody(ctx *BodyContext) (string, error)
}

// BodyContext provides information needed to generate a function body.
type BodyContext struct {
	// Query is the raw query string to execute.
	Query string

	// Signature describes the full function signature (including binding params like ctx).
	Signature *BindingSignature

	// QueryParams are the parameters extracted from the query ($userId → userId).
	// These are the params that should be passed to the database driver.
	// Does NOT include binding-added params like ctx.
	QueryParams []BindingParam

	// Schema provides type information from the user's codebase.
	// May be nil if no schema is available.
	Schema *analysis.TypeSchema

	// ReceiverName is the name of the receiver variable (e.g., "db", "client").
	// Empty if the function is not a method.
	ReceiverName string

	// ReceiverType is the type of the receiver (e.g., "*Client", "DB").
	// Empty if the function is not a method.
	ReceiverType string
}

// BindingSignature describes a function's parameters and return types.
// This is a simplified version of FuncSignature for binding use.
type BindingSignature struct {
	// Name is the function name.
	Name string

	// Params are the function parameters.
	Params []BindingParam

	// Returns are the return types.
	Returns []BindingReturn

	// ReturnsSlice indicates if the function returns multiple rows.
	ReturnsSlice bool

	// ReturnsError indicates if the function returns an error.
	ReturnsError bool
}

// BindingParam describes a function parameter.
type BindingParam struct {
	// Name is the parameter name (without $ prefix).
	Name string

	// Type is the Go type string.
	Type string
}

// BindingReturn describes a return value.
type BindingReturn struct {
	// Name is the field name for Go code (may be empty for single returns).
	// This is the alias if present, or inferred from the expression.
	Name string

	// ColumnName is the actual column name returned by the database.
	// For "RETURN u.name AS n", ColumnName is "n".
	// For "RETURN u.name", ColumnName is "u.name".
	ColumnName string

	// Type is the Go type string.
	Type string
}

// Registration for binding discovery.
var bindings = make(map[string]Binding)

// RegisterBinding registers a binding by name.
func RegisterBinding(b Binding) {
	bindings[b.Name()] = b
}

// GetBinding returns a binding by name, or nil if not registered.
func GetBinding(name string) Binding { //nolint:ireturn
	return bindings[name]
}

// RegisteredBindings returns the names of all registered bindings.
func RegisteredBindings() []string {
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}

	return names
}
