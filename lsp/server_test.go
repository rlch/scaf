package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/lsp"

	// Import dialects to register their analyzers via init().
	_ "github.com/rlch/scaf/dialects/cypher"
)

// mockClient implements protocol.Client for testing.
type mockClient struct {
	diagnostics []protocol.PublishDiagnosticsParams
}

func (m *mockClient) PublishDiagnostics(_ context.Context, params *protocol.PublishDiagnosticsParams) error {
	m.diagnostics = append(m.diagnostics, *params)

	return nil
}

// Stub out remaining Client interface methods.
func (m *mockClient) Progress(context.Context, *protocol.ProgressParams) error { return nil }
func (m *mockClient) WorkDoneProgressCreate(context.Context, *protocol.WorkDoneProgressCreateParams) error {
	return nil
}
func (m *mockClient) ShowMessage(context.Context, *protocol.ShowMessageParams) error { return nil }
func (m *mockClient) ShowMessageRequest(
	context.Context, *protocol.ShowMessageRequestParams,
) (*protocol.MessageActionItem, error) {
	return nil, nil //nolint:nilnil // Mock stub returns nil for tests
}
func (m *mockClient) LogMessage(context.Context, *protocol.LogMessageParams) error { return nil }
func (m *mockClient) Telemetry(context.Context, any) error                         { return nil }
func (m *mockClient) RegisterCapability(context.Context, *protocol.RegistrationParams) error {
	return nil
}
func (m *mockClient) UnregisterCapability(context.Context, *protocol.UnregistrationParams) error {
	return nil
}
func (m *mockClient) ApplyEdit(context.Context, *protocol.ApplyWorkspaceEditParams) (bool, error) {
	return false, nil
}
func (m *mockClient) Configuration(context.Context, *protocol.ConfigurationParams) ([]any, error) {
	return nil, nil
}
func (m *mockClient) WorkspaceFolders(context.Context) ([]protocol.WorkspaceFolder, error) {
	return nil, nil
}

func newTestServer(t *testing.T) (*lsp.Server, *mockClient) {
	t.Helper()

	logger := zap.NewNop()
	client := &mockClient{}
	server := lsp.NewServer(client, logger, "cypher")

	return server, client
}

func newTestServerWithDebug(t *testing.T) (*lsp.Server, *mockClient) {
	t.Helper()

	logger, _ := zap.NewDevelopment()
	client := &mockClient{}
	server := lsp.NewServer(client, logger, "cypher")

	return server, client
}

func TestServer_Initialize(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	result, err := server.Initialize(ctx, &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Check capabilities.
	if result.Capabilities.TextDocumentSync == nil {
		t.Error("TextDocumentSync capability not set")
	}

	hoverEnabled, ok := result.Capabilities.HoverProvider.(bool)
	if !ok || !hoverEnabled {
		t.Error("HoverProvider not enabled")
	}

	// Check server info.
	if result.ServerInfo == nil || result.ServerInfo.Name != "scaf-lsp" {
		t.Error("ServerInfo not set correctly")
	}
}

func TestServer_DidOpen_ValidFile(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	// Initialize first.
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a valid file.
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
		u.name: "Alice"
	}
}
`,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Should have received diagnostics (empty for valid file).
	if len(client.diagnostics) == 0 {
		t.Fatal("Expected diagnostics to be published")
	}

	diag := client.diagnostics[0]
	if len(diag.Diagnostics) != 0 {
		t.Errorf("Expected 0 diagnostics for valid file, got %d", len(diag.Diagnostics))
	}
}

func TestServer_DidOpen_ParseError(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with parse error.
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    `query GetUser`, // Missing body.
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	if len(client.diagnostics) == 0 {
		t.Fatal("Expected diagnostics to be published")
	}

	diag := client.diagnostics[0]
	if len(diag.Diagnostics) == 0 {
		t.Error("Expected parse error diagnostic")
	}
}

func TestServer_DidOpen_SemanticError(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with undefined query reference.
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`Q`" + `

UndefinedQuery {
	test "t" {}
}
`,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	if len(client.diagnostics) == 0 {
		t.Fatal("Expected diagnostics to be published")
	}

	diag := client.diagnostics[0]
	if len(diag.Diagnostics) == 0 {
		t.Error("Expected semantic error diagnostic for undefined query")
	}

	// Check it's the right error.
	found := false

	for _, d := range diag.Diagnostics {
		if d.Code == "undefined-query" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Expected undefined-query diagnostic, got: %v", diag.Diagnostics)
	}
}

func TestServer_DidChange(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a valid file.
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query Q ` + "`Q`" + `
Q { test "t" {} }
`,
		},
	})

	initialDiagCount := len(client.diagnostics)

	// Change to invalid content.
	err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{
				URI: "file:///test.scaf",
			},
			Version: 2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: `query Q`}, // Invalid - missing body.
		},
	})
	if err != nil {
		t.Fatalf("DidChange() error: %v", err)
	}

	// Should have new diagnostics.
	if len(client.diagnostics) <= initialDiagCount {
		t.Error("Expected new diagnostics after change")
	}

	// Latest diagnostics should have errors.
	latestDiag := client.diagnostics[len(client.diagnostics)-1]
	if len(latestDiag.Diagnostics) == 0 {
		t.Error("Expected parse error after invalid change")
	}
}

func TestServer_DidClose(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file.
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    `query Q ` + "`Q`" + ` Q { test "t" {} }`,
		},
	})

	diagCountAfterOpen := len(client.diagnostics)

	// Close the file.
	err := server.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: "file:///test.scaf",
		},
	})
	if err != nil {
		t.Fatalf("DidClose() error: %v", err)
	}

	// Should publish empty diagnostics to clear them.
	if len(client.diagnostics) <= diagCountAfterOpen {
		t.Error("Expected diagnostics to be cleared on close")
	}

	latestDiag := client.diagnostics[len(client.diagnostics)-1]
	if len(latestDiag.Diagnostics) != 0 {
		t.Error("Expected empty diagnostics after close")
	}
}

func TestServer_Hover_Query(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query.
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
	}
}
`,
		},
	})

	// Hover over the query definition (line 0, column 7 = "GetUser").
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 0, Character: 7},
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	if result.Contents.Kind != protocol.Markdown {
		t.Errorf("Expected markdown content, got %s", result.Contents.Kind)
	}

	// Should contain query name and body.
	if result.Contents.Value == "" {
		t.Error("Expected hover content")
	}
}

func TestServer_Hover_NoContent(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file.
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    `query Q ` + "`Q`" + ` Q { test "t" {} }`,
		},
	})

	// Hover over whitespace/empty area.
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 100, Character: 0}, // Beyond file.
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	// Should return nil for no content.
	if result != nil {
		t.Error("Expected nil hover result for position with no content")
	}
}

func TestServer_Hover_Parameter(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and parameter usage
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id, name: $name}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
		$name: "Alice"
	}
}
`,
		},
	})

	// Hover over the $id parameter (line 4, column 3)
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 3},
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for parameter")
	}

	// Should contain parameter info
	content := result.Contents.Value
	if content == "" {
		t.Error("Expected hover content")
	}

	// Should mention the parameter name
	if !contains(content, "$id") {
		t.Errorf("Expected $id in hover, got: %s", content)
	}

	// Should show the value
	if !contains(content, "1") {
		t.Errorf("Expected value 1 in hover, got: %s", content)
	}

	t.Logf("Parameter hover content:\n%s", content)
}

func TestServer_Hover_ReturnField(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a query and return field usage
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u.name, u.email`" + `

GetUser {
	test "finds user" {
		$id: 1
		u.name: "Alice"
	}
}
`,
		},
	})

	// Hover over the u.name return field (line 5, column 3)
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 5, Character: 3},
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for return field")
	}

	content := result.Contents.Value
	if content == "" {
		t.Error("Expected hover content")
	}

	// Should mention the return field
	if !contains(content, "u.name") {
		t.Errorf("Expected u.name in hover, got: %s", content)
	}

	// Should mention it's a return field
	if !contains(content, "Return Field") {
		t.Errorf("Expected 'Return Field' in hover, got: %s", content)
	}

	t.Logf("Return field hover content:\n%s", content)
}

func TestServer_Hover_AssertQuery(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an assert query
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `query GetUser ` + "`MATCH (u:User {id: $userId}) RETURN u.name`" + `
query CountPosts ` + "`MATCH (p:Post {authorId: $authorId}) RETURN count(p) as count`" + `

GetUser {
	test "finds user" {
		$userId: 1
		assert CountPosts($authorId: 1) { count == 0 }
	}
}
`,
		},
	})

	// Hover over the CountPosts query name in assert (line 6, column 10)
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 10},
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for assert query")
	}

	content := result.Contents.Value
	if content == "" {
		t.Error("Expected hover content")
	}

	// Should show the query being asserted
	if !contains(content, "CountPosts") {
		t.Errorf("Expected CountPosts in hover, got: %s", content)
	}

	t.Logf("Assert query hover content:\n%s", content)
}

// Helper to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestServer_DocumentSymbol(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with various symbols
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `import fixtures "./fixtures"

query GetUser ` + "`MATCH (u:User {id: $userId}) RETURN u.name`" + `
query CountPosts ` + "`MATCH (p:Post) RETURN count(p)`" + `

GetUser {
	setup fixtures.CreateUser($id: 1)
	test "finds user by id" {
		$userId: 1
		u.name: "Alice"
	}
	group "edge cases" {
		test "handles null" {
			$userId: null
		}
	}
}
`,
		},
	})

	result, err := server.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
	})
	if err != nil {
		t.Fatalf("DocumentSymbol() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected document symbols")
	}

	// Should have: import, 2 queries, 1 scope
	if len(result) < 4 {
		t.Errorf("Expected at least 4 top-level symbols, got %d", len(result))
	}

	// Check symbol names
	symbolNames := make(map[string]bool)
	for _, sym := range result {
		if docSym, ok := sym.(protocol.DocumentSymbol); ok {
			symbolNames[docSym.Name] = true
			t.Logf("Symbol: %s (%s) - %s", docSym.Name, docSym.Kind, docSym.Detail)
			
			// Check nested symbols for GetUser scope
			if docSym.Name == "GetUser" && docSym.Kind == protocol.SymbolKindClass {
				for _, child := range docSym.Children {
					t.Logf("  Child: %s (%s) - %s", child.Name, child.Kind, child.Detail)
					for _, grandchild := range child.Children {
						t.Logf("    Grandchild: %s (%s) - %s", grandchild.Name, grandchild.Kind, grandchild.Detail)
					}
				}
			}
		}
	}

	// Verify expected symbols
	expected := []string{"fixtures", "GetUser", "CountPosts"}
	for _, name := range expected {
		if !symbolNames[name] {
			t.Errorf("Expected symbol %q not found", name)
		}
	}
}

func TestServer_DocumentSymbol_Empty(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Test with non-existent document
	result, err := server.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.scaf"},
	})
	if err != nil {
		t.Fatalf("DocumentSymbol() error: %v", err)
	}

	// Should return nil/empty for non-existent document
	if result != nil && len(result) > 0 {
		t.Error("Expected nil or empty result for non-existent document")
	}
}

func TestServer_Hover_SetupCall_CrossFile(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf
	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := "query SetupUsers `CREATE (u:User {name: $name}) RETURN u`\nquery SetupPosts `CREATE (p:Post {title: $title}) RETURN p`\n"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf that imports fixtures
	mainPath := tmpDir + "/main.scaf"
	mainContent := "import fixtures \"./fixtures\"\n\nquery GetUser `MATCH (u:User {id: $id}) RETURN u`\n\nGetUser {\n\tsetup fixtures.SetupUsers($name: \"test\")\n\ttest \"finds user\" {\n\t\t$id: 1\n\t}\n}\n"
	if err := writeFile(mainPath, mainContent); err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	server, _ := newTestServerWithDebug(t)
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

	// Hover over "SetupUsers" in the setup call (line 5)
	// Line 5 is: "\tsetup fixtures.SetupUsers($name: "test")"
	// "\tsetup fixtures." = 17 chars, so SetupUsers starts at char 17
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 5, Character: 18}, // On "SetupUsers"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for setup call")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show the query info, NOT the "not found" error
	if contains(content, "not found") {
		t.Errorf("Should have found the query, got: %s", content)
	}

	// Should contain the query body
	if !contains(content, "CREATE") {
		t.Errorf("Expected query body in hover, got: %s", content)
	}
}

func TestServer_Diagnostic_UndefinedSetupQuery(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf with ONLY SetupUsers, not SetupPosts
	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := "query SetupUsers `CREATE (u:User {name: $name}) RETURN u`\n"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf that imports fixtures and calls a NON-EXISTENT query
	mainPath := tmpDir + "/main.scaf"
	mainContent := "import fixtures \"./fixtures\"\n\nquery GetUser `MATCH (u:User {id: $id}) RETURN u`\n\nGetUser {\n\tsetup fixtures.NonExistentQuery($name: \"test\")\n\ttest \"finds user\" {\n\t\t$id: 1\n\t}\n}\n"
	if err := writeFile(mainPath, mainContent); err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	server, client := newTestServer(t)
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

	// Check that we got the diagnostic
	if len(client.diagnostics) == 0 {
		t.Fatal("Expected diagnostics to be published")
	}

	lastDiag := client.diagnostics[len(client.diagnostics)-1]
	t.Logf("Got %d diagnostics", len(lastDiag.Diagnostics))
	for _, d := range lastDiag.Diagnostics {
		t.Logf("  Diagnostic: %s - %s", d.Code, d.Message)
	}

	// Should have undefined-setup-query diagnostic
	found := false
	for _, d := range lastDiag.Diagnostics {
		if d.Code == "undefined-setup-query" {
			found = true
			// Should mention the undefined query and available queries
			if !contains(d.Message, "NonExistentQuery") {
				t.Errorf("Expected message to mention NonExistentQuery, got: %s", d.Message)
			}
			if !contains(d.Message, "SetupUsers") {
				t.Errorf("Expected message to mention available query SetupUsers, got: %s", d.Message)
			}
			break
		}
	}

	if !found {
		t.Error("Expected undefined-setup-query diagnostic")
	}
}

func TestServer_Diagnostic_ValidSetupQuery(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create fixtures.scaf
	fixturesPath := tmpDir + "/fixtures.scaf"
	fixturesContent := "query SetupUsers `CREATE (u:User {name: $name}) RETURN u`\n"
	if err := writeFile(fixturesPath, fixturesContent); err != nil {
		t.Fatalf("Failed to write fixtures.scaf: %v", err)
	}

	// Create main.scaf that imports fixtures and calls a VALID query
	mainPath := tmpDir + "/main.scaf"
	mainContent := "import fixtures \"./fixtures\"\n\nquery GetUser `MATCH (u:User {id: $id}) RETURN u`\n\nGetUser {\n\tsetup fixtures.SetupUsers($name: \"test\")\n\ttest \"finds user\" {\n\t\t$id: 1\n\t}\n}\n"
	if err := writeFile(mainPath, mainContent); err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	server, client := newTestServer(t)
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

	// Should NOT have undefined-setup-query diagnostic
	if len(client.diagnostics) > 0 {
		lastDiag := client.diagnostics[len(client.diagnostics)-1]
		for _, d := range lastDiag.Diagnostics {
			if d.Code == "undefined-setup-query" {
				t.Errorf("Should not have undefined-setup-query diagnostic for valid setup call, got: %s", d.Message)
			}
		}
	}
}

func TestServer_CodeLens(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with tests, groups, and scopes
	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user by id" {
		$id: 1
		u.name: "Alice"
	}
	group "edge cases" {
		test "handles null id" {
			$id: null
		}
		test "handles zero id" {
			$id: 0
		}
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	result, err := server.CodeLens(ctx, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
	})
	if err != nil {
		t.Fatalf("CodeLens() error: %v", err)
	}

	if result == nil || len(result) == 0 {
		t.Fatal("Expected code lenses")
	}

	// Count different types of lenses
	var runAll, runTest, runGroup int
	for _, lens := range result {
		if lens.Command == nil {
			t.Error("Expected lens to have a command")
			continue
		}
		t.Logf("Lens: %s at line %d, cmd=%s, args=%v",
			lens.Command.Title, lens.Range.Start.Line, lens.Command.Command, lens.Command.Arguments)
		switch lens.Command.Command {
		case "scaf.runScope":
			runAll++
		case "scaf.runTest":
			runTest++
		case "scaf.runGroup":
			runGroup++
		}
	}

	// Should have: 1 scope (GetUser), 3 tests, 1 group
	if runAll != 1 {
		t.Errorf("Expected 1 runScope lens, got %d", runAll)
	}
	if runTest != 3 {
		t.Errorf("Expected 3 runTest lenses, got %d", runTest)
	}
	if runGroup != 1 {
		t.Errorf("Expected 1 runGroup lens, got %d", runGroup)
	}
}

func TestServer_CodeLens_Ranges(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with known positions
	content := `query Q ` + "`Q`" + `

Q {
	test "first test" {}
	group "my group" {
		test "nested test" {}
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	result, err := server.CodeLens(ctx, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
	})
	if err != nil {
		t.Fatalf("CodeLens() error: %v", err)
	}

	// Check that lens ranges point to correct lines
	// Line 2: Q { (scope)
	// Line 3: test "first test" {}
	// Line 4: group "my group" {
	// Line 5: test "nested test" {}

	expectations := map[string]uint32{
		"scaf.runScope:Q":                   2, // Q scope at line 2 (0-indexed)
		"scaf.runTest:Q/first test":         3, // first test at line 3
		"scaf.runGroup:Q/my group":          4, // group at line 4
		"scaf.runTest:Q/my group/nested test": 5, // nested test at line 5
	}

	for _, lens := range result {
		if lens.Command == nil || len(lens.Command.Arguments) < 2 {
			continue
		}
		key := lens.Command.Command + ":" + lens.Command.Arguments[1].(string)
		expectedLine, ok := expectations[key]
		if !ok {
			t.Errorf("Unexpected lens: %s", key)
			continue
		}
		if lens.Range.Start.Line != expectedLine {
			t.Errorf("Lens %s expected at line %d, got line %d", key, expectedLine, lens.Range.Start.Line)
		}
		delete(expectations, key)
	}

	for key := range expectations {
		t.Errorf("Missing expected lens: %s", key)
	}
}

func TestServer_CodeLens_Empty(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Test with non-existent document
	result, err := server.CodeLens(ctx, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.scaf"},
	})
	if err != nil {
		t.Fatalf("CodeLens() error: %v", err)
	}

	// Should return nil for non-existent document
	if result != nil && len(result) > 0 {
		t.Error("Expected nil or empty result for non-existent document")
	}
}

func TestServer_CodeLens_Arguments(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	content := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds Alice" {}
	group "edge cases" {
		test "handles null" {}
	}
}
`
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///path/to/test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	result, err := server.CodeLens(ctx, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///path/to/test.scaf"},
	})
	if err != nil {
		t.Fatalf("CodeLens() error: %v", err)
	}

	// Check that arguments contain expected values
	for _, lens := range result {
		if lens.Command == nil {
			continue
		}
		if len(lens.Command.Arguments) != 2 {
			t.Errorf("Expected 2 arguments for command %s, got %d", lens.Command.Command, len(lens.Command.Arguments))
			continue
		}

		// First argument should be file path
		filePath, ok := lens.Command.Arguments[0].(string)
		if !ok {
			t.Errorf("First argument should be string, got %T", lens.Command.Arguments[0])
			continue
		}
		if filePath != "/path/to/test.scaf" {
			t.Errorf("Expected file path '/path/to/test.scaf', got '%s'", filePath)
		}

		// Second argument should be a path string
		path, ok := lens.Command.Arguments[1].(string)
		if !ok {
			t.Errorf("Second argument should be string, got %T", lens.Command.Arguments[1])
			continue
		}

		// Verify path format based on command
		switch lens.Command.Command {
		case "scaf.runScope":
			if path != "GetUser" {
				t.Errorf("Expected scope name 'GetUser', got '%s'", path)
			}
		case "scaf.runTest":
			// Should be like "GetUser/finds Alice" or "GetUser/edge cases/handles null"
			if path != "GetUser/finds Alice" && path != "GetUser/edge cases/handles null" {
				t.Errorf("Unexpected test path: %s", path)
			}
		case "scaf.runGroup":
			if path != "GetUser/edge cases" {
				t.Errorf("Expected group path 'GetUser/edge cases', got '%s'", path)
			}
		}
	}
}

func TestServer_Formatting(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open an unformatted file (missing blank lines, inconsistent indentation, etc.)
	unformattedContent := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `
GetUser {
test "finds user" {
$id: 1
u.name: "Alice"
}
}
`

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    unformattedContent,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request formatting
	edits, err := server.Formatting(ctx, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: "file:///test.scaf",
		},
	})
	if err != nil {
		t.Fatalf("Formatting() error: %v", err)
	}

	// Should have edits (content needs formatting)
	if len(edits) == 0 {
		t.Fatal("Expected formatting edits")
	}

	// The edit should replace the entire document
	edit := edits[0]
	if edit.Range.Start.Line != 0 || edit.Range.Start.Character != 0 {
		t.Errorf("Expected edit to start at 0:0, got %d:%d", edit.Range.Start.Line, edit.Range.Start.Character)
	}

	// The formatted content should have proper structure
	formatted := edit.NewText

	// Should have proper indentation (tabs)
	if !contains(formatted, "\ttest") {
		t.Errorf("Expected indented test, got:\n%s", formatted)
	}

	// Should have blank line between query and scope
	if !contains(formatted, "`\n\nGetUser") {
		t.Errorf("Expected blank line between query and scope, got:\n%s", formatted)
	}

	// Should separate inputs from outputs with blank line
	if !contains(formatted, "$id: 1\n\n\t\tu.name") {
		t.Errorf("Expected blank line between inputs and outputs, got:\n%s", formatted)
	}

	t.Logf("Formatted content:\n%s", formatted)
}

func TestServer_Formatting_AlreadyFormatted(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open an already well-formatted file
	formattedContent := `query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1

		u.name: "Alice"
	}
}
`

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    formattedContent,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request formatting
	edits, err := server.Formatting(ctx, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: "file:///test.scaf",
		},
	})
	if err != nil {
		t.Fatalf("Formatting() error: %v", err)
	}

	// Should have empty edits (no changes needed)
	if len(edits) != 0 {
		t.Errorf("Expected no edits for already formatted content, got %d edits", len(edits))
		for _, edit := range edits {
			t.Logf("Edit: %+v", edit)
		}
	}
}

func TestServer_Formatting_ParseError(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a parse error
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    `query GetUser`, // Missing body
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request formatting
	edits, err := server.Formatting(ctx, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: "file:///test.scaf",
		},
	})
	if err != nil {
		t.Fatalf("Formatting() error: %v", err)
	}

	// Should return nil (can't format invalid content)
	if edits != nil {
		t.Errorf("Expected nil edits for parse error, got %d edits", len(edits))
	}
}

func TestServer_Formatting_UnknownDocument(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Request formatting for a document that was never opened
	edits, err := server.Formatting(ctx, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: "file:///nonexistent.scaf",
		},
	})
	if err != nil {
		t.Fatalf("Formatting() error: %v", err)
	}

	// Should return nil (document not found)
	if edits != nil {
		t.Errorf("Expected nil edits for unknown document, got %d edits", len(edits))
	}
}
