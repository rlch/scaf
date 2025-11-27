package scaf_test

import (
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rlch/scaf"
)

// ignorePositions is a cmp option that ignores lexer.Position and Token fields in comparisons.
// This allows tests to compare AST structure without specifying exact source positions or tokens.
// Also ignores Close fields (captured closing braces) and recovery metadata fields.
var ignorePositions = cmp.Options{
	cmpopts.IgnoreTypes(lexer.Position{}, lexer.Token{}, []lexer.Token{}),
	cmpopts.IgnoreFields(scaf.Suite{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.Import{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.Query{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.QueryScope{}, "LeadingComments", "TrailingComment", "Close", "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
	cmpopts.IgnoreFields(scaf.Group{}, "LeadingComments", "TrailingComment", "Close", "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
	cmpopts.IgnoreFields(scaf.Test{}, "LeadingComments", "TrailingComment", "Close", "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
	cmpopts.IgnoreFields(scaf.Assert{}, "Close", "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
	cmpopts.IgnoreFields(scaf.SetupClause{}, "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
	cmpopts.IgnoreFields(scaf.NamedSetup{}, "Recovered", "RecoveredSpan", "RecoveredEnd", "SkippedTokens"),
}

func ptr[T any](v T) *T {
	return &v
}

func boolPtr(v bool) *scaf.Boolean {
	b := scaf.Boolean(v)

	return &b
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *scaf.Suite
	}{
		{
			name: "basic query and test",
			input: `
				query GetUser ` + "`MATCH (u:User) RETURN u`" + `
				GetUser {
					test "finds user" {
						$id: 1
						u.name: "alice"
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
				},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "GetUser",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "finds user",
									Statements: []*scaf.Statement{
										scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
										scaf.NewStatement("u.name", &scaf.Value{Str: ptr("alice")}),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "global setup",
			input: `
				query Q ` + "`Q`" + `
				setup ` + "`CREATE (:User)`" + `
				Q {
					test "t" {}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Setup: &scaf.SetupClause{Inline: ptr("CREATE (:User)")},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
					},
				},
			},
		},
		{
			name: "scope setup",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup ` + "`SCOPE SETUP`" + `
					test "t" {}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Setup: &scaf.SetupClause{Inline: ptr("SCOPE SETUP")},
						Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
					},
				},
			},
		},
		{
			name: "test setup",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						setup ` + "`TEST SETUP`" + `
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{Test: &scaf.Test{Name: "t", Setup: &scaf.SetupClause{Inline: ptr("TEST SETUP")}}},
						},
					},
				},
			},
		},
		{
			name: "group with tests",
			input: `
				query Q ` + "`Q`" + `
				Q {
					group "users" {
						test "a" { $x: 1 }
						test "b" { $y: 2 }
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name: "users",
									Items: []*scaf.TestOrGroup{
										{
											Test: &scaf.Test{
												Name:       "a",
												Statements: []*scaf.Statement{scaf.NewStatement("$x", &scaf.Value{Number: ptr(1.0)})},
											},
										},
										{
											Test: &scaf.Test{
												Name:       "b",
												Statements: []*scaf.Statement{scaf.NewStatement("$y", &scaf.Value{Number: ptr(2.0)})},
											},
										},
									},
								},
							},
						},
					},
				},
			},
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
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name: "level1",
									Items: []*scaf.TestOrGroup{
										{
											Group: &scaf.Group{
												Name:  "level2",
												Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "deep"}}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "assertion block",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert ` + "`MATCH (n) RETURN count(n) as c`" + ` {
							c == 1
						}
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Asserts: []*scaf.Assert{
										{
											Query: &scaf.AssertQuery{
												Inline: ptr("MATCH (n) RETURN count(n) as c"),
											},
											Conditions: []*scaf.Expr{
												{ExprTokens: []*scaf.ExprToken{
													{Ident: ptr("c")},
													{Op: ptr("==")},
													{Number: ptr("1")},
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "assert with field ref param",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert OtherQuery($id: u.id) {
							count > 0
						}
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Asserts: []*scaf.Assert{
										{
											Query: &scaf.AssertQuery{
												QueryName: ptr("OtherQuery"),
												Params: []*scaf.SetupParam{
													{Name: "$id", Value: &scaf.ParamValue{FieldRef: &scaf.DottedIdent{Parts: []string{"u", "id"}}}},
												},
											},
											Conditions: []*scaf.Expr{
												{ExprTokens: []*scaf.ExprToken{
													{Ident: ptr("count")},
													{Op: ptr(">")},
													{Number: ptr("0")},
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple queries and scopes",
			input: `
				query A ` + "`A`" + `
				query B ` + "`B`" + `
				A { test "a" {} }
				B { test "b" {} }
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "A", Body: "A"},
					{Name: "B", Body: "B"},
				},
				Scopes: []*scaf.QueryScope{
					{QueryName: "A", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "a"}}}},
					{QueryName: "B", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "b"}}}},
				},
			},
		},
		{
			name: "global teardown",
			input: `
				query Q ` + "`Q`" + `
				setup ` + "`CREATE (:User)`" + `
				teardown ` + "`MATCH (u:User) DELETE u`" + `
				Q { test "t" {} }
			`,
			expected: &scaf.Suite{
				Queries:  []*scaf.Query{{Name: "Q", Body: "Q"}},
				Setup: &scaf.SetupClause{Inline: ptr("CREATE (:User)")},
				Teardown: ptr("MATCH (u:User) DELETE u"),
				Scopes: []*scaf.QueryScope{
					{QueryName: "Q", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}}},
				},
			},
		},
		{
			name: "scope teardown",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup ` + "`SCOPE SETUP`" + `
					teardown ` + "`SCOPE TEARDOWN`" + `
					test "t" {}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Setup: &scaf.SetupClause{Inline: ptr("SCOPE SETUP")},
						Teardown:  ptr("SCOPE TEARDOWN"),
						Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
					},
				},
			},
		},
		{
			name: "group teardown",
			input: `
				query Q ` + "`Q`" + `
				Q {
					group "g" {
						setup ` + "`GROUP SETUP`" + `
						teardown ` + "`GROUP TEARDOWN`" + `
						test "t" {}
					}
				}
			`,
			expected: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name:     "g",
									Setup: &scaf.SetupClause{Inline: ptr("GROUP SETUP")},
									Teardown: ptr("GROUP TEARDOWN"),
									Items:    []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if diff := cmp.Diff(tt.expected, result, ignorePositions); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseImports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []*scaf.Import
	}{
		{
			name: "simple import",
			input: `
				import "../../setup/db"
				query Q ` + "`Q`" + `
			`,
			expected: []*scaf.Import{
				{Path: "../../setup/db"},
			},
		},
		{
			name: "import with alias",
			input: `
				import fixtures "../shared/fixtures"
				query Q ` + "`Q`" + `
			`,
			expected: []*scaf.Import{
				{Alias: ptr("fixtures"), Path: "../shared/fixtures"},
			},
		},
		{
			name: "multiple imports",
			input: `
				import "../../setup/db"
				import fixtures "../shared/fixtures"
				import utils "./utils"
				query Q ` + "`Q`" + `
			`,
			expected: []*scaf.Import{
				{Path: "../../setup/db"},
				{Alias: ptr("fixtures"), Path: "../shared/fixtures"},
				{Alias: ptr("utils"), Path: "./utils"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if diff := cmp.Diff(tt.expected, result.Imports, ignorePositions); diff != "" {
				t.Errorf("Parse() imports mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseNamedSetup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *scaf.SetupClause
	}{
		{
			name: "named setup no params",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup SetupDB()
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Name: "SetupDB",
				},
			},
		},
		{
			name: "named setup with module",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup fixtures.SetupDB()
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Module: ptr("fixtures"),
					Name:   "SetupDB",
				},
			},
		},
		{
			name: "named setup with params",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup CreatePosts($n: 10, $title: "Post")
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Name: "CreatePosts",
					Params: []*scaf.SetupParam{
						{Name: "$n", Value: &scaf.ParamValue{Literal: &scaf.Value{Number: ptr(10.0)}}},
						{Name: "$title", Value: &scaf.ParamValue{Literal: &scaf.Value{Str: ptr("Post")}}},
					},
				},
			},
		},
		{
			name: "named setup with module and params",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup fixtures.CreatePosts($n: 10, $authorId: 1)
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Module: ptr("fixtures"),
					Name:   "CreatePosts",
					Params: []*scaf.SetupParam{
						{Name: "$n", Value: &scaf.ParamValue{Literal: &scaf.Value{Number: ptr(10.0)}}},
						{Name: "$authorId", Value: &scaf.ParamValue{Literal: &scaf.Value{Number: ptr(1.0)}}},
					},
				},
			},
		},
		{
			name: "setup block with single inline",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup { ` + "`CREATE (:User)`" + ` }
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Block: []*scaf.SetupItem{
					{Inline: ptr("CREATE (:User)")},
				},
			},
		},
		{
			name: "setup block with single named",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup { SetupUsers() }
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Block: []*scaf.SetupItem{
					{Named: &scaf.NamedSetup{Name: "SetupUsers"}},
				},
			},
		},
		{
			name: "setup block with multiple items",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup {
						` + "`CREATE (:User)`" + `
						SetupUsers()
						fixtures.CreatePosts($n: 10)
					}
					test "t" {}
				}
			`,
			expected: &scaf.SetupClause{
				Block: []*scaf.SetupItem{
					{Inline: ptr("CREATE (:User)")},
					{Named: &scaf.NamedSetup{Name: "SetupUsers"}},
					{Named: &scaf.NamedSetup{
						Module: ptr("fixtures"),
						Name:   "CreatePosts",
						Params: []*scaf.SetupParam{
							{Name: "$n", Value: &scaf.ParamValue{Literal: &scaf.Value{Number: ptr(10.0)}}},
						},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			gotSetup := result.Scopes[0].Setup
			if diff := cmp.Diff(tt.expected, gotSetup, ignorePositions); diff != "" {
				t.Errorf("Parse() setup mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *scaf.Value
	}{
		{name: "string", input: `"hello"`, expected: &scaf.Value{Str: ptr("hello")}},
		{name: "int", input: `42`, expected: &scaf.Value{Number: ptr(42.0)}},
		{name: "float", input: `3.14`, expected: &scaf.Value{Number: ptr(3.14)}},
		{name: "true", input: `true`, expected: &scaf.Value{Boolean: boolPtr(true)}},
		{name: "false", input: `false`, expected: &scaf.Value{Boolean: boolPtr(false)}},
		{name: "null", input: `null`, expected: &scaf.Value{Null: true}},
		{
			name:     "empty list",
			input:    `[]`,
			expected: &scaf.Value{List: &scaf.List{Values: nil}},
		},
		{
			name:  "list",
			input: `[1, "two"]`,
			expected: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Number: ptr(1.0)},
				{Str: ptr("two")},
			}}},
		},
		{
			name:     "empty map",
			input:    `{}`,
			expected: &scaf.Value{Map: &scaf.Map{Entries: nil}},
		},
		{
			name:  "map",
			input: `{a: 1, b: "two"}`,
			expected: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "a", Value: &scaf.Value{Number: ptr(1.0)}},
				{Key: "b", Value: &scaf.Value{Str: ptr("two")}},
			}}},
		},
		{
			name:  "nested",
			input: `{arr: [1, {x: true}]}`,
			expected: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "arr", Value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
					{Number: ptr(1.0)},
					{Map: &scaf.Map{Entries: []*scaf.MapEntry{{Key: "x", Value: &scaf.Value{Boolean: boolPtr(true)}}}}},
				}}}},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := `query Q ` + "`Q`" + ` Q { test "t" { v: ` + tt.input + ` } }`

			result, err := scaf.Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			gotValue := result.Scopes[0].Items[0].Test.Statements[0].Value
			if diff := cmp.Diff(tt.expected, gotValue, ignorePositions); diff != "" {
				t.Errorf("Value mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValueToGo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *scaf.Value
		expected any
	}{
		{name: "string", value: &scaf.Value{Str: ptr("hello")}, expected: "hello"},
		{name: "number", value: &scaf.Value{Number: ptr(42.0)}, expected: 42.0},
		{name: "bool", value: &scaf.Value{Boolean: boolPtr(true)}, expected: true},
		{name: "null", value: &scaf.Value{Null: true}, expected: nil},
		{name: "empty value", value: &scaf.Value{}, expected: nil},
		{
			name: "list",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Number: ptr(1.0)},
				{Str: ptr("two")},
			}}},
			expected: []any{1.0, "two"},
		},
		{
			name: "map",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "a", Value: &scaf.Value{Number: ptr(1.0)}},
			}}},
			expected: map[string]any{"a": 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.value.ToGo()
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("ToGo() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseComments(t *testing.T) {
	t.Parallel()

	src := `
		// This is a comment
		query Q ` + "`Q`" + `
		// Another comment
		Q {
			// Group comment
			test "t" {
				// Input comment
				$id: 1
			}
		}
	`

	result, err := scaf.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	expected := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{Test: &scaf.Test{Name: "t", Statements: []*scaf.Statement{scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)})}}},
				},
			},
		},
	}

	if diff := cmp.Diff(expected, result, ignorePositions); diff != "" {
		t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
	}
}

func TestValueString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *scaf.Value
		expected string
	}{
		{name: "null", value: &scaf.Value{Null: true}, expected: "null"},
		{name: "string", value: &scaf.Value{Str: ptr("hello")}, expected: `"hello"`},
		{name: "number", value: &scaf.Value{Number: ptr(42.0)}, expected: "42"},
		{name: "float", value: &scaf.Value{Number: ptr(3.14)}, expected: "3.14"},
		{name: "bool true", value: &scaf.Value{Boolean: boolPtr(true)}, expected: "true"},
		{name: "bool false", value: &scaf.Value{Boolean: boolPtr(false)}, expected: "false"},
		{name: "empty list", value: &scaf.Value{List: &scaf.List{}}, expected: "[]"},
		{
			name: "list",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Number: ptr(1.0)},
				{Str: ptr("two")},
			}}},
			expected: `[1, "two"]`,
		},
		{name: "empty map", value: &scaf.Value{Map: &scaf.Map{}}, expected: "{}"},
		{
			name: "map",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "a", Value: &scaf.Value{Number: ptr(1.0)}},
				{Key: "b", Value: &scaf.Value{Str: ptr("two")}},
			}}},
			expected: `{a: 1, b: "two"}`,
		},
		{name: "nil value", value: &scaf.Value{}, expected: "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.value.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseExprAssert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string // expected expression strings
	}{
		{
			name: "simple comparison",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert { u.age > 18 }
					}
				}
			`,
			expected: []string{"u.age > 18"},
		},
		{
			name: "expression with function call",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert { u.createdAt - now() < duration("24h") }
					}
				}
			`,
			expected: []string{`u.createdAt - now() < duration("24h")`},
		},
		{
			name: "complex expression",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert { len(u.posts) > 0 && u.verified == true }
					}
				}
			`,
			expected: []string{"len(u.posts) > 0 && u.verified == true"},
		},
		{
			name: "multiple expressions",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert { x > 0; y < 10; z == 5 }
					}
				}
			`,
			expected: []string{"x > 0", "y < 10", "z == 5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			test := result.Scopes[0].Items[0].Test
			if len(test.Asserts) == 0 {
				t.Fatal("Expected at least one assert")
			}

			assert := test.Asserts[0]
			if assert.Query != nil {
				t.Fatal("Expected standalone assert (no query)")
			}

			if len(assert.Conditions) != len(tt.expected) {
				t.Fatalf("Conditions count = %d, want %d", len(assert.Conditions), len(tt.expected))
			}

			for i, cond := range assert.Conditions {
				gotExpr := cond.String()
				if gotExpr != tt.expected[i] {
					t.Errorf("Condition[%d] = %q, want %q", i, gotExpr, tt.expected[i])
				}
			}
		})
	}
}

func TestParseQueryAssert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		expectedInline *string
		expectedQuery  *string
		expectedParams int
		expectedConds  int
	}{
		{
			name: "inline query with conditions",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert ` + "`MATCH (n) RETURN count(n) as cnt`" + ` {
							cnt > 0;
							cnt < 100
						}
					}
				}
			`,
			expectedInline: ptr("MATCH (n) RETURN count(n) as cnt"),
			expectedConds:  2,
		},
		{
			name: "named query without params",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert CheckCount() {
							c == 1
						}
					}
				}
			`,
			expectedQuery:  ptr("CheckCount"),
			expectedParams: 0,
			expectedConds:  1,
		},
		{
			name: "named query with params",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert CreatePost($title: "test", $authorId: 1) {
							p.title == "test"
						}
					}
				}
			`,
			expectedQuery:  ptr("CreatePost"),
			expectedParams: 2,
			expectedConds:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			test := result.Scopes[0].Items[0].Test
			if len(test.Asserts) == 0 {
				t.Fatal("Expected at least one assert")
			}

			assert := test.Asserts[0]
			if assert.Query == nil {
				t.Fatal("Expected QueryAssert")
			}

			qa := assert.Query
			if tt.expectedInline != nil {
				if qa.Inline == nil || *qa.Inline != *tt.expectedInline {
					t.Errorf("Inline = %v, want %v", qa.Inline, *tt.expectedInline)
				}
			}

			if tt.expectedQuery != nil {
				if qa.QueryName == nil || *qa.QueryName != *tt.expectedQuery {
					t.Errorf("QueryName = %v, want %v", qa.QueryName, *tt.expectedQuery)
				}
			}

			if len(qa.Params) != tt.expectedParams {
				t.Errorf("Params count = %d, want %d", len(qa.Params), tt.expectedParams)
			}

			if len(assert.Conditions) != tt.expectedConds {
				t.Errorf("Conditions count = %d, want %d", len(assert.Conditions), tt.expectedConds)
			}
		})
	}
}

// TODO: TestParseComputedField - ComputedFields feature removed temporarily
// Will be re-added with proper syntax disambiguation (e.g., "mock u { field: expr }")

func TestParseWithRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		expectError    bool
		checkAST       func(t *testing.T, suite *scaf.Suite)
	}{
		{
			name: "valid input with recovery enabled",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						$id: 1
					}
				}
			`,
			expectError: false,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				if len(suite.Scopes) != 1 {
					t.Errorf("Expected 1 scope, got %d", len(suite.Scopes))
				}
				if len(suite.Scopes[0].Items) != 1 {
					t.Errorf("Expected 1 item, got %d", len(suite.Scopes[0].Items))
				}
			},
		},
		// Note: Recovery only works for parse errors, not lexer errors.
		// Unterminated strings cause lexer errors before parsing begins.
		// Test cases below use syntactically invalid (but lexically valid) input.
		{
			name: "incomplete setup - missing close paren",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup Func(
					test "t" {}
				}
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				// Should still have partial AST
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Queries) != 1 {
					t.Errorf("Expected 1 query, got %d", len(suite.Queries))
				}
				if len(suite.Scopes) != 1 {
					t.Errorf("Expected 1 scope, got %d", len(suite.Scopes))
				}
			},
		},
		{
			name: "incomplete test - missing closing brace",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "unclosed" {
						$id: 1
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				// Should still have partial AST
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Queries) != 1 {
					t.Errorf("Expected 1 query, got %d", len(suite.Queries))
				}
			},
		},
		{
			name: "multiple scopes with error in second",
			input: `
				query Q1 ` + "`Q1`" + `
				query Q2 ` + "`Q2`" + `
				Q1 {
					test "valid" {}
				}
				Q2 {
					setup Func(
					test "after error" {}
				}
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Queries) != 2 {
					t.Errorf("Expected 2 queries, got %d", len(suite.Queries))
				}
				// Both scopes should be present
				if len(suite.Scopes) != 2 {
					t.Errorf("Expected 2 scopes, got %d", len(suite.Scopes))
				}
			},
		},
		{
			name: "extra token after setup",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup ` + "`Q`" + ` extra
					test "t" {}
				}
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Queries) != 1 {
					t.Errorf("Expected 1 query, got %d", len(suite.Queries))
				}
			},
		},
		{
			name: "empty setup before closing brace",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup 
				}
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Scopes) != 1 {
					t.Fatalf("Expected 1 scope, got %d", len(suite.Scopes))
				}
				scope := suite.Scopes[0]
				// ViaParser should have recovered the empty setup
				if scope.Setup == nil {
					t.Fatal("Expected scope.Setup to be non-nil (from ViaParser)")
				}
				if !scope.Setup.Recovered {
					t.Error("Expected scope.Setup.Recovered = true")
				}
				// The scope should still be complete (has closing brace)
				if !scope.IsComplete() {
					t.Error("Expected scope.IsComplete() = true")
				}
			},
		},
		{
			name: "empty setup before test keyword",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup 
					test "t" {}
				}
			`,
			expectError: true,
			checkAST: func(t *testing.T, suite *scaf.Suite) {
				if suite == nil {
					t.Fatal("Expected partial AST, got nil")
				}
				if len(suite.Scopes) != 1 {
					t.Fatalf("Expected 1 scope, got %d", len(suite.Scopes))
				}
				scope := suite.Scopes[0]
				// ViaParser should have recovered the empty setup
				if scope.Setup == nil {
					t.Fatal("Expected scope.Setup to be non-nil (from ViaParser)")
				}
				if !scope.Setup.Recovered {
					t.Error("Expected scope.Setup.Recovered = true")
				}
				// The test should still be parsed
				if len(scope.Items) != 1 {
					t.Errorf("Expected 1 item in scope, got %d", len(scope.Items))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			suite, err := scaf.ParseWithRecovery([]byte(tt.input), true)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.checkAST != nil {
				tt.checkAST(t, suite)
			}
		})
	}
}

func TestIsComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		checkFn  func(t *testing.T, suite *scaf.Suite)
	}{
		{
			name: "complete test has Close",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				test := suite.Scopes[0].Items[0].Test
				if test == nil {
					t.Fatal("Expected test, got nil")
				}
				if !test.IsComplete() {
					t.Error("Expected test.IsComplete() = true")
				}
				if test.Close != "}" {
					t.Errorf("Expected test.Close = '}', got %q", test.Close)
				}
			},
		},
		{
			name: "complete group has Close",
			input: `
				query Q ` + "`Q`" + `
				Q {
					group "g" {
						test "t" {}
					}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				group := suite.Scopes[0].Items[0].Group
				if group == nil {
					t.Fatal("Expected group, got nil")
				}
				if !group.IsComplete() {
					t.Error("Expected group.IsComplete() = true")
				}
				if group.Close != "}" {
					t.Errorf("Expected group.Close = '}', got %q", group.Close)
				}
			},
		},
		{
			name: "complete assert has Close",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {
						assert { true }
					}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				test := suite.Scopes[0].Items[0].Test
				if len(test.Asserts) != 1 {
					t.Fatalf("Expected 1 assert, got %d", len(test.Asserts))
				}
				assert := test.Asserts[0]
				if !assert.IsComplete() {
					t.Error("Expected assert.IsComplete() = true")
				}
				if assert.Close != "}" {
					t.Errorf("Expected assert.Close = '}', got %q", assert.Close)
				}
			},
		},
		{
			name: "complete QueryScope has Close",
			input: `
				query Q ` + "`Q`" + `
				Q {
					test "t" {}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				scope := suite.Scopes[0]
				if !scope.IsComplete() {
					t.Error("Expected scope.IsComplete() = true")
				}
				if scope.Close != "}" {
					t.Errorf("Expected scope.Close = '}', got %q", scope.Close)
				}
			},
		},
		{
			name: "complete SetupClause with inline",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup ` + "`CREATE (:N)`" + `
					test "t" {}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				setup := suite.Scopes[0].Setup
				if setup == nil {
					t.Fatal("Expected setup, got nil")
				}
				if !setup.IsComplete() {
					t.Error("Expected setup.IsComplete() = true")
				}
			},
		},
		{
			name: "complete NamedSetup",
			input: `
				query Q ` + "`Q`" + `
				Q {
					setup Func()
					test "t" {}
				}
			`,
			checkFn: func(t *testing.T, suite *scaf.Suite) {
				setup := suite.Scopes[0].Setup
				if setup == nil || setup.Named == nil {
					t.Fatal("Expected named setup")
				}
				if !setup.Named.IsComplete() {
					t.Error("Expected setup.Named.IsComplete() = true")
				}
				if setup.Named.Name != "Func" {
					t.Errorf("Expected setup.Named.Name = 'Func', got %q", setup.Named.Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			tt.checkFn(t, suite)
		})
	}
}
