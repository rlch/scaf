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

	// Get token context for precise position information
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	// Log token info for debugging
	if tokenCtx.Token != nil {
		s.logger.Debug("Definition at token",
			zap.String("value", tokenCtx.Token.Value),
			zap.Int("line", tokenCtx.Token.Pos.Line),
			zap.Int("col", tokenCtx.Token.Pos.Column))
	}

	// Try to find what's at this position and resolve its definition

	// 1. Check if we're on a query scope's query name reference
	if queryDef := s.findQueryDefinition(doc, pos, tokenCtx); queryDef != nil {
		return []protocol.Location{*queryDef}, nil
	}

	// 2. Check if we're on an import alias reference in a setup call
	if importDef := s.findImportDefinition(doc, tokenCtx); importDef != nil {
		return []protocol.Location{*importDef}, nil
	}

	// 3. Check if we're on a setup call (go to query definition)
	if setupCallDef := s.findSetupCallDefinition(doc, tokenCtx); setupCallDef != nil {
		return []protocol.Location{*setupCallDef}, nil
	}

	// 4. Check if we're on a parameter reference ($param in test)
	if paramDef := s.findParameterDefinition(doc, tokenCtx); paramDef != nil {
		return []protocol.Location{*paramDef}, nil
	}

	// 5. Check if we're on an assert query name reference
	if assertDef := s.findAssertQueryDefinition(doc, tokenCtx); assertDef != nil {
		return []protocol.Location{*assertDef}, nil
	}

	// 6. Check if we're on a return field reference (u.name in test)
	if returnFieldDef := s.findReturnFieldDefinition(doc, tokenCtx); returnFieldDef != nil {
		return []protocol.Location{*returnFieldDef}, nil
	}

	return nil, nil
}

// findQueryDefinition checks if the position is on a query reference and returns its definition.
func (s *Server) findQueryDefinition(doc *Document, pos lexer.Position, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Check if we're on a QueryScope node and the token is the query name
	if scope, ok := tokenCtx.Node.(*scaf.QueryScope); ok {
		// Check if the token is the query name (first identifier on the line)
		if tokenCtx.Token != nil && tokenCtx.Token.Value == scope.QueryName {
			if q, ok := doc.Analysis.Symbols.Queries[scope.QueryName]; ok {
				return &protocol.Location{
					URI:   doc.URI,
					Range: queryNameRange(q.Node),
				}
			}
		}
	}

	// Fallback: Check query scopes by position
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

// findImportDefinition checks if the position is on a module reference and returns the module file's location.
func (s *Server) findImportDefinition(doc *Document, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var moduleName string

	// Check if we're on a SetupClause with just a module reference (setup fixtures)
	if clause, ok := tokenCtx.Node.(*scaf.SetupClause); ok && clause.Module != nil {
		if tokenCtx.Token != nil && tokenCtx.Token.Value == *clause.Module {
			moduleName = *clause.Module
		}
	}

	// Check if we're on a SetupItem with just a module reference (fixtures in block)
	if item, ok := tokenCtx.Node.(*scaf.SetupItem); ok && item.Module != nil {
		if tokenCtx.Token != nil && tokenCtx.Token.Value == *item.Module {
			moduleName = *item.Module
		}
	}

	// Check if we're on a SetupCall node and the token is the module name
	if call, ok := tokenCtx.Node.(*scaf.SetupCall); ok {
		if tokenCtx.Token != nil && tokenCtx.Token.Value == call.Module {
			moduleName = call.Module
		}
	}

	if moduleName == "" {
		return nil
	}

	// Look up the import by module name
	imp, ok := doc.Analysis.Symbols.Imports[moduleName]
	if !ok {
		return nil
	}

	// Navigate to the imported file
	return s.findModuleFileLocation(doc, imp.Path)
}

// findModuleFileLocation resolves an import path and returns a location at the start of the file.
func (s *Server) findModuleFileLocation(doc *Document, importPath string) *protocol.Location {
	if s.fileLoader == nil {
		return nil
	}

	docPath := URIToPath(doc.URI)
	resolvedPath := s.fileLoader.ResolveImportPath(docPath, importPath)

	return &protocol.Location{
		URI: PathToURI(resolvedPath),
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
	}
}

// findSetupCallDefinition finds the definition of a setup call.
// This navigates to the query being called.
func (s *Server) findSetupCallDefinition(doc *Document, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Check if we're on a SetupCall node
	call, ok := tokenCtx.Node.(*scaf.SetupCall)
	if !ok {
		return nil
	}

	// Check if the token is the query name (not the module)
	if tokenCtx.Token != nil && tokenCtx.Token.Value == call.Query {
		// Look up in imported file
		return s.findCrossFileDefinition(doc, call.Module, call.Query)
	}

	return nil
}

// findParameterDefinition finds the definition of a parameter in the query body.
// When clicking on $param in a test statement, this navigates to where $param is used in the query.
func (s *Server) findParameterDefinition(doc *Document, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil || s.queryAnalyzer == nil {
		return nil
	}

	// Check if we're on a Statement node with a parameter key ($...)
	stmt, ok := tokenCtx.Node.(*scaf.Statement)
	if !ok || stmt.KeyParts == nil {
		return nil
	}

	key := stmt.Key()
	if len(key) == 0 || key[0] != '$' {
		return nil // Not a parameter
	}

	paramName := key[1:] // Strip the $ prefix

	// Find the enclosing query scope
	if tokenCtx.QueryScope == "" {
		return nil
	}

	// Get the query for this scope
	q, ok := doc.Analysis.Symbols.Queries[tokenCtx.QueryScope]
	if !ok || q.Body == "" {
		return nil
	}

	// Analyze the query to get parameter positions
	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for parameter definition", zap.Error(err))
		return nil
	}

	// Find the parameter in the query
	for _, param := range metadata.Parameters {
		if param.Name == paramName {
			// Calculate the position in the document
			// The query body starts after "query Name `" on the query definition line
			// We need to map the param position within the query body to the document

			// Get the query node to find where the body starts
			queryNode := q.Node
			if queryNode == nil {
				return nil
			}

			// The query body is on the same line after "query Name `"
			// Query position: line X, column Y
			// Body starts at: column Y + len("query ") + len(Name) + len(" `") = Y + 6 + len(Name) + 2

			// For single-line queries, offset the parameter position
			queryBodyStartCol := queryNode.Pos.Column + 6 + len(q.Name) + 2 // "query " + Name + " `"

			// The parameter position is relative to the start of the query body
			// Line is relative to query start (1-indexed in query, query is line 1)
			docLine := queryNode.Pos.Line + param.Line - 1
			docColumn := param.Column

			// For first line of query, add the offset
			if param.Line == 1 {
				docColumn = queryBodyStartCol + param.Column - 1
			}

			return &protocol.Location{
				URI: doc.URI,
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(docLine - 1),  //nolint:gosec // Line numbers are always small
						Character: uint32(docColumn - 1), //nolint:gosec // Column numbers are always small
					},
					End: protocol.Position{
						Line:      uint32(docLine - 1),               //nolint:gosec
						Character: uint32(docColumn - 1 + param.Length), //nolint:gosec
					},
				},
			}
		}
	}

	return nil
}

// findAssertQueryDefinition checks if the position is on an assert query name reference.
// This navigates from "assert QueryName(...)" to the query definition.
func (s *Server) findAssertQueryDefinition(doc *Document, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	// Check if we're on an AssertQuery node
	aq, ok := tokenCtx.Node.(*scaf.AssertQuery)
	if !ok {
		return nil
	}

	// Check if it's a named query (not inline)
	if aq.QueryName == nil {
		return nil
	}

	// Check if the token is the query name
	if tokenCtx.Token != nil && tokenCtx.Token.Value == *aq.QueryName {
		// Look up the query in symbols
		if q, ok := doc.Analysis.Symbols.Queries[*aq.QueryName]; ok {
			return &protocol.Location{
				URI:   doc.URI,
				Range: queryNameRange(q.Node),
			}
		}
	}

	return nil
}

// findReturnFieldDefinition finds the definition of a return field in the query body.
// When clicking on u.name in a test statement, this navigates to where u.name is returned in the query.
func (s *Server) findReturnFieldDefinition(doc *Document, tokenCtx *analysis.TokenContext) *protocol.Location {
	if doc.Analysis.Suite == nil || s.queryAnalyzer == nil {
		return nil
	}

	// Check if we're on a Statement node with a non-parameter key (not $...)
	stmt, ok := tokenCtx.Node.(*scaf.Statement)
	if !ok || stmt.KeyParts == nil {
		return nil
	}

	key := stmt.Key()
	if len(key) == 0 || key[0] == '$' {
		return nil // This is a parameter, not a return field
	}

	// Find the enclosing query scope
	if tokenCtx.QueryScope == "" {
		return nil
	}

	// Get the query for this scope
	q, ok := doc.Analysis.Symbols.Queries[tokenCtx.QueryScope]
	if !ok || q.Body == "" {
		return nil
	}

	// Analyze the query to get return field info
	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for return field definition", zap.Error(err))
		return nil
	}

	// Find the return field in the query
	// The key might be "u.name" or just "name" - match against both Name and Expression
	for _, ret := range metadata.Returns {
		if ret.Name == key || ret.Expression == key || ret.Alias == key {
			queryNode := q.Node
			if queryNode == nil {
				return nil
			}

			// If we have position info for the return field, navigate to it precisely
			if ret.Line > 0 && ret.Column > 0 {
				// Calculate the position in the document
				// The query body starts after "query Name `" on the query definition line
				// Query position: line X, column Y
				// Body starts at: column Y + len("query ") + len(Name) + len(" `") = Y + 6 + len(Name) + 2
				queryBodyStartCol := queryNode.Pos.Column + 6 + len(q.Name) + 2 // "query " + Name + " `"

				// The return field position is relative to the start of the query body
				docLine := queryNode.Pos.Line + ret.Line - 1
				docColumn := ret.Column

				// For first line of query, add the offset
				if ret.Line == 1 {
					docColumn = queryBodyStartCol + ret.Column - 1
				}

				return &protocol.Location{
					URI: doc.URI,
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(docLine - 1),   //nolint:gosec // Line numbers are always small
							Character: uint32(docColumn - 1), //nolint:gosec // Column numbers are always small
						},
						End: protocol.Position{
							Line:      uint32(docLine - 1),               //nolint:gosec
							Character: uint32(docColumn - 1 + ret.Length), //nolint:gosec
						},
					},
				}
			}

			// Fallback: navigate to the query name
			return &protocol.Location{
				URI:   doc.URI,
				Range: queryNameRange(queryNode),
			}
		}
	}

	return nil
}

// findCrossFileDefinition looks up a query definition in an imported module.
func (s *Server) findCrossFileDefinition(doc *Document, moduleAlias, queryName string) *protocol.Location {
	if s.fileLoader == nil {
		s.logger.Debug("FileLoader not available for cross-file definition")
		return nil
	}

	// Get the import for this alias
	imp, ok := doc.Analysis.Symbols.Imports[moduleAlias]
	if !ok {
		s.logger.Debug("Import not found for module alias", zap.String("alias", moduleAlias))
		return nil
	}

	// Resolve the import path to an absolute file path
	docPath := URIToPath(doc.URI)
	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

	s.logger.Debug("Resolving cross-file definition",
		zap.String("alias", moduleAlias),
		zap.String("queryName", queryName),
		zap.String("importPath", imp.Path),
		zap.String("resolvedPath", importedPath))

	// Load and analyze the imported file
	importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
	if err != nil {
		s.logger.Debug("Failed to load imported file",
			zap.String("path", importedPath),
			zap.Error(err))
		return nil
	}

	if importedFile.Symbols == nil {
		return nil
	}

	// Find the query in the imported file
	q, ok := importedFile.Symbols.Queries[queryName]
	if !ok {
		s.logger.Debug("Query not found in imported file",
			zap.String("queryName", queryName),
			zap.String("path", importedPath))
		return nil
	}

	// Return location in the imported file
	return &protocol.Location{
		URI:   PathToURI(importedPath),
		Range: queryNameRange(q.Node),
	}
}
