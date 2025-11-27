package neogo

import (
	"testing"

	"github.com/rlch/scaf/language/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindingName(t *testing.T) {
	t.Parallel()

	b := NewBinding()
	assert.Equal(t, "neogo", b.Name())
}

func TestBindingImports(t *testing.T) {
	t.Parallel()

	b := NewBinding()
	imports := b.Imports()

	assert.Contains(t, imports, "context")
	assert.Contains(t, imports, "github.com/rlch/neogo")
}

func TestBindingPrependParams(t *testing.T) {
	t.Parallel()

	b := NewBinding()
	params := b.PrependParams()

	// neogo prepends ctx and db
	require.Len(t, params, 2)
	assert.Equal(t, "ctx", params[0].Name)
	assert.Equal(t, "context.Context", params[0].Type)
	assert.Equal(t, "db", params[1].Name)
	assert.Equal(t, "neogo.Driver", params[1].Type)
}

func TestBindingReturnsError(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	// neogo always returns error
	assert.True(t, b.ReturnsError())
}

func TestBindingReceiverType(t *testing.T) {
	t.Parallel()

	b := NewBinding()
	assert.Equal(t, "", b.ReceiverType())
}

func TestBindingGenerateBody_BasicQuery(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User {id: $userId}) RETURN u.name AS name",
		Signature: &golang.BindingSignature{
			Name: "GetUserName",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "userId", Type: "string"},
			},
			Returns: []golang.BindingReturn{
				{Name: "name", ColumnName: "name", Type: "string"}, // alias used, so ColumnName = Name
			},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{
			{Name: "userId", Type: "string"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	expected := "var name string\n" +
		"err := db.Exec().\n" +
		"\tCypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name`).\n" +
		"\tRunWithParams(ctx, map[string]any{\"userId\": userId}, \"name\", &name)\n" +
		"if err != nil {\n" +
		"\treturn \"\", err\n" +
		"}\n" +
		"return name, nil"

	assert.Equal(t, expected, body)
}

func TestBindingGenerateBody_MultipleReturns(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User {id: $userId}) RETURN u.name AS name, u.age AS age",
		Signature: &golang.BindingSignature{
			Name: "GetUser",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "userId", Type: "string"},
			},
			Returns: []golang.BindingReturn{
				{Name: "name", ColumnName: "name", Type: "string"},
				{Name: "age", ColumnName: "age", Type: "int64"},
			},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{
			{Name: "userId", Type: "string"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	expected := "var name string\n" +
		"var age int64\n" +
		"err := db.Exec().\n" +
		"\tCypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name, u.age AS age`).\n" +
		"\tRunWithParams(ctx, map[string]any{\"userId\": userId}, \"name\", &name, \"age\", &age)\n" +
		"if err != nil {\n" +
		"\treturn \"\", 0, err\n" +
		"}\n" +
		"return name, age, nil"

	assert.Equal(t, expected, body)
}

func TestBindingGenerateBody_NoAlias(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	// When no alias is used, ColumnName is the full expression
	ctx := &golang.BodyContext{
		Query: "MATCH (u:User {id: $userId}) RETURN u.name, u.age",
		Signature: &golang.BindingSignature{
			Name: "GetUser",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "userId", Type: "string"},
			},
			Returns: []golang.BindingReturn{
				{Name: "name", ColumnName: "u.name", Type: "string"},
				{Name: "age", ColumnName: "u.age", Type: "int64"},
			},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{
			{Name: "userId", Type: "string"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	expected := "var name string\n" +
		"var age int64\n" +
		"err := db.Exec().\n" +
		"\tCypher(`MATCH (u:User {id: $userId}) RETURN u.name, u.age`).\n" +
		"\tRunWithParams(ctx, map[string]any{\"userId\": userId}, \"u.name\", &name, \"u.age\", &age)\n" +
		"if err != nil {\n" +
		"\treturn \"\", 0, err\n" +
		"}\n" +
		"return name, age, nil"

	assert.Equal(t, expected, body)
}

func TestBindingGenerateBody_NoParams(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User) RETURN count(u) AS count",
		Signature: &golang.BindingSignature{
			Name: "CountUsers",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
			},
			Returns: []golang.BindingReturn{
				{Name: "count", ColumnName: "count", Type: "int64"},
			},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{}, // No query params
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	expected := "var count int64\n" +
		"err := db.Exec().\n" +
		"\tCypher(`MATCH (u:User) RETURN count(u) AS count`).\n" +
		"\tRun(ctx, \"count\", &count)\n" +
		"if err != nil {\n" +
		"\treturn 0, err\n" +
		"}\n" +
		"return count, nil"

	assert.Equal(t, expected, body)
}

func TestBindingGenerateBody_NoReturns(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User {id: $userId}) DELETE u",
		Signature: &golang.BindingSignature{
			Name: "DeleteUser",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "userId", Type: "string"},
			},
			Returns:      []golang.BindingReturn{},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{
			{Name: "userId", Type: "string"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	expected := "err := db.Exec().\n" +
		"\tCypher(`MATCH (u:User {id: $userId}) DELETE u`).\n" +
		"\tRunWithParams(ctx, map[string]any{\"userId\": userId})\n" +
		"if err != nil {\n" +
		"\treturn err\n" +
		"}\n" +
		"return nil"

	assert.Equal(t, expected, body)
}

func TestBindingGenerateBody_MultipleQueryParams(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User) WHERE u.age >= $minAge AND u.age <= $maxAge RETURN u.name AS name",
		Signature: &golang.BindingSignature{
			Name: "GetUsersByAgeRange",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "minAge", Type: "int64"},
				{Name: "maxAge", Type: "int64"},
			},
			Returns: []golang.BindingReturn{
				{Name: "name", ColumnName: "name", Type: "string"},
			},
			ReturnsError: true,
		},
		QueryParams: []golang.BindingParam{
			{Name: "minAge", Type: "int64"},
			{Name: "maxAge", Type: "int64"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	assert.Contains(t, body, `"minAge": minAge`)
	assert.Contains(t, body, `"maxAge": maxAge`)
}

func TestBindingGenerateBody_NoError(t *testing.T) {
	t.Parallel()

	b := NewBinding()

	ctx := &golang.BodyContext{
		Query: "MATCH (u:User {id: $userId}) RETURN u.name AS name",
		Signature: &golang.BindingSignature{
			Name: "GetUserName",
			Params: []golang.BindingParam{
				{Name: "ctx", Type: "context.Context"},
				{Name: "db", Type: "neogo.Driver"},
				{Name: "userId", Type: "string"},
			},
			Returns: []golang.BindingReturn{
				{Name: "name", ColumnName: "name", Type: "string"},
			},
			ReturnsError: false, // No error return
		},
		QueryParams: []golang.BindingParam{
			{Name: "userId", Type: "string"},
		},
	}

	body, err := b.GenerateBody(ctx)
	require.NoError(t, err)

	// Should not have error handling
	assert.NotContains(t, body, "if err != nil")

	expected := "var name string\n" +
		"db.Exec().\n" +
		"\tCypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name`).\n" +
		"\tRunWithParams(ctx, map[string]any{\"userId\": userId}, \"name\", &name)\n" +
		"return name"

	assert.Equal(t, expected, body)
}

func TestBindingRegistered(t *testing.T) {
	t.Parallel()

	// The init() should have registered the binding
	b := golang.GetBinding("neogo")
	require.NotNil(t, b)
	assert.Equal(t, "neogo", b.Name())
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
		{"*User", "nil"},
		{"[]string", "nil"},
		{"map[string]any", "nil"},
		{"any", "nil"},
		{"error", "nil"},
		{"User", "User{}"},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, zeroValue(tt.typ))
		})
	}
}
