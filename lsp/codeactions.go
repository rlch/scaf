package lsp

import (
	"context"
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// CodeAction handles textDocument/codeAction requests.
// Returns a list of quick fixes and refactoring actions available at the cursor position.
func (s *Server) CodeAction(_ context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	s.logger.Debug("CodeAction",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Int("diagnosticCount", len(params.Context.Diagnostics)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil {
		return nil, nil
	}

	var actions []protocol.CodeAction

	// Generate quick fixes for diagnostics in range
	for _, diag := range params.Context.Diagnostics {
		diagActions := s.codeActionsForDiagnostic(doc, diag)
		actions = append(actions, diagActions...)
	}

	// Also check document diagnostics that overlap with the requested range
	for _, diag := range doc.Analysis.Diagnostics {
		diagRange := spanToRange(diag.Span)
		if rangesOverlap(diagRange, params.Range) {
			// Check if we already have this diagnostic from the request
			found := false
			for _, reqDiag := range params.Context.Diagnostics {
				if reqDiag.Code == diag.Code && reqDiag.Message == diag.Message {
					found = true
					break
				}
			}
			if !found {
				lspDiag := protocol.Diagnostic{
					Range:    diagRange,
					Severity: convertSeverityForAction(diag.Severity),
					Code:     diag.Code,
					Source:   diag.Source,
					Message:  diag.Message,
				}
				diagActions := s.codeActionsForDiagnostic(doc, lspDiag)
				actions = append(actions, diagActions...)
			}
		}
	}

	return actions, nil
}

// convertSeverityForAction converts analysis severity to protocol severity.
func convertSeverityForAction(sev analysis.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {
	case analysis.SeverityError:
		return protocol.DiagnosticSeverityError
	case analysis.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	case analysis.SeverityInformation:
		return protocol.DiagnosticSeverityInformation
	case analysis.SeverityHint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityError
	}
}

// rangesOverlap checks if two ranges overlap.
func rangesOverlap(a, b protocol.Range) bool {
	// a ends before b starts
	if a.End.Line < b.Start.Line || (a.End.Line == b.Start.Line && a.End.Character < b.Start.Character) {
		return false
	}
	// b ends before a starts
	if b.End.Line < a.Start.Line || (b.End.Line == a.Start.Line && b.End.Character < a.Start.Character) {
		return false
	}
	return true
}

// codeActionsForDiagnostic generates quick fix actions for a specific diagnostic.
func (s *Server) codeActionsForDiagnostic(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	var actions []protocol.CodeAction

	code, ok := diag.Code.(string)
	if !ok {
		return nil
	}

	switch code {
	case "missing-required-params":
		actions = append(actions, s.fixMissingParams(doc, diag)...)

	case "unused-import":
		actions = append(actions, s.fixUnusedImport(doc, diag)...)

	case "undefined-query":
		actions = append(actions, s.fixUndefinedQuery(doc, diag)...)

	case "empty-test":
		actions = append(actions, s.fixEmptyTest(doc, diag)...)

	case "empty-group":
		actions = append(actions, s.fixEmptyGroup(doc, diag)...)
	}

	return actions
}

// fixMissingParams generates a quick fix to add missing parameters to a test.
func (s *Server) fixMissingParams(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Parse the diagnostic message to extract missing params
	// Message format: "test is missing required parameters for QueryName: $param1, $param2"
	msg := diag.Message
	colonIdx := strings.LastIndex(msg, ": ")
	if colonIdx < 0 {
		return nil
	}
	paramsStr := msg[colonIdx+2:]
	params := strings.Split(paramsStr, ", ")

	// Find the test at this position
	var targetTest *scaf.Test
	for _, scope := range doc.Analysis.Suite.Scopes {
		for _, item := range scope.Items {
			test := s.findTestAtRange(item, diag.Range)
			if test != nil {
				targetTest = test
				break
			}
		}
		if targetTest != nil {
			break
		}
	}

	if targetTest == nil {
		return nil
	}

	// Generate the insertion text for missing parameters
	var insertLines []string
	for _, param := range params {
		param = strings.TrimSpace(param)
		if param != "" {
			insertLines = append(insertLines, fmt.Sprintf("\t\t%s: ", param))
		}
	}

	if len(insertLines) == 0 {
		return nil
	}

	// Insert after the test opening brace
	// Find the position right after "test "name" {"
	insertLine := diag.Range.Start.Line + 1
	insertText := strings.Join(insertLines, "\n") + "\n"

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			doc.URI: {
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: insertLine, Character: 0},
						End:   protocol.Position{Line: insertLine, Character: 0},
					},
					NewText: insertText,
				},
			},
		},
	}

	return []protocol.CodeAction{
		{
			Title:       "Add missing parameters",
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			Edit:        &edit,
		},
	}
}

// findTestAtRange finds a test that contains the given range.
func (s *Server) findTestAtRange(item *scaf.TestOrGroup, rng protocol.Range) *scaf.Test {
	if item.Test != nil {
		testRange := spanToRange(item.Test.Span())
		if rangesOverlap(testRange, rng) {
			return item.Test
		}
	}
	if item.Group != nil {
		for _, child := range item.Group.Items {
			if test := s.findTestAtRange(child, rng); test != nil {
				return test
			}
		}
	}
	return nil
}

// fixUnusedImport generates a quick fix to remove an unused import.
func (s *Server) fixUnusedImport(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Find the import at this position
	var targetImport *scaf.Import
	for _, imp := range doc.Analysis.Suite.Imports {
		impRange := spanToRange(imp.Span())
		if rangesOverlap(impRange, diag.Range) {
			targetImport = imp
			break
		}
	}

	if targetImport == nil {
		return nil
	}

	// Delete the entire import line (including newline)
	impRange := spanToRange(targetImport.Span())
	// Extend to include the entire line
	deleteRange := protocol.Range{
		Start: protocol.Position{Line: impRange.Start.Line, Character: 0},
		End:   protocol.Position{Line: impRange.End.Line + 1, Character: 0},
	}

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			doc.URI: {
				{
					Range:   deleteRange,
					NewText: "",
				},
			},
		},
	}

	alias := baseNameFromImport(targetImport)
	return []protocol.CodeAction{
		{
			Title:       fmt.Sprintf("Remove unused import '%s'", alias),
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			Edit:        &edit,
		},
	}
}

// fixUndefinedQuery generates a quick fix to create a missing query.
func (s *Server) fixUndefinedQuery(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Extract query name from diagnostic message
	// Message format: "undefined query: QueryName"
	prefix := "undefined query: "
	if !strings.HasPrefix(diag.Message, prefix) {
		return nil
	}
	queryName := strings.TrimPrefix(diag.Message, prefix)

	// Find where to insert the new query (before the first scope, or after last query)
	var insertLine uint32
	if len(doc.Analysis.Suite.Queries) > 0 {
		// Insert after the last query
		lastQuery := doc.Analysis.Suite.Queries[len(doc.Analysis.Suite.Queries)-1]
		insertLine = uint32(lastQuery.EndPos.Line) //nolint:gosec
	} else if len(doc.Analysis.Suite.Imports) > 0 {
		// Insert after imports
		lastImport := doc.Analysis.Suite.Imports[len(doc.Analysis.Suite.Imports)-1]
		insertLine = uint32(lastImport.EndPos.Line) //nolint:gosec
	} else {
		insertLine = 0
	}

	// Generate query template
	queryTemplate := fmt.Sprintf("\nquery %s `${1:MATCH (n) RETURN n}`\n", queryName)

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			doc.URI: {
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: insertLine, Character: 0},
						End:   protocol.Position{Line: insertLine, Character: 0},
					},
					NewText: queryTemplate,
				},
			},
		},
	}

	return []protocol.CodeAction{
		{
			Title:       fmt.Sprintf("Create query '%s'", queryName),
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			Edit:        &edit,
		},
	}
}

// fixEmptyTest generates quick fixes for an empty test.
func (s *Server) fixEmptyTest(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Find the test's query scope to get parameter info
	var queryScope string
	for _, scope := range doc.Analysis.Suite.Scopes {
		scopeRange := spanToRange(scope.Span())
		if rangesOverlap(scopeRange, diag.Range) {
			queryScope = scope.QueryName
			break
		}
	}

	if queryScope == "" {
		return nil
	}

	// Get query parameters and return fields
	query, ok := doc.Analysis.Symbols.Queries[queryScope]
	if !ok {
		return nil
	}

	// Build insertion text with parameters and return fields
	var lines []string
	
	// Add parameters
	for _, param := range query.Params {
		lines = append(lines, fmt.Sprintf("\t\t$%s: ", param))
	}

	// Try to get return fields from query analyzer
	if s.queryAnalyzer != nil && query.Body != "" {
		metadata, err := s.queryAnalyzer.AnalyzeQuery(query.Body)
		if err == nil {
			for _, ret := range metadata.Returns {
				name := ret.Expression
				if ret.Alias != "" {
					name = ret.Alias
				}
				lines = append(lines, fmt.Sprintf("\t\t%s: ", name))
			}
		}
	}

	if len(lines) == 0 {
		return nil
	}

	insertLine := diag.Range.Start.Line + 1
	insertText := strings.Join(lines, "\n") + "\n"

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			doc.URI: {
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: insertLine, Character: 0},
						End:   protocol.Position{Line: insertLine, Character: 0},
					},
					NewText: insertText,
				},
			},
		},
	}

	return []protocol.CodeAction{
		{
			Title:       "Add test skeleton",
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			Edit:        &edit,
		},
	}
}

// fixEmptyGroup generates a quick fix to add content to an empty group.
func (s *Server) fixEmptyGroup(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	// Add a test template inside the group
	insertLine := diag.Range.Start.Line + 1
	testTemplate := "\t\ttest \"example\" {\n\t\t\t\n\t\t}\n"

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			doc.URI: {
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: insertLine, Character: 0},
						End:   protocol.Position{Line: insertLine, Character: 0},
					},
					NewText: testTemplate,
				},
			},
		},
	}

	return []protocol.CodeAction{
		{
			Title:       "Add test to group",
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			Edit:        &edit,
		},
	}
}
