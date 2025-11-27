package golang

import (
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/language"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import cypher dialect to register the analyzer
	_ "github.com/rlch/scaf/dialects/cypher"
)

func TestExtractTestCases(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name, u.age
` + "`" + `

GetUser {
	test "finds Alice" {
		$userId: 1
		u.name: "Alice"
		u.age: 30
	}

	test "finds Bob" {
		$userId: 2
		u.name: "Bob"
		u.age: 25
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)
	require.Len(t, suite.Scopes, 1)

	cases := ExtractTestCases(suite.Scopes[0])
	require.Len(t, cases, 2)

	// First test case
	assert.Equal(t, "finds Alice", cases[0].Name)
	assert.Equal(t, float64(1), cases[0].Inputs["$userId"])
	assert.Equal(t, "Alice", cases[0].Outputs["u.name"])
	assert.Equal(t, float64(30), cases[0].Outputs["u.age"])

	// Second test case
	assert.Equal(t, "finds Bob", cases[1].Name)
	assert.Equal(t, float64(2), cases[1].Inputs["$userId"])
	assert.Equal(t, "Bob", cases[1].Outputs["u.name"])
	assert.Equal(t, float64(25), cases[1].Outputs["u.age"])
}

func TestExtractTestCasesWithGroups(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name
` + "`" + `

GetUser {
	test "basic test" {
		$userId: 1
		u.name: "Alice"
	}

	group "expected failures" {
		test "wrong name" {
			$userId: 1
			u.name: "Wrong"
		}

		test "missing user" {
			$userId: 999
			u.name: null
		}
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)
	require.Len(t, suite.Scopes, 1)

	cases := ExtractTestCases(suite.Scopes[0])
	require.Len(t, cases, 3)

	assert.Equal(t, "basic test", cases[0].Name)
	assert.Equal(t, "wrong name", cases[1].Name)
	assert.Equal(t, "missing user", cases[2].Name)
}

func TestExtractTestCasesNullValues(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name
` + "`" + `

GetUser {
	test "null result" {
		$userId: 999
		u.name: null
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)
	require.Len(t, suite.Scopes, 1)

	cases := ExtractTestCases(suite.Scopes[0])
	require.Len(t, cases, 1)

	assert.Equal(t, "null result", cases[0].Name)
	assert.Nil(t, cases[0].Outputs["u.name"])
}

func TestExtractTestCasesComplexValues(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User)
RETURN u
` + "`" + `

GetUser {
	test "with map input" {
		$filter: {name: "Alice", age: 30}
		u.name: "Alice"
	}

	test "with list input" {
		$ids: [1, 2, 3]
		u.name: "Multiple"
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)
	require.Len(t, suite.Scopes, 1)

	cases := ExtractTestCases(suite.Scopes[0])
	require.Len(t, cases, 2)

	// Map input
	mapVal, ok := cases[0].Inputs["$filter"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Alice", mapVal["name"])
	assert.Equal(t, float64(30), mapVal["age"])

	// List input
	listVal, ok := cases[1].Inputs["$ids"].([]any)
	require.True(t, ok)
	assert.Len(t, listVal, 3)
	assert.Equal(t, float64(1), listVal[0])
}

func TestBuildMockFuncs(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name
` + "`" + `

query CountUsers ` + "`" + `
MATCH (u:User)
RETURN count(u) as count
` + "`" + `

GetUser {
	test "finds Alice" {
		$userId: 1
		u.name: "Alice"
	}
}

CountUsers {
	test "counts all" {
		count: 2
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")
	signatures, err := ExtractSignatures(suite, analyzer, nil)
	require.NoError(t, err)

	mocks := BuildMockFuncs(suite, signatures)
	require.Len(t, mocks, 2)

	assert.Equal(t, "GetUser", mocks[0].Signature.Name)
	assert.Len(t, mocks[0].TestCases, 1)

	assert.Equal(t, "CountUsers", mocks[1].Signature.Name)
	assert.Len(t, mocks[1].TestCases, 1)
}

func TestGoLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, "nil"},
		{"string", "hello", `"hello"`},
		{"string with quotes", `say "hi"`, `"say \"hi\""`},
		{"integer float", float64(42), "42"},
		{"float", float64(3.14), "3.14"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"empty map", map[string]any{}, "map[string]any{}"},
		{"map", map[string]any{"a": "b"}, `map[string]any{"a": "b"}`},
		{"empty slice", []any{}, "[]any{}"},
		{"slice", []any{1.0, "two"}, `[]any{1, "two"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := goLiteral(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestZeroValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ      string
		expected string
	}{
		{"string", `""`},
		{"int", "0"},
		{"int64", "0"},
		{"float64", "0"},
		{"bool", "false"},
		{"any", "nil"},
		{"*User", "nil"},
		{"[]string", "nil"},
		{"map[string]int", "nil"},
		{"User", "User{}"},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, zeroValue(tt.typ))
		})
	}
}

func TestIsComplexType(t *testing.T) {
	t.Parallel()

	assert.True(t, isComplexType(map[string]any{}))
	assert.True(t, isComplexType([]any{}))
	assert.False(t, isComplexType("string"))
	assert.False(t, isComplexType(42))
	assert.False(t, isComplexType(true))
	assert.False(t, isComplexType(nil))
}

func TestGenerateMockFile(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name AS name, u.age AS age
` + "`" + `

GetUser {
	test "finds Alice" {
		$userId: 1
		name: "Alice"
		age: 30
	}

	test "finds Bob" {
		$userId: 2
		name: "Bob"
		age: 25
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)
	require.NotNil(t, content)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

func init() {
	getUserImpl = getUserMock
}

func getUserMock(userId any) (any, any) {
	if userId == 1 {
		return "Alice", 30
	} else if userId == 2 {
		return "Bob", 25
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestGenerateMockFileWithComplexTypes(t *testing.T) {
	t.Parallel()

	input := `
query FindUsers ` + "`" + `
MATCH (u:User)
WHERE u.id IN $ids
RETURN u.name AS name
` + "`" + `

FindUsers {
	test "multiple ids" {
		$ids: [1, 2, 3]
		name: "Multiple"
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

import (
	"reflect"
)

func init() {
	findUsersImpl = findUsersMock
}

func findUsersMock(ids any) any {
	if reflect.DeepEqual(ids, []any{1, 2, 3}) {
		return "Multiple"
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestGenerateMockFileNoScopes(t *testing.T) {
	t.Parallel()

	// Query without any test scopes
	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name AS name
` + "`" + `
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	// Should return nil when no test scopes
	assert.Nil(t, content)
}

func TestGenerateMockFileNilSuite(t *testing.T) {
	t.Parallel()

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite: nil,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	content, err := gen.generateMockFile(nil)
	require.NoError(t, err)
	assert.Nil(t, content)
}

func TestGenerateMockFileMultipleFunctions(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name AS name
` + "`" + `

query GetPost ` + "`" + `
MATCH (p:Post {id: $postId})
RETURN p.title AS title
` + "`" + `

GetUser {
	test "gets user" {
		$userId: 1
		name: "Alice"
	}
}

GetPost {
	test "gets post" {
		$postId: 100
		title: "Hello World"
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

func init() {
	getUserImpl = getUserMock
	getPostImpl = getPostMock
}

func getUserMock(userId any) any {
	if userId == 1 {
		return "Alice"
	}
	panic("no matching test case")
}

func getPostMock(postId any) any {
	if postId == 100 {
		return "Hello World"
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestGenerateMockFileBooleanValues(t *testing.T) {
	t.Parallel()

	input := `
query CheckActive ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.active AS active
` + "`" + `

CheckActive {
	test "active user" {
		$userId: 1
		active: true
	}

	test "inactive user" {
		$userId: 2
		active: false
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

func init() {
	checkActiveImpl = checkActiveMock
}

func checkActiveMock(userId any) any {
	if userId == 1 {
		return true
	} else if userId == 2 {
		return false
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestGenerateMockFileNoReturns(t *testing.T) {
	t.Parallel()

	input := `
query DeleteUser ` + "`" + `
MATCH (u:User {id: $userId})
DELETE u
` + "`" + `

DeleteUser {
	test "deletes user" {
		$userId: 1
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

func init() {
	deleteUserImpl = deleteUserMock
}

func deleteUserMock(userId any) {
	if userId == 1 {
		return
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestGenerateMockFileMissingOutputUsesZeroValue(t *testing.T) {
	t.Parallel()

	input := `
query GetUser ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name AS name, u.age AS age
` + "`" + `

GetUser {
	test "partial output" {
		$userId: 1
		name: "Alice"
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	require.NoError(t, err)

	analyzer := scaf.GetAnalyzer("cypher")

	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
		},
		PackageName: "testpkg",
	}

	gen := &generator{ctx: ctx}
	signatures, err := gen.extractSignatures()
	require.NoError(t, err)

	content, err := gen.generateMockFile(signatures)
	require.NoError(t, err)

	expected := `// Code generated by scaf. DO NOT EDIT.
//go:build !scaf_prod

package testpkg

func init() {
	getUserImpl = getUserMock
}

func getUserMock(userId any) (any, any) {
	if userId == 1 {
		return "Alice", nil
	}
	panic("no matching test case")
}
`
	assert.Equal(t, expected, string(content))
}

func TestToImplVarName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"GetUser", "getUserImpl"},
		{"DeletePost", "deletePostImpl"},
		{"A", "aImpl"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toImplVarName(tt.input))
		})
	}
}

func TestToMockFuncName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"GetUser", "getUserMock"},
		{"DeletePost", "deletePostMock"},
		{"A", "aMock"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toMockFuncName(tt.input))
		})
	}
}

func TestToProdFuncName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"GetUser", "getUserProd"},
		{"DeletePost", "deletePostProd"},
		{"A", "aProd"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toProdFuncName(tt.input))
		})
	}
}
