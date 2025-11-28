package lsp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_References_Query(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`
	uri := protocol.DocumentURI("file:///test.scaf")
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    content,
		},
	})

	// Request references on query scope "GetUser"
	result, err := server.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 3}, // On "GetUser" scope
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: true,
		},
	})
	if err != nil {
		t.Fatalf("References() error: %v", err)
	}

	// Should have 2 references: definition + scope
	if len(result) != 2 {
		t.Fatalf("Expected 2 references, got %d", len(result))
	}
}

func TestServer_References_Import_CrossFile(t *testing.T) {
	t.Parallel()

	// Create temp directory with files
	tmpDir := t.TempDir()

	// Create fixtures.scaf
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")
	fixturesContent := `query CreateUser ` + "`CREATE (u:User {name: $name}) RETURN u`" + `
`
	if err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o644); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf that uses the import
	mainPath := filepath.Join(tmpDir, "main.scaf")
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup fixtures.CreateUser($name: "Alice")
	test "finds user" {
		$id: 1
	}
}
`
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open main.scaf
	mainURI := protocol.DocumentURI("file://" + mainPath)
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     mainURI,
			Version: 1,
			Text:    mainContent,
		},
	})

	// Request references on import "fixtures"
	result, err := server.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 0, Character: 10}, // On "fixtures"
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: true,
		},
	})
	if err != nil {
		t.Fatalf("References() error: %v", err)
	}

	// Should have at least 2: import statement + setup usage
	if len(result) < 2 {
		t.Fatalf("Expected at least 2 references, got %d", len(result))
	}
}

func TestServer_References_NoDeclaration(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`
	uri := protocol.DocumentURI("file:///test.scaf")
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    content,
		},
	})

	// Request references excluding declaration
	result, err := server.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8}, // On "GetUser"
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: false,
		},
	})
	if err != nil {
		t.Fatalf("References() error: %v", err)
	}

	// Should only have the scope reference, not the definition
	if len(result) != 1 {
		t.Fatalf("Expected 1 reference (excluding decl), got %d", len(result))
	}

	// The reference should be on line 2 (the scope)
	if result[0].Range.Start.Line != 2 {
		t.Errorf("Expected reference on line 2, got line %d", result[0].Range.Start.Line)
	}
}
