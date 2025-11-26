// Package analysis provides semantic analysis for scaf DSL files.
package analysis

import (
	"github.com/rlch/scaf"
)

// AnalyzedFile holds semantic analysis results for a single file.
type AnalyzedFile struct {
	// Path is the file path (URI in LSP terms).
	Path string

	// Suite is the parsed AST. Nil if parsing failed.
	Suite *scaf.Suite

	// ParseError holds the parse error if parsing failed.
	ParseError error

	// Diagnostics contains all errors and warnings found during analysis.
	Diagnostics []Diagnostic

	// Symbols contains all definitions in this file.
	Symbols *SymbolTable
}

// SymbolTable holds all named definitions in a file.
type SymbolTable struct {
	// Queries maps query name to its symbol.
	Queries map[string]*QuerySymbol

	// Imports maps alias (or base name) to import symbol.
	Imports map[string]*ImportSymbol

	// Setups maps setup name to its symbol (for named setups in SetupClause blocks).
	// Note: scaf doesn't define setups in-file yet, but imports reference them.
	Setups map[string]*SetupSymbol

	// Tests contains all test symbols, keyed by full path (e.g., "QueryName/GroupName/TestName").
	Tests map[string]*TestSymbol
}

// NewSymbolTable creates an empty symbol table.
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Queries: make(map[string]*QuerySymbol),
		Imports: make(map[string]*ImportSymbol),
		Setups:  make(map[string]*SetupSymbol),
		Tests:   make(map[string]*TestSymbol),
	}
}

// SymbolKind represents the type of a symbol.
type SymbolKind int

// Symbol kind constants.
const (
	SymbolKindQuery SymbolKind = iota
	SymbolKindImport
	SymbolKindSetup
	SymbolKindTest
	SymbolKindGroup
	SymbolKindParam
)

// Symbol is the base type for all symbol kinds.
type Symbol struct {
	Name string
	Span scaf.Span
	Kind SymbolKind
}

// QuerySymbol represents a query definition.
type QuerySymbol struct {
	Symbol

	Body string

	// Params are the $-prefixed parameters extracted from the query body.
	// Useful for completion and validation.
	Params []string

	// Node is the AST node for this query.
	Node *scaf.Query
}

// ImportSymbol represents an import statement.
type ImportSymbol struct {
	Symbol

	// Alias is the import alias (nil if using default base name).
	Alias *string
	// Path is the import path.
	Path string
	// Node is the AST node for this import.
	Node *scaf.Import
	// Used tracks whether this import is referenced (for unused import warnings).
	Used bool
}

// SetupSymbol represents a named setup (from imports).
type SetupSymbol struct {
	Symbol

	// Module is the module alias this setup comes from.
	Module string
	// Params are the expected parameters.
	Params []string
}

// TestSymbol represents a test definition.
type TestSymbol struct {
	Symbol

	// FullPath is the full path to this test (e.g., "GetUser/basic lookups/finds Alice").
	FullPath string
	// QueryScope is the parent query scope name.
	QueryScope string
	// Node is the AST node for this test.
	Node *scaf.Test
}

// Diagnostic represents an error or warning found during analysis.
type Diagnostic struct {
	Span     scaf.Span
	Severity DiagnosticSeverity
	Message  string
	Code     string // e.g., "undefined-query", "unused-import"
	Source   string // "scaf"
}

// DiagnosticSeverity indicates the severity of a diagnostic.
type DiagnosticSeverity int

// Diagnostic severity constants.
const (
	SeverityError DiagnosticSeverity = iota + 1
	SeverityWarning
	SeverityInformation
	SeverityHint
)
