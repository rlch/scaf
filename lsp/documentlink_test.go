package lsp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_DocumentLink(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf
	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := `query SetupUsers ` + "`CREATE (u:User) RETURN u`" + `
`
	if err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0644); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf with import
	mainPath := tmpDir + "/main.scaf"
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup fixtures.SetupUsers()
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

	result, err := server.DocumentLink(ctx, &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
	})
	if err != nil {
		t.Fatalf("DocumentLink() error: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("Expected at least one document link for import")
	}

	// Check the link
	link := result[0]
	t.Logf("Link: range=%v, target=%s, tooltip=%s", link.Range, link.Target, link.Tooltip)

	// Link should be on line 0 (first line with import)
	if link.Range.Start.Line != 0 {
		t.Errorf("Expected link on line 0, got line %d", link.Range.Start.Line)
	}

	// Target should point to fixtures.scaf
	if !strings.Contains(string(link.Target), "fixtures") {
		t.Errorf("Expected target to contain 'fixtures', got %s", link.Target)
	}

	// Tooltip should mention the import path
	if !strings.Contains(link.Tooltip, "./fixtures") {
		t.Errorf("Expected tooltip to contain './fixtures', got %s", link.Tooltip)
	}
}

func TestServer_DocumentLink_WithAlias(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	_ = os.WriteFile(tmpDir+"/shared.scaf", []byte(`query Q `+"`Q`"+`
`), 0644)

	mainPath := tmpDir + "/main.scaf"
	mainContent := `import f "./shared"

query GetUser ` + "`Q`" + `
GetUser { test "t" {} }
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

	result, err := server.DocumentLink(ctx, &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
	})
	if err != nil {
		t.Fatalf("DocumentLink() error: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("Expected document link for aliased import")
	}

	link := result[0]
	t.Logf("Link with alias: range=%v, target=%s", link.Range, link.Target)

	// Target should resolve to shared.scaf
	if !strings.Contains(string(link.Target), "shared") {
		t.Errorf("Expected target to contain 'shared', got %s", link.Target)
	}
}

func TestServer_DocumentLink_NoImports(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    `query Q ` + "`Q`" + ` Q { test "t" {} }`,
		},
	})

	result, err := server.DocumentLink(ctx, &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
	})
	if err != nil {
		t.Fatalf("DocumentLink() error: %v", err)
	}

	// Should return empty for file without imports
	if len(result) != 0 {
		t.Errorf("Expected no links for file without imports, got %d", len(result))
	}
}
