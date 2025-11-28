package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
)

func TestServer_FoldingRanges(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with various foldable elements
	content := "import fixtures \"./fixtures\"\nimport utils \"./utils\"\n\n" +
		"query GetUser `MATCH (u:User {id: $id}) RETURN u`\n\n" +
		"query CountPosts `MATCH (p:Post)\nRETURN count(p)`\n\n" +
		"GetUser {\n" +
		"\tsetup fixtures.CreateUser($id: 1)\n" +
		"\ttest \"finds user by id\" {\n" +
		"\t\t$id: 1\n" +
		"\t\tu.name: \"Alice\"\n" +
		"\t}\n" +
		"\tgroup \"edge cases\" {\n" +
		"\t\ttest \"handles null\" {\n" +
		"\t\t\t$id: null\n" +
		"\t\t}\n" +
		"\t\ttest \"handles zero\" {\n" +
		"\t\t\t$id: 0\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	result, err := server.FoldingRanges(ctx, &protocol.FoldingRangeParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		},
	})
	if err != nil {
		t.Fatalf("FoldingRanges() error: %v", err)
	}

	if result == nil || len(result) == 0 {
		t.Fatal("Expected folding ranges")
	}

	// Count different types of ranges
	var imports, queries, scopes, tests, groups int
	for _, r := range result {
		t.Logf("Range: lines %d-%d, kind=%s", r.StartLine, r.EndLine, r.Kind)
		switch r.Kind {
		case protocol.ImportsFoldingRange:
			imports++
		case protocol.RegionFoldingRange:
			// Distinguish by looking at start line
			switch r.StartLine {
			case 3, 5: // query lines
				queries++
			case 8: // scope
				scopes++
			case 10, 17, 20: // tests
				tests++
			case 14: // group
				groups++
			}
		}
	}

	t.Logf("Found: imports=%d, queries=%d, scopes=%d, tests=%d, groups=%d", imports, queries, scopes, tests, groups)

	// Should have at least one import fold (for multiple imports)
	if imports < 1 {
		t.Error("Expected at least 1 import folding range")
	}

	// Should have at least 2 query folds
	if queries < 2 {
		t.Errorf("Expected at least 2 query folding ranges, got %d", queries)
	}
}

func TestServer_FoldingRanges_Empty(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Test with non-existent document
	result, err := server.FoldingRanges(ctx, &protocol.FoldingRangeParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.scaf"},
		},
	})
	if err != nil {
		t.Fatalf("FoldingRanges() error: %v", err)
	}

	// Should return nil for non-existent document
	if result != nil && len(result) > 0 {
		t.Error("Expected nil or empty result for non-existent document")
	}
}

func TestServer_FoldingRanges_SingleImport(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a single import (should not create import fold)
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `import fixtures "./fixtures"

query Q ` + "`Q`" + `
Q { test "t" {} }
`,
		},
	})

	result, err := server.FoldingRanges(ctx, &protocol.FoldingRangeParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		},
	})
	if err != nil {
		t.Fatalf("FoldingRanges() error: %v", err)
	}

	// Should not have import folding for single import
	for _, r := range result {
		if r.Kind == protocol.ImportsFoldingRange {
			t.Error("Should not have import folding range for single import")
		}
	}
}
