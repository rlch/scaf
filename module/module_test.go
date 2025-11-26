package module_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/module"
)

func TestNewModule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		suite         *scaf.Suite
		expectSetups  []string
		expectParams  map[string][]string
	}{
		{
			name: "no setup queries",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
				},
			},
			expectSetups: nil,
		},
		{
			name: "setup query by prefix",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "SetupTestDB", Body: "CREATE (:TestNode)"},
				},
			},
			expectSetups: []string{"SetupTestDB"},
		},
		{
			name: "setup query by suffix",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "DatabaseSetup", Body: "CREATE (:DB)"},
				},
			},
			expectSetups: []string{"DatabaseSetup"},
		},
		{
			name: "setup query with params",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "SetupUsers", Body: "CREATE (:User {id: $userId, name: $name})"},
				},
			},
			expectSetups: []string{"SetupUsers"},
			expectParams: map[string][]string{
				"SetupUsers": {"userId", "name"},
			},
		},
		{
			name: "mixed queries",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u) RETURN u"},
					{Name: "SetupDB", Body: "CREATE (:Node)"},
					{Name: "CreatePost", Body: "CREATE (:Post)"},
					{Name: "TestSetup", Body: "CREATE (:Test)"},
				},
			},
			expectSetups: []string{"SetupDB", "TestSetup"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mod := module.NewModule("/test/path.scaf", tt.suite)

			// Check setups
			gotSetups := make([]string, 0, len(mod.Setups))
			for name := range mod.Setups {
				gotSetups = append(gotSetups, name)
			}

			if len(gotSetups) != len(tt.expectSetups) {
				t.Errorf("Setups count = %d, want %d", len(gotSetups), len(tt.expectSetups))
			}

			for _, name := range tt.expectSetups {
				if _, ok := mod.Setups[name]; !ok {
					t.Errorf("Missing setup %q", name)
				}
			}

			// Check params
			for name, expectedParams := range tt.expectParams {
				setup, ok := mod.Setups[name]
				if !ok {
					t.Errorf("Setup %q not found", name)

					continue
				}

				if diff := cmp.Diff(expectedParams, setup.Params); diff != "" {
					t.Errorf("Setup %q params mismatch (-want +got):\n%s", name, diff)
				}
			}
		})
	}
}

func TestModuleBaseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected string
	}{
		{"/path/to/fixtures.scaf", "fixtures"},
		{"/a/b/test_db.scaf", "test_db"},
		{"./local.scaf", "local"},
		{"/single.scaf", "single"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			mod := &module.Module{Path: tt.path}
			if got := mod.BaseName(); got != tt.expected {
				t.Errorf("BaseName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestResolvedContext_ResolveSetup(t *testing.T) {
	t.Parallel()

	// Create root module
	rootSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "SetupRoot", Body: "CREATE (:Root)"},
		},
	}
	root := module.NewModule("/root.scaf", rootSuite)

	// Create imported module
	importedSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "SetupFixtures", Body: "CREATE (:Fixture {n: $n})"},
		},
	}
	imported := module.NewModule("/fixtures.scaf", importedSuite)

	// Build context
	ctx := module.NewResolvedContext(root)
	ctx.Imports["fixtures"] = imported
	ctx.AllModules[imported.Path] = imported

	tests := []struct {
		name        string
		moduleAlias string
		setupName   string
		expectQuery string
		expectError bool
	}{
		{
			name:        "local setup",
			moduleAlias: "",
			setupName:   "SetupRoot",
			expectQuery: "CREATE (:Root)",
		},
		{
			name:        "imported setup",
			moduleAlias: "fixtures",
			setupName:   "SetupFixtures",
			expectQuery: "CREATE (:Fixture {n: $n})",
		},
		{
			name:        "unknown module",
			moduleAlias: "nonexistent",
			setupName:   "Setup",
			expectError: true,
		},
		{
			name:        "unknown setup in known module",
			moduleAlias: "fixtures",
			setupName:   "UnknownSetup",
			expectError: true,
		},
		{
			name:        "unknown local setup",
			moduleAlias: "",
			setupName:   "UnknownSetup",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setup, err := ctx.ResolveSetup(tt.moduleAlias, tt.setupName)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if setup.Query != tt.expectQuery {
				t.Errorf("Query = %q, want %q", setup.Query, tt.expectQuery)
			}
		})
	}
}

func TestResolvedContext_GetQueries(t *testing.T) {
	t.Parallel()

	rootSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u) RETURN u"},
			{Name: "SetupDB", Body: "CREATE (:DB)"},
		},
	}
	root := module.NewModule("/root.scaf", rootSuite)

	importedSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "CreatePost", Body: "CREATE (:Post)"},
		},
	}
	imported := module.NewModule("/fixtures.scaf", importedSuite)

	ctx := module.NewResolvedContext(root)
	ctx.Imports["fixtures"] = imported

	queries := ctx.GetQueries()

	expected := map[string]string{
		"GetUser":           "MATCH (u) RETURN u",
		"SetupDB":           "CREATE (:DB)",
		"fixtures.CreatePost": "CREATE (:Post)",
	}

	if diff := cmp.Diff(expected, queries); diff != "" {
		t.Errorf("GetQueries() mismatch (-want +got):\n%s", diff)
	}
}
