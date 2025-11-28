package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_DocumentHighlight_Query(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and multiple references
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

	// Request highlight on query definition "GetUser" (line 0, column 6)
	result, err := server.DocumentHighlight(ctx, &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8}, // On "GetUser"
		},
	})
	if err != nil {
		t.Fatalf("DocumentHighlight() error: %v", err)
	}

	// Should have 2 highlights: definition + scope reference
	if len(result) != 2 {
		t.Fatalf("Expected 2 highlights, got %d", len(result))
	}

	// First should be the definition (Write)
	hasDefinition := false
	hasReference := false
	for _, h := range result {
		if h.Range.Start.Line == 0 && h.Kind == protocol.DocumentHighlightKindWrite {
			hasDefinition = true
		}
		if h.Range.Start.Line == 2 && h.Kind == protocol.DocumentHighlightKindRead {
			hasReference = true
		}
	}

	if !hasDefinition {
		t.Error("Expected highlight for query definition")
	}
	if !hasReference {
		t.Error("Expected highlight for scope reference")
	}
}

func TestServer_DocumentHighlight_Import(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an import and usages
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

	// Request highlight on import "fixtures" (line 0)
	result, err := server.DocumentHighlight(ctx, &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10}, // On "fixtures"
		},
	})
	if err != nil {
		t.Fatalf("DocumentHighlight() error: %v", err)
	}

	// Should have 2 highlights: import definition + setup usage
	if len(result) < 2 {
		t.Fatalf("Expected at least 2 highlights, got %d", len(result))
	}
}

func TestServer_DocumentHighlight_Parameter(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "test1" {
		$id: 1
	}
	test "test2" {
		$id: 2
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

	// Request highlight on $id in first test
	result, err := server.DocumentHighlight(ctx, &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "$id"
		},
	})
	if err != nil {
		t.Fatalf("DocumentHighlight() error: %v", err)
	}

	// Should highlight both $id occurrences in the scope
	if len(result) < 2 {
		t.Errorf("Expected at least 2 highlights for $id, got %d", len(result))
	}
}
