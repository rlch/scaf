package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/rlch/scaf"
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
