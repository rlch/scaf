//nolint:testpackage
package neo4j

import (
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/rlch/scaf"
)

func TestDatabase_Name(t *testing.T) {
	db := setupIntegrationTest(t)
	defer func() { _ = db.Close() }()

	if got := db.Name(); got != scaf.DatabaseNeo4j {
		t.Errorf("Name() = %q, want %q", got, scaf.DatabaseNeo4j)
	}
}

func TestDatabase_Dialect(t *testing.T) {
	db := setupIntegrationTest(t)
	defer func() { _ = db.Close() }()

	dialect := db.Dialect()
	if dialect == nil {
		t.Fatal("Dialect() returned nil")
	}

	if got := dialect.Name(); got != scaf.DialectCypher {
		t.Errorf("Dialect().Name() = %q, want %q", got, scaf.DialectCypher)
	}
}

func TestDatabase_ImplementsInterface(_ *testing.T) {
	var _ scaf.Database = (*Database)(nil)
	var _ scaf.TransactionalDatabase = (*Database)(nil)
	var _ scaf.DatabaseTransaction = (*Transaction)(nil)
}

func TestDatabase_Registration(t *testing.T) {
	databases := scaf.RegisteredDatabases()

	if !slices.Contains(databases, scaf.DatabaseNeo4j) {
		t.Error("neo4j database not registered")
	}
}

func TestFlattenRecord_Primitives(t *testing.T) {
	keys := []string{"name", "age", "active"}
	values := []any{"Alice", int64(30), true}

	result := flattenRecord(keys, values)

	want := map[string]any{
		"name":   "Alice",
		"age":    int64(30),
		"active": true,
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("flattenRecord() mismatch (-want +got):\n%s", diff)
	}
}

func TestFlattenRecord_Node(t *testing.T) {
	keys := []string{"u"}
	values := []any{
		dbtype.Node{
			ElementId: "4:abc:123",
			Labels:    []string{"User"},
			Props: map[string]any{
				"name":  "Alice",
				"email": "alice@example.com",
			},
		},
	}

	result := flattenRecord(keys, values)

	want := map[string]any{
		"u.name":      "Alice",
		"u.email":     "alice@example.com",
		"u.labels":    []string{"User"},
		"u.elementId": "4:abc:123",
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("flattenRecord() mismatch (-want +got):\n%s", diff)
	}
}

func TestFlattenRecord_Relationship(t *testing.T) {
	keys := []string{"r"}
	values := []any{
		dbtype.Relationship{
			ElementId: "5:abc:456",
			Type:      "FOLLOWS",
			Props: map[string]any{
				"since": int64(2020),
			},
		},
	}

	result := flattenRecord(keys, values)

	want := map[string]any{
		"r.since":     int64(2020),
		"r.type":      "FOLLOWS",
		"r.elementId": "5:abc:456",
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("flattenRecord() mismatch (-want +got):\n%s", diff)
	}
}

func TestFlattenRecord_Mixed(t *testing.T) {
	// Simulates: RETURN u.name AS name, u.age AS age
	keys := []string{"name", "age"}
	values := []any{"Alice", int64(30)}

	result := flattenRecord(keys, values)

	want := map[string]any{
		"name": "Alice",
		"age":  int64(30),
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("flattenRecord() mismatch (-want +got):\n%s", diff)
	}
}

func TestFlattenRecord_NestedMap(t *testing.T) {
	keys := []string{"props"}
	values := []any{
		map[string]any{
			"name": "Alice",
			"age":  int64(30),
		},
	}

	result := flattenRecord(keys, values)

	want := map[string]any{
		"props.name": "Alice",
		"props.age":  int64(30),
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("flattenRecord() mismatch (-want +got):\n%s", diff)
	}
}

// Integration tests - only run with a real Neo4j instance.
// Set SCAF_NEO4J_URI, SCAF_NEO4J_USER, SCAF_NEO4J_PASS to run.

func TestDatabase_Execute_Integration(t *testing.T) {
	db := setupIntegrationTest(t)
	defer func() { _ = db.Close() }()

	ctx := t.Context()

	// Clean up any existing test data
	_, _ = db.Execute(ctx, "MATCH (n:ScafTest) DELETE n", nil)

	// Create test node
	_, err := db.Execute(ctx, "CREATE (n:ScafTest {name: $name}) RETURN n", map[string]any{
		"name": "test-node",
	})
	if err != nil {
		t.Fatalf("failed to create test node: %v", err)
	}

	// Query it back
	results, err := db.Execute(ctx, "MATCH (n:ScafTest) RETURN n.name AS name", nil)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0]["name"] != "test-node" {
		t.Errorf("name = %v, want %v", results[0]["name"], "test-node")
	}

	// Clean up
	_, _ = db.Execute(ctx, "MATCH (n:ScafTest) DELETE n", nil)
}

func TestDatabase_Transaction_Integration(t *testing.T) {
	db := setupIntegrationTest(t)
	defer func() { _ = db.Close() }()

	ctx := t.Context()

	// Start transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	// Create node in transaction
	_, err = tx.Execute(ctx, "CREATE (n:ScafTxTest {name: 'in-tx'})", nil)
	if err != nil {
		t.Fatalf("failed to create node in tx: %v", err)
	}

	// Rollback
	err = tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Verify node doesn't exist
	results, err := db.Execute(ctx, "MATCH (n:ScafTxTest) RETURN n", nil)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if len(results) != 0 {
		t.Error("node should not exist after rollback")
	}
}

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name: "splits on RETURN",
			query: `MATCH (n) RETURN n
MATCH (m) RETURN m`,
			want: []string{
				"MATCH (n) RETURN n",
				"MATCH (m) RETURN m",
			},
		},
		{
			name: "chained CREATEs stay together",
			query: `MATCH (a:User {id: 1}), (b:User {id: 2})
CREATE (c:Comment {text: "hello"})
CREATE (a)-[:WROTE]->(c)
CREATE (b)-[:LIKES]->(c)`,
			want: []string{
				`MATCH (a:User {id: 1}), (b:User {id: 2})
CREATE (c:Comment {text: "hello"})
CREATE (a)-[:WROTE]->(c)
CREATE (b)-[:LIKES]->(c)`,
			},
		},
		{
			name: "write-only with MATCH stays together",
			query: `CREATE (:User {id: 1})
CREATE (:User {id: 2})
MATCH (a:User {id: 1}), (b:User {id: 2})
CREATE (a)-[:KNOWS]->(b)`,
			want: []string{
				`CREATE (:User {id: 1})
CREATE (:User {id: 2})
MATCH (a:User {id: 1}), (b:User {id: 2})
CREATE (a)-[:KNOWS]->(b)`,
			},
		},
		{
			name: "OPTIONAL MATCH stays with MATCH",
			query: `MATCH (c:Comment)
OPTIONAL MATCH (a:User)-[:WROTE]->(c)
RETURN c, a`,
			want: []string{
				`MATCH (c:Comment)
OPTIONAL MATCH (a:User)-[:WROTE]->(c)
RETURN c, a`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStatements(tt.query)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("splitStatements() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func setupIntegrationTest(t *testing.T) *Database {
	t.Helper()

	uri := os.Getenv("SCAF_NEO4J_URI")
	if uri == "" {
		t.Skip("SCAF_NEO4J_URI not set, skipping integration test")
	}

	cfg := &scaf.Neo4jConfig{
		URI:      uri,
		Username: os.Getenv("SCAF_NEO4J_USER"),
		Password: os.Getenv("SCAF_NEO4J_PASS"),
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	return db
}
