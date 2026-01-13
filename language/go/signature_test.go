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
fn getUserById() ` + "`" + `
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
fn findUsers() ` + "`" + `
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
fn countUsers() ` + "`" + `
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
						{Name: "count", Type: "*int", IsSlice: false}, // Conservative: complex expression bails to nullable
					},
				},
			},
		},
		{
			name: "snake_case query name",
			input: `
fn get_user_by_id() ` + "`" + `
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
fn getUser() ` + "`" + `
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
	// Non-required fields (age, balance) should be pointers
	require.Len(t, sig.Returns, 4)
	assert.Equal(t, "string", sig.Returns[0].Type, "name type from schema (required)")
	assert.Equal(t, "*int", sig.Returns[1].Type, "age type should be pointer (not required)")
	assert.Equal(t, "*float64", sig.Returns[2].Type, "balance type should be pointer (not required)")
	assert.Equal(t, "time.Time", sig.Returns[3].Type, "createdAt type from schema (required)")
}

func TestExtractSignaturesNullableReturnTypes(t *testing.T) {
	t.Parallel()

	// Reproduce the bug: schema field with required: false should generate pointer type
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeInt, Required: true, Unique: true},
					{Name: "name", Type: analysis.TypeString, Required: true},
					{Name: "bio", Type: analysis.TypeString, Required: false}, // nullable
				},
			},
		},
	}

	input := `
fn getUser() ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.id AS id, u.name AS name, u.bio AS bio
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, schema)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Returns, 3)

	// Required fields stay non-pointer
	assert.Equal(t, "int", sig.Returns[0].Type, "id should be int (required)")
	assert.Equal(t, "string", sig.Returns[1].Type, "name should be string (required)")

	// Non-required field should be pointer to allow nil
	assert.Equal(t, "*string", sig.Returns[2].Type, "bio should be *string (not required)")
}

func TestExtractSignaturesNullableNamedTypes(t *testing.T) {
	t.Parallel()

	// Test that named types (like time.Time) also get wrapped in pointer when nullable
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeInt, Required: true},
					{Name: "createdAt", Type: analysis.NamedType("time", "Time"), Required: true},
					{Name: "deletedAt", Type: analysis.NamedType("time", "Time"), Required: false}, // nullable
				},
			},
		},
	}

	input := `
fn getUser() ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.id AS id, u.createdAt AS createdAt, u.deletedAt AS deletedAt
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, schema)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Returns, 3)

	assert.Equal(t, "int", sig.Returns[0].Type, "id should be int (required)")
	assert.Equal(t, "time.Time", sig.Returns[1].Type, "createdAt should be time.Time (required)")
	assert.Equal(t, "*time.Time", sig.Returns[2].Type, "deletedAt should be *time.Time (not required)")
}

func TestExtractSignaturesNoDoublePointer(t *testing.T) {
	t.Parallel()

	// Test that already-pointer types don't get double-wrapped
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeInt, Required: true},
					// Field is already a pointer type AND not required
					{Name: "ref", Type: analysis.PointerTo(analysis.TypeString), Required: false},
				},
			},
		},
	}

	input := `
fn getUser() ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.id AS id, u.ref AS ref
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, schema)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Returns, 2)

	assert.Equal(t, "int", sig.Returns[0].Type, "id should be int")
	// Should NOT become **string - already a pointer
	assert.Equal(t, "*string", sig.Returns[1].Type, "ref should stay *string, not become **string")
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
fn getAllUsers() ` + "`" + `
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
fn getUser() ` + "`" + `
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

func TestExtractSignaturesWithExplicitTypes(t *testing.T) {
	t.Parallel()

	// Test explicit type annotations take precedence over schema inference
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Required: true}, // schema says string
				},
			},
		},
	}

	// Explicit type annotation: id: int (different from schema's string)
	input := `
fn getUser(id: int, name: string?) ` + "`" + `
MATCH (u:User {id: $id, name: $name})
RETURN u
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, schema)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]

	// Explicit type annotation should take precedence over schema
	require.Len(t, sig.Params, 2)
	assert.Equal(t, "id", sig.Params[0].Name)
	assert.Equal(t, "int", sig.Params[0].Type, "explicit type should override schema")
	assert.Equal(t, "name", sig.Params[1].Name)
	assert.Equal(t, "*string", sig.Params[1].Type, "nullable should become pointer")
}

func TestExtractSignaturesWithExplicitArrayType(t *testing.T) {
	t.Parallel()

	input := `
fn getUsersByIds(ids: [string]) ` + "`" + `
MATCH (u:User) WHERE u.id IN $ids
RETURN u
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Params, 1)
	assert.Equal(t, "ids", sig.Params[0].Name)
	assert.Equal(t, "[]string", sig.Params[0].Type, "array type should be []string")
}

func TestExtractSignaturesWithExplicitMapType(t *testing.T) {
	t.Parallel()

	input := `
fn createUser(data: {string: any}) ` + "`" + `
CREATE (u:User $data)
RETURN u
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Params, 1)
	assert.Equal(t, "data", sig.Params[0].Name)
	assert.Equal(t, "map[string]any", sig.Params[0].Type, "map type should be map[string]any")
}

func TestExtractSignaturesWithExplicitTypesNoAnalyzer(t *testing.T) {
	t.Parallel()

	// When no analyzer, explicit types should still be used
	input := `
fn getUser(id: int, active: bool) ` + "`" + `
MATCH (u:User {id: $id, active: $active})
RETURN u
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	// Without analyzer, we use explicit types from function definition
	sigs, err := ExtractSignatures(suite, nil, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]
	require.Len(t, sig.Params, 2)
	assert.Equal(t, "id", sig.Params[0].Name)
	assert.Equal(t, "int", sig.Params[0].Type)
	assert.Equal(t, "active", sig.Params[1].Name)
	assert.Equal(t, "bool", sig.Params[1].Type)
}

func TestInferParamTypeWithAnalyzerHint(t *testing.T) {
	t.Parallel()

	// When analyzer provides a type hint, use it
	param := scaf.ParameterInfo{
		Name: "userId",
		Type: scaf.TypeString, // Analyzer-provided type hint
	}

	typ := inferParamType(param, nil)
	assert.Equal(t, "string", typ)
}

func TestInferReturnTypeWithAnalyzerHint(t *testing.T) {
	t.Parallel()

	// When analyzer provides a type, use it directly
	// The Cypher analyzer now returns Go type strings from schema
	ret := scaf.ReturnInfo{
		Name:     "count",
		Type:     scaf.TypeInt64, // Analyzer-provided Go type from schema
		Required: true,           // Required field - no pointer wrapping
	}

	typ := inferReturnType(ret, nil)
	assert.Equal(t, "int64", typ)
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

func TestTypeToGoString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "any", TypeToGoString(nil))
	assert.Equal(t, "string", TypeToGoString(analysis.TypeString))
	assert.Equal(t, "int", TypeToGoString(analysis.TypeInt))
	assert.Equal(t, "[]string", TypeToGoString(analysis.SliceOf(analysis.TypeString)))
	assert.Equal(t, "*int", TypeToGoString(analysis.PointerTo(analysis.TypeInt)))
	assert.Equal(t, "time.Time", TypeToGoString(analysis.NamedType("time", "Time")))
}

func TestResultStructGeneration(t *testing.T) {
	t.Parallel()

	// Query with multiple returns should generate a result struct name
	input := `
fn getUser() ` + "`" + `
MATCH (u:User {id: $id})
RETURN u.name AS name, u.email AS email
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]

	// Should generate result struct name for multiple returns
	assert.Equal(t, "getUserResult", sig.ResultStruct, "should generate result struct name")

	// Returns should still be populated for column mapping
	require.Len(t, sig.Returns, 2)
	assert.Equal(t, "name", sig.Returns[0].Name)
	assert.Equal(t, "email", sig.Returns[1].Name)
}

func TestResultStructNotGeneratedForSingleReturn(t *testing.T) {
	t.Parallel()

	// Query with single return should NOT generate a result struct
	input := `
fn getName() ` + "`" + `
MATCH (u:User)
RETURN u.name AS name
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	sigs, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)
	require.Len(t, sigs, 1)

	sig := sigs[0]

	// No result struct for single return
	assert.Empty(t, sig.ResultStruct, "should not generate result struct for single return")
	require.Len(t, sig.Returns, 1)
}

func TestToResultStructName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"GetUser", "getUserResult"},
		{"FindAllPosts", "findAllPostsResult"},
		{"CountUsers", "countUsersResult"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toResultStructName(tt.input))
		})
	}
}

func TestToExportedFieldName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"name", "Name"},
		{"email", "Email"},
		{"user_id", "UserID"},
		{"created_at", "CreatedAt"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toExportedFieldName(tt.input))
		})
	}
}
