// Package neo4j provides a scaf Database implementation for Neo4j.
package neo4j

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/dialects/cypher"
)

// ErrInvalidConfig is returned when an invalid configuration is provided.
var ErrInvalidConfig = errors.New("neo4j: expected *scaf.Neo4jConfig")

//nolint:gochecknoinits // Database self-registration pattern
func init() {
	scaf.RegisterDatabase(scaf.DatabaseNeo4j, func(cfg any) (scaf.Database, error) {
		neo4jCfg, ok := cfg.(*scaf.Neo4jConfig)
		if !ok {
			return nil, fmt.Errorf("%w, got %T", ErrInvalidConfig, cfg)
		}

		return New(neo4jCfg)
	})
}

// Database implements scaf.Database and scaf.TransactionalDatabase for Neo4j.
type Database struct {
	driver  neo4j.DriverWithContext
	session neo4j.SessionWithContext
	db      string
	dialect scaf.Dialect
}

// New creates a new Neo4j database connection from the given configuration.
func New(cfg *scaf.Neo4jConfig) (*Database, error) {
	auth := neo4j.NoAuth()
	if cfg.Username != "" {
		auth = neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return nil, fmt.Errorf("neo4j: failed to create driver: %w", err)
	}

	d := &Database{
		driver:  driver,
		db:      cfg.Database,
		dialect: cypher.NewDialect(),
	}

	// Verify connectivity
	ctx := context.Background()

	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		_ = driver.Close(ctx)

		return nil, fmt.Errorf("neo4j: failed to connect: %w", err)
	}

	// Create session config
	sessionCfg := neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	}
	if d.db != "" {
		sessionCfg.DatabaseName = d.db
	}

	d.session = driver.NewSession(ctx, sessionCfg)

	return d, nil
}

// Name returns the database identifier.
func (d *Database) Name() string {
	return scaf.DatabaseNeo4j
}

// Dialect returns the Cypher dialect for query analysis.
func (d *Database) Dialect() scaf.Dialect {
	return d.dialect
}

// Execute runs a Cypher query and returns the results.
// Results are flattened so that node/relationship properties are accessible
// as "alias.property" keys (e.g., "u.name" for RETURN u).
// Multi-statement queries (separated by newlines) are executed sequentially,
// returning results from the last statement.
func (d *Database) Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
	statements := splitStatements(query)

	var rows []map[string]any

	for _, stmt := range statements {
		result, err := d.session.Run(ctx, stmt, params)
		if err != nil {
			return nil, fmt.Errorf("neo4j: query execution failed: %w", err)
		}

		records, err := result.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("neo4j: failed to collect results: %w", err)
		}

		// Keep results from the last statement
		rows = make([]map[string]any, len(records))
		for i, record := range records {
			rows[i] = flattenRecord(record.Keys, record.Values)
		}
	}

	return rows, nil
}

// Close releases the database connection.
func (d *Database) Close() error {
	ctx := context.Background()

	if d.session != nil {
		err := d.session.Close(ctx)
		if err != nil {
			return fmt.Errorf("neo4j: failed to close session: %w", err)
		}
	}

	if d.driver != nil {
		err := d.driver.Close(ctx)
		if err != nil {
			return fmt.Errorf("neo4j: failed to close driver: %w", err)
		}
	}

	return nil
}

// Begin starts a new transaction for isolated test execution.
func (d *Database) Begin(ctx context.Context) (scaf.DatabaseTransaction, error) {
	tx, err := d.session.BeginTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("neo4j: failed to begin transaction: %w", err)
	}

	return &Transaction{tx: tx}, nil
}

// Transaction wraps a Neo4j transaction to implement scaf.DatabaseTransaction.
type Transaction struct {
	tx neo4j.ExplicitTransaction
}

// Execute runs a Cypher query within this transaction.
func (t *Transaction) Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
	statements := splitStatements(query)

	var rows []map[string]any

	for _, stmt := range statements {
		result, err := t.tx.Run(ctx, stmt, params)
		if err != nil {
			return nil, fmt.Errorf("neo4j: query execution failed: %w", err)
		}

		records, err := result.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("neo4j: failed to collect results: %w", err)
		}

		rows = make([]map[string]any, len(records))
		for i, record := range records {
			rows[i] = flattenRecord(record.Keys, record.Values)
		}
	}

	return rows, nil
}

// Commit commits the transaction.
func (t *Transaction) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Rollback aborts the transaction.
func (t *Transaction) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

// splitStatements splits a multi-statement query into individual statements.
// Statements are split when we see a new "starter" keyword (MATCH, MERGE, etc.)
// at the beginning of a line, AND the previous accumulated statement looks complete.
//
// A statement is complete if it has a RETURN clause. Write-only statements (CREATE, DELETE, etc.)
// are NOT considered complete on their own - they may be followed by more CREATE clauses
// that are part of the same logical statement.
func splitStatements(query string) []string {
	lines := strings.Split(strings.TrimSpace(query), "\n")

	var statements []string

	var current strings.Builder

	// These keywords can start a new statement. Split only happens if previous has RETURN.
	starterKeywords := []string{"MATCH", "CREATE", "MERGE", "DETACH", "OPTIONAL", "CALL", "UNWIND", "FOREACH"}

	isStarter := func(s string) bool {
		upper := strings.ToUpper(strings.TrimSpace(s))
		for _, kw := range starterKeywords {
			if strings.HasPrefix(upper, kw) {
				return true
			}
		}

		return false
	}

	isComplete := func(s string) bool {
		upper := strings.ToUpper(s)
		// Only complete if it has RETURN clause
		return strings.Contains(upper, "RETURN ") || strings.HasSuffix(upper, "RETURN")
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check if this line starts a new statement AND previous is complete
		if isStarter(trimmed) && current.Len() > 0 && isComplete(current.String()) {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}

			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}

		current.WriteString(line)
	}

	// Don't forget the last statement
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

// flattenRecord converts a Neo4j record into a flat map.
// Nodes and relationships are expanded so their properties are accessible
// as "alias.property" (e.g., u.name, r.since).
func flattenRecord(keys []string, values []any) map[string]any {
	result := make(map[string]any)

	for i, key := range keys {
		value := values[i]
		flattenValue(result, key, value)
	}

	return result
}

func flattenValue(result map[string]any, key string, value any) {
	switch v := value.(type) {
	case dbtype.Node:
		// Expand node properties: u -> u.name, u.email, etc.
		for prop, propVal := range v.Props {
			result[key+"."+prop] = propVal
		}
		// Also store labels for assertions like u.labels
		result[key+".labels"] = v.Labels
		result[key+".elementId"] = v.ElementId

	case dbtype.Relationship:
		// Expand relationship properties
		for prop, propVal := range v.Props {
			result[key+"."+prop] = propVal
		}

		result[key+".type"] = v.Type
		result[key+".elementId"] = v.ElementId

	case dbtype.Path:
		// For paths, store nodes and relationships separately
		result[key+".nodes"] = v.Nodes
		result[key+".relationships"] = v.Relationships

	case map[string]any:
		// Nested maps: expand with dot notation
		for k, val := range v {
			result[key+"."+k] = val
		}

	case []any:
		// Lists: keep as-is for now
		result[key] = v

	default:
		// Primitives: store directly
		result[key] = v
	}
}

// Compile-time interface checks.
var (
	_ scaf.Database              = (*Database)(nil)
	_ scaf.TransactionalDatabase = (*Database)(nil)
	_ scaf.DatabaseTransaction   = (*Transaction)(nil)
)
