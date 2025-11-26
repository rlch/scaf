// Package cypher provides a scaf dialect for Cypher query execution against Neo4j.
package cypher

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/rlch/scaf"
)

//nolint:gochecknoinits // Dialect self-registration pattern
func init() {
	scaf.RegisterDialect("cypher", New)
}

// Dialect implements scaf.Dialect for Cypher queries against Neo4j.
type Dialect struct {
	driver  neo4j.DriverWithContext
	session neo4j.SessionWithContext
	db      string
}

// New creates a new Cypher dialect from the given configuration.
func New(cfg scaf.DialectConfig) (scaf.Dialect, error) { //nolint:ireturn // Factory returns interface per Dialect pattern
	auth := neo4j.NoAuth()
	if cfg.Username != "" {
		auth = neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return nil, fmt.Errorf("cypher: failed to create driver: %w", err)
	}

	d := &Dialect{
		driver: driver,
	}

	// Apply options from config
	if db, ok := cfg.Options["database"].(string); ok {
		d.db = db
	}

	// Verify connectivity
	ctx := context.Background()

	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		_ = driver.Close(ctx)

		return nil, fmt.Errorf("cypher: failed to connect: %w", err)
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

// Name returns the dialect identifier.
func (d *Dialect) Name() string {
	return "cypher"
}

// Execute runs a Cypher query and returns the results.
// Results are flattened so that node/relationship properties are accessible
// as "alias.property" keys (e.g., "u.name" for RETURN u).
// Multi-statement queries (separated by newlines) are executed sequentially,
// returning results from the last statement.
func (d *Dialect) Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
	statements := splitStatements(query)

	var rows []map[string]any

	for _, stmt := range statements {
		result, err := d.session.Run(ctx, stmt, params)
		if err != nil {
			return nil, fmt.Errorf("cypher: query execution failed: %w", err)
		}

		records, err := result.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("cypher: failed to collect results: %w", err)
		}

		// Keep results from the last statement
		rows = make([]map[string]any, len(records))
		for i, record := range records {
			rows[i] = flattenRecord(record.Keys, record.Values)
		}
	}

	return rows, nil
}

// splitStatements splits a multi-statement query into individual statements.
// Statements are split when we see a new "starter" keyword (MATCH, CREATE, MERGE, etc.)
// at the beginning of a line, AND the previous accumulated statement looks complete
// (contains RETURN, or is a write-only statement like CREATE/DELETE).
func splitStatements(query string) []string {
	lines := strings.Split(strings.TrimSpace(query), "\n")

	var statements []string

	var current strings.Builder

	starterKeywords := []string{"MATCH", "CREATE", "MERGE", "DETACH", "OPTIONAL", "CALL", "UNWIND", "FOREACH"}
	writeKeywords := []string{"CREATE", "MERGE", "DELETE", "DETACH DELETE", "SET", "REMOVE"}

	isStarter := func(s string) bool {
		upper := strings.ToUpper(s)
		for _, kw := range starterKeywords {
			if strings.HasPrefix(upper, kw) {
				return true
			}
		}

		return false
	}

	isComplete := func(s string) bool {
		upper := strings.ToUpper(s)
		// Has RETURN clause
		if strings.Contains(upper, "RETURN ") || strings.HasSuffix(upper, "RETURN") {
			return true
		}
		// Is a write-only statement (CREATE, DELETE, etc. without needing RETURN)
		for _, kw := range writeKeywords {
			if strings.Contains(upper, kw) {
				return true
			}
		}

		return false
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

// Close releases the database connection.
func (d *Dialect) Close() error {
	ctx := context.Background()

	if d.session != nil {
		err := d.session.Close(ctx)
		if err != nil {
			return fmt.Errorf("cypher: failed to close session: %w", err)
		}
	}

	if d.driver != nil {
		err := d.driver.Close(ctx)
		if err != nil {
			return fmt.Errorf("cypher: failed to close driver: %w", err)
		}
	}

	return nil
}

// Begin starts a new transaction for isolated test execution.
func (d *Dialect) Begin(ctx context.Context) (scaf.Transaction, error) { //nolint:ireturn // Interface return per Transactional contract
	tx, err := d.session.BeginTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("cypher: failed to begin transaction: %w", err)
	}

	return &Transaction{tx: tx}, nil
}

// Transaction wraps a Neo4j transaction to implement scaf.Transaction.
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
			return nil, fmt.Errorf("cypher: query execution failed: %w", err)
		}

		records, err := result.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("cypher: failed to collect results: %w", err)
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

// Ensure Dialect implements scaf.Dialect and scaf.Transactional.
var (
	_ scaf.Dialect       = (*Dialect)(nil)
	_ scaf.Transactional = (*Dialect)(nil)
	_ scaf.Transaction   = (*Transaction)(nil)
)
