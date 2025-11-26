package lsp

import (
	"context"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// Definition handles textDocument/definition requests.
func (s *Server) Definition(_ context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	s.logger.Debug("Definition",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)

	// Try to find what's at this position and resolve its definition

	// 1. Check if we're on a query scope's query name reference
	if queryDef := s.findQueryDefinition(doc, pos); queryDef != nil {
		return []protocol.Location{*queryDef}, nil
	}

	// 2. Check if we're on an import alias reference in a setup call
	if importDef := s.findImportDefinition(doc, pos); importDef != nil {
		return []protocol.Location{*importDef}, nil
	}

	return nil, nil
}

// findQueryDefinition checks if the position is on a query reference and returns its definition.
func (s *Server) findQueryDefinition(doc *Document, pos lexer.Position) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Check query scopes - if cursor is on the scope line, find the query definition
	for _, scope := range doc.Analysis.Suite.Scopes {
		// Check if position is on the query name part of the scope declaration
		// The query name starts at the beginning of the line and goes until the '{'
		if pos.Line == scope.Pos.Line && pos.Column <= len(scope.QueryName)+1 {
			// Find the query definition
			if q, ok := doc.Analysis.Symbols.Queries[scope.QueryName]; ok {
				return &protocol.Location{
					URI:   doc.URI,
					Range: queryNameRange(q.Node),
				}
			}
		}
	}

	return nil
}

// queryNameRange returns the range of just the query name (not the whole query).
// The name starts after "query " (6 characters).
func queryNameRange(q *scaf.Query) protocol.Range {
	nameStartCol := q.Pos.Column + 6 // "query " = 6 chars
	nameEndCol := nameStartCol + len(q.Name)

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(q.Pos.Line - 1),   //nolint:gosec // Line numbers are always small positive integers
			Character: uint32(nameStartCol - 1), //nolint:gosec // Column numbers are always small positive integers
		},
		End: protocol.Position{
			Line:      uint32(q.Pos.Line - 1),  //nolint:gosec // Line numbers are always small positive integers
			Character: uint32(nameEndCol - 1), //nolint:gosec // Column numbers are always small positive integers
		},
	}
}

// findImportDefinition checks if the position is on an import alias reference and returns its definition.
func (s *Server) findImportDefinition(doc *Document, pos lexer.Position) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Check setup clauses for module references
	// This includes top-level setup, scope-level setup, and test/group setup

	// Check top-level setup
	if loc := s.findImportInSetup(doc, doc.Analysis.Suite.Setup, pos); loc != nil {
		return loc
	}

	// Check scope-level setups and nested items
	for _, scope := range doc.Analysis.Suite.Scopes {
		if loc := s.findImportInSetup(doc, scope.Setup, pos); loc != nil {
			return loc
		}

		if loc := s.findImportInItems(doc, scope.Items, pos); loc != nil {
			return loc
		}
	}

	return nil
}

// findImportInSetup looks for import references in a setup clause.
func (s *Server) findImportInSetup(doc *Document, setup *scaf.SetupClause, pos lexer.Position) *protocol.Location {
	if setup == nil {
		return nil
	}

	// Check named setup
	if setup.Named != nil && setup.Named.Module != nil {
		if loc := s.checkModuleReference(doc, *setup.Named.Module, pos); loc != nil {
			return loc
		}
	}

	// Check block items
	for _, item := range setup.Block {
		if item.Named != nil && item.Named.Module != nil {
			if loc := s.checkModuleReference(doc, *item.Named.Module, pos); loc != nil {
				return loc
			}
		}
	}

	return nil
}

// checkModuleReference checks if the cursor is on a module reference and returns the import location.
func (s *Server) checkModuleReference(doc *Document, moduleName string, _ lexer.Position) *protocol.Location {
	// Look up the import by module name
	if imp, ok := doc.Analysis.Symbols.Imports[moduleName]; ok {
		return &protocol.Location{
			URI:   doc.URI,
			Range: spanToRange(imp.Span),
		}
	}

	return nil
}

// findImportInItems recursively searches test/group items for import references.
func (s *Server) findImportInItems(doc *Document, items []*scaf.TestOrGroup, pos lexer.Position) *protocol.Location {
	for _, item := range items {
		if item.Test != nil {
			if loc := s.findImportInSetup(doc, item.Test.Setup, pos); loc != nil {
				return loc
			}
		}

		if item.Group != nil {
			if loc := s.findImportInSetup(doc, item.Group.Setup, pos); loc != nil {
				return loc
			}

			if loc := s.findImportInItems(doc, item.Group.Items, pos); loc != nil {
				return loc
			}
		}
	}

	return nil
}
