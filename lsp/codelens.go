package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
)

// CodeLens handles textDocument/codeLens requests.
// Returns code lenses for running tests, groups, and query scopes.
func (s *Server) CodeLens(_ context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	s.logger.Debug("CodeLens",
		zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	filePath := URIToPath(params.TextDocument.URI)
	var lenses []protocol.CodeLens

	// Walk through all query scopes
	for _, scope := range doc.Analysis.Suite.Scopes {
		// Add "Run All" lens for the query scope
		lenses = append(lenses, protocol.CodeLens{
			Range: scopeNameRange(scope),
			Command: &protocol.Command{
				Title:     "▶ Run All",
				Command:   "scaf.runScope",
				Arguments: []interface{}{filePath, scope.QueryName},
			},
		})

		// Walk through tests and groups in this scope
		lenses = append(lenses, s.collectItemLenses(filePath, scope.QueryName, "", scope.Items)...)
	}

	return lenses, nil
}

// collectItemLenses recursively collects code lenses for tests and groups.
func (s *Server) collectItemLenses(filePath, queryScope, groupPath string, items []*scaf.TestOrGroup) []protocol.CodeLens {
	var lenses []protocol.CodeLens

	for _, item := range items {
		if item == nil {
			continue
		}

		if item.Test != nil {
			testFullPath := buildPath(queryScope, groupPath, item.Test.Name)
			lenses = append(lenses, protocol.CodeLens{
				Range: testNameRange(item.Test),
				Command: &protocol.Command{
					Title:     "▶ Run Test",
					Command:   "scaf.runTest",
					Arguments: []interface{}{filePath, testFullPath},
				},
			})
		}

		if item.Group != nil {
			groupFullPath := buildPath(queryScope, groupPath, item.Group.Name)
			lenses = append(lenses, protocol.CodeLens{
				Range: groupNameRange(item.Group),
				Command: &protocol.Command{
					Title:     "▶ Run Group",
					Command:   "scaf.runGroup",
					Arguments: []interface{}{filePath, groupFullPath},
				},
			})

			// Recurse into nested items
			newGroupPath := groupPath
			if newGroupPath != "" {
				newGroupPath += "/"
			}
			newGroupPath += item.Group.Name

			lenses = append(lenses, s.collectItemLenses(filePath, queryScope, newGroupPath, item.Group.Items)...)
		}
	}

	return lenses
}

// buildPath constructs a full path from scope, group path, and name.
func buildPath(queryScope, groupPath, name string) string {
	path := queryScope
	if groupPath != "" {
		path += "/" + groupPath
	}
	path += "/" + name
	return path
}
