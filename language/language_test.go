package language

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockLanguage is a test implementation of Language.
type mockLanguage struct {
	name string
}

func (m *mockLanguage) Name() string {
	return m.name
}

func (m *mockLanguage) Generate(_ *GenerateContext) (map[string][]byte, error) {
	return nil, nil
}

func TestRegisterAndGet(t *testing.T) {
	t.Parallel()

	// Register a test language
	lang := &mockLanguage{name: "testlang"}
	Register(lang)

	// Should be able to get it back
	got := Get("testlang")
	assert.NotNil(t, got)
	assert.Equal(t, "testlang", got.Name())

	// Non-existent language returns nil
	assert.Nil(t, Get("nonexistent"))
}

func TestRegisteredLanguages(t *testing.T) {
	t.Parallel()

	// Register another test language to ensure it shows up
	lang := &mockLanguage{name: "testlang2"}
	Register(lang)

	names := RegisteredLanguages()
	assert.Contains(t, names, "testlang2")
}

func TestGenerateContext(t *testing.T) {
	t.Parallel()

	// GenerateContext should be constructable with all fields
	ctx := &GenerateContext{
		Suite:         nil,
		Schema:        nil,
		QueryAnalyzer: nil,
		OutputDir:     "/tmp/output",
	}

	assert.Equal(t, "/tmp/output", ctx.OutputDir)
	assert.Nil(t, ctx.Suite)
}
