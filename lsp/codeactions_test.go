package lsp_test

import (
	"context"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_CodeAction_MissingParams(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// File with missing parameter
	content := `query GetUser ` + "`MATCH (u:User {id: $id, name: $name}) RETURN u`" + `

GetUser {
	test "incomplete test" {
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

	// Request code actions for the test
	result, err := server.CodeAction(ctx, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 5, Character: 0},
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 3, Character: 1},
						End:   protocol.Position{Line: 5, Character: 2},
					},
					Message: "test is missing required parameters for GetUser: $name",
					Code:    "missing-required-params",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction() error: %v", err)
	}

	// Should have at least one quick fix
	if len(result) == 0 {
		t.Fatal("Expected at least one code action")
	}

	// Find the "Add missing parameters" action
	var addParamsAction *protocol.CodeAction
	for i := range result {
		if strings.Contains(result[i].Title, "missing parameters") {
			addParamsAction = &result[i]
			break
		}
	}

	if addParamsAction == nil {
		t.Fatal("Expected 'Add missing parameters' action")
	}

	if addParamsAction.Kind != protocol.QuickFix {
		t.Errorf("Expected QuickFix kind, got %s", addParamsAction.Kind)
	}

	if addParamsAction.Edit == nil {
		t.Fatal("Expected edit in code action")
	}
}

func TestServer_CodeAction_UnusedImport(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// File with unused import
	content := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

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

	// Request code actions for the import line
	result, err := server.CodeAction(ctx, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 30},
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: 0, Character: 28},
					},
					Message: "unused import: fixtures",
					Code:    "unused-import",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction() error: %v", err)
	}

	// Find the "Remove unused import" action
	var removeAction *protocol.CodeAction
	for i := range result {
		if strings.Contains(result[i].Title, "Remove unused") {
			removeAction = &result[i]
			break
		}
	}

	if removeAction == nil {
		t.Fatal("Expected 'Remove unused import' action")
	}

	if removeAction.Edit == nil {
		t.Fatal("Expected edit in code action")
	}

	// The edit should delete the import line
	edits, ok := removeAction.Edit.Changes[uri]
	if !ok {
		t.Fatal("Expected edits for document")
	}

	if len(edits) != 1 {
		t.Fatalf("Expected 1 edit, got %d", len(edits))
	}

	if edits[0].NewText != "" {
		t.Error("Expected deletion (empty new text)")
	}
}

func TestServer_CodeAction_UndefinedQuery(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// File with undefined query reference
	content := `GetUser {
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

	// Request code actions for the undefined query scope
	result, err := server.CodeAction(ctx, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 7},
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: 4, Character: 1},
					},
					Message: "undefined query: GetUser",
					Code:    "undefined-query",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction() error: %v", err)
	}

	// Find the "Create query" action
	var createAction *protocol.CodeAction
	for i := range result {
		if strings.Contains(result[i].Title, "Create query") {
			createAction = &result[i]
			break
		}
	}

	if createAction == nil {
		t.Fatal("Expected 'Create query' action")
	}

	if createAction.Edit == nil {
		t.Fatal("Expected edit in code action")
	}

	// The edit should insert a query definition
	edits, ok := createAction.Edit.Changes[uri]
	if !ok {
		t.Fatal("Expected edits for document")
	}

	if len(edits) != 1 {
		t.Fatalf("Expected 1 edit, got %d", len(edits))
	}

	if !strings.Contains(edits[0].NewText, "query GetUser") {
		t.Errorf("Expected query template, got: %s", edits[0].NewText)
	}
}

func TestServer_CodeAction_NoDiagnostics(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Valid file with no issues
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

	// Request code actions with no diagnostics
	result, err := server.CodeAction(ctx, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 10},
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction() error: %v", err)
	}

	// Should return empty (no actions needed)
	if len(result) != 0 {
		t.Errorf("Expected no code actions, got %d", len(result))
	}
}
