package analysis

import (
	"errors"
	"regexp"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

// Analyzer performs semantic analysis on scaf files.
type Analyzer struct {
	// loader is used for resolving imports (cross-file analysis).
	// Can be nil for single-file analysis.
	loader FileLoader

	// resolver is used for cross-file validation (e.g., checking setup call targets).
	// Can be nil if cross-file validation is not needed.
	resolver CrossFileResolver

	// rules is the set of semantic checks to run.
	rules []*Rule
}

// FileLoader is an interface for loading files during analysis.
// This allows the analyzer to resolve imports.
type FileLoader interface {
	// Load returns the content of a file at the given path.
	Load(path string) ([]byte, error)
}

// CrossFileResolver is an interface for resolving and analyzing imported files.
// This enables cross-file validation like checking if a setup call references
// a query that exists in the imported module.
type CrossFileResolver interface {
	// ResolveImportPath resolves a relative import path to an absolute file path.
	ResolveImportPath(basePath, importPath string) string

	// LoadAndAnalyze loads and analyzes an imported file, returning its analysis.
	// Returns nil if the file cannot be loaded or analyzed.
	LoadAndAnalyze(path string) *AnalyzedFile
}

// NewAnalyzer creates a new analyzer with default rules.
// Pass nil for loader to do single-file analysis only.
func NewAnalyzer(loader FileLoader) *Analyzer {
	return &Analyzer{
		loader: loader,
		rules:  DefaultRules(),
	}
}

// NewAnalyzerWithResolver creates an analyzer with cross-file resolution support.
// The resolver enables validation of setup calls against imported modules.
func NewAnalyzerWithResolver(loader FileLoader, resolver CrossFileResolver) *Analyzer {
	return &Analyzer{
		loader:   loader,
		resolver: resolver,
		rules:    DefaultRules(),
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
// On parse errors, still extracts symbols from the partial AST so that
// LSP features like completion and hover continue to work.
func (a *Analyzer) Analyze(path string, content []byte) *AnalyzedFile {
	result := &AnalyzedFile{
		Path:        path,
		Diagnostics: []Diagnostic{},
		Symbols:     NewSymbolTable(),
		Resolver:    a.resolver,
	}

	// Parse the file - returns partial AST even on error.
	// NOTE: We use non-recovery mode here because recovery can break parsing of valid
	// syntax like "setup FunctionName()" (the recovery mechanism is too aggressive
	// with optional groups).
	suite, err := scaf.Parse(content)
	result.Suite = suite
	result.ParseError = err

	if err != nil {
		// Convert parse errors to diagnostics
		result.Diagnostics = append(result.Diagnostics, parseErrorsToDiagnostics(err)...)
		// Don't return early - continue with partial AST for better LSP support
		
		// Also do a recovery parse for completion context.
		// This may give us better information about what the user was typing
		// even though it might misparse valid syntax elsewhere.
		recoverySuite, recoveryErr := scaf.ParseWithRecovery(content, true)
		result.RecoverySuite = recoverySuite
		result.RecoveryError = recoveryErr
	}

	// Build symbol table from partial or complete AST.
	// Symbols defined before the error location will still be available.
	if suite != nil {
		buildSymbols(result)
	} else if err != nil {
		// Fallback: if Participle returned nil AST, use regex extraction
		extractPartialSymbols(result, content)
	}

	// Run semantic rules only on complete parses to avoid spurious errors.
	// Partial ASTs may have nil fields that rules don't expect.
	if result.ParseError == nil {
		for _, rule := range a.rules {
			rule.Run(result)
		}
	}

	return result
}

// parseErrorToDiagnostic converts a parse error to a diagnostic.
// If the error is a RecoveryError (containing multiple errors), it returns
// a slice of diagnostics - one for each recovered error.
func parseErrorToDiagnostic(err error) Diagnostic {
	// Check if this is a RecoveryError with multiple errors
	var recoveryErr *participle.RecoveryError
	if errors.As(err, &recoveryErr) && len(recoveryErr.Errors) > 0 {
		// Return the first error as the primary diagnostic
		// Additional errors are still accessible via the RecoveryError
		return singleErrorToDiagnostic(recoveryErr.Errors[0])
	}

	return singleErrorToDiagnostic(err)
}

// parseErrorsToDiagnostics converts a parse error to multiple diagnostics.
// If the error is a RecoveryError, returns diagnostics for all errors.
func parseErrorsToDiagnostics(err error) []Diagnostic {
	var recoveryErr *participle.RecoveryError
	if errors.As(err, &recoveryErr) {
		diagnostics := make([]Diagnostic, 0, len(recoveryErr.Errors))
		for _, e := range recoveryErr.Errors {
			diagnostics = append(diagnostics, singleErrorToDiagnostic(e))
		}
		return diagnostics
	}

	return []Diagnostic{singleErrorToDiagnostic(err)}
}

// singleErrorToDiagnostic converts a single error to a diagnostic.
func singleErrorToDiagnostic(err error) Diagnostic {
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
// Handles partial ASTs gracefully - symbols defined before parse errors
// will still be extracted.
func buildSymbols(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	// Extract imports.
	for _, imp := range f.Suite.Imports {
		if imp == nil || imp.Path == "" {
			continue // Skip incomplete imports in partial AST
		}

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
		if q == nil || q.Name == "" {
			continue // Skip incomplete queries in partial AST
		}

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
		if scope == nil || scope.QueryName == "" {
			continue // Skip incomplete scopes in partial AST
		}
		extractTestSymbols(f, scope.QueryName, "", scope.Items)
	}
}

// extractTestSymbols recursively extracts test symbols from items.
// Handles partial ASTs gracefully with nil checks.
func extractTestSymbols(f *AnalyzedFile, queryScope, groupPath string, items []*scaf.TestOrGroup) {
	for _, item := range items {
		if item == nil {
			continue // Skip nil items in partial AST
		}

		if item.Test != nil && item.Test.Name != "" {
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

		if item.Group != nil && item.Group.Name != "" {
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

// extractPartialSymbols extracts symbols from source text when parsing fails.
// This enables completion to work while the user is typing (and the file is temporarily invalid).
// It uses regex-based extraction which is less accurate than AST-based but works on broken code.
func extractPartialSymbols(f *AnalyzedFile, content []byte) {
	text := string(content)

	// Extract imports: import [alias] "path"
	// Pattern: import fixtures "./fixtures" OR import "./fixtures"
	importRegex := regexp.MustCompile(`(?m)^import\s+(?:(\w+)\s+)?"([^"]+)"`)
	for _, match := range importRegex.FindAllStringSubmatch(text, -1) {
		if len(match) >= 3 {
			path := match[2]
			alias := match[1] // May be empty if no alias specified

			// If no alias, derive from path
			if alias == "" {
				alias = baseNameFromPath(path)
			}

			f.Symbols.Imports[alias] = &ImportSymbol{
				Symbol: Symbol{
					Name: alias,
					Kind: SymbolKindImport,
				},
				Alias: nil, // Simplified - not tracking alias pointer in partial extraction
				Path:  path,
				Node:  nil, // No AST node available
			}
		}
	}

	// Extract queries: query Name `body`
	queryRegex := regexp.MustCompile("(?m)^query\\s+(\\w+)\\s+`([^`]*)`")
	for _, match := range queryRegex.FindAllStringSubmatch(text, -1) {
		if len(match) >= 3 {
			name := match[1]
			body := match[2]
			params := extractQueryParams(body)

			f.Symbols.Queries[name] = &QuerySymbol{
				Symbol: Symbol{
					Name: name,
					Kind: SymbolKindQuery,
				},
				Body:   body,
				Params: params,
				Node:   nil, // No AST node available
			}
		}
	}
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
