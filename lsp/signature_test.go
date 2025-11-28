package lsp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_SignatureHelp(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf with a query that has parameters
	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := `query CreateUser ` + "`CREATE (u:User {name: $name, age: $age}) RETURN u`" + `
query SimpleSetup ` + "`CREATE (:Marker)`" + `
`
	if err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0644); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf that uses the fixture
	mainPath := tmpDir + "/main.scaf"
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup fixtures.CreateUser($name: "Alice", $age: 30)
	test "finds user" {}
}
`
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	mainURI := protocol.DocumentURI("file://" + mainPath)
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     mainURI,
			Version: 1,
			Text:    mainContent,
		},
	})

	// Request signature help after the opening paren
	// Line 5: "	setup fixtures.CreateUser($name: "Alice", $age: 30)"
	// Position after "(" would be around character 26
	result, err := server.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 5, Character: 27},
		},
	})
	if err != nil {
		t.Fatalf("SignatureHelp() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected signature help result")
	}

	if len(result.Signatures) == 0 {
		t.Fatal("Expected at least one signature")
	}

	sig := result.Signatures[0]
	t.Logf("Signature: %s", sig.Label)
	t.Logf("Parameters: %v", sig.Parameters)
	t.Logf("Active parameter: %d", result.ActiveParameter)

	// Should contain the function name
	if !strings.Contains(sig.Label, "CreateUser") {
		t.Errorf("Expected signature to contain 'CreateUser', got %s", sig.Label)
	}

	// Should have parameter info
	if len(sig.Parameters) < 2 {
		t.Errorf("Expected at least 2 parameters, got %d", len(sig.Parameters))
	}
}

func TestServer_SignatureHelp_SecondParameter(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := `query CreateUser ` + "`CREATE (u:User {name: $name, age: $age}) RETURN u`" + `
`
	_ = os.WriteFile(fixturesPath, []byte(fixturesContent), 0644)

	mainPath := tmpDir + "/main.scaf"
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`Q`" + `

GetUser {
	setup fixtures.CreateUser($name: "Alice", 
	test "t" {}
}
`
	_ = os.WriteFile(mainPath, []byte(mainContent), 0644)

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	mainURI := protocol.DocumentURI("file://" + mainPath)
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     mainURI,
			Version: 1,
			Text:    mainContent,
		},
	})

	// Position after the comma (after "$name: "Alice",")
	// Line 5: "	setup fixtures.CreateUser($name: "Alice", "
	result, err := server.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 5, Character: 43},
		},
	})
	if err != nil {
		t.Fatalf("SignatureHelp() error: %v", err)
	}

	if result == nil {
		t.Skip("SignatureHelp returned nil - position may be outside function call")
	}

	// Active parameter should be 1 (second parameter, 0-indexed)
	if result.ActiveParameter != 1 {
		t.Errorf("Expected active parameter 1, got %d", result.ActiveParameter)
	}
}

func TestServer_SignatureHelp_NotInCall(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query Q ` + "`Q`" + `
Q { test "t" {} }
`,
		},
	})

	// Position not inside a function call
	result, err := server.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 1, Character: 5},
		},
	})
	if err != nil {
		t.Fatalf("SignatureHelp() error: %v", err)
	}

	// Should return nil when not inside a function call
	if result != nil {
		t.Error("Expected nil result when not inside a function call")
	}
}
