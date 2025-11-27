package golang

import (
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import cypher dialect to register the analyzer
	_ "github.com/rlch/scaf/dialects/cypher"
)

func TestExtractSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []*FuncSignature
	}{
		{
			name: "simple query with parameter and return",
			input: `
query getUserById ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name AS name
` + "`" + `
`,
			expected: []*FuncSignature{
				{
					Name:      "GetUserById",
					QueryName: "getUserById",
					Params: []FuncParam{
						{Name: "userId", Type: "any", Required: true},
					},
					Returns: []FuncReturn{
						{Name: "name", Type: "any", IsSlice: false},
					},
				},
			},
		},
		{
			name: "query with multiple parameters",
			input: `
query findUsers ` + "`" + `
MATCH (u:User)
WHERE u.age > $minAge AND u.active = $isActive
RETURN u.id AS id, u.name AS name
` + "`" + `
`,
			expected: []*FuncSignature{
				{
					Name:      "FindUsers",
					QueryName: "findUsers",
					Params: []FuncParam{
						{Name: "minAge", Type: "any", Required: true},
						{Name: "isActive", Type: "any", Required: true},
					},
					Returns: []FuncReturn{
						{Name: "id", Type: "any", IsSlice: false},
						{Name: "name", Type: "any", IsSlice: false},
					},
				},
			},
		},
		{
			name: "query with aggregate return",
			input: `
query countUsers ` + "`" + `
MATCH (u:User)
RETURN count(u) AS count
` + "`" + `
`,
			expected: []*FuncSignature{
				{
					Name:      "CountUsers",
					QueryName: "countUsers",
					Params:    []FuncParam{},
					Returns: []FuncReturn{
						{Name: "count", Type: "any", IsSlice: false},
					},
				},
			},
		},
		{
			name: "snake_case query name",
			input: `
query get_user_by_id ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name
` + "`" + `
`,
			expected: []*FuncSignature{
				{
					Name:      "GetUserByID",
					QueryName: "get_user_by_id",
					Params: []FuncParam{
						{Name: "userId", Type: "any", Required: true},
					},
					Returns: []FuncReturn{
						{Name: "name", Type: "any", IsSlice: false},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			suite, err := scaf.Parse([]byte(tt.input))
			require.NoError(t, err)

			analyzer := scaf.GetAnalyzer("cypher")
			require.NotNil(t, analyzer, "cypher analyzer should be registered")

			sigs, err := ExtractSignatures(suite, analyzer, nil)
			require.NoError(t, err)
			require.Len(t, sigs, len(tt.expected))

			for i, expected := range tt.expected {
				actual := sigs[i]
				assert.Equal(t, expected.Name, actual.Name, "function name")
				assert.Equal(t, expected.QueryName, actual.QueryName, "query name")

				require.Len(t, actual.Params, len(expected.Params), "param count")
				for j, ep := range expected.Params {
					ap := actual.Params[j]
					assert.Equal(t, ep.Name, ap.Name, "param name")
					assert.Equal(t, ep.Type, ap.Type, "param type for %s", ep.Name)
					assert.Equal(t, ep.Required, ap.Required, "param required")
				}

				require.Len(t, actual.Returns, len(expected.Returns), "return count")
				for j, er := range expected.Returns {
					ar := actual.Returns[j]
					assert.Equal(t, er.Name, ar.Name, "return name")
					assert.Equal(t, er.Type, ar.Type, "return type for %s", er.Name)
				}
			}
		})
	}
}

func TestExtractSignaturesWithSchema(t *testing.T) {
	t.Parallel()

	// Create a schema with User model
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Required: true},
					{Name: "name", Type: analysis.TypeString, Required: true},
					{Name: "age", Type: analysis.TypeInt, Required: false},
					{Name: "email", Type: analysis.TypeString, Required: true},
					{Name: "balance", Type: analysis.TypeFloat64, Required: false},
					{Name: "active", Type: analysis.TypeBool, Required: true},
					{Name: "createdAt", Type: analysis.NamedType("time", "Time"), Required: true},
				},
			},
		},
	}

	input := `
query getUser ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.name AS name, u.age AS age, u.balance AS balance, u.createdAt AS createdAt
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, schema)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]

	// Param type should come from schema
	require.Len(t, sig.Params, 1)
	assert.Equal(t, "id", sig.Params[0].Name)
	assert.Equal(t, "string", sig.Params[0].Type)

	// Return types should come from schema
	require.Len(t, sig.Returns, 4)
	assert.Equal(t, "string", sig.Returns[0].Type, "name type from schema")
	assert.Equal(t, "int", sig.Returns[1].Type, "age type from schema")
	assert.Equal(t, "float64", sig.Returns[2].Type, "balance type from schema")
	assert.Equal(t, "time.Time", sig.Returns[3].Type, "createdAt type from schema")
}

func TestExtractSignaturesNilSuite(t *testing.T) {
	t.Parallel()

	sigs, err := ExtractSignatures(nil, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, sigs)
}

func TestExtractSignatureWithWildcard(t *testing.T) {
	t.Parallel()

	// Query with RETURN * should skip wildcard returns
	input := `
query getAllUsers ` + "`" + `
MATCH (u:User)
RETURN *
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	// Wildcard returns should be skipped
	assert.Empty(t, sigs[0].Returns)
}

func TestExtractSignaturesNilAnalyzer(t *testing.T) {
	t.Parallel()

	input := `
query getUser ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.name AS name
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	// Without analyzer, we get basic signature with no params/returns
	sigs, err := ExtractSignatures(suite, nil, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	assert.Equal(t, "GetUser", sig.Name)
	assert.Empty(t, sig.Params)
	assert.Empty(t, sig.Returns)
}

func TestInferParamTypeWithAnalyzerHint(t *testing.T) {
	t.Parallel()

	// When analyzer provides a type hint, use it
	param := scaf.ParameterInfo{
		Name: "userId",
		Type: "string", // Analyzer-provided type hint
	}

	typ := inferParamType(param, nil)
	assert.Equal(t, "string", typ)
}

func TestInferReturnTypeWithAnalyzerHint(t *testing.T) {
	t.Parallel()

	// When analyzer provides a type hint, use it
	ret := scaf.ReturnInfo{
		Name: "count",
		Type: "integer", // Analyzer-provided type hint
	}

	typ := inferReturnType(ret, nil)
	assert.Equal(t, "int64", typ)
}

func TestLookupFieldType(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
			"Post": {
				Name: "Post",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
				},
			},
		},
	}

	// Found in first model
	typ := lookupFieldType("id", schema)
	require.NotNil(t, typ)
	assert.Equal(t, "string", typ.String())

	// Found in second model
	typ = lookupFieldType("title", schema)
	require.NotNil(t, typ)
	assert.Equal(t, "string", typ.String())

	// Not found
	assert.Nil(t, lookupFieldType("nonexistent", schema))

	// Empty field name
	assert.Nil(t, lookupFieldType("", schema))

	// Nil schema
	assert.Nil(t, lookupFieldType("id", nil))
}

func TestToExportedName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"getUserById", "GetUserById"},
		{"get_user_by_id", "GetUserByID"},
		{"GetUser", "GetUser"},
		{"createUser", "CreateUser"},
		{"find_all_users", "FindAllUsers"},
		{"get_user_url", "GetUserURL"},
		{"get_api_key", "GetAPIKey"},
		{"", ""},
		// Edge case: leading/trailing underscores
		{"_private_func", "PrivateFunc"},
		{"get__double", "GetDouble"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toExportedName(tt.input))
		})
	}
}

func TestLookupFieldTypeInModel(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}

	// Found
	typ := LookupFieldTypeInModel("User", "id", schema)
	require.NotNil(t, typ)
	assert.Equal(t, "string", typ.String())

	typ = LookupFieldTypeInModel("User", "age", schema)
	require.NotNil(t, typ)
	assert.Equal(t, "int", typ.String())

	// Not found - wrong model
	assert.Nil(t, LookupFieldTypeInModel("Post", "id", schema))

	// Not found - wrong field
	assert.Nil(t, LookupFieldTypeInModel("User", "email", schema))

	// Nil schema
	assert.Nil(t, LookupFieldTypeInModel("User", "id", nil))
}

func TestMapAnalyzerType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		// String types
		{"string", "string"},
		{"text", "string"},
		{"STRING", "string"},

		// Integer types
		{"int", "int64"},
		{"integer", "int64"},
		{"long", "int64"},

		// Float types
		{"float", "float64"},
		{"double", "float64"},
		{"decimal", "float64"},

		// Boolean types
		{"bool", "bool"},
		{"boolean", "bool"},

		// Date/time types
		{"date", "time.Time"},
		{"datetime", "time.Time"},
		{"timestamp", "time.Time"},

		// Collection types
		{"list", "[]any"},
		{"array", "[]any"},
		{"map", "map[string]any"},
		{"object", "map[string]any"},

		// Unknown defaults to any
		{"unknown", "any"},
		{"", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, mapAnalyzerType(tt.input))
		})
	}
}

func TestTypeToGoString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "any", TypeToGoString(nil))
	assert.Equal(t, "string", TypeToGoString(analysis.TypeString))
	assert.Equal(t, "int", TypeToGoString(analysis.TypeInt))
	assert.Equal(t, "[]string", TypeToGoString(analysis.SliceOf(analysis.TypeString)))
	assert.Equal(t, "*int", TypeToGoString(analysis.PointerTo(analysis.TypeInt)))
	assert.Equal(t, "time.Time", TypeToGoString(analysis.NamedType("time", "Time")))
}
