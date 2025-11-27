package scaf

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoTransactionSupport is returned when a dialect does not support transactions.
var ErrNoTransactionSupport = errors.New("dialect does not support transactions")

// Dialect defines the interface for database backends.
type Dialect interface {
	// Name returns the dialect identifier (e.g., "neo4j", "postgres").
	Name() string

	// Execute runs a query with parameters and returns the results.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Close releases any resources held by the dialect.
	Close() error
}

// Transaction represents an active database transaction.
// Queries executed through a transaction are isolated until Commit or Rollback.
type Transaction interface {
	// Execute runs a query within this transaction.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Commit commits the transaction.
	Commit(ctx context.Context) error

	// Rollback aborts the transaction.
	Rollback(ctx context.Context) error
}

// Transactional is an optional interface for dialects that support transactions.
// The runner uses this for test isolation (rollback after each test).
type Transactional interface {
	Dialect

	// Begin starts a new transaction.
	Begin(ctx context.Context) (Transaction, error)
}

// DialectFactory creates a Dialect from connection configuration.
type DialectFactory func(cfg DialectConfig) (Dialect, error)

// DialectConfig holds connection settings for a dialect.
type DialectConfig struct {
	// Connection URI (e.g., "bolt://localhost:7687", "postgres://localhost/db")
	URI string `yaml:"uri"`

	// Optional credentials (if not in URI)
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// Dialect-specific options
	Options map[string]any `yaml:"options,omitempty"`
}

var dialects = make(map[string]DialectFactory)

// RegisterDialect registers a dialect factory by name.
func RegisterDialect(name string, factory DialectFactory) {
	dialects[name] = factory
}

// NewDialect creates a dialect instance by name.
func NewDialect(name string, cfg DialectConfig) (Dialect, error) { //nolint:ireturn
	factory, ok := dialects[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDialect, name)
	}

	d, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	return &dialectWrapper{d}, nil
}

// dialectWrapper wraps a Dialect to return concrete type.
type dialectWrapper struct {
	Dialect
}

// Begin delegates to the underlying dialect if it supports transactions.
func (w *dialectWrapper) Begin(ctx context.Context) (Transaction, error) { //nolint:ireturn
	if tx, ok := w.Dialect.(Transactional); ok {
		return tx.Begin(ctx)
	}

	return nil, ErrNoTransactionSupport
}

// Transactional returns true if the underlying dialect supports transactions.
func (w *dialectWrapper) Transactional() bool {
	_, ok := w.Dialect.(Transactional)

	return ok
}

// RegisteredDialects returns the names of all registered dialects.
func RegisteredDialects() []string {
	names := make([]string, 0, len(dialects))
	for name := range dialects {
		names = append(names, name)
	}

	return names
}

// QueryAnalyzerFactory creates a QueryAnalyzer for a dialect.
type QueryAnalyzerFactory func() QueryAnalyzer

var analyzers = make(map[string]QueryAnalyzerFactory)

// RegisterAnalyzer registers a query analyzer factory by dialect name.
// Dialects should call this in their init() function.
func RegisterAnalyzer(dialectName string, factory QueryAnalyzerFactory) {
	analyzers[dialectName] = factory
}

// GetAnalyzer returns a QueryAnalyzer for the given dialect name.
// Returns nil if no analyzer is registered for that dialect.
func GetAnalyzer(dialectName string) QueryAnalyzer { //nolint:ireturn
	factory, ok := analyzers[dialectName]
	if !ok {
		return nil
	}

	return factory()
}

// RegisteredAnalyzers returns the names of all registered analyzers.
func RegisteredAnalyzers() []string {
	names := make([]string, 0, len(analyzers))
	for name := range analyzers {
		names = append(names, name)
	}

	return names
}

// MarkdownLanguage returns the markdown language identifier for a dialect.
// Used for syntax highlighting in IDE hover/completion documentation.
func MarkdownLanguage(dialectName string) string {
	// Common dialect name to markdown language mapping
	switch dialectName {
	case "cypher", "neo4j":
		return "cypher"
	case "postgres", "postgresql", "mysql", "sqlite", "sql":
		return "sql"
	default:
		return dialectName
	}
}

// QueryAnalyzer provides static analysis of queries for IDE features.
// Dialects can optionally implement this to provide better completions.
type QueryAnalyzer interface {
	// AnalyzeQuery extracts metadata from a query string.
	AnalyzeQuery(query string) (*QueryMetadata, error)
}

// QueryMetadata holds extracted information about a query.
type QueryMetadata struct {
	// Parameters are the $-prefixed parameters used in the query.
	Parameters []ParameterInfo

	// Returns are the fields returned by the query.
	Returns []ReturnInfo
}

// ParameterInfo describes a query parameter.
type ParameterInfo struct {
	// Name is the parameter name (without $ prefix).
	Name string

	// Type is the inferred type, if known (e.g., "string", "int").
	Type string

	// Position is the character offset in the query.
	Position int

	// Line is the 1-indexed line number in the query.
	Line int

	// Column is the 1-indexed column in the query.
	Column int

	// Length is the length of the parameter reference in characters.
	Length int

	// Count is how many times this parameter appears.
	Count int
}

// ReturnInfo describes a returned field.
type ReturnInfo struct {
	// Name is the field name or alias.
	Name string

	// Type is the inferred type, if known.
	Type string

	// Expression is the original expression text.
	Expression string

	// Alias is the explicit alias if AS keyword was used, empty otherwise.
	// When Alias is set, the database column name is Alias.
	// When Alias is empty, the database column name is Expression.
	Alias string

	// IsAggregate indicates this is an aggregate function result.
	IsAggregate bool

	// IsWildcard indicates this is a wildcard (*) return.
	IsWildcard bool
}
