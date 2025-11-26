package analysis

import (
	"regexp"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

// Analyzer performs semantic analysis on scaf files.
type Analyzer struct {
	// loader is used for resolving imports (cross-file analysis).
	// Can be nil for single-file analysis.
	loader FileLoader

	// rules is the set of semantic checks to run.
	rules []*Rule
}

// FileLoader is an interface for loading files during analysis.
// This allows the analyzer to resolve imports.
type FileLoader interface {
	// Load returns the content of a file at the given path.
	Load(path string) ([]byte, error)
}

// NewAnalyzer creates a new analyzer with default rules.
// Pass nil for loader to do single-file analysis only.
func NewAnalyzer(loader FileLoader) *Analyzer {
	return &Analyzer{
		loader: loader,
		rules:  DefaultRules(),
	}
}

// NewAnalyzerWithRules creates an analyzer with custom rules.
func NewAnalyzerWithRules(loader FileLoader, rules []*Rule) *Analyzer {
	return &Analyzer{
		loader: loader,
		rules:  rules,
	}
}

// Analyze parses and analyzes a scaf file.
func (a *Analyzer) Analyze(path string, content []byte) *AnalyzedFile {
	result := &AnalyzedFile{
		Path:        path,
		Diagnostics: []Diagnostic{},
		Symbols:     NewSymbolTable(),
	}

	// Parse the file.
	suite, err := scaf.Parse(content)
	if err != nil {
		result.ParseError = err
		result.Diagnostics = append(result.Diagnostics, parseErrorToDiagnostic(err))

		return result
	}

	result.Suite = suite

	// Build symbol table.
	buildSymbols(result)

	// Run all semantic rules.
	for _, rule := range a.rules {
		rule.Run(result)
	}

	return result
}

// parseErrorToDiagnostic converts a parse error to a diagnostic.
func parseErrorToDiagnostic(err error) Diagnostic {
	// participle errors implement Error interface with Position().
	span := scaf.Span{}
	msg := err.Error()

	// Try to extract position from participle error
	type participleError interface {
		Position() lexer.Position
		Message() string
	}

	if pe, ok := err.(participleError); ok {
		pos := pe.Position()
		span = scaf.Span{Start: pos, End: pos}
		msg = pe.Message()
	}

	return Diagnostic{
		Span:     span,
		Severity: SeverityError,
		Message:  msg,
		Code:     "parse-error",
		Source:   "scaf",
	}
}

// buildSymbols extracts all symbol definitions from the AST.
func buildSymbols(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	// Extract imports.
	for _, imp := range f.Suite.Imports {
		alias := baseNameFromPath(imp.Path)
		if imp.Alias != nil {
			alias = *imp.Alias
		}

		f.Symbols.Imports[alias] = &ImportSymbol{
			Symbol: Symbol{
				Name: alias,
				Span: imp.Span(),
				Kind: SymbolKindImport,
			},
			Alias: imp.Alias,
			Path:  imp.Path,
			Node:  imp,
		}
	}

	// Extract queries.
	for _, q := range f.Suite.Queries {
		params := extractQueryParams(q.Body)
		f.Symbols.Queries[q.Name] = &QuerySymbol{
			Symbol: Symbol{
				Name: q.Name,
				Span: q.Span(),
				Kind: SymbolKindQuery,
			},
			Body:   q.Body,
			Params: params,
			Node:   q,
		}
	}

	// Extract tests from scopes.
	for _, scope := range f.Suite.Scopes {
		extractTestSymbols(f, scope.QueryName, "", scope.Items)
	}
}

// extractTestSymbols recursively extracts test symbols from items.
func extractTestSymbols(f *AnalyzedFile, queryScope, groupPath string, items []*scaf.TestOrGroup) {
	for _, item := range items {
		if item.Test != nil {
			fullPath := buildTestPath(queryScope, groupPath, item.Test.Name)
			f.Symbols.Tests[fullPath] = &TestSymbol{
				Symbol: Symbol{
					Name: item.Test.Name,
					Span: item.Test.Span(),
					Kind: SymbolKindTest,
				},
				FullPath:   fullPath,
				QueryScope: queryScope,
				Node:       item.Test,
			}
		}

		if item.Group != nil {
			newGroupPath := groupPath
			if newGroupPath != "" {
				newGroupPath += "/"
			}

			newGroupPath += item.Group.Name
			extractTestSymbols(f, queryScope, newGroupPath, item.Group.Items)
		}
	}
}

// ----------------------------------------------------------------------------
// Helper functions
// ----------------------------------------------------------------------------

// baseNameFromPath extracts the base name from an import path
// (e.g., "./shared/fixtures" -> "fixtures").
func baseNameFromPath(path string) string {
	// Remove leading ./ or ../
	path = strings.TrimPrefix(path, "./")
	for strings.HasPrefix(path, "../") {
		path = strings.TrimPrefix(path, "../")
	}

	// Get last component.
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return path
}

// extractQueryParams extracts $-prefixed parameters from a query body.
var paramRegex = regexp.MustCompile(`\$(\w+)`)

func extractQueryParams(body string) []string {
	matches := paramRegex.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)

	var params []string

	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			params = append(params, m[1])
		}
	}

	return params
}

// buildTestPath constructs a full test path.
func buildTestPath(queryScope, groupPath, testName string) string {
	parts := []string{queryScope}
	if groupPath != "" {
		parts = append(parts, groupPath)
	}

	parts = append(parts, testName)

	return strings.Join(parts, "/")
}
