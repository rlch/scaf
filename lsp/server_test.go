package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/lsp"
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
	server := lsp.NewServer(client, logger)

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
