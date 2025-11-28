package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// DocumentSymbol handles textDocument/documentSymbol requests.
// Returns a hierarchical tree of symbols for the outline view.
func (s *Server) DocumentSymbol(_ context.Context, params *protocol.DocumentSymbolParams) ([]any, error) {
	s.logger.Debug("DocumentSymbol",
		zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	symbols := s.buildDocumentSymbols(doc.Analysis)
	
	// Convert to []any for the protocol
	result := make([]any, len(symbols))
	for i, sym := range symbols {
		result[i] = sym
	}

	return result, nil
}

// buildDocumentSymbols creates a hierarchical symbol tree from the AST.
func (s *Server) buildDocumentSymbols(f *analysis.AnalyzedFile) []protocol.DocumentSymbol {
	var symbols []protocol.DocumentSymbol

	// Add imports
	for _, imp := range f.Suite.Imports {
		name := imp.Path
		if imp.Alias != nil {
			name = *imp.Alias
		}
		symbols = append(symbols, protocol.DocumentSymbol{
			Name:           name,
			Kind:           protocol.SymbolKindModule,
			Range:          spanToRange(imp.Span()),
			SelectionRange: spanToRange(imp.Span()),
			Detail:         "import",
		})
	}

	// Add queries
	for _, q := range f.Suite.Queries {
		symbols = append(symbols, protocol.DocumentSymbol{
			Name:           q.Name,
			Kind:           protocol.SymbolKindFunction,
			Range:          spanToRange(q.Span()),
			SelectionRange: queryNameRange(q),
			Detail:         "query",
		})
	}

	// Add global setup if present
	if f.Suite.Setup != nil {
		symbols = append(symbols, s.buildSetupSymbol(f.Suite.Setup, "setup"))
	}

	// Add query scopes with nested tests/groups
	for _, scope := range f.Suite.Scopes {
		symbols = append(symbols, s.buildScopeSymbol(scope))
	}

	return symbols
}

// buildScopeSymbol creates a symbol for a query scope with nested children.
func (s *Server) buildScopeSymbol(scope *scaf.QueryScope) protocol.DocumentSymbol {
	sym := protocol.DocumentSymbol{
		Name:           scope.QueryName,
		Kind:           protocol.SymbolKindClass,
		Range:          spanToRange(scope.Span()),
		SelectionRange: scopeNameRange(scope),
		Detail:         "query scope",
	}

	var children []protocol.DocumentSymbol

	// Add setup if present
	if scope.Setup != nil {
		children = append(children, s.buildSetupSymbol(scope.Setup, "setup"))
	}

	// Add tests and groups
	for _, item := range scope.Items {
		if item.Test != nil {
			children = append(children, s.buildTestSymbol(item.Test))
		}
		if item.Group != nil {
			children = append(children, s.buildGroupSymbol(item.Group))
		}
	}

	sym.Children = children
	return sym
}

// buildTestSymbol creates a symbol for a test.
func (s *Server) buildTestSymbol(test *scaf.Test) protocol.DocumentSymbol {
	sym := protocol.DocumentSymbol{
		Name:           test.Name,
		Kind:           protocol.SymbolKindMethod,
		Range:          spanToRange(test.Span()),
		SelectionRange: testNameRange(test),
		Detail:         "test",
	}

	var children []protocol.DocumentSymbol

	// Add setup if present
	if test.Setup != nil {
		children = append(children, s.buildSetupSymbol(test.Setup, "setup"))
	}

	// Add assertions as children
	for i, assert := range test.Asserts {
		name := "assert"
		if assert.Query != nil && assert.Query.QueryName != nil {
			name = "assert " + *assert.Query.QueryName
		} else if i > 0 {
			name = "assert " + string(rune('1'+i))
		}
		children = append(children, protocol.DocumentSymbol{
			Name:           name,
			Kind:           protocol.SymbolKindEvent,
			Range:          spanToRange(assert.Span()),
			SelectionRange: spanToRange(assert.Span()),
			Detail:         "assertion",
		})
	}

	sym.Children = children
	return sym
}

// buildGroupSymbol creates a symbol for a group with nested children.
func (s *Server) buildGroupSymbol(group *scaf.Group) protocol.DocumentSymbol {
	sym := protocol.DocumentSymbol{
		Name:           group.Name,
		Kind:           protocol.SymbolKindNamespace,
		Range:          spanToRange(group.Span()),
		SelectionRange: groupNameRange(group),
		Detail:         "group",
	}

	var children []protocol.DocumentSymbol

	// Add setup if present
	if group.Setup != nil {
		children = append(children, s.buildSetupSymbol(group.Setup, "setup"))
	}

	// Add nested tests and groups
	for _, item := range group.Items {
		if item.Test != nil {
			children = append(children, s.buildTestSymbol(item.Test))
		}
		if item.Group != nil {
			children = append(children, s.buildGroupSymbol(item.Group))
		}
	}

	sym.Children = children
	return sym
}

// buildSetupSymbol creates a symbol for a setup clause.
func (s *Server) buildSetupSymbol(setup *scaf.SetupClause, name string) protocol.DocumentSymbol {
	detail := "setup"
	if setup.Module != nil {
		detail = "setup " + *setup.Module
	} else if setup.Call != nil {
		detail = "setup " + setup.Call.Module + "." + setup.Call.Query
	} else if setup.Inline != nil {
		detail = "inline setup"
	} else if len(setup.Block) > 0 {
		detail = "setup block"
	}

	return protocol.DocumentSymbol{
		Name:           name,
		Kind:           protocol.SymbolKindConstructor,
		Range:          spanToRange(setup.Span()),
		SelectionRange: spanToRange(setup.Span()),
		Detail:         detail,
	}
}

// scopeNameRange returns the range for the query name in a scope declaration.
func scopeNameRange(scope *scaf.QueryScope) protocol.Range {
	// The scope name starts at the beginning of the line
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(scope.Pos.Line - 1),
			Character: uint32(scope.Pos.Column - 1),
		},
		End: protocol.Position{
			Line:      uint32(scope.Pos.Line - 1),
			Character: uint32(scope.Pos.Column - 1 + len(scope.QueryName)),
		},
	}
}

// testNameRange returns the range for the test name.
func testNameRange(test *scaf.Test) protocol.Range {
	// The test name is after "test " and within quotes
	// Position points to "test", name starts after 'test "'
	nameStartCol := test.Pos.Column + 6 // 'test "' = 6 chars
	
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(test.Pos.Line - 1),
			Character: uint32(nameStartCol - 1),
		},
		End: protocol.Position{
			Line:      uint32(test.Pos.Line - 1),
			Character: uint32(nameStartCol - 1 + len(test.Name)),
		},
	}
}

// groupNameRange returns the range for the group name.
func groupNameRange(group *scaf.Group) protocol.Range {
	// The group name is after "group " and within quotes
	nameStartCol := group.Pos.Column + 7 // 'group "' = 7 chars
	
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(group.Pos.Line - 1),
			Character: uint32(nameStartCol - 1),
		},
		End: protocol.Position{
			Line:      uint32(group.Pos.Line - 1),
			Character: uint32(nameStartCol - 1 + len(group.Name)),
		},
	}
}
