package example_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/module"
)

func TestExamplesParse(t *testing.T) {
	// Test all .scaf files in current directory
	files, err := filepath.Glob("*.scaf")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f) //nolint:gosec // test file reading from controlled glob pattern
			if err != nil {
				t.Fatalf("read error: %v", err)
			}

			_, err = scaf.Parse(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
		})
	}

	// Test shared fixtures
	sharedFiles, err := filepath.Glob("shared/*.scaf")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range sharedFiles {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f) //nolint:gosec // test file reading from controlled glob pattern
			if err != nil {
				t.Fatalf("read error: %v", err)
			}

			suite, err := scaf.Parse(data)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// Verify it's a valid fixture module (has queries)
			if len(suite.Queries) == 0 {
				t.Error("fixture module should have queries")
			}
		})
	}
}

func TestExamplesModuleResolution(t *testing.T) {
	// Test files with imports that should resolve successfully
	filesWithImports := []string{
		"with_imports.scaf",
		"all_features.cypher.scaf",
	}

	for _, file := range filesWithImports {
		t.Run(file, func(t *testing.T) {
			loader := module.NewLoader()
			resolver := module.NewResolver(loader)

			absPath, err := filepath.Abs(file)
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			ctx, err := resolver.Resolve(absPath)
			if err != nil {
				t.Fatalf("module resolution failed: %v", err)
			}

			// Verify root module loaded
			if ctx.Root == nil {
				t.Fatal("root module is nil")
			}

			// Verify fixtures import is resolved
			if _, ok := ctx.Imports["fixtures"]; !ok {
				t.Error("fixtures import not resolved")
			}

			// Verify setup resolution works
			setup, err := ctx.ResolveSetup("fixtures", "SetupUsers")
			if err != nil {
				t.Fatalf("failed to resolve fixtures.SetupUsers: %v", err)
			}

			if setup.Query == "" {
				t.Error("SetupUsers query is empty")
			}
		})
	}
}

func TestFixturesModuleDetails(t *testing.T) {
	// Detailed test for the fixtures module
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	absPath, err := filepath.Abs("with_imports.scaf")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	ctx, err := resolver.Resolve(absPath)
	if err != nil {
		t.Fatalf("module resolution failed: %v", err)
	}

	// Verify SetupCleanDB is available
	setup, err := ctx.ResolveSetup("fixtures", "SetupCleanDB")
	if err != nil {
		t.Fatalf("failed to resolve fixtures.SetupCleanDB: %v", err)
	}

	if setup.Query == "" {
		t.Error("SetupCleanDB query is empty")
	}

	// Verify SetupPosts is available with params
	setup, err = ctx.ResolveSetup("fixtures", "SetupPosts")
	if err != nil {
		t.Fatalf("failed to resolve fixtures.SetupPosts: %v", err)
	}

	expectedParams := []string{"postId", "title", "authorId"}
	if len(setup.Params) != len(expectedParams) {
		t.Errorf("SetupPosts params count = %d, want %d", len(setup.Params), len(expectedParams))
	}

	// Check each expected param is present
	paramSet := make(map[string]bool)
	for _, p := range setup.Params {
		paramSet[p] = true
	}

	for _, expected := range expectedParams {
		if !paramSet[expected] {
			t.Errorf("SetupPosts missing param %q, got %v", expected, setup.Params)
		}
	}
}
