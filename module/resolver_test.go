package module_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rlch/scaf/module"
)

func TestResolver_Resolve(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a module structure:
	// root.scaf imports fixtures, which imports utils

	utilsContent := `
query SetupUtils ` + "`CREATE (:Utils)`" + `
`
	utilsPath := filepath.Join(tmpDir, "utils.scaf")

	err := os.WriteFile(utilsPath, []byte(utilsContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	fixturesContent := `
import "./utils"
query SetupFixtures ` + "`CREATE (:Fixture {n: $n})`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err = os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	rootContent := `
import fixtures "./fixtures"
query GetUser ` + "`MATCH (u) RETURN u`" + `
query SetupRoot ` + "`CREATE (:Root)`" + `

GetUser {
	test "basic" {}
}
`
	rootPath := filepath.Join(tmpDir, "root.scaf")

	err = os.WriteFile(rootPath, []byte(rootContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	ctx, err := resolver.Resolve(rootPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Check root module
	if ctx.Root == nil {
		t.Fatal("Root module is nil")
	}

	if ctx.Root.Path != rootPath {
		t.Errorf("Root path = %q, want %q", ctx.Root.Path, rootPath)
	}

	// Check imports
	if len(ctx.Imports) != 2 {
		t.Errorf("Imports count = %d, want 2", len(ctx.Imports))
	}

	// Check explicit alias
	if _, ok := ctx.Imports["fixtures"]; !ok {
		t.Error("Missing 'fixtures' import")
	}

	// Check auto-derived alias from basename
	if _, ok := ctx.Imports["utils"]; !ok {
		t.Error("Missing 'utils' import (derived from basename)")
	}

	// Check all modules
	if len(ctx.AllModules) != 3 {
		t.Errorf("AllModules count = %d, want 3", len(ctx.AllModules))
	}

	// Test setup resolution
	t.Run("resolve local setup", func(t *testing.T) {
		t.Parallel()

		setup, err := ctx.ResolveSetup("", "SetupRoot")
		if err != nil {
			t.Fatalf("ResolveSetup() error: %v", err)
		}

		if setup.Query != "CREATE (:Root)" {
			t.Errorf("Query = %q", setup.Query)
		}
	})

	t.Run("resolve imported setup", func(t *testing.T) {
		t.Parallel()

		setup, err := ctx.ResolveSetup("fixtures", "SetupFixtures")
		if err != nil {
			t.Fatalf("ResolveSetup() error: %v", err)
		}

		if setup.Query != "CREATE (:Fixture {n: $n})" {
			t.Errorf("Query = %q", setup.Query)
		}
	})

	t.Run("resolve transitive import setup", func(t *testing.T) {
		t.Parallel()

		setup, err := ctx.ResolveSetup("utils", "SetupUtils")
		if err != nil {
			t.Fatalf("ResolveSetup() error: %v", err)
		}

		if setup.Query != "CREATE (:Utils)" {
			t.Errorf("Query = %q", setup.Query)
		}
	})
}

func TestResolver_CycleDetection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a cycle: a.scaf -> b.scaf -> c.scaf -> a.scaf

	aContent := `
import "./b"
query Q ` + "`Q`" + `
`
	aPath := filepath.Join(tmpDir, "a.scaf")

	err := os.WriteFile(aPath, []byte(aContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	bContent := `
import "./c"
query Q ` + "`Q`" + `
`
	bPath := filepath.Join(tmpDir, "b.scaf")

	err = os.WriteFile(bPath, []byte(bContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cContent := `
import "./a"
query Q ` + "`Q`" + `
`
	cPath := filepath.Join(tmpDir, "c.scaf")

	err = os.WriteFile(cPath, []byte(cContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	_, err = resolver.Resolve(aPath)
	if err == nil {
		t.Fatal("Expected cycle error, got nil")
	}

	var cycleErr *module.CycleError
	if !errors.As(err, &cycleErr) {
		t.Errorf("Expected CycleError, got %T: %v", err, err)
	}

	// Cycle path should show the cycle
	if len(cycleErr.Path) < 4 {
		t.Errorf("Cycle path too short: %v", cycleErr.Path)
	}
}

func TestResolver_SelfCycle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create self-referential module
	content := `
import "./self"
query Q ` + "`Q`" + `
`
	selfPath := filepath.Join(tmpDir, "self.scaf")

	err := os.WriteFile(selfPath, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	_, err = resolver.Resolve(selfPath)
	if err == nil {
		t.Fatal("Expected cycle error for self-import")
	}

	if !errors.Is(err, module.ErrCyclicDependency) {
		t.Errorf("Expected ErrCyclicDependency, got: %v", err)
	}
}

func TestResolver_DiamondDependency(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Diamond: root -> a, b; a -> common; b -> common
	// This is NOT a cycle and should work fine

	commonContent := `
query SetupCommon ` + "`CREATE (:Common)`" + `
`
	commonPath := filepath.Join(tmpDir, "common.scaf")

	err := os.WriteFile(commonPath, []byte(commonContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	aContent := `
import "./common"
query SetupA ` + "`CREATE (:A)`" + `
`
	aPath := filepath.Join(tmpDir, "a.scaf")

	err = os.WriteFile(aPath, []byte(aContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	bContent := `
import "./common"
query SetupB ` + "`CREATE (:B)`" + `
`
	bPath := filepath.Join(tmpDir, "b.scaf")

	err = os.WriteFile(bPath, []byte(bContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	rootContent := `
import "./a"
import "./b"
query SetupRoot ` + "`CREATE (:Root)`" + `
`
	rootPath := filepath.Join(tmpDir, "root.scaf")

	err = os.WriteFile(rootPath, []byte(rootContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	ctx, err := resolver.Resolve(rootPath)
	if err != nil {
		t.Fatalf("Diamond dependency should not cause error: %v", err)
	}

	// Should have 4 modules total
	if len(ctx.AllModules) != 4 {
		t.Errorf("AllModules = %d, want 4", len(ctx.AllModules))
	}

	// Common should be accessible
	if _, ok := ctx.Imports["common"]; !ok {
		t.Error("Common module should be accessible")
	}
}

func TestResolver_MissingImport(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	content := `
import "./nonexistent"
query Q ` + "`Q`" + `
`
	rootPath := filepath.Join(tmpDir, "root.scaf")

	err := os.WriteFile(rootPath, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	_, err = resolver.Resolve(rootPath)
	if err == nil {
		t.Fatal("Expected error for missing import")
	}

	if !errors.Is(err, module.ErrModuleNotFound) {
		t.Errorf("Expected ErrModuleNotFound, got: %v", err)
	}
}

func TestResolver_NestedDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create nested directory structure:
	// root.scaf
	// shared/
	//   fixtures.scaf
	//   lib/
	//     helpers.scaf

	sharedDir := filepath.Join(tmpDir, "shared")
	libDir := filepath.Join(sharedDir, "lib")

	err := os.MkdirAll(libDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	helpersContent := `
query SetupHelpers ` + "`CREATE (:Helper)`" + `
`
	helpersPath := filepath.Join(libDir, "helpers.scaf")

	err = os.WriteFile(helpersPath, []byte(helpersContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	fixturesContent := `
import "./lib/helpers"
query SetupFixtures ` + "`CREATE (:Fixture)`" + `
`
	fixturesPath := filepath.Join(sharedDir, "fixtures.scaf")

	err = os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	rootContent := `
import fixtures "./shared/fixtures"
query GetData ` + "`MATCH (n) RETURN n`" + `
`
	rootPath := filepath.Join(tmpDir, "root.scaf")

	err = os.WriteFile(rootPath, []byte(rootContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	ctx, err := resolver.Resolve(rootPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Check that all three modules are loaded
	if len(ctx.AllModules) != 3 {
		t.Errorf("AllModules = %d, want 3", len(ctx.AllModules))
	}

	// Verify fixtures alias works
	if _, ok := ctx.Imports["fixtures"]; !ok {
		t.Error("Missing 'fixtures' import")
	}

	// Verify helpers is accessible (from transitive import)
	if _, ok := ctx.Imports["helpers"]; !ok {
		t.Error("Missing 'helpers' import (transitive)")
	}

	// Verify setup resolution works through nested imports
	setup, err := ctx.ResolveSetup("helpers", "SetupHelpers")
	if err != nil {
		t.Fatalf("Failed to resolve transitive setup: %v", err)
	}

	if setup.Query != "CREATE (:Helper)" {
		t.Errorf("Setup query = %q", setup.Query)
	}
}

func TestResolver_SetupWithMultipleParams(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	content := `
query SetupUserWithDetails ` + "`CREATE (:User {name: $name, email: $email, age: $age, active: $active})`" + `
`
	modulePath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(modulePath, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()

	mod, err := loader.Load(modulePath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	setup, ok := mod.Setups["SetupUserWithDetails"]
	if !ok {
		t.Fatal("Setup not found")
	}

	// Verify all params are extracted
	expectedParams := map[string]bool{
		"name":   true,
		"email":  true,
		"age":    true,
		"active": true,
	}

	if len(setup.Params) != len(expectedParams) {
		t.Errorf("Params count = %d, want %d", len(setup.Params), len(expectedParams))
	}

	for _, p := range setup.Params {
		if !expectedParams[p] {
			t.Errorf("Unexpected param: %s", p)
		}
	}
}

func TestResolver_AliasCollision(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create two different modules
	mod1Content := `query SetupMod1 ` + "`CREATE (:Mod1)`" + `
`
	mod1Path := filepath.Join(tmpDir, "mod1.scaf")

	err := os.WriteFile(mod1Path, []byte(mod1Content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	mod2Content := `query SetupMod2 ` + "`CREATE (:Mod2)`" + `
`
	mod2Path := filepath.Join(tmpDir, "mod2.scaf")

	err = os.WriteFile(mod2Path, []byte(mod2Content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Root imports both with same alias
	rootContent := `
import shared "./mod1"
import shared "./mod2"
query Q ` + "`Q`" + `
`
	rootPath := filepath.Join(tmpDir, "root.scaf")

	err = os.WriteFile(rootPath, []byte(rootContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	_, err = resolver.Resolve(rootPath)
	if err == nil {
		t.Fatal("Expected error for alias collision")
	}

	// The error should indicate a conflict
	t.Logf("Got expected error: %v", err)
}

func TestResolver_ResolveFromSuite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a fixture module
	fixturesContent := `query SetupData ` + "`CREATE (:Data)`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Parse a suite directly (simulating test-time construction)
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	// Load the fixtures module to get its suite
	fixMod, err := loader.Load(fixturesPath)
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	// Now use ResolveFromSuite to resolve from an in-memory suite
	rootPath := filepath.Join(tmpDir, "virtual.scaf")

	ctx, err := resolver.ResolveFromSuite(rootPath, fixMod.Suite)
	if err != nil {
		t.Fatalf("ResolveFromSuite() error: %v", err)
	}

	// The virtual module should be the root
	if ctx.Root.Path != rootPath {
		t.Errorf("Root path = %q, want %q", ctx.Root.Path, rootPath)
	}

	// The setup should be resolvable locally
	setup, err := ctx.ResolveSetup("", "SetupData")
	if err != nil {
		t.Fatalf("Failed to resolve local setup: %v", err)
	}

	if setup.Query != "CREATE (:Data)" {
		t.Errorf("Setup query = %q", setup.Query)
	}
}
