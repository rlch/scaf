package runner_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/module"
	"github.com/rlch/scaf/runner"
)

// paramTrackingDatabase tracks executed queries and their parameters.
type paramTrackingDatabase struct {
	results  []map[string]any
	executed []struct {
		query  string
		params map[string]any
	}
}

func (d *paramTrackingDatabase) Name() string { return "param-tracking" }

func (d *paramTrackingDatabase) Dialect() scaf.Dialect { return nil }

func (d *paramTrackingDatabase) Execute(_ context.Context, query string, params map[string]any) ([]map[string]any, error) {
	// Clone params to avoid mutation issues
	clonedParams := make(map[string]any)
	maps.Copy(clonedParams, params)

	d.executed = append(d.executed, struct {
		query  string
		params map[string]any
	}{query, clonedParams})

	return d.results, nil
}

func (d *paramTrackingDatabase) Close() error { return nil }

func TestRunner_FileBasedModuleResolution(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a fixture module
	fixturesContent := `
query SetupTestData ` + "`CREATE (:Test {name: $name})`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create a main module that imports fixtures
	mainContent := `
import fixtures "./fixtures"

query GetTest ` + "`MATCH (t:Test) RETURN t.name`" + `

GetTest {
	setup fixtures.SetupTestData($name: "hello")
	
	test "basic" {
		t.name: "hello"
	}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Load and resolve modules
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(mainPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Run with tracking dialect
	d := &paramTrackingDatabase{results: []map[string]any{{"t.name": "hello"}}}
	r := runner.New(
		runner.WithDatabase(d),
		runner.WithModules(resolved),
	)

	result, err := r.Run(context.Background(), resolved.Root.Suite, mainPath)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify test passed
	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Verify setup was executed with correct params
	if len(d.executed) < 2 {
		t.Fatalf("Expected at least 2 queries, got %d", len(d.executed))
	}

	// First should be the setup
	if d.executed[0].query != "CREATE (:Test {name: $name})" {
		t.Errorf("First query = %q", d.executed[0].query)
	}

	// Check params were passed correctly
	if d.executed[0].params["name"] != "hello" {
		t.Errorf("Setup params = %v, want name=hello", d.executed[0].params)
	}
}

func TestRunner_TransitiveImports(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create helper module
	helperContent := `
query SetupHelperData ` + "`CREATE (:Helper {value: $value})`" + `
`
	helperPath := filepath.Join(tmpDir, "helper.scaf")

	err := os.WriteFile(helperPath, []byte(helperContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create fixture module that imports helper
	fixturesContent := `
import "./helper"
query SetupFixtures ` + "`CREATE (:Fixture)`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err = os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module that uses transitive import
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	setup helper.SetupHelperData($value: 42)
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Load and resolve modules
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(mainPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Run with tracking dialect
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(
		runner.WithDatabase(d),
		runner.WithModules(resolved),
	)

	result, err := r.Run(context.Background(), resolved.Root.Suite, mainPath)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify test passed
	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Verify transitive setup was executed with correct params
	if len(d.executed) < 2 {
		t.Fatalf("Expected at least 2 queries, got %d", len(d.executed))
	}

	// First should be the helper setup
	if d.executed[0].query != "CREATE (:Helper {value: $value})" {
		t.Errorf("First query = %q", d.executed[0].query)
	}

	// Check params were passed correctly (float64 from parser)
	if d.executed[0].params["value"] != float64(42) {
		t.Errorf("Setup params = %v, want value=42", d.executed[0].params)
	}
}

func TestRunner_MultipleSetupLevels(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create fixture module
	fixturesContent := `
query SetupGlobal ` + "`CREATE (:Global)`" + `
query SetupScope ` + "`CREATE (:Scope {id: $id})`" + `
query SetupTest ` + "`CREATE (:Test {name: $name})`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module with setups at multiple levels
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

setup fixtures.SetupGlobal()

GetData {
	setup fixtures.SetupScope($id: 1)
	
	test "with test setup" {
		setup fixtures.SetupTest($name: "inner")
	}
	
	test "without test setup" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Load and resolve modules
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(mainPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Run with tracking dialect
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(
		runner.WithDatabase(d),
		runner.WithModules(resolved),
	)

	result, err := r.Run(context.Background(), resolved.Root.Suite, mainPath)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Both tests should pass
	if result.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Passed)
	}

	// Track which setups were called
	var hasGlobal, hasScope, hasTest bool

	for _, e := range d.executed {
		if e.query == "CREATE (:Global)" {
			hasGlobal = true
		}

		if e.query == "CREATE (:Scope {id: $id})" && e.params["id"] == float64(1) {
			hasScope = true
		}

		if e.query == "CREATE (:Test {name: $name})" && e.params["name"] == "inner" {
			hasTest = true
		}
	}

	if !hasGlobal {
		t.Error("Global setup was not executed")
	}

	if !hasScope {
		t.Error("Scope setup was not executed with correct params")
	}

	if !hasTest {
		t.Error("Test setup was not executed with correct params")
	}
}

func TestRunner_RunFileConvenience(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a simple module with inline setup
	content := `
query GetData ` + "`MATCH (d:Data) RETURN d`" + `

GetData {
	setup ` + "`CREATE (:Data)`" + `
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err := os.WriteFile(mainPath, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Use RunFile which handles resolution automatically
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(runner.WithDatabase(d))

	result, err := r.RunFile(context.Background(), mainPath)
	if err != nil {
		t.Fatalf("RunFile() error: %v", err)
	}

	// Test should pass
	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Verify setup was executed
	if len(d.executed) < 2 {
		t.Fatalf("Expected at least 2 queries, got %d", len(d.executed))
	}

	if d.executed[0].query != "CREATE (:Data)" {
		t.Errorf("First query = %q, want setup query", d.executed[0].query)
	}
}

func TestRunner_NestedDirectoryImports(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create nested directory structure
	sharedDir := filepath.Join(tmpDir, "shared")

	err := os.MkdirAll(sharedDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	// Create fixture in shared directory
	fixturesContent := `
query SetupShared ` + "`CREATE (:Shared {key: $key})`" + `
`
	fixturesPath := filepath.Join(sharedDir, "fixtures.scaf")

	err = os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module at root
	mainContent := `
import fixtures "./shared/fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	setup fixtures.SetupShared($key: "test-value")
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Use RunFile
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(runner.WithDatabase(d))

	result, err := r.RunFile(context.Background(), mainPath)
	if err != nil {
		t.Fatalf("RunFile() error: %v", err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Verify setup was called with correct params
	if len(d.executed) < 1 {
		t.Fatal("Expected at least 1 query")
	}

	found := false

	for _, e := range d.executed {
		if e.query == "CREATE (:Shared {key: $key})" && e.params["key"] == "test-value" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Setup not executed with correct params. Executed: %v", d.executed)
	}
}

func TestRunner_SetupWithVariousParamTypes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create fixture module with different param types
	fixturesContent := `
query SetupWithTypes ` + "`CREATE (:Node {str: $str, num: $num, bool: $bool, nothing: $nothing})`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module that passes various types
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	setup fixtures.SetupWithTypes($str: "hello", $num: 42.5, $bool: true, $nothing: null)
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Use RunFile
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(runner.WithDatabase(d))

	result, err := r.RunFile(context.Background(), mainPath)
	if err != nil {
		t.Fatalf("RunFile() error: %v", err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Find the setup execution
	var setupParams map[string]any

	for _, e := range d.executed {
		if e.query == "CREATE (:Node {str: $str, num: $num, bool: $bool, nothing: $nothing})" {
			setupParams = e.params

			break
		}
	}

	if setupParams == nil {
		t.Fatal("Setup was not executed")
	}

	// Verify each param type
	if setupParams["str"] != "hello" {
		t.Errorf("str = %v (%T), want \"hello\"", setupParams["str"], setupParams["str"])
	}

	if setupParams["num"] != 42.5 {
		t.Errorf("num = %v (%T), want 42.5", setupParams["num"], setupParams["num"])
	}

	if setupParams["bool"] != true {
		t.Errorf("bool = %v (%T), want true", setupParams["bool"], setupParams["bool"])
	}

	if setupParams["nothing"] != nil {
		t.Errorf("nothing = %v (%T), want nil", setupParams["nothing"], setupParams["nothing"])
	}
}

func TestRunner_UnknownModuleError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create main module referencing unknown module
	mainContent := `
query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	setup nonexistent.SetupData()
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err := os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Load and resolve modules
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(mainPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Run - should fail because nonexistent module
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(
		runner.WithDatabase(d),
		runner.WithModules(resolved),
	)

	_, err = r.Run(context.Background(), resolved.Root.Suite, mainPath)
	if err == nil {
		t.Error("Expected error for unknown module, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestRunner_UnknownSetupError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create fixture module without the setup we'll reference
	fixturesContent := `
query SetupOther ` + "`CREATE (:Other)`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module referencing non-existent setup
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	setup fixtures.SetupNonexistent()
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Load and resolve modules
	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(mainPath)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	// Run - should fail because setup doesn't exist
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(
		runner.WithDatabase(d),
		runner.WithModules(resolved),
	)

	_, err = r.Run(context.Background(), resolved.Root.Suite, mainPath)
	if err == nil {
		t.Error("Expected error for unknown setup, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestRunner_GroupLevelSetup(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create fixture module
	fixturesContent := `
query SetupGroup ` + "`CREATE (:Group {name: $name})`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module with group-level setup
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

GetData {
	group "my group" {
		setup fixtures.SetupGroup($name: "group-setup")
		
		test "test1" {}
		test "test2" {}
	}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Use RunFile
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(runner.WithDatabase(d))

	result, err := r.RunFile(context.Background(), mainPath)
	if err != nil {
		t.Fatalf("RunFile() error: %v", err)
	}

	if result.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Passed)
	}

	// Count group setup executions
	setupCount := 0

	for _, e := range d.executed {
		if e.query == "CREATE (:Group {name: $name})" && e.params["name"] == "group-setup" {
			setupCount++
		}
	}

	// Group setup should run once before all tests in the group
	if setupCount != 1 {
		t.Errorf("Group setup executed %d times, want 1", setupCount)
	}
}

func TestRunner_MixedInlineAndNamedSetups(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create fixture module
	fixturesContent := `
query SetupNamed ` + "`CREATE (:Named)`" + `
`
	fixturesPath := filepath.Join(tmpDir, "fixtures.scaf")

	err := os.WriteFile(fixturesPath, []byte(fixturesContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Create main module with both inline and named setups
	mainContent := `
import fixtures "./fixtures"

query GetData ` + "`MATCH (n) RETURN n`" + `

setup ` + "`CREATE (:Inline)`" + `

GetData {
	setup fixtures.SetupNamed()
	
	test "basic" {}
}
`
	mainPath := filepath.Join(tmpDir, "main.scaf")

	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Use RunFile
	d := &paramTrackingDatabase{results: []map[string]any{{}}}
	r := runner.New(runner.WithDatabase(d))

	result, err := r.RunFile(context.Background(), mainPath)
	if err != nil {
		t.Fatalf("RunFile() error: %v", err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}

	// Both setups should have run
	var hasInline, hasNamed bool

	for _, e := range d.executed {
		if e.query == "CREATE (:Inline)" {
			hasInline = true
		}

		if e.query == "CREATE (:Named)" {
			hasNamed = true
		}
	}

	if !hasInline {
		t.Error("Inline setup was not executed")
	}

	if !hasNamed {
		t.Error("Named setup was not executed")
	}
}
