package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rlch/scaf/module"
)

func TestLoader_Load(t *testing.T) {
	t.Parallel()

	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create a simple .scaf file
	scafContent := `
query SetupTest ` + "`CREATE (:Test)`" + `

SetupTest {
	test "basic" {
		$x: 1
	}
}
`
	scafPath := filepath.Join(tmpDir, "test.scaf")

	err := os.WriteFile(scafPath, []byte(scafContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	loader := module.NewLoader()

	// Test loading by full path
	t.Run("load by full path", func(t *testing.T) {
		t.Parallel()

		mod, err := loader.Load(scafPath)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}

		if mod.Suite == nil {
			t.Fatal("Suite is nil")
		}

		if len(mod.Suite.Queries) != 1 {
			t.Errorf("Queries count = %d, want 1", len(mod.Suite.Queries))
		}
	})

	// Test loading without extension
	t.Run("load without extension", func(t *testing.T) {
		t.Parallel()

		pathWithoutExt := filepath.Join(tmpDir, "test")

		mod, err := loader.Load(pathWithoutExt)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}

		if mod.Suite == nil {
			t.Fatal("Suite is nil")
		}
	})

	// Test caching
	t.Run("caching", func(t *testing.T) {
		t.Parallel()

		loader := module.NewLoader()

		mod1, err := loader.Load(scafPath)
		if err != nil {
			t.Fatalf("First Load() error: %v", err)
		}

		mod2, err := loader.Load(scafPath)
		if err != nil {
			t.Fatalf("Second Load() error: %v", err)
		}

		if mod1 != mod2 {
			t.Error("Expected same module instance from cache")
		}
	})

	// Test loading nonexistent file
	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()

		_, err := loader.Load(filepath.Join(tmpDir, "nonexistent.scaf"))
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})
}

func TestLoader_LoadFrom(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create directory structure
	subDir := filepath.Join(tmpDir, "sub")

	err := os.MkdirAll(subDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create main.scaf in root
	mainContent := `
import "./sub/helper"
query Q ` + "`Q`" + `
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write main.scaf: %v", err)
	}

	// Create helper.scaf in sub
	helperContent := `
query SetupHelper ` + "`CREATE (:Helper)`" + `
`

	helperPath := filepath.Join(subDir, "helper.scaf")

	err = os.WriteFile(helperPath, []byte(helperContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write helper.scaf: %v", err)
	}

	loader := module.NewLoader()

	// Load main module
	mainMod, err := loader.Load(mainPath)
	if err != nil {
		t.Fatalf("Failed to load main: %v", err)
	}

	// Load helper relative to main
	helperMod, err := loader.LoadFrom("./sub/helper", mainMod)
	if err != nil {
		t.Fatalf("Failed to load helper: %v", err)
	}

	if len(helperMod.Suite.Queries) != 1 {
		t.Errorf("Helper queries = %d, want 1", len(helperMod.Suite.Queries))
	}

	if helperMod.Suite.Queries[0].Name != "SetupHelper" {
		t.Errorf("Query name = %s, want SetupHelper", helperMod.Suite.Queries[0].Name)
	}
}

func TestLoader_Clear(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	scafPath := filepath.Join(tmpDir, "test.scaf")

	err := os.WriteFile(scafPath, []byte(`query Q `+"`Q`"), 0o600)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	loader := module.NewLoader()

	// Load and cache
	mod1, _ := loader.Load(scafPath)

	// Clear cache
	loader.Clear()

	// Load again - should be different instance
	mod2, _ := loader.Load(scafPath)

	if mod1 == mod2 {
		t.Error("Expected different module instance after Clear()")
	}
}
