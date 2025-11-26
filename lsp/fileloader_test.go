package lsp_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/lsp"
)

func TestURIToPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uri      protocol.DocumentURI
		expected string
	}{
		{
			name:     "basic file URI",
			uri:      "file:///Users/test/project/file.scaf",
			expected: "/Users/test/project/file.scaf",
		},
		{
			name:     "with spaces encoded",
			uri:      "file:///Users/test/my%20project/file.scaf",
			expected: "/Users/test/my project/file.scaf", // url.Parse decodes the path
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := lsp.URIToPath(tt.uri)
			if result != tt.expected {
				t.Errorf("URIToPath(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestPathToURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected protocol.DocumentURI
	}{
		{
			name:     "basic path",
			path:     "/Users/test/project/file.scaf",
			expected: "file:///Users/test/project/file.scaf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := lsp.PathToURI(tt.path)
			if result != tt.expected {
				t.Errorf("PathToURI(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestLSPFileLoader_ResolveImportPath(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	loader := lsp.NewLSPFileLoader(logger, "/workspace")

	tests := []struct {
		name       string
		basePath   string
		importPath string
		expected   string
	}{
		{
			name:       "same directory",
			basePath:   "/workspace/tests/main.scaf",
			importPath: "./fixtures",
			expected:   "/workspace/tests/fixtures.scaf",
		},
		{
			name:       "parent directory",
			basePath:   "/workspace/tests/main.scaf",
			importPath: "../shared/fixtures",
			expected:   "/workspace/shared/fixtures.scaf",
		},
		{
			name:       "already has extension",
			basePath:   "/workspace/tests/main.scaf",
			importPath: "./fixtures.scaf",
			expected:   "/workspace/tests/fixtures.scaf",
		},
		{
			name:       "deeply nested",
			basePath:   "/workspace/tests/nested/deep/main.scaf",
			importPath: "../../../shared/common/fixtures",
			expected:   "/workspace/shared/common/fixtures.scaf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := loader.ResolveImportPath(tt.basePath, tt.importPath)
			if result != tt.expected {
				t.Errorf("ResolveImportPath(%q, %q) = %q, want %q",
					tt.basePath, tt.importPath, result, tt.expected)
			}
		})
	}
}

func TestLSPFileLoader_Load(t *testing.T) {
	t.Parallel()

	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.scaf")
	testContent := "query Test `MATCH (n) RETURN n`\n"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	logger := zap.NewNop()
	loader := lsp.NewLSPFileLoader(logger, tmpDir)

	// First load
	content, err := loader.Load(testFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Load() = %q, want %q", string(content), testContent)
	}

	// Second load should use cache (no way to verify from outside, but should work)
	content2, err := loader.Load(testFile)
	if err != nil {
		t.Fatalf("Second Load() error: %v", err)
	}

	if string(content2) != testContent {
		t.Errorf("Second Load() = %q, want %q", string(content2), testContent)
	}
}

func TestLSPFileLoader_LoadAndAnalyze(t *testing.T) {
	t.Parallel()

	// Create a temp file with valid scaf content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.scaf")
	testContent := `query CreateUser ` + "`CREATE (u:User {name: $name}) RETURN u`" + `
query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `
`

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	logger := zap.NewNop()
	loader := lsp.NewLSPFileLoader(logger, tmpDir)

	// Load and analyze
	result, err := loader.LoadAndAnalyze(testFile)
	if err != nil {
		t.Fatalf("LoadAndAnalyze() error: %v", err)
	}

	if result == nil {
		t.Fatal("LoadAndAnalyze() returned nil")
	}

	if result.Symbols == nil {
		t.Fatal("LoadAndAnalyze() returned nil Symbols")
	}

	// Check queries were parsed
	if len(result.Symbols.Queries) != 2 {
		t.Errorf("Expected 2 queries, got %d", len(result.Symbols.Queries))
	}

	if _, ok := result.Symbols.Queries["CreateUser"]; !ok {
		t.Error("Expected CreateUser query")
	}

	if _, ok := result.Symbols.Queries["GetUser"]; !ok {
		t.Error("Expected GetUser query")
	}
}

func TestLSPFileLoader_InvalidatePath(t *testing.T) {
	t.Parallel()

	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.scaf")
	testContent := "query Test `MATCH (n) RETURN n`\n"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	logger := zap.NewNop()
	loader := lsp.NewLSPFileLoader(logger, tmpDir)

	// Load to populate cache
	_, _ = loader.Load(testFile)

	// Invalidate
	loader.InvalidatePath(testFile)

	// Update file content
	newContent := "query Updated `MATCH (m) RETURN m`\n"
	if err := os.WriteFile(testFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	// Load again should get new content
	content, err := loader.Load(testFile)
	if err != nil {
		t.Fatalf("Load() after invalidate error: %v", err)
	}

	if string(content) != newContent {
		t.Errorf("Load() after invalidate = %q, want %q", string(content), newContent)
	}
}
