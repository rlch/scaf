// Package golang provides Go code generation from scaf DSL files.
//
// This package generates two files from a .scaf file:
//   - scaf.go: Production functions that execute queries via a binding
//   - scaf_test.go: Test mocks using function variable swapping for test isolation
//
// # Usage
//
// The generator is typically invoked via the scaf CLI:
//
//	scaf generate --lang go ./queries.scaf
//
// # Architecture
//
// The generator uses bindings to produce database-specific code. Each binding
// (e.g., neogo) knows how to generate the function body for executing queries
// against its database driver.
//
// # Test Mock Pattern
//
// Production code uses function variables for indirection:
//
//	func GetUser(userId int) string { return getUserImpl(userId) }
//	var getUserImpl = getUserProd
//	func getUserProd(userId int) string { /* real implementation */ }
//
// Test code swaps the implementation in init():
//
//	func init() { getUserImpl = getUserMock }
//	func getUserMock(userId int) string { /* if-chain matching test cases */ }
//
// This works because _test.go files in the same package can access unexported
// variables, and init() runs before all tests.
package golang

import (
	"github.com/rlch/scaf/language"
)

// Context provides Go-specific information needed for code generation.
// It embeds the base language.GenerateContext.
type Context struct {
	language.GenerateContext

	// Binding generates database-specific code.
	// May be nil if no binding is configured.
	Binding Binding

	// PackageName is the Go package name for generated files.
	PackageName string
}

// FuncSignature represents a generated function's signature.
type FuncSignature struct {
	// Name is the function name (derived from query name).
	Name string

	// Params are the function parameters (from query $variables).
	Params []FuncParam

	// Returns are the function return types (from query RETURN clause).
	Returns []FuncReturn

	// Query is the raw query body.
	Query string

	// QueryName is the original query name from the scaf file.
	QueryName string
}

// FuncParam represents a function parameter.
type FuncParam struct {
	// Name is the parameter name (without $ prefix).
	Name string

	// Type is the Go type (e.g., "string", "int64", "*User").
	Type string

	// Required indicates if the parameter must be provided.
	Required bool
}

// FuncReturn represents a function return value.
type FuncReturn struct {
	// Name is the return field name for Go code.
	// This is the alias if present, or inferred from the expression.
	Name string

	// ColumnName is the actual column name returned by the database.
	// For "RETURN u.name AS n", ColumnName is "n".
	// For "RETURN u.name", ColumnName is "u.name".
	ColumnName string

	// Type is the Go type.
	Type string

	// IsSlice indicates if this returns multiple rows.
	IsSlice bool
}

// GoLanguage implements language.Language for Go code generation.
type GoLanguage struct{}

// Name returns "go".
func (g *GoLanguage) Name() string {
	return "go"
}

// Generate produces scaf.go and scaf_test.go from the suite.
func (g *GoLanguage) Generate(ctx *language.GenerateContext) (map[string][]byte, error) {
	// Wrap in Go-specific context with defaults
	goCtx := &Context{
		GenerateContext: *ctx,
		PackageName:     "main", // Default, should be overridden by caller
	}

	gen := &generator{ctx: goCtx}

	return gen.Generate()
}

// GenerateWithContext produces scaf.go and scaf_test.go using Go-specific context.
func (g *GoLanguage) GenerateWithContext(ctx *Context) (map[string][]byte, error) {
	gen := &generator{ctx: ctx}

	return gen.Generate()
}

// New creates a new Go language generator.
func New() *GoLanguage {
	return &GoLanguage{}
}

// generator holds state during code generation.
type generator struct {
	ctx *Context
}

// Generate produces all output files.
func (g *generator) Generate() (map[string][]byte, error) {
	files := make(map[string][]byte)

	// Extract function signatures from queries
	signatures, err := g.extractSignatures()
	if err != nil {
		return nil, err
	}

	// Generate production code
	prod, err := g.generateProduction(signatures)
	if err != nil {
		return nil, err
	}

	files["scaf.go"] = prod

	// Generate test mocks
	test, err := g.generateTest(signatures)
	if err != nil {
		return nil, err
	}

	files["scaf_test.go"] = test

	return files, nil
}

// extractSignatures builds FuncSignature for each query in the suite.
func (g *generator) extractSignatures() ([]*FuncSignature, error) {
	return ExtractSignatures(g.ctx.Suite, g.ctx.QueryAnalyzer, g.ctx.Schema)
}

// generateProduction generates the scaf.go file.
func (g *generator) generateProduction(signatures []*FuncSignature) ([]byte, error) {
	return g.generateProductionFile(signatures)
}

// generateTest generates the scaf_test.go file.
func (g *generator) generateTest(signatures []*FuncSignature) ([]byte, error) {
	return g.generateMockFile(signatures)
}

//nolint:gochecknoinits // Registration pattern requires init.
func init() {
	language.Register(New())
}
