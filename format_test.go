package scaf_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
)

// inlineSetup creates a SetupClause with an inline query.
func inlineSetup(body string) *scaf.SetupClause {
	return &scaf.SetupClause{Inline: ptr(body)}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		suite    *scaf.Suite
		expected string
	}{
		{
			name: "single query",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
				},
			},
			expected: "query GetUser `MATCH (u:User) RETURN u`\n",
		},
		{
			name: "multiple queries",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "A", Body: "A"},
					{Name: "B", Body: "B"},
				},
			},
			expected: `query A ` + "`A`" + `

query B ` + "`B`" + `
`,
		},
		{
			name: "query with global setup",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Setup: inlineSetup("CREATE (:User)"),
			},
			expected: `query Q ` + "`Q`" + `

setup ` + "`CREATE (:User)`" + `
`,
		},
		{
			name: "basic scope with test",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "GetUser", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "GetUser",
						Items: []*scaf.TestOrGroup{
							{Test: &scaf.Test{Name: "finds user"}},
						},
					},
				},
			},
			expected: `query GetUser ` + "`Q`" + `

GetUser {
	test "finds user" {
	}
}
`,
		},
		{
			name: "scope with setup",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Setup:     inlineSetup("SCOPE SETUP"),
						Items: []*scaf.TestOrGroup{
							{Test: &scaf.Test{Name: "t"}},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	setup ` + "`SCOPE SETUP`" + `

	test "t" {
	}
}
`,
		},
		{
			name: "test with inputs and outputs",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "test",
									Statements: []*scaf.Statement{
										{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
										{Key: "$name", Value: &scaf.Value{Str: ptr("alice")}},
										{Key: "u.name", Value: &scaf.Value{Str: ptr("Alice")}},
										{Key: "u.age", Value: &scaf.Value{Number: ptr(30.0)}},
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	test "test" {
		$id: 1
		$name: "alice"

		u.name: "Alice"
		u.age: 30
	}
}
`,
		},
		{
			name: "test with setup",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name:  "t",
									Setup: inlineSetup("TEST SETUP"),
									Statements: []*scaf.Statement{
										{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	test "t" {
		setup ` + "`TEST SETUP`" + `

		$id: 1
	}
}
`,
		},
		{
			name: "test with assertion",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Statements: []*scaf.Statement{
										{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
									},
									Assertion: &scaf.Assertion{
										Query: "MATCH (n) RETURN count(n) as c",
										Expectations: []*scaf.Statement{
											{Key: "c", Value: &scaf.Value{Number: ptr(1.0)}},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	test "t" {
		$id: 1

		assert ` + "`MATCH (n) RETURN count(n) as c`" + ` {
			c: 1
		}
	}
}
`,
		},
		{
			name: "group with tests",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name: "users",
									Items: []*scaf.TestOrGroup{
										{Test: &scaf.Test{Name: "a"}},
										{Test: &scaf.Test{Name: "b"}},
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	group "users" {
		test "a" {
		}

		test "b" {
		}
	}
}
`,
		},
		{
			name: "group with setup",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name:  "users",
									Setup: inlineSetup("GROUP SETUP"),
									Items: []*scaf.TestOrGroup{
										{Test: &scaf.Test{Name: "a"}},
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	group "users" {
		setup ` + "`GROUP SETUP`" + `

		test "a" {
		}
	}
}
`,
		},
		{
			name: "nested groups",
			suite: &scaf.Suite{
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
			expected: `query Q ` + "`Q`" + `

Q {
	group "level1" {
		group "level2" {
			test "deep" {
			}
		}
	}
}
`,
		},
		{
			name: "multiple scopes",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{
					{Name: "A", Body: "A"},
					{Name: "B", Body: "B"},
				},
				Scopes: []*scaf.QueryScope{
					{QueryName: "A", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "a"}}}},
					{QueryName: "B", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "b"}}}},
				},
			},
			expected: `query A ` + "`A`" + `

query B ` + "`B`" + `

A {
	test "a" {
	}
}

B {
	test "b" {
	}
}
`,
		},
		{
			name: "empty assertion",
			suite: &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Assertion: &scaf.Assertion{
										Query: "MATCH (n) RETURN n",
									},
								},
							},
						},
					},
				},
			},
			expected: `query Q ` + "`Q`" + `

Q {
	test "t" {
		assert ` + "`MATCH (n) RETURN n`" + ` {
		}
	}
}
`,
		},
		{
			name: "scope only with global setup",
			suite: &scaf.Suite{
				Setup: inlineSetup("GLOBAL SETUP"),
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
					},
				},
			},
			expected: `setup ` + "`GLOBAL SETUP`" + `

Q {
	test "t" {
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := scaf.Format(tt.suite)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Format() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *scaf.Value
		expected string
	}{
		{name: "null", value: &scaf.Value{Null: true}, expected: "null"},
		{name: "string", value: &scaf.Value{Str: ptr("hello")}, expected: `"hello"`},
		{name: "integer", value: &scaf.Value{Number: ptr(42.0)}, expected: "42"},
		{name: "float", value: &scaf.Value{Number: ptr(3.14)}, expected: "3.14"},
		{name: "negative int", value: &scaf.Value{Number: ptr(-5.0)}, expected: "-5"},
		{name: "negative float", value: &scaf.Value{Number: ptr(-2.5)}, expected: "-2.5"},
		{name: "zero", value: &scaf.Value{Number: ptr(0.0)}, expected: "0"},
		{name: "bool true", value: &scaf.Value{Boolean: boolPtr(true)}, expected: "true"},
		{name: "bool false", value: &scaf.Value{Boolean: boolPtr(false)}, expected: "false"},
		{name: "empty list", value: &scaf.Value{List: &scaf.List{}}, expected: "[]"},
		{
			name: "list with values",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Number: ptr(1.0)},
				{Str: ptr("two")},
				{Boolean: boolPtr(true)},
			}}},
			expected: `[1, "two", true]`,
		},
		{name: "empty map", value: &scaf.Value{Map: &scaf.Map{}}, expected: "{}"},
		{
			name: "map with values",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "a", Value: &scaf.Value{Number: ptr(1.0)}},
				{Key: "b", Value: &scaf.Value{Str: ptr("two")}},
			}}},
			expected: `{a: 1, b: "two"}`,
		},
		{
			name: "nested map in list",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Map: &scaf.Map{Entries: []*scaf.MapEntry{
					{Key: "x", Value: &scaf.Value{Number: ptr(1.0)}},
				}}},
			}}},
			expected: `[{x: 1}]`,
		},
		{
			name: "nested list in map",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "arr", Value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
					{Number: ptr(1.0)},
					{Number: ptr(2.0)},
				}}}},
			}}},
			expected: `{arr: [1, 2]}`,
		},
		{name: "empty value defaults to null", value: &scaf.Value{}, expected: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a minimal suite with the value
			suite := &scaf.Suite{
				Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						QueryName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Statements: []*scaf.Statement{
										{Key: "v", Value: tt.value},
									},
								},
							},
						},
					},
				},
			}

			expected := `query Q ` + "`Q`" + `

Q {
	test "t" {
		v: ` + tt.expected + `
	}
}
`

			got := scaf.Format(suite)
			if diff := cmp.Diff(expected, got); diff != "" {
				t.Errorf("Format() value mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatRoundTrip(t *testing.T) {
	t.Parallel()

	// Test that parsing and then formatting produces parseable output
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "basic query and test",
			input: `query GetUser ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1

		u.name: "alice"
	}
}
`,
		},
		{
			name: "with global setup",
			input: `query Q ` + "`Q`" + `

setup ` + "`CREATE (:User)`" + `

Q {
	test "t" {
	}
}
`,
		},
		{
			name: "nested groups",
			input: `query Q ` + "`Q`" + `

Q {
	group "level1" {
		group "level2" {
			test "deep" {
				$x: 1
			}
		}
	}
}
`,
		},
		{
			name: "complex values",
			input: `query Q ` + "`Q`" + `

Q {
	test "complex" {
		list: [1, "two", true, null]
		map: {a: 1, b: "two"}
		nested: {arr: [1, {x: true}]}
	}
}
`,
		},
		{
			name: "assertion",
			input: `query Q ` + "`Q`" + `

Q {
	test "t" {
		$id: 1

		assert ` + "`MATCH (n) RETURN count(n) as c`" + ` {
			c: 1
		}
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse
			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			// Format
			formatted := scaf.Format(suite)

			// Parse again
			suite2, err := scaf.Parse([]byte(formatted))
			if err != nil {
				t.Fatalf("Parse() of formatted output error: %v\nFormatted:\n%s", err, formatted)
			}

			// Format again
			formatted2 := scaf.Format(suite2)

			// The two formatted outputs should be identical (idempotent)
			if diff := cmp.Diff(formatted, formatted2); diff != "" {
				t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
			}
		})
	}
}

func TestFormatPreservesSemantics(t *testing.T) {
	t.Parallel()

	// Test that formatting preserves the AST structure
	suite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u:User {id: $id}) RETURN u"},
		},
		Setup: inlineSetup("CREATE (:User {id: 1, name: \"Alice\"})"),
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "GetUser",
				Setup:     inlineSetup("MATCH (u:User) SET u.active = true"),
				Items: []*scaf.TestOrGroup{
					{
						Group: &scaf.Group{
							Name:  "active users",
							Setup: inlineSetup("CREATE (:Session)"),
							Items: []*scaf.TestOrGroup{
								{
									Test: &scaf.Test{
										Name:  "finds user",
										Setup: inlineSetup("SET u.verified = true"),
										Statements: []*scaf.Statement{
											{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
											{Key: "u.name", Value: &scaf.Value{Str: ptr("Alice")}},
											{Key: "u.active", Value: &scaf.Value{Boolean: boolPtr(true)}},
										},
										Assertion: &scaf.Assertion{
											Query: "MATCH (s:Session) RETURN count(s) as c",
											Expectations: []*scaf.Statement{
												{Key: "c", Value: &scaf.Value{Number: ptr(1.0)}},
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
	}

	formatted := scaf.Format(suite)

	// Parse the formatted output
	parsed, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() error: %v\nFormatted:\n%s", err, formatted)
	}

	// Compare ASTs
	if diff := cmp.Diff(suite, parsed); diff != "" {
		t.Errorf("AST mismatch after format+parse (-original +parsed):\n%s", diff)
	}
}

func TestFormatOnlyOutputs(t *testing.T) {
	t.Parallel()

	// Test that outputs without inputs are formatted correctly (no blank line)
	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "outputs only",
							Statements: []*scaf.Statement{
								{Key: "name", Value: &scaf.Value{Str: ptr("Alice")}},
								{Key: "age", Value: &scaf.Value{Number: ptr(30.0)}},
							},
						},
					},
				},
			},
		},
	}

	expected := `query Q ` + "`Q`" + `

Q {
	test "outputs only" {
		name: "Alice"
		age: 30
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatOnlyInputs(t *testing.T) {
	t.Parallel()

	// Test that inputs without outputs are formatted correctly (no trailing blank line)
	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "inputs only",
							Statements: []*scaf.Statement{
								{Key: "$id", Value: &scaf.Value{Number: ptr(1.0)}},
								{Key: "$name", Value: &scaf.Value{Str: ptr("alice")}},
							},
						},
					},
				},
			},
		},
	}

	expected := `query Q ` + "`Q`" + `

Q {
	test "inputs only" {
		$id: 1
		$name: "alice"
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatMultipleTestsInGroup(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Group: &scaf.Group{
							Name: "group",
							Items: []*scaf.TestOrGroup{
								{
									Test: &scaf.Test{
										Name: "first",
										Statements: []*scaf.Statement{
											{Key: "$x", Value: &scaf.Value{Number: ptr(1.0)}},
										},
									},
								},
								{
									Test: &scaf.Test{
										Name: "second",
										Statements: []*scaf.Statement{
											{Key: "$y", Value: &scaf.Value{Number: ptr(2.0)}},
										},
									},
								},
								{
									Test: &scaf.Test{
										Name: "third",
										Statements: []*scaf.Statement{
											{Key: "$z", Value: &scaf.Value{Number: ptr(3.0)}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	expected := `query Q ` + "`Q`" + `

Q {
	group "group" {
		test "first" {
			$x: 1
		}

		test "second" {
			$y: 2
		}

		test "third" {
			$z: 3
		}
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatMixedGroupsAndTests(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{Test: &scaf.Test{Name: "standalone"}},
					{
						Group: &scaf.Group{
							Name:  "group",
							Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "in group"}}},
						},
					},
					{Test: &scaf.Test{Name: "another standalone"}},
				},
			},
		},
	}

	expected := `query Q ` + "`Q`" + `

Q {
	test "standalone" {
	}

	group "group" {
		test "in group" {
		}
	}

	test "another standalone" {
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatEmptySuite(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{}
	got := scaf.Format(suite)

	if got != "\n" {
		t.Errorf("Format() empty suite = %q, want %q", got, "\n")
	}
}

func TestFormatQueryOnly(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
		},
	}

	expected := "query GetUser `MATCH (u:User) RETURN u`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatSetupOnly(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Setup: inlineSetup("CREATE (:Node)"),
	}

	expected := "setup `CREATE (:Node)`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatLargeNumbers(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				QueryName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "t",
							Statements: []*scaf.Statement{
								{Key: "big", Value: &scaf.Value{Number: ptr(1000000.0)}},
								{Key: "precise", Value: &scaf.Value{Number: ptr(123.456789)}},
							},
						},
					},
				},
			},
		},
	}

	expected := `query Q ` + "`Q`" + `

Q {
	test "t" {
		big: 1000000
		precise: 123.456789
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}
