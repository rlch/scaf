package lsp_test

import (
	"context"
	"os"
	"testing"

	"go.lsp.dev/protocol"
)

// TestServer_Definition_QueryScope tests go-to-definition from query scope name
// to the query definition.
func TestServer_Definition_QueryScope(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and a scope referencing it
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

	// Request definition on the query scope name "GetUser" (line 2, character 3)
	// Line 2 is "GetUser {" (0-indexed)
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 3}, // On "GetUser"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	if loc.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, loc.URI)
	}

	// Should point to line 0 (query definition)
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	// Should point to the query name "GetUser" (after "query ")
	// "query " = 6 chars, so column should be 6
	if loc.Range.Start.Character != 6 {
		t.Errorf("Expected definition at character 6, got %d", loc.Range.Start.Character)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_LocalNamedSetup tests go-to-definition from a local
// named setup call to the query definition.
func TestServer_Definition_LocalNamedSetup(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and a scope that uses it as a setup
	content := `query SetupUser ` + "`CREATE (u:User {name: $name}) RETURN u`" + `
query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	setup SetupUser($name: "test")
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

	// Request definition on "SetupUser" in the setup clause (line 4)
	// Line 4 is "\tsetup SetupUser($name: "test")"
	// "	setup " = 7 chars (tab + "setup "), so SetupUser starts at char 7
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 8}, // On "SetupUser"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	if loc.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, loc.URI)
	}

	// Should point to line 0 (SetupUser query definition)
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_ImportAlias tests go-to-definition from an import alias
// in a named setup call to the import statement.
func TestServer_Definition_ImportAlias(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	setup fixtures.CreateUser($name: "test")
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

	// Request definition on "fixtures" in the setup clause (line 5)
	// Line 5 is "\tsetup fixtures.CreateUser($name: "test")"
	// "\tsetup " = 7 chars, so "fixtures" starts at char 7
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 5, Character: 8}, // On "fixtures"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	if loc.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, loc.URI)
	}

	// Should point to line 0 (import statement)
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_CrossFile_NamedSetup tests go-to-definition from a
// module-prefixed named setup call to the query in the imported file.
func TestServer_Definition_CrossFile_NamedSetup(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure for cross-file test
	tmpDir := t.TempDir()

	// Create fixtures.scaf with queries
	fixturesContent := `query CreateUser ` + "`CREATE (u:User {name: $name}) RETURN u`" + `
query CreatePost ` + "`CREATE (p:Post {title: $title}) RETURN p`" + `
`
	fixturesPath := tmpDir + "/fixtures.scaf"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to create fixtures file: %v", err)
	}

	// Create main file that imports fixtures
	mainContent := `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	setup fixtures.CreateUser($name: "test")
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

	// Request definition on "CreateUser" in the setup clause (line 5)
	// Line 5 is "\tsetup fixtures.CreateUser($name: "test")"
	// "\tsetup fixtures." = 17 chars, so "CreateUser" starts at char 17
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 5, Character: 18}, // On "CreateUser"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	fixturesURI := protocol.DocumentURI("file://" + fixturesPath)
	if loc.URI != fixturesURI {
		t.Errorf("Expected URI %s, got %s", fixturesURI, loc.URI)
	}

	// Should point to line 0 (CreateUser query in fixtures.scaf)
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: %s line %d, char %d-%d",
		loc.URI, loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_NoResult tests that definition returns empty
// when cursor is not on a navigable symbol.
func TestServer_Definition_NoResult(t *testing.T) {
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

	// Request definition on "test" keyword (should return no results)
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 2}, // On "test"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected no results for keyword, got %d", len(result))
	}
}

// TestServer_Definition_NoDocument tests that definition returns nil
// when the document doesn't exist.
func TestServer_Definition_NoDocument(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Request definition on non-existent document
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.scaf"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if result != nil {
		t.Errorf("Expected nil result for non-existent document, got %v", result)
	}
}

// TestServer_Definition_MultipleQueries tests go-to-definition
// with multiple queries to ensure we go to the correct one.
func TestServer_Definition_MultipleQueries(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User) RETURN u`" + `
query GetPost ` + "`MATCH (p:Post) RETURN p`" + `
query GetComment ` + "`MATCH (c:Comment) RETURN c`" + `

GetPost {
	test "finds post" {
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

	// Request definition on "GetPost" scope reference (line 4)
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "GetPost"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	// Should point to line 1 (GetPost query definition)
	if loc.Range.Start.Line != 1 {
		t.Errorf("Expected definition on line 1, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_Parameter tests go-to-definition from a parameter
// in a test statement to the parameter usage in the query body.
func TestServer_Definition_Parameter(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Query with parameter on same line
	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
		u.name: "alice"
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

	// Request definition on "$id" in the test (line 4)
	// Line 4 is "\t\t$id: 1"
	// "\t\t" = 2 chars, "$id" starts at char 2
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "$id"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	if loc.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, loc.URI)
	}

	// Should point to line 0 (query definition line)
	// The parameter $id is in the query body
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)

	// Verify the range covers "$id" (3 characters)
	rangeLen := loc.Range.End.Character - loc.Range.Start.Character
	if rangeLen != 3 {
		t.Errorf("Expected range length 3, got %d", rangeLen)
	}
}

// TestServer_Definition_Parameter_MultipleUses tests go-to-definition when a
// parameter is used multiple times in the query (should go to first occurrence).
func TestServer_Definition_Parameter_MultipleUses(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUserWithManager ` + "`MATCH (u:User {id: $id})-[:REPORTS_TO]->(m:User {id: $id}) RETURN u`" + `

GetUserWithManager {
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

	// Line 4 is "\t\t$id: 1"
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 3},
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	// Should go to the FIRST occurrence of $id in the query
	loc := result[0]
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_Parameter_NotFound tests that definition returns empty
// when the parameter doesn't exist in the query.
func TestServer_Definition_Parameter_NotFound(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$unknownParam: 1
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

	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "$unknownParam"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	// Should return no results since $unknownParam isn't in the query
	if len(result) != 0 {
		t.Errorf("Expected no results for unknown parameter, got %d", len(result))
	}
}

// TestServer_Definition_InTestSetup tests go-to-definition from a setup
// inside a test block.
func TestServer_Definition_InTestSetup(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query SetupData ` + "`CREATE (d:Data) RETURN d`" + `
query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "with setup" {
		setup SetupData()
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

	// Request definition on "SetupData" in test setup (line 5)
	// Line 5 is "\t\tsetup SetupData()"
	// "\t\tsetup " = 8 chars, so "SetupData" starts at char 8
	result, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 5, Character: 9}, // On "SetupData"
		},
	})
	if err != nil {
		t.Fatalf("Definition() error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result))
	}

	loc := result[0]
	// Should point to line 0 (SetupData query definition)
	if loc.Range.Start.Line != 0 {
		t.Errorf("Expected definition on line 0, got line %d", loc.Range.Start.Line)
	}

	t.Logf("Definition location: line %d, char %d-%d",
		loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
}

// TestServer_Definition_AllContexts is a comprehensive test that validates
// go-to-definition works for all different contexts similar to with_imports.scaf.
// This tests:
// - Query scope name -> query definition
// - Global setup import alias -> import statement
// - Global setup function name -> query in imported file
// - Scope setup import alias -> import statement
// - Scope setup function name -> query in imported file
// - Test setup with import -> query in imported file
// - Assert query name -> query definition
// - Parameters -> parameter in query body
func TestServer_Definition_AllContexts(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf with setup queries (mirrors example/basic/shared/fixtures.cypher.scaf)
	fixturesContent := `// Shared fixtures
query SetupUsers ` + "`" + `
CREATE (alice:User {id: 1, name: "Alice", email: "alice@example.com", age: 30, verified: true})
CREATE (bob:User {id: 2, name: "Bob", email: "bob@example.com", age: 25, verified: false})
` + "`" + `

query SetupPosts ` + "`" + `
CREATE (p:Post {id: $postId, title: $title, authorId: $authorId, views: 0})
` + "`" + `

query SetupCleanDB ` + "`" + `
MATCH (n) DETACH DELETE n
` + "`" + `
`
	fixturesDir := tmpDir + "/shared"
	if err := mkdirAll(fixturesDir); err != nil {
		t.Fatalf("Failed to create shared dir: %v", err)
	}
	fixturesPath := fixturesDir + "/fixtures.scaf"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to create fixtures file: %v", err)
	}

	// Create main file similar to with_imports.scaf
	// Line numbers (0-indexed):
	// 0: import fixtures "./shared/fixtures"
	// 1: (empty)
	// 2: query GetUser `...`
	// 3: (empty)
	// 4: query CountUserPosts `...`
	// 5: (empty)
	// 6: setup fixtures.SetupCleanDB()
	// 7: (empty)
	// 8: GetUser {
	// 9:   setup fixtures.SetupUsers()
	// 10:  group "grouped tests" {
	// 11:    setup fixtures.SetupUsers()
	// 12:    test "finds user" {
	// 13:      setup fixtures.SetupPosts($postId: 1, $title: "Hello", $authorId: 1)
	// 14:      $userId: 1
	// 15:      u.name: "Alice"
	// 16:      assert CountUserPosts($authorId: u.id) { postCount == 0 }
	// 17:    }
	// 18:  }
	// 19: }
	mainContent := `import fixtures "./shared/fixtures"

query GetUser ` + "`MATCH (u:User {id: $userId}) RETURN u.id, u.name, u.email`" + `

query CountUserPosts ` + "`MATCH (p:Post {authorId: $authorId}) RETURN count(p) as postCount`" + `

setup fixtures.SetupCleanDB()

GetUser {
	setup fixtures.SetupUsers()
	group "grouped tests" {
		setup fixtures.SetupUsers()
		test "finds user" {
			setup fixtures.SetupPosts($postId: 1, $title: "Hello", $authorId: 1)
			$userId: 1
			u.name: "Alice"
			assert CountUserPosts($authorId: u.id) { postCount == 0 }
		}
	}
}
`
	mainPath := tmpDir + "/main.scaf"
	if err := writeFile(mainPath, mainContent); err != nil {
		t.Fatalf("Failed to create main file: %v", err)
	}

	server, _ := newTestServerWithDebug(t)
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

	fixturesURI := protocol.DocumentURI("file://" + fixturesPath)

	// Define test cases
	tests := []struct {
		name         string
		line         uint32
		character    uint32
		wantURI      protocol.DocumentURI
		wantLine     uint32
		wantFound    bool
		description  string
	}{
		// 1. Query scope name -> query definition
		{
			name:        "query_scope_name",
			line:        8,  // "GetUser {"
			character:   3,  // On "GetUser"
			wantURI:     mainURI,
			wantLine:    2,  // query GetUser line
			wantFound:   true,
			description: "Query scope name should go to query definition",
		},

		// 2. Global setup - import alias
		{
			name:        "global_setup_import_alias",
			line:        6,  // "setup fixtures.SetupCleanDB()"
			character:   7,  // On "fixtures"
			wantURI:     mainURI,
			wantLine:    0,  // import line
			wantFound:   true,
			description: "Import alias in global setup should go to import statement",
		},

		// 3. Global setup - function name (cross-file)
		{
			name:        "global_setup_function_name",
			line:        6,  // "setup fixtures.SetupCleanDB()"
			character:   17, // On "SetupCleanDB"
			wantURI:     fixturesURI,
			wantLine:    10, // query SetupCleanDB line in fixtures (0-indexed)
			wantFound:   true,
			description: "Function name in global setup should go to query in imported file",
		},

		// 4. Scope setup - import alias
		{
			name:        "scope_setup_import_alias",
			line:        9,  // "\tsetup fixtures.SetupUsers()"
			character:   7,  // On "fixtures"
			wantURI:     mainURI,
			wantLine:    0,  // import line
			wantFound:   true,
			description: "Import alias in scope setup should go to import statement",
		},

		// 5. Scope setup - function name (cross-file)
		{
			name:        "scope_setup_function_name",
			line:        9,  // "\tsetup fixtures.SetupUsers()"
			character:   17, // On "SetupUsers"
			wantURI:     fixturesURI,
			wantLine:    1,  // query SetupUsers line in fixtures (0-indexed, after comment)
			wantFound:   true,
			description: "Function name in scope setup should go to query in imported file",
		},

		// 6. Group setup - import alias
		{
			name:        "group_setup_import_alias",
			line:        11, // "\t\tsetup fixtures.SetupUsers()"
			character:   9,  // On "fixtures"
			wantURI:     mainURI,
			wantLine:    0,  // import line
			wantFound:   true,
			description: "Import alias in group setup should go to import statement",
		},

		// 7. Group setup - function name (cross-file)
		{
			name:        "group_setup_function_name",
			line:        11, // "\t\tsetup fixtures.SetupUsers()"
			character:   18, // On "SetupUsers"
			wantURI:     fixturesURI,
			wantLine:    1,  // query SetupUsers line in fixtures
			wantFound:   true,
			description: "Function name in group setup should go to query in imported file",
		},

		// 8. Test setup with params - import alias
		{
			name:        "test_setup_import_alias",
			line:        13, // "\t\t\tsetup fixtures.SetupPosts(...)"
			character:   10, // On "fixtures"
			wantURI:     mainURI,
			wantLine:    0,  // import line
			wantFound:   true,
			description: "Import alias in test setup should go to import statement",
		},

		// 9. Test setup with params - function name (cross-file)
		{
			name:        "test_setup_function_name",
			line:        13, // "\t\t\tsetup fixtures.SetupPosts(...)"
			character:   19, // On "SetupPosts"
			wantURI:     fixturesURI,
			wantLine:    6,  // query SetupPosts line in fixtures
			wantFound:   true,
			description: "Function name in test setup should go to query in imported file",
		},

		// 10. Parameter in test -> parameter in query
		{
			name:        "parameter_in_test",
			line:        14, // "\t\t\t$userId: 1"
			character:   4,  // On "$userId"
			wantURI:     mainURI,
			wantLine:    2,  // query GetUser line (where $userId is used)
			wantFound:   true,
			description: "Parameter in test should go to parameter in query body",
		},

		// 11. Assert query name -> query definition
		{
			name:        "assert_query_name",
			line:        16, // "\t\t\tassert CountUserPosts(...)"
			character:   11, // On "CountUserPosts"
			wantURI:     mainURI,
			wantLine:    4,  // query CountUserPosts line
			wantFound:   true,
			description: "Query name in assert should go to query definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := server.Definition(ctx, &protocol.DefinitionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
					Position:     protocol.Position{Line: tt.line, Character: tt.character},
				},
			})
			if err != nil {
				t.Fatalf("Definition() error: %v", err)
			}

			if tt.wantFound {
				if len(result) != 1 {
					t.Fatalf("%s: expected 1 location, got %d", tt.description, len(result))
				}

				loc := result[0]
				if loc.URI != tt.wantURI {
					t.Errorf("%s: expected URI %s, got %s", tt.description, tt.wantURI, loc.URI)
				}
				if loc.Range.Start.Line != tt.wantLine {
					t.Errorf("%s: expected line %d, got line %d", tt.description, tt.wantLine, loc.Range.Start.Line)
				}

				t.Logf("%s: found at %s line %d, char %d-%d",
					tt.name, loc.URI, loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
			} else {
				if len(result) != 0 {
					t.Errorf("%s: expected no results, got %d", tt.description, len(result))
				}
			}
		})
	}
}

// mkdirAll is a test helper to create directories.
func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}
