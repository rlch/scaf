package analysis_test

import (
	"slices"
	"testing"

	"github.com/rlch/scaf/analysis"
)

func TestAnalyzer_Analyze(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		wantQueries     []string
		wantImports     []string
		wantTests       []string
		wantDiagnostics int
	}{
		{
			name: "basic query and test",
			input: `
query GetUser ` + "`MATCH (u:User {id: $id}) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1
		u.name: "Alice"
	}
}
`,
			wantQueries: []string{"GetUser"},
			wantTests:   []string{"GetUser/finds user"},
		},
		{
			name: "multiple queries",
			input: `
query GetUser ` + "`Q1`" + `
query GetPost ` + "`Q2`" + `

GetUser {
	test "t1" {}
}
GetPost {
	test "t2" {}
}
`,
			wantQueries: []string{"GetUser", "GetPost"},
			wantTests:   []string{"GetUser/t1", "GetPost/t2"},
		},
		{
			name: "with imports",
			input: `
import fixtures "./shared/fixtures"
import myalias "./other"

query Q ` + "`Q`" + `

Q {
	test "t" {}
}
`,
			wantQueries: []string{"Q"},
			wantImports: []string{"fixtures", "myalias"},
			wantTests:   []string{"Q/t"},
		},
		{
			name: "undefined query scope",
			input: `
query Q ` + "`Q`" + `

UndefinedQuery {
	test "t" {}
}
`,
			wantQueries:     []string{"Q"},
			wantDiagnostics: 1, // undefined query error
		},
		{
			name: "nested groups",
			input: `
query Q ` + "`Q`" + `

Q {
	group "level1" {
		group "level2" {
			test "deep" {}
		}
		test "shallow" {}
	}
}
`,
			wantQueries: []string{"Q"},
			wantTests:   []string{"Q/level1/level2/deep", "Q/level1/shallow"},
		},
		{
			name: "parse error",
			input: `
query Q ` + "`Q`" + `

Q {
	test "unclosed
`,
			wantDiagnostics: 1, // parse error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := analysis.NewAnalyzer(nil)
			result := analyzer.Analyze("test.scaf", []byte(tt.input))

			// Check queries.
			if len(tt.wantQueries) > 0 {
				if len(result.Symbols.Queries) != len(tt.wantQueries) {
					t.Errorf("got %d queries, want %d", len(result.Symbols.Queries), len(tt.wantQueries))
				}

				for _, name := range tt.wantQueries {
					if _, ok := result.Symbols.Queries[name]; !ok {
						t.Errorf("missing query %q", name)
					}
				}
			}

			// Check imports.
			if len(tt.wantImports) > 0 {
				if len(result.Symbols.Imports) != len(tt.wantImports) {
					t.Errorf("got %d imports, want %d", len(result.Symbols.Imports), len(tt.wantImports))
				}

				for _, alias := range tt.wantImports {
					if _, ok := result.Symbols.Imports[alias]; !ok {
						t.Errorf("missing import %q", alias)
					}
				}
			}

			// Check tests.
			if len(tt.wantTests) > 0 {
				if len(result.Symbols.Tests) != len(tt.wantTests) {
					t.Errorf("got %d tests, want %d", len(result.Symbols.Tests), len(tt.wantTests))

					for path := range result.Symbols.Tests {
						t.Logf("  found test: %s", path)
					}
				}

				for _, path := range tt.wantTests {
					if _, ok := result.Symbols.Tests[path]; !ok {
						t.Errorf("missing test %q", path)
					}
				}
			}

			// Check diagnostics count.
			if tt.wantDiagnostics > 0 {
				if len(result.Diagnostics) < tt.wantDiagnostics {
					t.Errorf("got %d diagnostics, want at least %d", len(result.Diagnostics), tt.wantDiagnostics)
				}
			}
		})
	}
}

func TestAnalyzer_PartialParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantQueries []string // Queries should be extracted even with parse errors
		wantImports []string // Imports should be extracted even with parse errors
		wantError   bool     // Whether we expect a parse error
	}{
		{
			name: "error after valid queries",
			input: `
query GetUser ` + "`MATCH (u:User) RETURN u`" + `
query GetPost ` + "`MATCH (p:Post) RETURN p`" + `

GetUser {
	test "broken" {
		$id: 1
		// Missing closing brace
`,
			wantQueries: []string{"GetUser", "GetPost"},
			wantError:   true,
		},
		{
			name: "error after valid imports and queries",
			input: `
import fixtures "./fixtures"
import utils "./utils"

query Q ` + "`Q`" + `

Q {
	test "incomplete
`,
			wantQueries: []string{"Q"},
			wantImports: []string{"fixtures", "utils"},
			wantError:   true,
		},
		{
			name: "mid-query error still gets earlier queries",
			input: "query Valid1 `Q1`\nquery Valid2 `Q2`\nquery Broken `incomplete",
			wantQueries: []string{"Valid1", "Valid2"},
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := analysis.NewAnalyzer(nil)
			result := analyzer.Analyze("test.scaf", []byte(tt.input))

			// Verify parse error presence
			if tt.wantError && result.ParseError == nil {
				t.Error("expected parse error, got none")
			}
			if !tt.wantError && result.ParseError != nil {
				t.Errorf("unexpected parse error: %v", result.ParseError)
			}

			// Check that queries were still extracted despite parse error
			for _, name := range tt.wantQueries {
				if _, ok := result.Symbols.Queries[name]; !ok {
					t.Errorf("missing query %q (should be extracted from partial AST)", name)
				}
			}

			// Check that imports were still extracted despite parse error
			for _, alias := range tt.wantImports {
				if _, ok := result.Symbols.Imports[alias]; !ok {
					t.Errorf("missing import %q (should be extracted from partial AST)", alias)
				}
			}
		})
	}
}

func TestAnalyzer_ExtractQueryParams(t *testing.T) {
	t.Parallel()

	analyzer := analysis.NewAnalyzer(nil)
	result := analyzer.Analyze("test.scaf", []byte(`
query GetUser `+"`MATCH (u:User {id: $userId, name: $name}) WHERE u.age > $minAge RETURN u`"+`

GetUser {
	test "t" {
		$userId: 1
		$name: "test"
		$minAge: 18
	}
}
`))

	q, ok := result.Symbols.Queries["GetUser"]
	if !ok {
		t.Fatal("GetUser query not found")
	}

	wantParams := []string{"userId", "name", "minAge"}
	if len(q.Params) != len(wantParams) {
		t.Errorf("got %d params, want %d: %v", len(q.Params), len(wantParams), q.Params)
	}

	for _, p := range wantParams {
		if !slices.Contains(q.Params, p) {
			t.Errorf("missing param %q in %v", p, q.Params)
		}
	}
}
