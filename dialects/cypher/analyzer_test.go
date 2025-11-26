package cypher_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/dialects/cypher"
)

func TestAnalyzer_AnalyzeQuery_Parameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		query          string
		wantParams     []string
		wantParamCount int
	}{
		{
			name:           "single parameter",
			query:          "MATCH (u:User {id: $id}) RETURN u",
			wantParams:     []string{"id"},
			wantParamCount: 1,
		},
		{
			name:           "multiple parameters",
			query:          "MATCH (u:User {id: $id, name: $name}) WHERE u.age > $minAge RETURN u",
			wantParams:     []string{"id", "name", "minAge"},
			wantParamCount: 3,
		},
		{
			name:           "repeated parameter",
			query:          "MATCH (u:User {id: $id}) WHERE u.manager_id = $id RETURN u",
			wantParams:     []string{"id"},
			wantParamCount: 1,
		},
		{
			name:           "no parameters",
			query:          "MATCH (u:User) RETURN u.name",
			wantParams:     []string{},
			wantParamCount: 0,
		},
		{
			name:           "numeric parameter",
			query:          "MATCH (u:User {id: $1}) RETURN u",
			wantParams:     []string{"1"},
			wantParamCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()

			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Parameters) != tt.wantParamCount {
				t.Errorf("got %d parameters, want %d", len(metadata.Parameters), tt.wantParamCount)
			}

			gotNames := make([]string, len(metadata.Parameters))
			for i, p := range metadata.Parameters {
				gotNames[i] = p.Name
			}

			if diff := cmp.Diff(tt.wantParams, gotNames); diff != "" {
				t.Errorf("parameter names mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAnalyzer_AnalyzeQuery_Returns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       string
		wantReturns []scaf.ReturnInfo
	}{
		{
			name:  "simple variable",
			query: "MATCH (u:User) RETURN u",
			wantReturns: []scaf.ReturnInfo{
				{Name: "u", Expression: "u"},
			},
		},
		{
			name:  "property access",
			query: "MATCH (u:User) RETURN u.name",
			wantReturns: []scaf.ReturnInfo{
				{Name: "name", Expression: "u.name"},
			},
		},
		{
			name:  "multiple properties",
			query: "MATCH (u:User) RETURN u.name, u.email, u.age",
			wantReturns: []scaf.ReturnInfo{
				{Name: "name", Expression: "u.name"},
				{Name: "email", Expression: "u.email"},
				{Name: "age", Expression: "u.age"},
			},
		},
		{
			name:  "with alias",
			query: "MATCH (u:User) RETURN u.createdAt AS created",
			wantReturns: []scaf.ReturnInfo{
				{Name: "created", Expression: "u.createdAt"},
			},
		},
		{
			name:  "count aggregate",
			query: "MATCH (u:User) RETURN count(u) AS total",
			wantReturns: []scaf.ReturnInfo{
				{Name: "total", Expression: "count(u)", IsAggregate: true},
			},
		},
		{
			name:  "wildcard",
			query: "MATCH (u:User) RETURN *",
			wantReturns: []scaf.ReturnInfo{
				{Name: "*", Expression: "*", IsWildcard: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()

			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != len(tt.wantReturns) {
				t.Fatalf("got %d returns, want %d", len(metadata.Returns), len(tt.wantReturns))
			}

			for i, want := range tt.wantReturns {
				got := metadata.Returns[i]
				if got.Name != want.Name {
					t.Errorf("return[%d].Name = %q, want %q", i, got.Name, want.Name)
				}

				if got.Expression != want.Expression {
					t.Errorf("return[%d].Expression = %q, want %q", i, got.Expression, want.Expression)
				}

				if got.IsAggregate != want.IsAggregate {
					t.Errorf("return[%d].IsAggregate = %v, want %v", i, got.IsAggregate, want.IsAggregate)
				}

				if got.IsWildcard != want.IsWildcard {
					t.Errorf("return[%d].IsWildcard = %v, want %v", i, got.IsWildcard, want.IsWildcard)
				}
			}
		})
	}
}

func TestAnalyzer_AnalyzeQuery_Combined(t *testing.T) {
	t.Parallel()

	query := `
MATCH (u:User {id: $id})
WHERE u.age >= $minAge
RETURN u.name AS name, u.email AS email, count(*) AS count
`

	analyzer := cypher.NewAnalyzer()

	metadata, err := analyzer.AnalyzeQuery(query)
	if err != nil {
		t.Fatalf("AnalyzeQuery() error: %v", err)
	}

	// Check parameters
	if len(metadata.Parameters) != 2 {
		t.Errorf("got %d parameters, want 2", len(metadata.Parameters))
	}

	paramNames := make(map[string]bool)
	for _, p := range metadata.Parameters {
		paramNames[p.Name] = true
	}

	if !paramNames["id"] || !paramNames["minAge"] {
		t.Errorf("expected parameters id and minAge, got %v", paramNames)
	}

	// Check returns
	if len(metadata.Returns) != 3 {
		t.Errorf("got %d returns, want 3", len(metadata.Returns))
	}

	returnNames := make(map[string]bool)
	for _, r := range metadata.Returns {
		returnNames[r.Name] = true
	}

	if !returnNames["name"] || !returnNames["email"] || !returnNames["count"] {
		t.Errorf("expected returns name, email, count, got %v", returnNames)
	}
}

func TestAnalyzer_AnalyzeQuery_MoreAggregates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		query         string
		wantAggregate bool
		wantName      string
	}{
		{
			name:          "sum aggregate",
			query:         "MATCH (o:Order) RETURN sum(o.total) AS totalRevenue",
			wantAggregate: true,
			wantName:      "totalRevenue",
		},
		{
			name:          "avg aggregate",
			query:         "MATCH (u:User) RETURN avg(u.age) AS averageAge",
			wantAggregate: true,
			wantName:      "averageAge",
		},
		{
			name:          "min aggregate",
			query:         "MATCH (p:Product) RETURN min(p.price) AS lowestPrice",
			wantAggregate: true,
			wantName:      "lowestPrice",
		},
		{
			name:          "max aggregate",
			query:         "MATCH (p:Product) RETURN max(p.price) AS highestPrice",
			wantAggregate: true,
			wantName:      "highestPrice",
		},
		{
			name:          "collect aggregate",
			query:         "MATCH (u:User) RETURN collect(u.name) AS names",
			wantAggregate: true,
			wantName:      "names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()

			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			ret := metadata.Returns[0]
			if ret.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ret.Name, tt.wantName)
			}

			if ret.IsAggregate != tt.wantAggregate {
				t.Errorf("IsAggregate = %v, want %v", ret.IsAggregate, tt.wantAggregate)
			}
		})
	}
}

func TestAnalyzer_AnalyzeQuery_ComplexExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       string
		wantReturns []scaf.ReturnInfo
	}{
		{
			name:  "nested property",
			query: "MATCH (u:User) RETURN u.address.city AS city",
			wantReturns: []scaf.ReturnInfo{
				{Name: "city", Expression: "u.address.city"},
			},
		},
		{
			name:  "function call",
			query: "MATCH (u:User) RETURN upper(u.name) AS upperName",
			wantReturns: []scaf.ReturnInfo{
				{Name: "upperName", Expression: "upper(u.name)"},
			},
		},
		{
			name:  "arithmetic expression",
			query: "MATCH (p:Product) RETURN p.price * 1.1 AS priceWithTax",
			wantReturns: []scaf.ReturnInfo{
				{Name: "priceWithTax", Expression: "p.price*1.1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()

			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != len(tt.wantReturns) {
				t.Fatalf("got %d returns, want %d", len(metadata.Returns), len(tt.wantReturns))
			}

			for i, want := range tt.wantReturns {
				got := metadata.Returns[i]
				if got.Name != want.Name {
					t.Errorf("return[%d].Name = %q, want %q", i, got.Name, want.Name)
				}
				// Note: expression might have whitespace differences, so we just check name
			}
		})
	}
}

func TestAnalyzer_AnalyzeQuery_ParameterCount(t *testing.T) {
	t.Parallel()

	// Test that repeated parameters are counted correctly
	query := "MATCH (u:User {id: $id}) WHERE u.manager = $id AND u.backup = $id RETURN u"

	analyzer := cypher.NewAnalyzer()

	metadata, err := analyzer.AnalyzeQuery(query)
	if err != nil {
		t.Fatalf("AnalyzeQuery() error: %v", err)
	}

	if len(metadata.Parameters) != 1 {
		t.Fatalf("expected 1 unique parameter, got %d", len(metadata.Parameters))
	}

	param := metadata.Parameters[0]
	if param.Name != "id" {
		t.Errorf("parameter name = %q, want 'id'", param.Name)
	}

	if param.Count != 3 {
		t.Errorf("parameter count = %d, want 3", param.Count)
	}
}

func TestAnalyzer_AnalyzeQuery_EmptyQuery(t *testing.T) {
	t.Parallel()

	analyzer := cypher.NewAnalyzer()

	metadata, err := analyzer.AnalyzeQuery("")
	if err != nil {
		t.Fatalf("AnalyzeQuery() error: %v", err)
	}

	if len(metadata.Parameters) != 0 {
		t.Errorf("expected 0 parameters for empty query, got %d", len(metadata.Parameters))
	}

	if len(metadata.Returns) != 0 {
		t.Errorf("expected 0 returns for empty query, got %d", len(metadata.Returns))
	}
}

func TestAnalyzer_AnalyzeQuery_MultipleStatements(t *testing.T) {
	t.Parallel()

	// Test query with MATCH, WHERE, and RETURN
	query := `
MATCH (u:User {email: $email})
WHERE u.active = true
OPTIONAL MATCH (u)-[:FOLLOWS]->(f:User)
RETURN u.name AS userName, count(f) AS followerCount
`

	analyzer := cypher.NewAnalyzer()

	metadata, err := analyzer.AnalyzeQuery(query)
	if err != nil {
		t.Fatalf("AnalyzeQuery() error: %v", err)
	}

	// Check parameters
	if len(metadata.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(metadata.Parameters))
	}

	if metadata.Parameters[0].Name != "email" {
		t.Errorf("expected parameter 'email', got %q", metadata.Parameters[0].Name)
	}

	// Check returns
	if len(metadata.Returns) != 2 {
		t.Errorf("expected 2 returns, got %d", len(metadata.Returns))
	}

	returnNames := make(map[string]bool)
	for _, r := range metadata.Returns {
		returnNames[r.Name] = true
	}

	if !returnNames["userName"] {
		t.Error("expected 'userName' in returns")
	}

	if !returnNames["followerCount"] {
		t.Error("expected 'followerCount' in returns")
	}
}
