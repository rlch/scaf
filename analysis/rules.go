package analysis

import (
	"strconv"
	"strings"

	"github.com/rlch/scaf"
)

// Rule represents a semantic analysis check.
// Inspired by go/analysis.Analyzer pattern.
type Rule struct {
	// Name is a short identifier for the rule (used in diagnostic codes).
	Name string

	// Doc is a brief description of what the rule checks.
	Doc string

	// Severity is the default severity for diagnostics from this rule.
	Severity DiagnosticSeverity

	// Run executes the rule and appends any diagnostics to the file.
	Run func(f *AnalyzedFile)
}

// DefaultRules returns all built-in semantic analysis rules.
func DefaultRules() []*Rule {
	return []*Rule{
		// Error-level checks.
		undefinedQueryRule,
		undefinedImportRule,
		duplicateQueryRule,
		duplicateImportRule,
		undefinedAssertQueryRule,
		undefinedSetupQueryRule, // Cross-file validation

		// Warning-level checks.
		unusedImportRule,
		unknownParameterRule,
		duplicateTestRule,
		duplicateGroupRule,
		missingRequiredParamsRule,
		emptyGroupRule,

		// Hint-level checks.
		emptyTestRule,
		unusedQueryParamRule,
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-query
// ----------------------------------------------------------------------------

var undefinedQueryRule = &Rule{
	Name:     "undefined-query",
	Doc:      "Reports query scopes that reference undefined queries.",
	Severity: SeverityError,
	Run:      checkUndefinedQueries,
}

func checkUndefinedQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		if _, ok := f.Symbols.Queries[scope.QueryName]; !ok {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     scope.Span(),
				Severity: SeverityError,
				Message:  "undefined query: " + scope.QueryName,
				Code:     "undefined-query",
				Source:   "scaf",
			})
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-import
// ----------------------------------------------------------------------------

var undefinedImportRule = &Rule{
	Name:     "undefined-import",
	Doc:      "Reports setup calls that reference undefined imports.",
	Severity: SeverityError,
	Run:      checkUndefinedImports,
}

func checkUndefinedImports(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	checkSetup := func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}

		if setup.Module != nil {
			checkSetupModuleImport(f, *setup.Module, setup.Span())
		}

		if setup.Call != nil {
			checkSetupCallImport(f, setup.Call)
		}

		for _, item := range setup.Block {
			if item.Module != nil {
				checkSetupModuleImport(f, *item.Module, item.Span())
			}

			if item.Call != nil {
				checkSetupCallImport(f, item.Call)
			}
		}
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				checkSetup(item.Test.Setup)
			}

			if item.Group != nil {
				checkSetup(item.Group.Setup)
				checkItems(item.Group.Items)
			}
		}
	}

	checkSetup(f.Suite.Setup)

	for _, scope := range f.Suite.Scopes {
		checkSetup(scope.Setup)
		checkItems(scope.Items)
	}
}

func checkSetupModuleImport(f *AnalyzedFile, moduleAlias string, span scaf.Span) {
	if imp, ok := f.Symbols.Imports[moduleAlias]; !ok {
		f.Diagnostics = append(f.Diagnostics, Diagnostic{
			Span:     span,
			Severity: SeverityError,
			Message:  "undefined import: " + moduleAlias,
			Code:     "undefined-import",
			Source:   "scaf",
		})
	} else {
		imp.Used = true
	}
}

func checkSetupCallImport(f *AnalyzedFile, call *scaf.SetupCall) {
	if imp, ok := f.Symbols.Imports[call.Module]; !ok {
		f.Diagnostics = append(f.Diagnostics, Diagnostic{
			Span:     call.Span(),
			Severity: SeverityError,
			Message:  "undefined import: " + call.Module,
			Code:     "undefined-import",
			Source:   "scaf",
		})
	} else {
		imp.Used = true
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-query
// ----------------------------------------------------------------------------

var duplicateQueryRule = &Rule{
	Name:     "duplicate-query",
	Doc:      "Reports duplicate query name definitions.",
	Severity: SeverityError,
	Run:      checkDuplicateQueries,
}

func checkDuplicateQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	seen := make(map[string]scaf.Span)

	for _, q := range f.Suite.Queries {
		if firstSpan, exists := seen[q.Name]; exists {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     q.Span(),
				Severity: SeverityError,
				Message:  "duplicate query name: " + q.Name + " (first defined at line " + formatLine(firstSpan) + ")",
				Code:     "duplicate-query",
				Source:   "scaf",
			})
		} else {
			seen[q.Name] = q.Span()
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-import
// ----------------------------------------------------------------------------

var duplicateImportRule = &Rule{
	Name:     "duplicate-import",
	Doc:      "Reports duplicate import aliases.",
	Severity: SeverityError,
	Run:      checkDuplicateImports,
}

func checkDuplicateImports(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	seen := make(map[string]scaf.Span)

	for _, imp := range f.Suite.Imports {
		alias := baseNameFromPath(imp.Path)
		if imp.Alias != nil {
			alias = *imp.Alias
		}

		if firstSpan, exists := seen[alias]; exists {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     imp.Span(),
				Severity: SeverityError,
				Message:  "duplicate import alias: " + alias + " (first defined at line " + formatLine(firstSpan) + ")",
				Code:     "duplicate-import",
				Source:   "scaf",
			})
		} else {
			seen[alias] = imp.Span()
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: unused-import
// ----------------------------------------------------------------------------

var unusedImportRule = &Rule{
	Name:     "unused-import",
	Doc:      "Reports imports that are never referenced.",
	Severity: SeverityWarning,
	Run:      checkUnusedImports,
}

func checkUnusedImports(f *AnalyzedFile) {
	// Note: Must run after undefinedImportRule which marks imports as used.
	for alias, imp := range f.Symbols.Imports {
		if !imp.Used {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     imp.Span,
				Severity: SeverityWarning,
				Message:  "unused import: " + alias,
				Code:     "unused-import",
				Source:   "scaf",
			})
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: unknown-parameter
// ----------------------------------------------------------------------------

var unknownParameterRule = &Rule{
	Name:     "unknown-parameter",
	Doc:      "Reports test parameters that don't exist in the query.",
	Severity: SeverityWarning,
	Run:      checkUnknownParameters,
}

func checkUnknownParameters(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.QueryName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		queryParams := make(map[string]bool)
		for _, p := range query.Params {
			queryParams[p] = true
		}

		checkItemParams(f, scope.Items, queryParams, scope.QueryName)
	}
}

func checkItemParams(f *AnalyzedFile, items []*scaf.TestOrGroup, queryParams map[string]bool, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					if !queryParams[paramName] {
						f.Diagnostics = append(f.Diagnostics, Diagnostic{
							Span:     item.Test.Span(),
							Severity: SeverityWarning,
							Message:  "parameter $" + paramName + " not found in query " + queryName,
							Code:     "unknown-parameter",
							Source:   "scaf",
						})
					}
				}
			}
		}

		if item.Group != nil {
			checkItemParams(f, item.Group.Items, queryParams, queryName)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: empty-test
// ----------------------------------------------------------------------------

var emptyTestRule = &Rule{
	Name:     "empty-test",
	Doc:      "Reports tests with no statements or assertions.",
	Severity: SeverityHint,
	Run:      checkEmptyTests,
}

func checkEmptyTests(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				if len(item.Test.Statements) == 0 && len(item.Test.Asserts) == 0 && item.Test.Setup == nil {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     item.Test.Span(),
						Severity: SeverityHint,
						Message:  "empty test: " + item.Test.Name,
						Code:     "empty-test",
						Source:   "scaf",
					})
				}
			}

			if item.Group != nil {
				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-test
// ----------------------------------------------------------------------------

var duplicateTestRule = &Rule{
	Name:     "duplicate-test",
	Doc:      "Reports duplicate test names within the same scope.",
	Severity: SeverityWarning,
	Run:      checkDuplicateTests,
}

func checkDuplicateTests(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		checkDuplicateTestNamesInItems(f, scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-group
// ----------------------------------------------------------------------------

var duplicateGroupRule = &Rule{
	Name:     "duplicate-group",
	Doc:      "Reports duplicate group names within the same scope.",
	Severity: SeverityWarning,
	Run:      checkDuplicateGroups,
}

func checkDuplicateGroups(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		checkDuplicateGroupNamesInItems(f, scope.Items)
	}
}

func checkDuplicateTestNamesInItems(f *AnalyzedFile, items []*scaf.TestOrGroup) {
	testNames := make(map[string]scaf.Span)

	for _, item := range items {
		if item.Test != nil {
			if firstSpan, exists := testNames[item.Test.Name]; exists {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Test.Span(),
					Severity: SeverityWarning,
					Message: "duplicate test name in scope: " + item.Test.Name +
						" (first defined at line " + formatLine(firstSpan) + ")",
					Code:   "duplicate-test",
					Source: "scaf",
				})
			} else {
				testNames[item.Test.Name] = item.Test.Span()
			}
		}

		if item.Group != nil {
			// Recurse into group.
			checkDuplicateTestNamesInItems(f, item.Group.Items)
		}
	}
}

func checkDuplicateGroupNamesInItems(f *AnalyzedFile, items []*scaf.TestOrGroup) {
	groupNames := make(map[string]scaf.Span)

	for _, item := range items {
		if item.Group != nil {
			if firstSpan, exists := groupNames[item.Group.Name]; exists {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Group.Span(),
					Severity: SeverityWarning,
					Message: "duplicate group name in scope: " + item.Group.Name +
						" (first defined at line " + formatLine(firstSpan) + ")",
					Code:   "duplicate-group",
					Source: "scaf",
				})
			} else {
				groupNames[item.Group.Name] = item.Group.Span()
			}

			// Recurse into group.
			checkDuplicateGroupNamesInItems(f, item.Group.Items)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-assert-query
// ----------------------------------------------------------------------------

var undefinedAssertQueryRule = &Rule{
	Name:     "undefined-assert-query",
	Doc:      "Reports assert blocks that reference undefined queries.",
	Severity: SeverityError,
	Run:      checkUndefinedAssertQueries,
}

func checkUndefinedAssertQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				for _, assert := range item.Test.Asserts {
					if assert.Query != nil && assert.Query.QueryName != nil {
						queryName := *assert.Query.QueryName
						if _, ok := f.Symbols.Queries[queryName]; !ok {
							f.Diagnostics = append(f.Diagnostics, Diagnostic{
								Span:     item.Test.Span(),
								Severity: SeverityError,
								Message:  "assert references undefined query: " + queryName,
								Code:     "undefined-assert-query",
								Source:   "scaf",
							})
						}
					}
				}
			}

			if item.Group != nil {
				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: missing-required-params
// ----------------------------------------------------------------------------

var missingRequiredParamsRule = &Rule{
	Name:     "missing-required-params",
	Doc:      "Reports tests that don't provide all required query parameters.",
	Severity: SeverityWarning,
	Run:      checkMissingRequiredParams,
}

func checkMissingRequiredParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.QueryName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		checkItemMissingParams(f, scope.Items, query.Params, scope.QueryName)
	}
}

func checkItemMissingParams(f *AnalyzedFile, items []*scaf.TestOrGroup, queryParams []string, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			providedParams := make(map[string]bool)

			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					providedParams[paramName] = true
				}
			}

			var missing []string

			for _, p := range queryParams {
				if !providedParams[p] {
					missing = append(missing, "$"+p)
				}
			}

			if len(missing) > 0 {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Test.Span(),
					Severity: SeverityWarning,
					Message:  "test is missing required parameters for " + queryName + ": " + strings.Join(missing, ", "),
					Code:     "missing-required-params",
					Source:   "scaf",
				})
			}
		}

		if item.Group != nil {
			checkItemMissingParams(f, item.Group.Items, queryParams, queryName)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: empty-group
// ----------------------------------------------------------------------------

var emptyGroupRule = &Rule{
	Name:     "empty-group",
	Doc:      "Reports groups with no tests or nested groups.",
	Severity: SeverityWarning,
	Run:      checkEmptyGroups,
}

func checkEmptyGroups(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Group != nil {
				if len(item.Group.Items) == 0 {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     item.Group.Span(),
						Severity: SeverityWarning,
						Message:  "empty group: " + item.Group.Name,
						Code:     "empty-group",
						Source:   "scaf",
					})
				}

				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-setup-query
// ----------------------------------------------------------------------------

var undefinedSetupQueryRule = &Rule{
	Name:     "undefined-setup-query",
	Doc:      "Reports setup calls that reference queries not found in the imported module.",
	Severity: SeverityError,
	Run:      checkUndefinedSetupQueries,
}

func checkUndefinedSetupQueries(f *AnalyzedFile) {
	if f.Suite == nil || f.Resolver == nil {
		return // Cross-file validation requires a resolver
	}

	// Helper to check a setup call
	checkSetupCall := func(call *scaf.SetupCall) {
		if call == nil {
			return
		}

		// Get the import for this module
		imp, ok := f.Symbols.Imports[call.Module]
		if !ok {
			// undefined-import rule handles this
			return
		}

		// Resolve the import path
		importedPath := f.Resolver.ResolveImportPath(f.Path, imp.Path)
		importedFile := f.Resolver.LoadAndAnalyze(importedPath)
		if importedFile == nil || importedFile.Symbols == nil {
			// Can't load/analyze the file - don't report error since file might just not exist yet
			return
		}

		// Check if the query exists in the imported module
		if _, ok := importedFile.Symbols.Queries[call.Query]; !ok {
			// Build list of available queries for better error message
			var available []string
			for name := range importedFile.Symbols.Queries {
				available = append(available, name)
			}

			msg := "undefined query in module " + call.Module + ": " + call.Query
			if len(available) > 0 {
				msg += " (available: " + strings.Join(available, ", ") + ")"
			}

			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     call.Span(),
				Severity: SeverityError,
				Message:  msg,
				Code:     "undefined-setup-query",
				Source:   "scaf",
			})
		}
	}

	// Helper to check a setup clause
	checkSetup := func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}

		checkSetupCall(setup.Call)

		for _, item := range setup.Block {
			checkSetupCall(item.Call)
		}
	}

	// Check test/group items recursively
	var checkItems func([]*scaf.TestOrGroup)
	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				checkSetup(item.Test.Setup)
			}
			if item.Group != nil {
				checkSetup(item.Group.Setup)
				checkItems(item.Group.Items)
			}
		}
	}

	// Check global setup
	checkSetup(f.Suite.Setup)

	// Check all scopes
	for _, scope := range f.Suite.Scopes {
		checkSetup(scope.Setup)
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: unused-query-param
// ----------------------------------------------------------------------------

var unusedQueryParamRule = &Rule{
	Name:     "unused-query-param",
	Doc:      "Reports query parameters that are never provided in any test.",
	Severity: SeverityHint,
	Run:      checkUnusedQueryParams,
}

func checkUnusedQueryParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.QueryName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		if len(query.Params) == 0 {
			continue // No parameters to check.
		}

		// Collect all parameters provided across all tests in this scope.
		providedParams := make(map[string]bool)
		collectProvidedParams(scope.Items, providedParams)

		// Report parameters that exist in query but never appear in any test.
		for _, param := range query.Params {
			if !providedParams[param] {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     scope.Span(),
					Severity: SeverityHint,
					Message:  "query parameter $" + param + " is never provided in any test within this scope",
					Code:     "unused-query-param",
					Source:   "scaf",
				})
			}
		}
	}
}

func collectProvidedParams(items []*scaf.TestOrGroup, provided map[string]bool) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					provided[paramName] = true
				}
			}
		}

		if item.Group != nil {
			collectProvidedParams(item.Group.Items, provided)
		}
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func formatLine(span scaf.Span) string {
	return strconv.Itoa(span.Start.Line)
}