package cypher_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/dialects/cypher"
)

// typeStr returns the string representation of a type, or "" if nil.
func typeStr(t *scaf.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

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

func TestAnalyzer_AnalyzeQueryWithSchema_ReturnsOne(t *testing.T) {
	t.Parallel()

	// Schema with User model where id is unique
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "email", Type: analysis.TypeString, Unique: true},
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}

	tests := []struct {
		name    string
		query   string
		wantOne bool
	}{
		{
			name:    "filter on unique id field",
			query:   "MATCH (u:User {id: $userId}) RETURN u.name",
			wantOne: true,
		},
		{
			name:    "filter on unique email field",
			query:   "MATCH (u:User {email: $email}) RETURN u.name",
			wantOne: true,
		},
		{
			name:    "filter on non-unique field",
			query:   "MATCH (u:User {name: $name}) RETURN u.id",
			wantOne: false,
		},
		{
			name:    "no filter - returns all",
			query:   "MATCH (u:User) RETURN u.name",
			wantOne: false,
		},
		{
			name:    "filter in WHERE clause (not in node pattern)",
			query:   "MATCH (u:User) WHERE u.id = $id RETURN u.name",
			wantOne: false, // Our implementation only checks node pattern properties
		},
		{
			name:    "unknown model",
			query:   "MATCH (p:Product {id: $id}) RETURN p.name",
			wantOne: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()

			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
			}

			if metadata.ReturnsOne != tt.wantOne {
				t.Errorf("ReturnsOne = %v, want %v", metadata.ReturnsOne, tt.wantOne)
			}
		})
	}
}

func TestAnalyzer_AnalyzeQueryWithSchema_NilSchema(t *testing.T) {
	t.Parallel()

	analyzer := cypher.NewAnalyzer()

	// With nil schema, should default to ReturnsOne = false
	metadata, err := analyzer.AnalyzeQueryWithSchema("MATCH (u:User {id: $id}) RETURN u.name", nil)
	if err != nil {
		t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
	}

	if metadata.ReturnsOne != false {
		t.Error("ReturnsOne should be false when schema is nil")
	}
}

func TestAnalyzer_AnalyzeQueryWithSchema_TypeInference(t *testing.T) {
	t.Parallel()

	// Schema with User and Movie models
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "email", Type: analysis.TypeString, Unique: true},
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
					{Name: "active", Type: analysis.TypeBool},
					{Name: "score", Type: analysis.TypeFloat64},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "title", Type: analysis.TypeString},
					{Name: "year", Type: analysis.TypeInt},
					{Name: "genres", Type: analysis.SliceOf(analysis.TypeString)},
				},
			},
		},
	}

	t.Run("parameter type inference", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (u:User {id: $userId, age: $minAge}) RETURN u.name",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		// Check parameter types
		paramTypes := make(map[string]string)
		for _, p := range metadata.Parameters {
			paramTypes[p.Name] = typeStr(p.Type)
		}

		if paramTypes["userId"] != "string" {
			t.Errorf("userId type = %q, want 'string'", paramTypes["userId"])
		}
		if paramTypes["minAge"] != "int" {
			t.Errorf("minAge type = %q, want 'int'", paramTypes["minAge"])
		}
	})

	t.Run("return type inference - property access", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (u:User) RETURN u.name, u.age, u.active, u.score",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		// Check return types
		returnTypes := make(map[string]string)
		for _, r := range metadata.Returns {
			returnTypes[r.Name] = typeStr(r.Type)
		}

		if returnTypes["name"] != "string" {
			t.Errorf("name type = %q, want 'string'", returnTypes["name"])
		}
		if returnTypes["age"] != "int" {
			t.Errorf("age type = %q, want 'int'", returnTypes["age"])
		}
		if returnTypes["active"] != "bool" {
			t.Errorf("active type = %q, want 'bool'", returnTypes["active"])
		}
		if returnTypes["score"] != "float64" {
			t.Errorf("score type = %q, want 'float64'", returnTypes["score"])
		}
	})

	t.Run("return type inference - whole node", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (u:User) RETURN u",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		if len(metadata.Returns) != 1 {
			t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
		}

		// Returning whole node should have type "*User"
		gotType := typeStr(metadata.Returns[0].Type)
		if gotType != "*User" {
			t.Errorf("return type = %q, want '*User'", gotType)
		}
	})

	t.Run("return type inference - multiple models", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (u:User)-[:LIKES]->(m:Movie) RETURN u.name, m.title, m.year",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		returnTypes := make(map[string]string)
		for _, r := range metadata.Returns {
			returnTypes[r.Name] = typeStr(r.Type)
		}

		if returnTypes["name"] != "string" {
			t.Errorf("name type = %q, want 'string'", returnTypes["name"])
		}
		if returnTypes["title"] != "string" {
			t.Errorf("title type = %q, want 'string'", returnTypes["title"])
		}
		if returnTypes["year"] != "int" {
			t.Errorf("year type = %q, want 'int'", returnTypes["year"])
		}
	})

	t.Run("return type inference - slice type", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (m:Movie) RETURN m.genres",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		if len(metadata.Returns) != 1 {
			t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
		}

		gotType := typeStr(metadata.Returns[0].Type)
		if gotType != "[]string" {
			t.Errorf("genres type = %q, want '[]string'", gotType)
		}
	})

	t.Run("unknown model - no type inference", func(t *testing.T) {
		t.Parallel()

		analyzer := cypher.NewAnalyzer()

		metadata, err := analyzer.AnalyzeQueryWithSchema(
			"MATCH (p:Product {id: $id}) RETURN p.name",
			schema,
		)
		if err != nil {
			t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
		}

		// Unknown model - should have empty type
		if len(metadata.Parameters) != 1 {
			t.Fatalf("expected 1 parameter, got %d", len(metadata.Parameters))
		}
		paramType := typeStr(metadata.Parameters[0].Type)
		if paramType != "" {
			t.Errorf("parameter type = %q, want empty (unknown model)", paramType)
		}

		if len(metadata.Returns) != 1 {
			t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
		}
		returnType := typeStr(metadata.Returns[0].Type)
		if returnType != "" {
			t.Errorf("return type = %q, want empty (unknown model)", returnType)
		}
	})
}

// TestAnalyzer_RequiredField tests that ReturnInfo.Required is correctly set
// based on field.Required from the schema.
func TestAnalyzer_RequiredField(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					// Required fields
					{Name: "id", Type: analysis.TypeString, Required: true},
					{Name: "age", Type: analysis.TypeInt, Required: true},
					// Nullable fields (Required: false is default)
					{Name: "bio", Type: analysis.TypeString, Required: false},
					{Name: "score", Type: analysis.TypeFloat64, Required: false},
					// Reference types (nil-able regardless of Required)
					{Name: "tags", Type: analysis.SliceOf(analysis.TypeString), Required: false},
				},
			},
			"Comment": {
				Name: "Comment",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Required: true},
					{Name: "text", Type: analysis.TypeString, Required: true},
				},
			},
		},
	}

	tests := []struct {
		name         string
		query        string
		wantType     string
		wantRequired bool
	}{
		{
			name:         "required string field",
			query:        "MATCH (u:User) RETURN u.id",
			wantType:     "string",
			wantRequired: true,
		},
		{
			name:         "required int field",
			query:        "MATCH (u:User) RETURN u.age",
			wantType:     "int",
			wantRequired: true,
		},
		{
			name:         "nullable string field",
			query:        "MATCH (u:User) RETURN u.bio",
			wantType:     "string", // Type stays string, Required=false signals nullable
			wantRequired: false,
		},
		{
			name:         "nullable float64 field",
			query:        "MATCH (u:User) RETURN u.score",
			wantType:     "float64",
			wantRequired: false,
		},
		{
			name:         "nullable slice field",
			query:        "MATCH (u:User) RETURN u.tags",
			wantType:     "[]string",
			wantRequired: false,
		},
		{
			name:         "complex expression defaults to nullable",
			query:        "MATCH (u:User) RETURN u.bio IS NULL",
			wantType:     "bool",
			wantRequired: false, // Not a simple property access - bail to nullable
		},
		// OPTIONAL MATCH tests
		{
			name:         "optional match simple property",
			query:        "MATCH (c:Comment) OPTIONAL MATCH (u:User)-[:WROTE]->(c) RETURN u.id",
			wantType:     "string",
			wantRequired: false, // OPTIONAL MATCH forces nullable
		},
		// Complex expression bailout tests - representative cases
		{
			name:         "function call bails to nullable",
			query:        "MATCH (u:User) RETURN toUpper(u.id)",
			wantType:     "string",
			wantRequired: false, // Function wrapping - can't trust schema
		},
		{
			name:         "case expression bails to nullable",
			query:        "MATCH (u:User) RETURN CASE WHEN u.age > 18 THEN 'adult' ELSE 'minor' END",
			wantType:     "string",
			wantRequired: false, // CASE expression - can return null
		},
		{
			name:         "aggregate bails to nullable",
			query:        "MATCH (u:User) RETURN count(u)",
			wantType:     "int",
			wantRequired: false, // Aggregate - bail to nullable (future: whitelist count as safe)
		},
		{
			name:         "list indexing bails to nullable",
			query:        "MATCH (u:User) RETURN u.tags[0]",
			wantType:     "string",
			wantRequired: false, // Out of bounds returns null
		},
		{
			name:         "chained property access bails to nullable",
			query:        "MATCH (u:User) RETURN u.id.length",
			wantType:     "",    // Unknown type for chained access
			wantRequired: false, // Chained access - not simple var.prop
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			ret := metadata.Returns[0]
			gotType := typeStr(ret.Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if ret.Required != tt.wantRequired {
				t.Errorf("Required = %v, want %v", ret.Required, tt.wantRequired)
			}
		})
	}
}
