package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// DocumentHighlight handles textDocument/documentHighlight requests.
// Highlights all occurrences of the symbol under the cursor within the same document.
func (s *Server) DocumentHighlight(_ context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	s.logger.Debug("DocumentHighlight",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	var highlights []protocol.DocumentHighlight

	// Determine what kind of symbol we're on and find all occurrences
	switch node := tokenCtx.Node.(type) {
	case *scaf.Query:
		// On a query definition - highlight definition + all scope usages
		highlights = s.highlightQueryUsages(doc, node.Name)

	case *scaf.QueryScope:
		// On a query scope - highlight scope + query definition + other scopes
		highlights = s.highlightQueryUsages(doc, node.QueryName)

	case *scaf.Import:
		// On an import - highlight import + all usages
		alias := baseNameFromImport(node)
		highlights = s.highlightImportUsages(doc, alias)

	case *scaf.SetupCall:
		// Check if on the module name or query name
		if tokenCtx.Token != nil {
			if tokenCtx.Token.Value == node.Module {
				highlights = s.highlightImportUsages(doc, node.Module)
			} else if tokenCtx.Token.Value == node.Query {
				// Could highlight the query in the imported module, but that's cross-file
				// For now, highlight all setup calls to the same query
				highlights = s.highlightSetupCallQuery(doc, node.Module, node.Query)
			}
		}

	case *scaf.SetupClause:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			highlights = s.highlightImportUsages(doc, *node.Module)
		}

	case *scaf.SetupItem:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			highlights = s.highlightImportUsages(doc, *node.Module)
		}

	case *scaf.AssertQuery:
		if node.QueryName != nil {
			highlights = s.highlightQueryUsages(doc, *node.QueryName)
		}

	case *scaf.Statement:
		// Highlight parameter or return field usages within the test scope
		if node.KeyParts != nil {
			key := node.Key()
			if len(key) > 0 && key[0] == '$' {
				// Parameter - highlight all uses of this param in the current scope
				highlights = s.highlightParameterUsages(doc, tokenCtx.QueryScope, key)
			} else {
				// Return field - highlight all uses of this field in current scope
				highlights = s.highlightReturnFieldUsages(doc, tokenCtx.QueryScope, key)
			}
		}
	}

	return highlights, nil
}

// highlightQueryUsages finds all occurrences of a query name in the document.
func (s *Server) highlightQueryUsages(doc *Document, queryName string) []protocol.DocumentHighlight {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var highlights []protocol.DocumentHighlight

	// Find the query definition
	for _, q := range doc.Analysis.Suite.Queries {
		if q.Name == queryName {
			highlights = append(highlights, protocol.DocumentHighlight{
				Range: queryNameRange(q),
				Kind:  protocol.DocumentHighlightKindWrite,
			})
			break
		}
	}

	// Find all query scopes referencing this query
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName == queryName {
			highlights = append(highlights, protocol.DocumentHighlight{
				Range: scopeNameRange(scope),
				Kind:  protocol.DocumentHighlightKindRead,
			})
		}

		// Check for assert query references
		s.findAssertQueryHighlights(scope.Items, queryName, &highlights)
	}

	return highlights
}

// findAssertQueryHighlights recursively finds assert query references.
func (s *Server) findAssertQueryHighlights(items []*scaf.TestOrGroup, queryName string, highlights *[]protocol.DocumentHighlight) {
	for _, item := range items {
		if item.Test != nil {
			for _, assert := range item.Test.Asserts {
				if assert.Query != nil && assert.Query.QueryName != nil && *assert.Query.QueryName == queryName {
					*highlights = append(*highlights, protocol.DocumentHighlight{
						Range: assertQueryNameRange(assert.Query),
						Kind:  protocol.DocumentHighlightKindRead,
					})
				}
			}
		}
		if item.Group != nil {
			s.findAssertQueryHighlights(item.Group.Items, queryName, highlights)
		}
	}
}

// assertQueryNameRange returns the range for just the query name in an assert.
func assertQueryNameRange(aq *scaf.AssertQuery) protocol.Range {
	if aq.QueryName == nil {
		return spanToRange(aq.Span())
	}
	// The query name starts after "assert " (7 chars)
	nameStartCol := aq.Pos.Column + 7
	nameEndCol := nameStartCol + len(*aq.QueryName)

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(aq.Pos.Line - 1),   //nolint:gosec
			Character: uint32(nameStartCol - 1), //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(aq.Pos.Line - 1),  //nolint:gosec
			Character: uint32(nameEndCol - 1), //nolint:gosec
		},
	}
}

// highlightImportUsages finds all occurrences of an import alias in the document.
func (s *Server) highlightImportUsages(doc *Document, alias string) []protocol.DocumentHighlight {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var highlights []protocol.DocumentHighlight

	// Find the import definition
	for _, imp := range doc.Analysis.Suite.Imports {
		impAlias := baseNameFromImport(imp)
		if impAlias == alias {
			highlights = append(highlights, protocol.DocumentHighlight{
				Range: importAliasRange(imp),
				Kind:  protocol.DocumentHighlightKindWrite,
			})
			break
		}
	}

	// Find all usages in setup clauses
	s.findSetupImportHighlights(doc.Analysis.Suite.Setup, alias, &highlights)

	for _, scope := range doc.Analysis.Suite.Scopes {
		s.findSetupImportHighlights(scope.Setup, alias, &highlights)
		s.findItemSetupImportHighlights(scope.Items, alias, &highlights)
	}

	return highlights
}

// importAliasRange returns the range for the import alias (or path basename).
func importAliasRange(imp *scaf.Import) protocol.Range {
	if imp.Alias != nil {
		// "import alias" - alias starts after "import "
		aliasStartCol := imp.Pos.Column + 7 // "import " = 7 chars
		return protocol.Range{
			Start: protocol.Position{
				Line:      uint32(imp.Pos.Line - 1),     //nolint:gosec
				Character: uint32(aliasStartCol - 1), //nolint:gosec
			},
			End: protocol.Position{
				Line:      uint32(imp.Pos.Line - 1),                    //nolint:gosec
				Character: uint32(aliasStartCol - 1 + len(*imp.Alias)), //nolint:gosec
			},
		}
	}
	// No explicit alias - the alias is derived from path, highlight the path
	return spanToRange(imp.Span())
}

// findSetupImportHighlights finds import references in a setup clause.
func (s *Server) findSetupImportHighlights(setup *scaf.SetupClause, alias string, highlights *[]protocol.DocumentHighlight) {
	if setup == nil {
		return
	}

	if setup.Module != nil && *setup.Module == alias {
		*highlights = append(*highlights, protocol.DocumentHighlight{
			Range: setupModuleRange(setup),
			Kind:  protocol.DocumentHighlightKindRead,
		})
	}

	if setup.Call != nil && setup.Call.Module == alias {
		*highlights = append(*highlights, protocol.DocumentHighlight{
			Range: setupCallModuleRange(setup.Call),
			Kind:  protocol.DocumentHighlightKindRead,
		})
	}

	for _, item := range setup.Block {
		if item.Module != nil && *item.Module == alias {
			*highlights = append(*highlights, protocol.DocumentHighlight{
				Range: setupItemModuleRange(item),
				Kind:  protocol.DocumentHighlightKindRead,
			})
		}
		if item.Call != nil && item.Call.Module == alias {
			*highlights = append(*highlights, protocol.DocumentHighlight{
				Range: setupCallModuleRange(item.Call),
				Kind:  protocol.DocumentHighlightKindRead,
			})
		}
	}
}

// setupModuleRange returns the range for the module name in a setup clause.
func setupModuleRange(setup *scaf.SetupClause) protocol.Range {
	if setup.Module == nil {
		return spanToRange(setup.Span())
	}
	// Module name is the token value, find it in the span
	// "setup ModuleName" - module starts after "setup "
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(setup.Pos.Line - 1),
			Character: uint32(setup.Pos.Column - 1 + 6), // "setup " = 6 chars
		},
		End: protocol.Position{
			Line:      uint32(setup.Pos.Line - 1),
			Character: uint32(setup.Pos.Column - 1 + 6 + len(*setup.Module)),
		},
	}
}

// setupCallModuleRange returns the range for the module name in a setup call.
func setupCallModuleRange(call *scaf.SetupCall) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(call.Pos.Line - 1),      //nolint:gosec
			Character: uint32(call.Pos.Column - 1), //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(call.Pos.Line - 1),                    //nolint:gosec
			Character: uint32(call.Pos.Column - 1 + len(call.Module)), //nolint:gosec
		},
	}
}

// setupItemModuleRange returns the range for the module name in a setup item.
func setupItemModuleRange(item *scaf.SetupItem) protocol.Range {
	if item.Module == nil {
		return spanToRange(item.Span())
	}
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(item.Pos.Line - 1),      //nolint:gosec
			Character: uint32(item.Pos.Column - 1), //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(item.Pos.Line - 1),                    //nolint:gosec
			Character: uint32(item.Pos.Column - 1 + len(*item.Module)), //nolint:gosec
		},
	}
}

// findItemSetupImportHighlights recursively finds import references in test/group items.
func (s *Server) findItemSetupImportHighlights(items []*scaf.TestOrGroup, alias string, highlights *[]protocol.DocumentHighlight) {
	for _, item := range items {
		if item.Test != nil {
			s.findSetupImportHighlights(item.Test.Setup, alias, highlights)
		}
		if item.Group != nil {
			s.findSetupImportHighlights(item.Group.Setup, alias, highlights)
			s.findItemSetupImportHighlights(item.Group.Items, alias, highlights)
		}
	}
}

// highlightSetupCallQuery finds all setup calls to the same module.query.
func (s *Server) highlightSetupCallQuery(doc *Document, module, query string) []protocol.DocumentHighlight {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var highlights []protocol.DocumentHighlight

	var findInSetup func(*scaf.SetupClause)
	findInSetup = func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}
		if setup.Call != nil && setup.Call.Module == module && setup.Call.Query == query {
			highlights = append(highlights, protocol.DocumentHighlight{
				Range: setupCallQueryRange(setup.Call),
				Kind:  protocol.DocumentHighlightKindRead,
			})
		}
		for _, item := range setup.Block {
			if item.Call != nil && item.Call.Module == module && item.Call.Query == query {
				highlights = append(highlights, protocol.DocumentHighlight{
					Range: setupCallQueryRange(item.Call),
					Kind:  protocol.DocumentHighlightKindRead,
				})
			}
		}
	}

	var findInItems func([]*scaf.TestOrGroup)
	findInItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				findInSetup(item.Test.Setup)
			}
			if item.Group != nil {
				findInSetup(item.Group.Setup)
				findInItems(item.Group.Items)
			}
		}
	}

	findInSetup(doc.Analysis.Suite.Setup)
	for _, scope := range doc.Analysis.Suite.Scopes {
		findInSetup(scope.Setup)
		findInItems(scope.Items)
	}

	return highlights
}

// setupCallQueryRange returns the range for the query name in a setup call.
func setupCallQueryRange(call *scaf.SetupCall) protocol.Range {
	// Query name starts after "Module."
	queryStartCol := call.Pos.Column + len(call.Module) + 1 // +1 for dot
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(call.Pos.Line - 1),      //nolint:gosec
			Character: uint32(queryStartCol - 1), //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(call.Pos.Line - 1),                    //nolint:gosec
			Character: uint32(queryStartCol - 1 + len(call.Query)), //nolint:gosec
		},
	}
}

// highlightParameterUsages finds all occurrences of a parameter in the current scope.
func (s *Server) highlightParameterUsages(doc *Document, queryScope, paramKey string) []protocol.DocumentHighlight {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return nil
	}

	var highlights []protocol.DocumentHighlight

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}

		// Find all parameter usages in this scope's tests
		s.findParamInItems(scope.Items, paramKey, &highlights)
	}

	return highlights
}

// findParamInItems recursively finds parameter usages in test items.
func (s *Server) findParamInItems(items []*scaf.TestOrGroup, paramKey string, highlights *[]protocol.DocumentHighlight) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				if stmt.Key() == paramKey {
					*highlights = append(*highlights, protocol.DocumentHighlight{
						Range: statementKeyRange(stmt),
						Kind:  protocol.DocumentHighlightKindWrite,
					})
				}
			}
		}
		if item.Group != nil {
			s.findParamInItems(item.Group.Items, paramKey, highlights)
		}
	}
}

// statementKeyRange returns the range for the key part of a statement.
func statementKeyRange(stmt *scaf.Statement) protocol.Range {
	key := stmt.Key()
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(stmt.Pos.Line - 1),      //nolint:gosec
			Character: uint32(stmt.Pos.Column - 1), //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(stmt.Pos.Line - 1),             //nolint:gosec
			Character: uint32(stmt.Pos.Column - 1 + len(key)), //nolint:gosec
		},
	}
}

// highlightReturnFieldUsages finds all occurrences of a return field in the current scope.
func (s *Server) highlightReturnFieldUsages(doc *Document, queryScope, fieldKey string) []protocol.DocumentHighlight {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return nil
	}

	var highlights []protocol.DocumentHighlight

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}

		// Find all field usages in this scope's tests
		s.findFieldInItems(scope.Items, fieldKey, &highlights)
	}

	return highlights
}

// findFieldInItems recursively finds return field usages in test items.
func (s *Server) findFieldInItems(items []*scaf.TestOrGroup, fieldKey string, highlights *[]protocol.DocumentHighlight) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if len(key) > 0 && key[0] != '$' && key == fieldKey {
					*highlights = append(*highlights, protocol.DocumentHighlight{
						Range: statementKeyRange(stmt),
						Kind:  protocol.DocumentHighlightKindRead,
					})
				}
			}
		}
		if item.Group != nil {
			s.findFieldInItems(item.Group.Items, fieldKey, highlights)
		}
	}
}

// baseNameFromImport extracts the alias or base name from an import.
func baseNameFromImport(imp *scaf.Import) string {
	if imp.Alias != nil {
		return *imp.Alias
	}
	return baseNameFromPath(imp.Path)
}

// baseNameFromPath extracts the base name from an import path.
func baseNameFromPath(path string) string {
	// Remove leading ./ or ../
	for len(path) > 0 && (path[0] == '.' || path[0] == '/') {
		if len(path) > 1 && path[0] == '.' && path[1] == '.' {
			path = path[2:]
			if len(path) > 0 && path[0] == '/' {
				path = path[1:]
			}
			continue
		}
		if path[0] == '.' && len(path) > 1 && path[1] == '/' {
			path = path[2:]
			continue
		}
		if path[0] == '/' {
			path = path[1:]
			continue
		}
		break
	}

	// Get last component
	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash >= 0 {
		path = path[lastSlash+1:]
	}

	return path
}
