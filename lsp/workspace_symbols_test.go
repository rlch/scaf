package lsp_test

import (
	"context"
	"os"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_Symbols(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create first file
	file1Path := tmpDir + "/queries.scaf"
	file1Content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `
query GetPost ` + "`MATCH (p:Post {id: $id}) RETURN p`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`
	if err := os.WriteFile(file1Path, []byte(file1Content), 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}

	// Create second file
	file2Path := tmpDir + "/more_queries.scaf"
	file2Content := `query CountUsers ` + "`MATCH (u:User) RETURN count(u)`" + `

CountUsers {
	test "counts all users" {}
	group "edge cases" {
		test "empty database" {}
	}
}
`
	if err := os.WriteFile(file2Path, []byte(file2Content), 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Search for all symbols (empty query)
	result, err := server.Symbols(ctx, &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatalf("Symbols() error: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("Expected symbols from workspace")
	}

	// Log all symbols found
	symbolNames := make(map[string]bool)
	for _, sym := range result {
		t.Logf("Symbol: %s (%s) in %s [container: %s]", sym.Name, sym.Kind, sym.Location.URI, sym.ContainerName)
		symbolNames[sym.Name] = true
	}

	// Should find queries from both files
	expectedQueries := []string{"GetUser", "GetPost", "CountUsers"}
	for _, name := range expectedQueries {
		if !symbolNames[name] {
			t.Errorf("Expected to find query %q", name)
		}
	}

	// Should find tests
	expectedTests := []string{"finds user", "counts all users", "empty database"}
	for _, name := range expectedTests {
		if !symbolNames[name] {
			t.Errorf("Expected to find test %q", name)
		}
	}
}

func TestServer_Symbols_FilterByQuery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	filePath := tmpDir + "/test.scaf"
	fileContent := `query GetUser ` + "`Q`" + `
query GetPost ` + "`Q`" + `
query CountUsers ` + "`Q`" + `

GetUser {
	test "finds user" {}
}
`
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Search for "user" (case-insensitive)
	result, err := server.Symbols(ctx, &protocol.WorkspaceSymbolParams{
		Query: "user",
	})
	if err != nil {
		t.Fatalf("Symbols() error: %v", err)
	}

	// Should find GetUser, CountUsers, and "finds user" test
	if len(result) < 3 {
		t.Errorf("Expected at least 3 symbols matching 'user', got %d", len(result))
	}

	for _, sym := range result {
		t.Logf("Matched: %s (%s)", sym.Name, sym.Kind)
	}
}

func TestServer_Symbols_NoWorkspace(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize without workspace root
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	result, err := server.Symbols(ctx, &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatalf("Symbols() error: %v", err)
	}

	// Should return nil/empty when no workspace
	if result != nil && len(result) > 0 {
		t.Error("Expected nil or empty result without workspace")
	}
}
