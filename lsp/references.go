package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// References handles textDocument/references requests.
// Finds all references to the symbol under the cursor across the workspace.
func (s *Server) References(_ context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	s.logger.Debug("References",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character),
		zap.Bool("includeDeclaration", params.Context.IncludeDeclaration))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	var locations []protocol.Location
	includeDecl := params.Context.IncludeDeclaration

	// Determine what kind of symbol we're on and find all references
	switch node := tokenCtx.Node.(type) {
	case *scaf.Query:
		locations = s.findQueryReferences(doc, node.Name, includeDecl)

	case *scaf.QueryScope:
		locations = s.findQueryReferences(doc, node.QueryName, includeDecl)

	case *scaf.Import:
		alias := baseNameFromImport(node)
		locations = s.findImportReferences(doc, alias, includeDecl)

	case *scaf.SetupCall:
		if tokenCtx.Token != nil {
			if tokenCtx.Token.Value == node.Module {
				locations = s.findImportReferences(doc, node.Module, includeDecl)
			} else if tokenCtx.Token.Value == node.Query {
				// Find references to this query in the imported module
				locations = s.findCrossFileQueryReferences(doc, node.Module, node.Query, includeDecl)
			}
		}

	case *scaf.SetupClause:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			locations = s.findImportReferences(doc, *node.Module, includeDecl)
		}

	case *scaf.SetupItem:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			locations = s.findImportReferences(doc, *node.Module, includeDecl)
		}

	case *scaf.AssertQuery:
		if node.QueryName != nil {
			locations = s.findQueryReferences(doc, *node.QueryName, includeDecl)
		}

	case *scaf.Statement:
		if node.KeyParts != nil {
			key := node.Key()
			if len(key) > 0 && key[0] == '$' {
				locations = s.findParameterReferences(doc, tokenCtx.QueryScope, key, includeDecl)
			} else {
				locations = s.findReturnFieldReferences(doc, tokenCtx.QueryScope, key, includeDecl)
			}
		}
	}

	return locations, nil
}

// findQueryReferences finds all references to a query in the current document.
func (s *Server) findQueryReferences(doc *Document, queryName string, includeDecl bool) []protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var locations []protocol.Location

	// Include declaration if requested
	if includeDecl {
		for _, q := range doc.Analysis.Suite.Queries {
			if q.Name == queryName {
				locations = append(locations, protocol.Location{
					URI:   doc.URI,
					Range: queryNameRange(q),
				})
				break
			}
		}
	}

	// Find all query scope references
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName == queryName {
			locations = append(locations, protocol.Location{
				URI:   doc.URI,
				Range: scopeNameRange(scope),
			})
		}

		// Find assert query references
		s.collectAssertQueryRefs(doc.URI, scope.Items, queryName, &locations)
	}

	// Also search other open documents in the workspace
	s.mu.RLock()
	for uri, otherDoc := range s.documents {
		if uri == doc.URI || otherDoc.Analysis == nil || otherDoc.Analysis.Suite == nil {
			continue
		}

		// Check if this document imports the current document and references the query
		// For now, just check same-file references
		// Cross-file query references happen through setup calls, which are handled separately
	}
	s.mu.RUnlock()

	return locations
}

// collectAssertQueryRefs recursively collects assert query references.
func (s *Server) collectAssertQueryRefs(uri protocol.DocumentURI, items []*scaf.TestOrGroup, queryName string, locations *[]protocol.Location) {
	for _, item := range items {
		if item.Test != nil {
			for _, assert := range item.Test.Asserts {
				if assert.Query != nil && assert.Query.QueryName != nil && *assert.Query.QueryName == queryName {
					*locations = append(*locations, protocol.Location{
						URI:   uri,
						Range: assertQueryNameRange(assert.Query),
					})
				}
			}
		}
		if item.Group != nil {
			s.collectAssertQueryRefs(uri, item.Group.Items, queryName, locations)
		}
	}
}

// findImportReferences finds all references to an import alias.
func (s *Server) findImportReferences(doc *Document, alias string, includeDecl bool) []protocol.Location {
	if doc.Analysis.Suite == nil {
		return nil
	}

	var locations []protocol.Location

	// Include declaration if requested
	if includeDecl {
		for _, imp := range doc.Analysis.Suite.Imports {
			if baseNameFromImport(imp) == alias {
				locations = append(locations, protocol.Location{
					URI:   doc.URI,
					Range: importAliasRange(imp),
				})
				break
			}
		}
	}

	// Find all usages
	s.collectSetupImportRefs(doc.URI, doc.Analysis.Suite.Setup, alias, &locations)

	for _, scope := range doc.Analysis.Suite.Scopes {
		s.collectSetupImportRefs(doc.URI, scope.Setup, alias, &locations)
		s.collectItemSetupImportRefs(doc.URI, scope.Items, alias, &locations)
	}

	return locations
}

// collectSetupImportRefs collects import references in a setup clause.
func (s *Server) collectSetupImportRefs(uri protocol.DocumentURI, setup *scaf.SetupClause, alias string, locations *[]protocol.Location) {
	if setup == nil {
		return
	}

	if setup.Module != nil && *setup.Module == alias {
		*locations = append(*locations, protocol.Location{
			URI:   uri,
			Range: setupModuleRange(setup),
		})
	}

	if setup.Call != nil && setup.Call.Module == alias {
		*locations = append(*locations, protocol.Location{
			URI:   uri,
			Range: setupCallModuleRange(setup.Call),
		})
	}

	for _, item := range setup.Block {
		if item.Module != nil && *item.Module == alias {
			*locations = append(*locations, protocol.Location{
				URI:   uri,
				Range: setupItemModuleRange(item),
			})
		}
		if item.Call != nil && item.Call.Module == alias {
			*locations = append(*locations, protocol.Location{
				URI:   uri,
				Range: setupCallModuleRange(item.Call),
			})
		}
	}
}

// collectItemSetupImportRefs recursively collects import references in test/group items.
func (s *Server) collectItemSetupImportRefs(uri protocol.DocumentURI, items []*scaf.TestOrGroup, alias string, locations *[]protocol.Location) {
	for _, item := range items {
		if item.Test != nil {
			s.collectSetupImportRefs(uri, item.Test.Setup, alias, locations)
		}
		if item.Group != nil {
			s.collectSetupImportRefs(uri, item.Group.Setup, alias, locations)
			s.collectItemSetupImportRefs(uri, item.Group.Items, alias, locations)
		}
	}
}

// findCrossFileQueryReferences finds references to a query from an imported module.
func (s *Server) findCrossFileQueryReferences(doc *Document, moduleAlias, queryName string, includeDecl bool) []protocol.Location {
	var locations []protocol.Location

	// Find the import to get the file path
	imp, ok := doc.Analysis.Symbols.Imports[moduleAlias]
	if !ok || s.fileLoader == nil {
		return nil
	}

	docPath := URIToPath(doc.URI)
	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

	// Include the declaration in the imported file if requested
	if includeDecl {
		importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
		if err == nil && importedFile.Symbols != nil {
			if q, ok := importedFile.Symbols.Queries[queryName]; ok && q.Node != nil {
				locations = append(locations, protocol.Location{
					URI:   PathToURI(importedPath),
					Range: queryNameRange(q.Node),
				})
			}
		}
	}

	// Find all setup calls to this module.query in current document
	s.collectSetupCallQueryRefs(doc.URI, doc.Analysis.Suite, moduleAlias, queryName, &locations)

	// Search other open documents that might also call this module.query
	s.mu.RLock()
	for uri, otherDoc := range s.documents {
		if uri == doc.URI || otherDoc.Analysis == nil || otherDoc.Analysis.Suite == nil {
			continue
		}
		// Check if this document imports the same module
		for otherAlias, otherImp := range otherDoc.Analysis.Symbols.Imports {
			otherDocPath := URIToPath(uri)
			otherImportedPath := s.fileLoader.ResolveImportPath(otherDocPath, otherImp.Path)
			if otherImportedPath == importedPath {
				// Same imported file - collect references
				s.collectSetupCallQueryRefs(uri, otherDoc.Analysis.Suite, otherAlias, queryName, &locations)
			}
		}
	}
	s.mu.RUnlock()

	// Also search workspace files that aren't currently open
	if s.workspaceRoot != "" {
		s.searchWorkspaceForQueryRefs(importedPath, queryName, doc.URI, &locations)
	}

	return locations
}

// collectSetupCallQueryRefs collects setup call references to a specific module.query.
func (s *Server) collectSetupCallQueryRefs(uri protocol.DocumentURI, suite *scaf.Suite, moduleAlias, queryName string, locations *[]protocol.Location) {
	if suite == nil {
		return
	}

	var findInSetup func(*scaf.SetupClause)
	findInSetup = func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}
		if setup.Call != nil && setup.Call.Module == moduleAlias && setup.Call.Query == queryName {
			*locations = append(*locations, protocol.Location{
				URI:   uri,
				Range: setupCallQueryRange(setup.Call),
			})
		}
		for _, item := range setup.Block {
			if item.Call != nil && item.Call.Module == moduleAlias && item.Call.Query == queryName {
				*locations = append(*locations, protocol.Location{
					URI:   uri,
					Range: setupCallQueryRange(item.Call),
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

	findInSetup(suite.Setup)
	for _, scope := range suite.Scopes {
		findInSetup(scope.Setup)
		findInItems(scope.Items)
	}
}

// searchWorkspaceForQueryRefs searches workspace files for references to a query.
func (s *Server) searchWorkspaceForQueryRefs(importedPath, queryName string, excludeURI protocol.DocumentURI, locations *[]protocol.Location) {
	// Walk workspace looking for .scaf files
	err := filepath.Walk(s.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".scaf") {
			return nil
		}

		uri := PathToURI(path)
		if uri == excludeURI {
			return nil // Already processed
		}

		// Check if already open
		s.mu.RLock()
		_, isOpen := s.documents[uri]
		s.mu.RUnlock()
		if isOpen {
			return nil // Already processed in main loop
		}

		// Load and analyze the file
		analyzed, err := s.fileLoader.LoadAndAnalyze(path)
		if err != nil || analyzed.Suite == nil {
			return nil
		}

		// Check if this file imports the target module
		for alias, imp := range analyzed.Symbols.Imports {
			resolvedPath := s.fileLoader.ResolveImportPath(path, imp.Path)
			if resolvedPath == importedPath {
				s.collectSetupCallQueryRefs(uri, analyzed.Suite, alias, queryName, locations)
			}
		}

		return nil
	})
	if err != nil {
		s.logger.Debug("Error walking workspace for references", zap.Error(err))
	}
}

// findParameterReferences finds all references to a parameter in the current scope.
func (s *Server) findParameterReferences(doc *Document, queryScope, paramKey string, includeDecl bool) []protocol.Location {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return nil
	}

	var locations []protocol.Location

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}
		s.collectParamRefs(doc.URI, scope.Items, paramKey, &locations)
	}

	// Also include the parameter in the query body if it exists
	if includeDecl {
		if q, ok := doc.Analysis.Symbols.Queries[queryScope]; ok && s.queryAnalyzer != nil {
			metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
			if err == nil {
				paramName := paramKey[1:] // Remove $ prefix
				for _, p := range metadata.Parameters {
					if p.Name == paramName && p.Line > 0 && p.Column > 0 {
						// Calculate document position
						queryBodyStartCol := q.Node.Pos.Column + 6 + len(q.Name) + 2
						docLine := q.Node.Pos.Line + p.Line - 1
						docColumn := p.Column
						if p.Line == 1 {
							docColumn = queryBodyStartCol + p.Column - 1
						}

						locations = append(locations, protocol.Location{
							URI: doc.URI,
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(docLine - 1),   //nolint:gosec
									Character: uint32(docColumn - 1), //nolint:gosec
								},
								End: protocol.Position{
									Line:      uint32(docLine - 1),               //nolint:gosec
									Character: uint32(docColumn - 1 + p.Length), //nolint:gosec
								},
							},
						})
						break
					}
				}
			}
		}
	}

	return locations
}

// collectParamRefs recursively collects parameter references in test items.
func (s *Server) collectParamRefs(uri protocol.DocumentURI, items []*scaf.TestOrGroup, paramKey string, locations *[]protocol.Location) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				if stmt.Key() == paramKey {
					*locations = append(*locations, protocol.Location{
						URI:   uri,
						Range: statementKeyRange(stmt),
					})
				}
			}
		}
		if item.Group != nil {
			s.collectParamRefs(uri, item.Group.Items, paramKey, locations)
		}
	}
}

// findReturnFieldReferences finds all references to a return field in the current scope.
func (s *Server) findReturnFieldReferences(doc *Document, queryScope, fieldKey string, includeDecl bool) []protocol.Location {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return nil
	}

	var locations []protocol.Location

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}
		s.collectFieldRefs(doc.URI, scope.Items, fieldKey, &locations)
	}

	// Include the field in the query RETURN clause if requested
	if includeDecl {
		if q, ok := doc.Analysis.Symbols.Queries[queryScope]; ok && s.queryAnalyzer != nil {
			metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
			if err == nil {
				for _, ret := range metadata.Returns {
					if ret.Name == fieldKey || ret.Expression == fieldKey || ret.Alias == fieldKey {
						if ret.Line > 0 && ret.Column > 0 {
							queryBodyStartCol := q.Node.Pos.Column + 6 + len(q.Name) + 2
							docLine := q.Node.Pos.Line + ret.Line - 1
							docColumn := ret.Column
							if ret.Line == 1 {
								docColumn = queryBodyStartCol + ret.Column - 1
							}

							locations = append(locations, protocol.Location{
								URI: doc.URI,
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(docLine - 1),   //nolint:gosec
										Character: uint32(docColumn - 1), //nolint:gosec
									},
									End: protocol.Position{
										Line:      uint32(docLine - 1),                //nolint:gosec
										Character: uint32(docColumn - 1 + ret.Length), //nolint:gosec
									},
								},
							})
							break
						}
					}
				}
			}
		}
	}

	return locations
}

// collectFieldRefs recursively collects return field references in test items.
func (s *Server) collectFieldRefs(uri protocol.DocumentURI, items []*scaf.TestOrGroup, fieldKey string, locations *[]protocol.Location) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if len(key) > 0 && key[0] != '$' && key == fieldKey {
					*locations = append(*locations, protocol.Location{
						URI:   uri,
						Range: statementKeyRange(stmt),
					})
				}
			}
		}
		if item.Group != nil {
			s.collectFieldRefs(uri, item.Group.Items, fieldKey, locations)
		}
	}
}
