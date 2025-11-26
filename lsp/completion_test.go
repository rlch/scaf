package lsp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_Completion_QueryNames(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with queries defined
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `
query GetUsers ` + "`MATCH (u:User) RETURN u`" + `
query CreateUser ` + "`CREATE (u:User {name: $name}) RETURN u`" + `

`,
		},
	})

	// Request completion at end of file (where a scope could be declared)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Should offer keywords at top level
	hasQueryKeyword := false

	for _, item := range result.Items {
		if item.Label == "query" && item.Kind == protocol.CompletionItemKindKeyword {
			hasQueryKeyword = true

			break
		}
	}

	if !hasQueryKeyword {
		t.Error("Expected 'query' keyword in completions")
	}
}

func TestServer_Completion_Parameters(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and test scope - valid content with a partial $param
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id, name: $name}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`,
		},
	})

	// Request completion after $ (cursor after the $ character)
	// Line 4 is: \t\t$id: 1
	// Character 3 is right after the $
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 3}, // After the $
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug: print all items
	t.Logf("Got %d completion items", len(result.Items))

	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should offer parameters from the query
	paramLabels := make(map[string]bool)

	for _, item := range result.Items {
		if item.Kind == protocol.CompletionItemKindVariable {
			paramLabels[item.Label] = true
		}
	}

	if !paramLabels["$id"] {
		t.Errorf("Expected $id parameter completion, got: %v", paramLabels)
	}

	if !paramLabels["$name"] {
		t.Errorf("Expected $name parameter completion, got: %v", paramLabels)
	}
}

func TestServer_Completion_ReturnFields(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query returning fields
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u.name AS name, u.email AS email`" + `

GetUser {
	test "finds user" {
		$id: 1
		name: "Alice"
	}
}
`,
		},
	})

	// Request completion at start of line for return field suggestions
	// Line 5 is: \t\tname: "Alice"
	// Character 2 is at the start of the identifier (after tabs)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 5, Character: 2}, // Start of line content
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug: print all items
	t.Logf("Got %d completion items for return fields", len(result.Items))

	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v, detail=%s)", item.Label, item.Kind, item.Detail)
	}

	// Should offer return fields from the query
	fieldLabels := make(map[string]bool)

	for _, item := range result.Items {
		if item.Kind == protocol.CompletionItemKindField {
			fieldLabels[item.Label] = true
		}
	}

	// When typing at start of line, we should get all fields
	// Note: with prefix 'n', only 'name' would be returned
	if !fieldLabels["name"] && !fieldLabels["email"] {
		t.Errorf("Expected return field completions, got: %v", fieldLabels)
	}
	
	// Let's also test with prefix filtering by making second request after 'n'
	resultWithPrefix, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 5, Character: 3}, // After 'n'
		},
	})
	if err != nil {
		t.Fatalf("Completion() with prefix error: %v", err)
	}

	// With prefix 'n', should only get 'name'
	prefixFieldLabels := make(map[string]bool)

	for _, item := range resultWithPrefix.Items {
		if item.Kind == protocol.CompletionItemKindField {
			prefixFieldLabels[item.Label] = true
		}
	}

	if !prefixFieldLabels["name"] {
		t.Errorf("Expected 'name' with prefix 'n', got: %v", prefixFieldLabels)
	}
}

func TestServer_Completion_ImportAliases(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a valid file with imports and a complete setup line
	// The user is positioned right after "setup " to get import alias suggestions
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `import fixtures "../shared/fixtures"
import db "./setup_db"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup ` + "`CREATE (n:Node)`" + `
	test "t" {}
}
`,
		},
	})

	// Request completion at position right after "setup " (before the backtick)
	// Line 6: "\tsetup `CREATE (n:Node)`"
	// Character 7 is right after "setup "
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 7}, // After "setup "
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug output
	t.Logf("Got %d completion items for import aliases", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should offer import aliases
	aliasLabels := make(map[string]bool)

	for _, item := range result.Items {
		if item.Kind == protocol.CompletionItemKindModule {
			aliasLabels[item.Label] = true
		}
	}

	if !aliasLabels["fixtures"] {
		t.Errorf("Expected 'fixtures' alias completion, got: %v", aliasLabels)
	}

	if !aliasLabels["db"] {
		t.Errorf("Expected 'db' alias completion, got: %v", aliasLabels)
	}
}

func TestServer_Completion_Keywords_InTest(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "finds user" {
		
	}
}
`,
		},
	})

	// Request completion at start of test body
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 2},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Check we get some completion items
	if len(result.Items) == 0 {
		t.Error("Expected completion items")
	}
}

func TestServer_Completion_Capabilities(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	result, err := server.Initialize(ctx, &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Check completion capability is enabled
	if result.Capabilities.CompletionProvider == nil {
		t.Fatal("CompletionProvider capability not set")
	}

	// Check trigger characters
	triggers := result.Capabilities.CompletionProvider.TriggerCharacters
	hasDollar := false
	hasDot := false

	for _, c := range triggers {
		if c == "$" {
			hasDollar = true
		}

		if c == "." {
			hasDot = true
		}
	}

	if !hasDollar {
		t.Error("Expected '$' as trigger character")
	}

	if !hasDot {
		t.Error("Expected '.' as trigger character")
	}
}

func TestServer_Completion_NoDocument(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Request completion for non-existent document
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.scaf"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	// Should return nil for non-existent document
	if result != nil {
		t.Error("Expected nil result for non-existent document")
	}
}

func TestServer_Completion_FilterByPrefix(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with queries
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User) RETURN u`" + `
query GetPosts ` + "`MATCH (p:Post) RETURN p`" + `
query CreateUser ` + "`CREATE (u:User) RETURN u`" + `

Get
`,
		},
	})

	// Request completion with prefix "Get"
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 3}, // After "Get"
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// All items should start with "Get" (if filtering works)
	for _, item := range result.Items {
		if item.Kind == protocol.CompletionItemKindFunction {
			if !strings.HasPrefix(item.Label, "Get") {
				t.Errorf("Expected query completions to be filtered by prefix, got: %s", item.Label)
			}
		}
	}
}

func TestServer_Completion_SetupFunctions_CrossFile(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure for cross-file test
	tmpDir := t.TempDir()

	// Create fixtures.scaf with queries
	fixturesContent := `query CreateUser ` + "`CREATE (u:User {name: $name, email: $email}) RETURN u`" + `
query CreatePost ` + "`CREATE (p:Post {title: $title, authorId: $authorId}) RETURN p`" + `
query SetupDatabase ` + "`CREATE CONSTRAINT FOR (u:User) REQUIRE u.id IS UNIQUE`" + `
`
	fixturesPath := tmpDir + "/fixtures.scaf"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to create fixtures file: %v", err)
	}

	// Create main test file that imports fixtures
	// Use valid scaf syntax - the "setup fixtures." triggers completion but
	// the actual syntax needs a function call like "setup fixtures.CreateUser()"
	// For completion testing, we use a placeholder comment to make it parse
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`
	mainPath := tmpDir + "/main.scaf"
	if err := writeFile(mainPath, mainContent); err != nil {
		t.Fatalf("Failed to create main file: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize with workspace root
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: protocol.DocumentURI("file://" + tmpDir),
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open the main file
	mainURI := protocol.DocumentURI("file://" + mainPath)
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     mainURI,
			Version: 1,
			Text:    mainContent,
		},
	})

	// For testing completion, we need valid syntax that still has imports.
	// We'll request completion on a line that has "fixtures." pattern
	// where the file still parses correctly
	
	// The original content should parse correctly, so let's work with it
	// Instead, we'll test completion at the end of a line where we type "fixtures."
	// But we need the imports to be recognized first
	
	// Let's verify the document has the import by triggering import alias completion
	importAliasResult, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
		Context: &protocol.CompletionContext{
			TriggerCharacter: ".",
			TriggerKind:      protocol.CompletionTriggerKindTriggerCharacter,
		},
	})
	if err != nil {
		t.Fatalf("Import alias completion error: %v", err)
	}
	t.Logf("Import alias completion at line 0: %d items", len(importAliasResult.Items))
	for _, item := range importAliasResult.Items {
		t.Logf("  Alias item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Now update with content that has fixtures. on a line but is still valid
	// We'll use setup block syntax which can contain setup calls
	validContentWithSetup := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup {
		fixtures.CreateUser($name: "test", $email: "test@example.com")
	}
	test "finds user" {
		$id: 1
	}
}
`
	// Update with content that includes "fixtures." usage
	_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: mainURI},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: validContentWithSetup},
		},
	})

	// Line 6 in validContentWithSetup is: "\t\tfixtures.CreateUser($name: "test", $email: "test@example.com")"
	// Request completion after "fixtures." at character 11 (after the dot)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 6, Character: 11}, // After "fixtures."
		},
		Context: &protocol.CompletionContext{
			TriggerCharacter: ".",
			TriggerKind:      protocol.CompletionTriggerKindTriggerCharacter,
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug: print all items
	t.Logf("Got %d completion items for setup functions", len(result.Items))

	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v, detail=%s)", item.Label, item.Kind, item.Detail)
	}

	// Should offer queries from fixtures module
	queryLabels := make(map[string]bool)

	for _, item := range result.Items {
		if item.Kind == protocol.CompletionItemKindFunction {
			queryLabels[item.Label] = true
		}
	}

	// Check we got the expected queries
	if !queryLabels["CreateUser"] {
		t.Errorf("Expected CreateUser query completion, got: %v", queryLabels)
	}

	if !queryLabels["CreatePost"] {
		t.Errorf("Expected CreatePost query completion, got: %v", queryLabels)
	}

	if !queryLabels["SetupDatabase"] {
		t.Errorf("Expected SetupDatabase query completion, got: %v", queryLabels)
	}
}

// writeFile is a test helper to write content to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644) //nolint:gosec // Test helper
}

func TestServer_Completion_SetupImportAlias_InScope(t *testing.T) {
	t.Parallel()

	// This test simulates typing "setup " inside a scope.
	// We first open a valid document, then simulate a change to add the incomplete line.
	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// First, open a valid document
	validContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    validContent,
		},
	})

	// Now simulate typing "setup " - document becomes temporarily invalid
	invalidContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	setup 
	test "t" {}
}
`
	_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: invalidContent},
		},
	})

	// Request completion after "setup " (line 5, character 7)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 5, Character: 7},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug
	t.Logf("Got %d completion items", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should offer import aliases
	found := false
	for _, item := range result.Items {
		if item.Label == "fixtures" && item.Kind == protocol.CompletionItemKindModule {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'fixtures' import alias in completions")
	}
}

func TestServer_Completion_SetupWithPrefix(t *testing.T) {
	t.Parallel()

	// This test simulates the EXACT scenario that fails in real usage:
	// User types "setup fi" and expects to see "fixtures" filtered
	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// First, open a valid document
	validContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    validContent,
		},
	})

	// Now simulate typing "setup fi" inside the test
	invalidContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		setup fi
	}
}
`
	_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: invalidContent},
		},
	})

	// Request completion after "setup fi" (line 6, character 10)
	// Line 6 is: "\t\tsetup fi"
	// Character 10 is at the end after "fi"
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 10},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug
	t.Logf("Got %d completion items", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should offer "fixtures" import alias (filtered by prefix "fi")
	found := false
	for _, item := range result.Items {
		if item.Label == "fixtures" && item.Kind == protocol.CompletionItemKindModule {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'fixtures' import alias in completions when typing 'setup fi'")
	}
}

func TestServer_Completion_SetupWithPrefix_FileNeverValid(t *testing.T) {
	t.Parallel()

	// This test simulates a scenario where the file was NEVER valid:
	// User opens a file that already has parse errors, no LastValidAnalysis
	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file that's already invalid (no LastValidAnalysis will be stored)
	invalidContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		setup fi
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    invalidContent,
		},
	})

	// Request completion after "setup fi" (line 6, character 10)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 10},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug
	t.Logf("Got %d completion items (file never valid)", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should still offer "fixtures" import alias even when file never parsed correctly
	// This requires the partial analysis to still extract imports
	found := false
	for _, item := range result.Items {
		if item.Label == "fixtures" && item.Kind == protocol.CompletionItemKindModule {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'fixtures' import alias in completions even when file never valid")
	}
}

func TestServer_Completion_SetupImportAlias_InTest(t *testing.T) {
	t.Parallel()

	// Simulates typing "setup " inside a test - uses LastValidAnalysis fallback
	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// First, open a valid document
	validContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    validContent,
		},
	})

	// Now simulate typing "setup " inside the test
	invalidContent := `import fixtures "../shared/fixtures"

query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		setup 
	}
}
`
	_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: invalidContent},
		},
	})

	// Request completion after "setup " (line 6, character 8)
	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 8},
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Debug
	t.Logf("Got %d completion items", len(result.Items))
	for _, item := range result.Items {
		t.Logf("  Item: %s (kind=%v)", item.Label, item.Kind)
	}

	// Should offer import aliases
	found := false
	for _, item := range result.Items {
		if item.Label == "fixtures" && item.Kind == protocol.CompletionItemKindModule {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'fixtures' import alias in completions")
	}
}

// completionTestCase represents a test case for completion using ^ as cursor position.
type completionTestCase struct {
	name     string
	content  string // Use ^ to mark cursor position
	wantKind protocol.CompletionItemKind
	wantAny  []string // At least one of these labels should be present
	wantAll  []string // All of these labels should be present
	wantNone []string // None of these labels should be present
}

// parseContentWithCursor parses test content with ^ as cursor marker.
// Returns the content without the marker and the cursor position.
func parseContentWithCursor(content string) (string, protocol.Position) {
	lines := strings.Split(content, "\n")
	var line, char uint32

	for i, l := range lines {
		if idx := strings.Index(l, "^"); idx >= 0 {
			line = uint32(i)
			char = uint32(idx)
			// Remove the ^ from the line, keeping everything else
			lines[i] = l[:idx] + l[idx+1:]
			break
		}
	}

	return strings.Join(lines, "\n"), protocol.Position{Line: line, Character: char}
}

// runCompletionTest runs a completion test case.
func runCompletionTest(t *testing.T, tc completionTestCase) {
	t.Helper()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content, pos := parseContentWithCursor(tc.content)

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	result, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("Completion() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected completion result")
	}

	// Build label set
	labels := make(map[string]protocol.CompletionItemKind)
	for _, item := range result.Items {
		labels[item.Label] = item.Kind
	}

	// Check wantAny
	if len(tc.wantAny) > 0 {
		found := false
		for _, want := range tc.wantAny {
			if _, ok := labels[want]; ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected at least one of %v in completions, got: %v", tc.wantAny, labels)
		}
	}

	// Check wantAll
	for _, want := range tc.wantAll {
		if _, ok := labels[want]; !ok {
			t.Errorf("Expected %q in completions, got: %v", want, labels)
		}
	}

	// Check wantNone
	for _, notWant := range tc.wantNone {
		if _, ok := labels[notWant]; ok {
			t.Errorf("Did not expect %q in completions, got: %v", notWant, labels)
		}
	}

	// Check kind if specified
	if tc.wantKind != 0 && len(tc.wantAll) > 0 {
		for _, want := range tc.wantAll {
			if kind, ok := labels[want]; ok && kind != tc.wantKind {
				t.Errorf("Expected %q to have kind %v, got %v", want, tc.wantKind, kind)
			}
		}
	}
}
