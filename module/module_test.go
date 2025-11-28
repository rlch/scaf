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
		expectQueries []string
	}{
		{
			name: "no queries",
			suite: &scaf.Suite{
				Queries: nil,
			},
			expectQueries: nil,
		},
		{
			name: "single query",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
				},
			},
			expectQueries: []string{"GetUser"},
		},
		{
			name: "multiple queries",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u) RETURN u"},
					{Name: "CreatePost", Body: "CREATE (:Post)"},
					{Name: "SetupDB", Body: "CREATE (:Node)"},
				},
			},
			expectQueries: []string{"GetUser", "CreatePost", "SetupDB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mod := module.NewModule("/test/path.scaf", tt.suite)

			// Check queries
			gotQueries := make([]string, 0, len(mod.Queries))
			for name := range mod.Queries {
				gotQueries = append(gotQueries, name)
			}

			if len(gotQueries) != len(tt.expectQueries) {
				t.Errorf("Queries count = %d, want %d", len(gotQueries), len(tt.expectQueries))
			}

			for _, name := range tt.expectQueries {
				if _, ok := mod.Queries[name]; !ok {
					t.Errorf("Missing query %q", name)
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

func TestResolvedContext_ResolveModule(t *testing.T) {
	t.Parallel()

	// Create root module
	rootSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u) RETURN u"},
		},
	}
	root := module.NewModule("/root.scaf", rootSuite)

	// Create imported module
	importedSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "CreateFixtures", Body: "CREATE (:Fixture {n: $n})"},
		},
	}
	imported := module.NewModule("/fixtures.scaf", importedSuite)

	// Build context
	ctx := module.NewResolvedContext(root)
	ctx.Imports["fixtures"] = imported
	ctx.AllModules[imported.Path] = imported

	tests := []struct {
		name        string
		alias       string
		expectPath  string
		expectError bool
	}{
		{
			name:       "resolve imported module",
			alias:      "fixtures",
			expectPath: "/fixtures.scaf",
		},
		{
			name:        "unknown module",
			alias:       "nonexistent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mod, err := ctx.ResolveModule(tt.alias)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if mod.Path != tt.expectPath {
				t.Errorf("Path = %q, want %q", mod.Path, tt.expectPath)
			}
		})
	}
}

func TestResolvedContext_ResolveQuery(t *testing.T) {
	t.Parallel()

	// Create root module
	rootSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u) RETURN u"},
		},
	}
	root := module.NewModule("/root.scaf", rootSuite)

	// Create imported module
	importedSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "CreateFixtures", Body: "CREATE (:Fixture {n: $n})"},
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
		queryName   string
		expectQuery string
		expectError bool
	}{
		{
			name:        "resolve imported query",
			moduleAlias: "fixtures",
			queryName:   "CreateFixtures",
			expectQuery: "CREATE (:Fixture {n: $n})",
		},
		{
			name:        "unknown module",
			moduleAlias: "nonexistent",
			queryName:   "SomeQuery",
			expectError: true,
		},
		{
			name:        "unknown query in known module",
			moduleAlias: "fixtures",
			queryName:   "UnknownQuery",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query, err := ctx.ResolveQuery(tt.moduleAlias, tt.queryName)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if query != tt.expectQuery {
				t.Errorf("Query = %q, want %q", query, tt.expectQuery)
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

func TestModule_GetSetup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		suite     *scaf.Suite
		hasSetup  bool
	}{
		{
			name:     "no setup",
			suite:    &scaf.Suite{},
			hasSetup: false,
		},
		{
			name: "with setup",
			suite: &scaf.Suite{
				Setup: &scaf.SetupClause{Inline: ptr("CREATE (:Node)")},
			},
			hasSetup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mod := module.NewModule("/test.scaf", tt.suite)

			if got := mod.HasSetup(); got != tt.hasSetup {
				t.Errorf("HasSetup() = %v, want %v", got, tt.hasSetup)
			}

			setup := mod.GetSetup()
			if tt.hasSetup && setup == nil {
				t.Error("GetSetup() returned nil, want setup")
			}
			if !tt.hasSetup && setup != nil {
				t.Error("GetSetup() returned setup, want nil")
			}
		})
	}
}

func TestModule_GetQuery(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u) RETURN u"},
		},
	}
	mod := module.NewModule("/test.scaf", suite)

	// Test existing query
	query, ok := mod.GetQuery("GetUser")
	if !ok {
		t.Error("GetQuery() returned false for existing query")
	}
	if query != "MATCH (u) RETURN u" {
		t.Errorf("GetQuery() = %q, want %q", query, "MATCH (u) RETURN u")
	}

	// Test non-existing query
	_, ok = mod.GetQuery("NonExistent")
	if ok {
		t.Error("GetQuery() returned true for non-existing query")
	}
}

func ptr[T any](v T) *T {
	return &v
}
