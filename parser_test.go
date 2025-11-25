package scaf_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
)

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
										{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
										{Key: "u.name", Value: &scaf.Value{Str: ptr("alice")}},
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
				Setup:   ptr("CREATE (:User)"),
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
						Setup:     ptr("SCOPE SETUP"),
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
							{Test: &scaf.Test{Name: "t", Setup: ptr("TEST SETUP")}},
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
												Statements: []*scaf.Statement{{Key: "$x", Value: &scaf.Value{Number: ptr(1.0)}}},
											},
										},
										{
											Test: &scaf.Test{
												Name:       "b",
												Statements: []*scaf.Statement{{Key: "$y", Value: &scaf.Value{Number: ptr(2.0)}}},
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
							c: 1
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
									Assertion: &scaf.Assertion{
										Query:        "MATCH (n) RETURN count(n) as c",
										Expectations: []*scaf.Statement{{Key: "c", Value: &scaf.Value{Number: ptr(1.0)}}},
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
				Setup:    ptr("CREATE (:User)"),
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
						Setup:     ptr("SCOPE SETUP"),
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
									Setup:    ptr("GROUP SETUP"),
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

			got, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
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

			got, err := scaf.Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			gotValue := got.Scopes[0].Items[0].Test.Statements[0].Value
			if diff := cmp.Diff(tt.expected, gotValue); diff != "" {
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

	got, err := scaf.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	expected := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{Test: &scaf.Test{Name: "t", Statements: []*scaf.Statement{{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}}}}},
				},
			},
		},
	}

	if diff := cmp.Diff(expected, got); diff != "" {
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
