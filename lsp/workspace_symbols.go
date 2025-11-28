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

// Symbols handles workspace/symbol requests.
// Searches for queries, tests, and groups across all .scaf files in the workspace.
func (s *Server) Symbols(_ context.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	s.logger.Debug("Symbols",
		zap.String("query", params.Query))

	if s.workspaceRoot == "" {
		return nil, nil
	}

	var symbols []protocol.SymbolInformation
	query := strings.ToLower(params.Query)

	// Walk workspace looking for .scaf files
	err := filepath.Walk(s.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".scaf") {
			return nil
		}

		// Load and analyze the file
		analyzed, err := s.fileLoader.LoadAndAnalyze(path)
		if err != nil || analyzed.Suite == nil {
			return nil
		}

		uri := PathToURI(path)
		fileSymbols := s.extractWorkspaceSymbols(uri, analyzed, query)
		symbols = append(symbols, fileSymbols...)

		return nil
	})
	if err != nil {
		s.logger.Debug("Error walking workspace for symbols", zap.Error(err))
	}

	return symbols, nil
}

// extractWorkspaceSymbols extracts symbols from an analyzed file that match the query.
func (s *Server) extractWorkspaceSymbols(uri protocol.DocumentURI, f *analysis.AnalyzedFile, query string) []protocol.SymbolInformation {
	var symbols []protocol.SymbolInformation

	// Add imports
	for _, imp := range f.Suite.Imports {
		name := imp.Path
		if imp.Alias != nil {
			name = *imp.Alias
		}
		if query == "" || strings.Contains(strings.ToLower(name), query) {
			symbols = append(symbols, protocol.SymbolInformation{
				Name: name,
				Kind: protocol.SymbolKindModule,
				Location: protocol.Location{
					URI:   uri,
					Range: spanToRange(imp.Span()),
				},
				ContainerName: "",
			})
		}
	}

	// Add queries
	for _, q := range f.Suite.Queries {
		if query == "" || strings.Contains(strings.ToLower(q.Name), query) {
			symbols = append(symbols, protocol.SymbolInformation{
				Name: q.Name,
				Kind: protocol.SymbolKindFunction,
				Location: protocol.Location{
					URI:   uri,
					Range: spanToRange(q.Span()),
				},
				ContainerName: "",
			})
		}
	}

	// Add scopes, tests, and groups
	for _, scope := range f.Suite.Scopes {
		if query == "" || strings.Contains(strings.ToLower(scope.QueryName), query) {
			symbols = append(symbols, protocol.SymbolInformation{
				Name: scope.QueryName,
				Kind: protocol.SymbolKindClass,
				Location: protocol.Location{
					URI:   uri,
					Range: spanToRange(scope.Span()),
				},
				ContainerName: "",
			})
		}

		// Extract tests and groups from scope
		symbols = append(symbols, s.extractItemSymbols(uri, scope.QueryName, scope.Items, query)...)
	}

	return symbols
}

// extractItemSymbols recursively extracts test and group symbols from items.
func (s *Server) extractItemSymbols(uri protocol.DocumentURI, container string, items []*scaf.TestOrGroup, query string) []protocol.SymbolInformation {
	var symbols []protocol.SymbolInformation

	for _, item := range items {
		if item.Test != nil {
			if query == "" || strings.Contains(strings.ToLower(item.Test.Name), query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: item.Test.Name,
					Kind: protocol.SymbolKindMethod,
					Location: protocol.Location{
						URI:   uri,
						Range: spanToRange(item.Test.Span()),
					},
					ContainerName: container,
				})
			}
		}

		if item.Group != nil {
			groupContainer := container + "/" + item.Group.Name
			if query == "" || strings.Contains(strings.ToLower(item.Group.Name), query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: item.Group.Name,
					Kind: protocol.SymbolKindNamespace,
					Location: protocol.Location{
						URI:   uri,
						Range: spanToRange(item.Group.Span()),
					},
					ContainerName: container,
				})
			}

			// Recursively extract from nested items
			symbols = append(symbols, s.extractItemSymbols(uri, groupContainer, item.Group.Items, query)...)
		}
	}

	return symbols
}
