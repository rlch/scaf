package runner //nolint:testpackage

import (
	"context"
	"errors"
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/module"
)

type mockDialect struct {
	name     string
	results  []map[string]any
	err      error
	executed []string
}

func (m *mockDialect) Name() string { return m.name }

func (m *mockDialect) Execute(_ context.Context, query string, _ map[string]any) ([]map[string]any, error) {
	m.executed = append(m.executed, query)

	return m.results, m.err
}

func (m *mockDialect) Close() error { return nil }

func TestRunner_NoDialect(t *testing.T) {
	r := New()

	_, err := r.Run(context.Background(), &scaf.Suite{}, "test.scaf")

	if !errors.Is(err, ErrNoDialect) {
		t.Errorf("got %v, want ErrNoDialect", err)
	}
}

func TestRunner_EmptySuite(t *testing.T) {
	r := New(WithDialect(&mockDialect{}))

	result, err := r.Run(context.Background(), &scaf.Suite{}, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestRunner_GlobalSetup(t *testing.T) {
	d := &mockDialect{}
	r := New(WithDialect(d))

	setup := "CREATE (n:Node)"

	_, err := r.Run(context.Background(), &scaf.Suite{Setup: &scaf.SetupClause{Inline: &setup}}, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if len(d.executed) != 1 || d.executed[0] != setup {
		t.Errorf("executed = %v, want [%q]", d.executed, setup)
	}
}

func TestRunner_SetupError(t *testing.T) {
	d := &mockDialect{err: errTestSetupFailed}
	r := New(WithDialect(d))

	setup := "INVALID"

	_, err := r.Run(context.Background(), &scaf.Suite{Setup: &scaf.SetupClause{Inline: &setup}}, "test.scaf")

	if !errors.Is(err, errTestSetupFailed) {
		t.Errorf("got %v, want errTestSetupFailed", err)
	}
}

func TestRunner_SimpleTest(t *testing.T) {
	d := &mockDialect{}
	h := &mockHandler{}
	r := New(WithDialect(d), WithHandler(h))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "finds user"}}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 1 || result.Passed != 1 {
		t.Errorf("got %d/%d, want 1/1", result.Total, result.Passed)
	}

	var hasRun, hasPass bool

	for _, e := range h.events {
		if e.Action == ActionRun {
			hasRun = true
		}

		if e.Action == ActionPass {
			hasPass = true
		}
	}

	if !hasRun || !hasPass {
		t.Error("missing run or pass event")
	}
}

func TestRunner_NestedGroups(t *testing.T) {
	r := New(WithDialect(&mockDialect{}))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Query", Body: "Q"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "Query",
			Items: []*scaf.TestOrGroup{{
				Group: &scaf.Group{
					Name: "outer",
					Items: []*scaf.TestOrGroup{{
						Group: &scaf.Group{
							Name: "inner",
							Items: []*scaf.TestOrGroup{
								{Test: &scaf.Test{Name: "test1"}},
								{Test: &scaf.Test{Name: "test2"}},
							},
						},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}

	if _, ok := result.Tests["Query/outer/inner/test1"]; !ok {
		t.Error("test1 not found at expected path")
	}

	if _, ok := result.Tests["Query/outer/inner/test2"]; !ok {
		t.Error("test2 not found at expected path")
	}
}

func TestRunner_FailFast(t *testing.T) {
	d := &mockDialect{err: errTestFail}
	r := New(WithDialect(d), WithFailFast(true))

	setup := "SETUP"
	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Query", Body: "Q"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "Query",
			Items: []*scaf.TestOrGroup{
				{Test: &scaf.Test{Name: "test1", Setup: &scaf.SetupClause{Inline: &setup}}},
				{Test: &scaf.Test{Name: "test2"}},
				{Test: &scaf.Test{Name: "test3"}},
			},
		}},
	}

	result, _ := r.Run(context.Background(), suite, "test.scaf")

	if result.Total > 1 {
		t.Errorf("Total = %d, should stop after first failure", result.Total)
	}
}

func TestRunner_ScopeAndGroupSetup(t *testing.T) {
	d := &mockDialect{}
	r := New(WithDialect(d))

	scopeSetup := "SCOPE"
	groupSetup := "GROUP"
	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "Query", Body: "Q"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "Query",
			Setup: &scaf.SetupClause{Inline: &scopeSetup},
			Items: []*scaf.TestOrGroup{{
				Group: &scaf.Group{
					Name:  "group",
					Setup: &scaf.SetupClause{Inline: &groupSetup},
					Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "test"}}},
				},
			}},
		}},
	}

	_, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if len(d.executed) < 2 {
		t.Fatalf("executed = %v, want at least 2", d.executed)
	}

	if d.executed[0] != scopeSetup {
		t.Errorf("first = %q, want %q", d.executed[0], scopeSetup)
	}

	if d.executed[1] != groupSetup {
		t.Errorf("second = %q, want %q", d.executed[1], groupSetup)
	}
}

func TestRunner_AssertPassing(t *testing.T) {
	d := &mockDialect{
		results: []map[string]any{{"age": int64(30), "name": "Alice"}},
	}
	r := New(WithDialect(d))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{
					Name: "user is adult",
					Asserts: []*scaf.Assert{{
						Conditions: []*scaf.Expr{{
							Tokens: []*scaf.ExprToken{
								{Ident: ptr("age")},
								{Op: ptr(">=")},
								{Number: ptr("18")},
							},
						}},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestRunner_AssertFailing(t *testing.T) {
	d := &mockDialect{
		results: []map[string]any{{"age": int64(15), "name": "Bob"}},
	}
	r := New(WithDialect(d))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{
					Name: "user is adult",
					Asserts: []*scaf.Assert{{
						Conditions: []*scaf.Expr{{
							Tokens: []*scaf.ExprToken{
								{Ident: ptr("age")},
								{Op: ptr(">=")},
								{Number: ptr("18")},
							},
						}},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestRunner_AssertMultipleConditions(t *testing.T) {
	d := &mockDialect{
		results: []map[string]any{{"age": int64(30), "verified": true}},
	}
	r := New(WithDialect(d))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{
					Name: "multiple conditions",
					Asserts: []*scaf.Assert{{
						Conditions: []*scaf.Expr{
							// age >= 18
							{Tokens: []*scaf.ExprToken{
								{Ident: ptr("age")},
								{Op: ptr(">=")},
								{Number: ptr("18")},
							}},
							// verified == true
							{Tokens: []*scaf.ExprToken{
								{Ident: ptr("verified")},
								{Op: ptr("==")},
								{Ident: ptr("true")},
							}},
						},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestRunner_AssertWithInlineQuery(t *testing.T) {
	r := New(WithDialect(&queryAwareDialect{
		results: map[string][]map[string]any{
			"MAIN":  {{"name": "Alice"}},
			"COUNT": {{"cnt": int64(5)}},
		},
	}))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MAIN"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{
					Name: "with inline query assert",
					Asserts: []*scaf.Assert{{
						Query: &scaf.AssertQuery{
							Inline: ptr("COUNT"),
						},
						Conditions: []*scaf.Expr{{
							Tokens: []*scaf.ExprToken{
								{Ident: ptr("cnt")},
								{Op: ptr(">")},
								{Number: ptr("0")},
							},
						}},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestRunner_AssertWithNamedQuery(t *testing.T) {
	r := New(WithDialect(&queryAwareDialect{
		results: map[string][]map[string]any{
			"MAIN":    {{"name": "Alice"}},
			"COUNTER": {{"total": int64(10)}},
		},
	}))

	suite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MAIN"},
			{Name: "CountAll", Body: "COUNTER"},
		},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{
					Name: "with named query assert",
					Asserts: []*scaf.Assert{{
						Query: &scaf.AssertQuery{
							QueryName: ptr("CountAll"),
						},
						Conditions: []*scaf.Expr{{
							Tokens: []*scaf.ExprToken{
								{Ident: ptr("total")},
								{Op: ptr("==")},
								{Number: ptr("10")},
							},
						}},
					}},
				},
			}},
		}},
	}

	result, err := r.Run(context.Background(), suite, "test.scaf")
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

// queryAwareDialect returns different results based on the query body.
type queryAwareDialect struct {
	results map[string][]map[string]any
}

func (d *queryAwareDialect) Name() string { return "query-aware" }

func (d *queryAwareDialect) Execute(_ context.Context, query string, _ map[string]any) ([]map[string]any, error) {
	if res, ok := d.results[query]; ok {
		return res, nil
	}

	return nil, nil
}

func (d *queryAwareDialect) Close() error { return nil }

func ptr[T any](v T) *T {
	return &v
}

func TestRunner_NamedSetupWithModules(t *testing.T) {
	// Create a mock dialect that tracks executed queries
	d := &mockDialect{}

	// Create a module with a setup query
	fixturesSuite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "SetupUsers", Body: "CREATE (:User {name: $name})"},
		},
	}

	// Create the root module
	rootSuite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u.name"}},
		Imports: []*scaf.Import{{Alias: ptr("fixtures"), Path: "./fixtures.scaf"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Setup: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Module: ptr("fixtures"),
					Name:   "SetupUsers",
					Params: []*scaf.SetupParam{
						{Name: "$name", Value: &scaf.ParamValue{Literal: &scaf.Value{Str: ptr("Alice")}}},
					},
				},
			},
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{Name: "test"},
			}},
		}},
	}

	// Build the module context manually
	rootMod := module.NewModule("/root.scaf", rootSuite)
	fixturesMod := module.NewModule("/fixtures.scaf", fixturesSuite)

	ctx := module.NewResolvedContext(rootMod)
	ctx.Imports["fixtures"] = fixturesMod
	ctx.AllModules[fixturesMod.Path] = fixturesMod

	r := New(WithDialect(d), WithModules(ctx))

	result, err := r.Run(context.Background(), rootSuite, "/root.scaf")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify setup was executed
	if len(d.executed) < 2 {
		t.Fatalf("Expected at least 2 queries (setup + test), got %d: %v", len(d.executed), d.executed)
	}

	// First query should be the setup
	if d.executed[0] != "CREATE (:User {name: $name})" {
		t.Errorf("First query = %q, want setup query", d.executed[0])
	}

	// Test should pass
	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestRunner_NamedSetupWithoutModules(t *testing.T) {
	d := &mockDialect{}
	r := New(WithDialect(d)) // No modules configured

	suite := &scaf.Suite{
		Queries: []*scaf.Query{{Name: "GetUser", Body: "MATCH (u:User) RETURN u"}},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Setup: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Name: "SomeSetup",
				},
			},
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{Name: "test"},
			}},
		}},
	}

	_, err := r.Run(context.Background(), suite, "test.scaf")

	// Should error because no modules configured
	if err == nil {
		t.Error("Expected error for named setup without modules")
	}
}

func TestRunner_LocalNamedSetup(t *testing.T) {
	d := &mockDialect{}

	// Create suite with local setup query
	suite := &scaf.Suite{
		Queries: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u:User) RETURN u.name"},
			{Name: "SetupTestDB", Body: "CREATE (:TestNode)"},
		},
		Scopes: []*scaf.QueryScope{{
			QueryName: "GetUser",
			Setup: &scaf.SetupClause{
				Named: &scaf.NamedSetup{
					Name: "SetupTestDB", // No module prefix = local
				},
			},
			Items: []*scaf.TestOrGroup{{
				Test: &scaf.Test{Name: "test"},
			}},
		}},
	}

	// Build module context for local module
	rootMod := module.NewModule("/root.scaf", suite)
	ctx := module.NewResolvedContext(rootMod)

	r := New(WithDialect(d), WithModules(ctx))

	result, err := r.Run(context.Background(), suite, "/root.scaf")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify setup was executed
	if len(d.executed) < 2 {
		t.Fatalf("Expected at least 2 queries, got %d: %v", len(d.executed), d.executed)
	}

	if d.executed[0] != "CREATE (:TestNode)" {
		t.Errorf("First query = %q, want setup query", d.executed[0])
	}

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}
