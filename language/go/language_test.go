package golang

import (
	"testing"

	"github.com/rlch/scaf/language"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoLanguageName(t *testing.T) {
	t.Parallel()

	lang := New()
	assert.Equal(t, "go", lang.Name())
}

func TestLanguageRegistry(t *testing.T) {
	t.Parallel()

	// Go language should be auto-registered via init()
	lang := language.Get("go")
	require.NotNil(t, lang)
	assert.Equal(t, "go", lang.Name())

	// Non-existent language
	assert.Nil(t, language.Get("nonexistent"))

	// RegisteredLanguages should include "go"
	names := language.RegisteredLanguages()
	assert.Contains(t, names, "go")
}

func TestGoLanguageGenerate(t *testing.T) {
	t.Parallel()

	lang := New()

	// With nil suite, should return empty files
	ctx := &language.GenerateContext{
		Suite: nil,
	}

	files, err := lang.Generate(ctx)
	require.NoError(t, err)

	// Returns nil files when no suite
	assert.Nil(t, files["scaf.go"])
	assert.Nil(t, files["scaf_test.go"])
}

func TestGoLanguageGenerateWithContext(t *testing.T) {
	t.Parallel()

	lang := New()

	// With nil suite via Go-specific context
	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite: nil,
		},
		PackageName: "testpkg",
	}

	files, err := lang.GenerateWithContext(ctx)
	require.NoError(t, err)

	// Returns nil files when no suite
	assert.Nil(t, files["scaf.go"])
	assert.Nil(t, files["scaf_test.go"])
}
