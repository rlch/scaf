package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_PrepareRename_Query(t *testing.T) {
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

	// Prepare rename on query definition
	result, err := server.PrepareRename(ctx, &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8}, // On "GetUser"
		},
	})
	if err != nil {
		t.Fatalf("PrepareRename() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected prepare rename result")
	}

	// Should return range covering "GetUser"
	if result.Start.Line != 0 {
		t.Errorf("Expected line 0, got %d", result.Start.Line)
	}
}

func TestServer_Rename_Query(t *testing.T) {
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

	// Rename "GetUser" to "FindUser"
	result, err := server.Rename(ctx, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8}, // On "GetUser"
		},
		NewName: "FindUser",
	})
	if err != nil {
		t.Fatalf("Rename() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected workspace edit")
	}

	// Should have edits for this document
	edits, ok := result.Changes[uri]
	if !ok {
		t.Fatal("Expected edits for document")
	}

	// Should have 2 edits: query definition + scope reference
	if len(edits) != 2 {
		t.Fatalf("Expected 2 edits, got %d", len(edits))
	}

	for _, edit := range edits {
		if edit.NewText != "FindUser" {
			t.Errorf("Expected new text 'FindUser', got '%s'", edit.NewText)
		}
	}
}

func TestServer_Rename_Import(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup fixtures
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

	// Rename import alias
	result, err := server.Rename(ctx, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10}, // On "fixtures"
		},
		NewName: "fx",
	})
	if err != nil {
		t.Fatalf("Rename() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected workspace edit")
	}

	edits, ok := result.Changes[uri]
	if !ok {
		t.Fatal("Expected edits for document")
	}

	// Should have at least 2 edits: import alias + setup usage
	if len(edits) < 2 {
		t.Fatalf("Expected at least 2 edits, got %d", len(edits))
	}
}

func TestServer_Rename_Conflict(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `
query FindUser ` + "`MATCH (u:User) RETURN u`" + `

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

	// Try to rename to an existing query name
	_, err := server.Rename(ctx, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8}, // On "GetUser"
		},
		NewName: "FindUser", // Already exists!
	})

	// Should return an error
	if err == nil {
		t.Error("Expected error for conflicting rename")
	}
}

func TestServer_Rename_InvalidName(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User) RETURN u`" + `
`
	uri := protocol.DocumentURI("file:///test.scaf")
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    content,
		},
	})

	// Try to rename to invalid name
	_, err := server.Rename(ctx, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
		NewName: "123Invalid", // Starts with number
	})

	if err == nil {
		t.Error("Expected error for invalid name")
	}
}
