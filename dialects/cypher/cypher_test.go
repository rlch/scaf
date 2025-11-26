//nolint:testpackage
package cypher

import (
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/rlch/scaf"
)

func TestDialect_Name(t *testing.T) {
	d := setupIntegrationTest(t)

	defer func() { _ = d.Close() }()

	if got := d.Name(); got != "cypher" {
		t.Errorf("Name() = %q, want %q", got, "cypher")
	}
}

func TestDialect_ImplementsInterface(_ *testing.T) {
	var _ scaf.Dialect = (*Dialect)(nil)

	var _ scaf.Transactional = (*Dialect)(nil)
}

func TestDialect_Registration(t *testing.T) {
	dialects := scaf.RegisteredDialects()

	if !slices.Contains(dialects, "cypher") {
		t.Error("cypher dialect not registered")
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

func TestDialect_Execute_Integration(t *testing.T) {
	d := setupIntegrationTest(t)

	defer func() { _ = d.Close() }()

	ctx := t.Context()

	// Clean up any existing test data
	_, _ = d.Execute(ctx, "MATCH (n:ScafTest) DELETE n", nil)

	// Create test node
	_, err := d.Execute(ctx, "CREATE (n:ScafTest {name: $name}) RETURN n", map[string]any{
		"name": "test-node",
	})
	if err != nil {
		t.Fatalf("failed to create test node: %v", err)
	}

	// Query it back
	results, err := d.Execute(ctx, "MATCH (n:ScafTest) RETURN n.name AS name", nil)
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
	_, _ = d.Execute(ctx, "MATCH (n:ScafTest) DELETE n", nil)
}

func TestDialect_Transaction_Integration(t *testing.T) {
	d := setupIntegrationTest(t)

	defer func() { _ = d.Close() }()

	ctx := t.Context()

	// Start transaction
	tx, err := d.Begin(ctx)
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
	results, err := d.Execute(ctx, "MATCH (n:ScafTxTest) RETURN n", nil)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if len(results) != 0 {
		t.Error("node should not exist after rollback")
	}
}

func setupIntegrationTest(t *testing.T) *Dialect {
	t.Helper()

	uri := os.Getenv("SCAF_NEO4J_URI")
	if uri == "" {
		t.Skip("SCAF_NEO4J_URI not set, skipping integration test")
	}

	cfg := scaf.DialectConfig{
		URI:      uri,
		Username: os.Getenv("SCAF_NEO4J_USER"),
		Password: os.Getenv("SCAF_NEO4J_PASS"),
	}

	dialect, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create dialect: %v", err)
	}

	d, ok := dialect.(*Dialect)
	if !ok {
		t.Fatal("dialect is not *Dialect")
	}

	return d
}
