// Package language provides interfaces for code generation from scaf DSL files.
//
// Each target language (Go, TypeScript, etc.) implements the Language interface
// to generate source files from parsed scaf suites.
package language

import (
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// Language represents a target language for code generation.
// Implementations generate source files from parsed scaf suites.
type Language interface {
	// Name returns the language identifier (e.g., "go", "typescript").
	Name() string

	// Generate produces source files from the given context.
	// Returns a map of filename to content.
	Generate(ctx *GenerateContext) (map[string][]byte, error)
}

// GenerateContext provides information needed for code generation.
// Language-specific generators may extend this with additional fields.
type GenerateContext struct {
	// Suite is the parsed scaf AST.
	Suite *scaf.Suite

	// Schema provides type information from the user's codebase.
	// May be nil if no schema is available.
	Schema *analysis.TypeSchema

	// QueryAnalyzer extracts parameters and returns from queries.
	// May be nil if no analyzer is available for the dialect.
	QueryAnalyzer scaf.QueryAnalyzer

	// OutputDir is the directory where files will be written.
	OutputDir string
}

// Registration for language discovery.
var languages = make(map[string]Language)

// Register registers a language by name.
func Register(lang Language) {
	languages[lang.Name()] = lang
}

// Get returns a language by name, or nil if not registered.
func Get(name string) Language { //nolint:ireturn
	return languages[name]
}

// RegisteredLanguages returns the names of all registered languages.
func RegisteredLanguages() []string {
	names := make([]string, 0, len(languages))
	for name := range languages {
		names = append(names, name)
	}

	return names
}
