package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockBinding is a test implementation of Binding.
type mockBinding struct {
	name string
}

func (m *mockBinding) Name() string {
	return m.name
}

func (m *mockBinding) Imports() []string {
	return []string{"github.com/example/db"}
}

func (m *mockBinding) ReceiverType() string {
	return "" // No receiver for mock
}

func (m *mockBinding) PrependParams() []BindingParam {
	return nil // No prepend params for mock
}

func (m *mockBinding) ReturnsError() bool {
	return false // No error return for mock
}

func (m *mockBinding) GenerateBody(_ *BodyContext) (string, error) {
	return "return db.Query(query)", nil
}

func TestRegisterBindingAndGet(t *testing.T) {
	t.Parallel()

	// Register a test binding
	binding := &mockBinding{name: "testbinding"}
	RegisterBinding(binding)

	// Should be able to get it back
	got := GetBinding("testbinding")
	assert.NotNil(t, got)
	assert.Equal(t, "testbinding", got.Name())

	// Non-existent binding returns nil
	assert.Nil(t, GetBinding("nonexistent"))
}

func TestRegisteredBindings(t *testing.T) {
	t.Parallel()

	// Register another test binding
	binding := &mockBinding{name: "testbinding2"}
	RegisterBinding(binding)

	names := RegisteredBindings()
	assert.Contains(t, names, "testbinding2")
}

func TestBindingInterface(t *testing.T) {
	t.Parallel()

	binding := &mockBinding{name: "test"}

	// Test Imports
	imports := binding.Imports()
	assert.Len(t, imports, 1)
	assert.Equal(t, "github.com/example/db", imports[0])

	// Test GenerateBody
	body, err := binding.GenerateBody(&BodyContext{
		Query: "MATCH (n) RETURN n",
	})
	assert.NoError(t, err)
	assert.Equal(t, "return db.Query(query)", body)
}

func TestBodyContext(t *testing.T) {
	t.Parallel()

	ctx := &BodyContext{
		Query: "MATCH (n) RETURN n",
		Signature: &BindingSignature{
			Name: "GetNodes",
			Params: []BindingParam{
				{Name: "limit", Type: "int"},
			},
			Returns: []BindingReturn{
				{Name: "nodes", Type: "[]Node"},
			},
			ReturnsSlice: true,
			ReturnsError: true,
		},
		ReceiverName: "db",
		ReceiverType: "*Client",
	}

	assert.Equal(t, "MATCH (n) RETURN n", ctx.Query)
	assert.Equal(t, "GetNodes", ctx.Signature.Name)
	assert.True(t, ctx.Signature.ReturnsSlice)
	assert.True(t, ctx.Signature.ReturnsError)
	assert.Equal(t, "db", ctx.ReceiverName)
	assert.Equal(t, "*Client", ctx.ReceiverType)
}
