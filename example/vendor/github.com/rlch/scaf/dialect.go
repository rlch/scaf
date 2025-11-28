package scaf

// Dialect represents a query language (cypher, sql).
// It provides static analysis of queries without requiring a database connection.
type Dialect interface {
	// Name returns the dialect identifier (e.g., "cypher", "sql").
	Name() string

	// Analyze extracts metadata from a query string.
	Analyze(query string) (*QueryMetadata, error)
}

var dialects = make(map[string]Dialect)

// RegisterDialect registers a dialect instance by name.
func RegisterDialect(d Dialect) {
	dialects[d.Name()] = d
}

// GetDialect returns a dialect by name.
// Returns nil if no dialect is registered with that name.
func GetDialect(name string) Dialect { //nolint:ireturn
	return dialects[name]
}

// RegisteredDialects returns the names of all registered dialects.
func RegisteredDialects() []string {
	names := make([]string, 0, len(dialects))
	for name := range dialects {
		names = append(names, name)
	}

	return names
}

// QueryAnalyzer provides static analysis of queries for IDE features.
// This interface exists for backwards compatibility with LSP and language packages.
type QueryAnalyzer interface {
	// AnalyzeQuery extracts metadata from a query string.
	AnalyzeQuery(query string) (*QueryMetadata, error)
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
	// First check dialect instances (Dialect implements Analyze which maps to AnalyzeQuery)
	if d := GetDialect(dialectName); d != nil {
		return &dialectAnalyzerAdapter{d}
	}

	// Fall back to explicit analyzer registry
	factory, ok := analyzers[dialectName]
	if !ok {
		return nil
	}

	return factory()
}

// dialectAnalyzerAdapter adapts a Dialect to the QueryAnalyzer interface.
type dialectAnalyzerAdapter struct {
	dialect Dialect
}

func (a *dialectAnalyzerAdapter) AnalyzeQuery(query string) (*QueryMetadata, error) {
	return a.dialect.Analyze(query)
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
	case DialectCypher, DatabaseNeo4j:
		return DialectCypher
	case DatabasePostgres, "postgresql", DatabaseMySQL, DatabaseSQLite, DialectSQL:
		return DialectSQL
	default:
		return dialectName
	}
}

// QueryMetadata holds extracted information about a query.
type QueryMetadata struct {
	// Parameters are the $-prefixed parameters used in the query.
	Parameters []ParameterInfo

	// Returns are the fields returned by the query.
	Returns []ReturnInfo

	// ReturnsOne indicates the query returns at most one row.
	// When false (default), the query may return multiple rows (slice).
	// Set to true when the query filters on a unique field with equality,
	// uses LIMIT 1, or is otherwise guaranteed to return a single row.
	ReturnsOne bool
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

	// Line is the 1-indexed line number in the query where this return field appears.
	Line int

	// Column is the 1-indexed column in the query where this return field starts.
	Column int

	// Length is the length of the return field expression in characters.
	Length int
}
