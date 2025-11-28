package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
)

// FoldingRanges handles textDocument/foldingRange requests.
// Returns folding ranges for queries, scopes, groups, tests, and setup blocks.
func (s *Server) FoldingRanges(_ context.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	s.logger.Debug("FoldingRanges",
		zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	var ranges []protocol.FoldingRange

	// Add folding ranges for imports (if multiple)
	if len(doc.Analysis.Suite.Imports) > 1 {
		firstImport := doc.Analysis.Suite.Imports[0]
		lastImport := doc.Analysis.Suite.Imports[len(doc.Analysis.Suite.Imports)-1]
		ranges = append(ranges, protocol.FoldingRange{
			StartLine: uint32(firstImport.Pos.Line - 1),   //nolint:gosec
			EndLine:   uint32(lastImport.EndPos.Line - 1), //nolint:gosec
			Kind:      protocol.ImportsFoldingRange,
		})
	}

	// Add folding ranges for queries
	for _, q := range doc.Analysis.Suite.Queries {
		ranges = append(ranges, s.queryFoldingRange(q))
	}

	// Add folding range for global setup
	if doc.Analysis.Suite.Setup != nil {
		ranges = append(ranges, s.setupFoldingRange(doc.Analysis.Suite.Setup))
	}

	// Add folding ranges for scopes
	for _, scope := range doc.Analysis.Suite.Scopes {
		ranges = append(ranges, s.scopeFoldingRanges(scope)...)
	}

	return ranges, nil
}

// queryFoldingRange creates a folding range for a query definition.
func (s *Server) queryFoldingRange(q *scaf.Query) protocol.FoldingRange {
	return protocol.FoldingRange{
		StartLine: uint32(q.Pos.Line - 1),    //nolint:gosec
		EndLine:   uint32(q.EndPos.Line - 1), //nolint:gosec
		Kind:      protocol.RegionFoldingRange,
	}
}

// setupFoldingRange creates a folding range for a setup clause.
func (s *Server) setupFoldingRange(setup *scaf.SetupClause) protocol.FoldingRange {
	return protocol.FoldingRange{
		StartLine: uint32(setup.Pos.Line - 1),    //nolint:gosec
		EndLine:   uint32(setup.EndPos.Line - 1), //nolint:gosec
		Kind:      protocol.RegionFoldingRange,
	}
}

// scopeFoldingRanges creates folding ranges for a query scope and its contents.
func (s *Server) scopeFoldingRanges(scope *scaf.QueryScope) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the scope itself
	ranges = append(ranges, protocol.FoldingRange{
		StartLine: uint32(scope.Pos.Line - 1),    //nolint:gosec
		EndLine:   uint32(scope.EndPos.Line - 1), //nolint:gosec
		Kind:      protocol.RegionFoldingRange,
	})

	// Add range for scope setup if present
	if scope.Setup != nil {
		ranges = append(ranges, s.setupFoldingRange(scope.Setup))
	}

	// Add ranges for items (tests and groups)
	for _, item := range scope.Items {
		ranges = append(ranges, s.itemFoldingRanges(item)...)
	}

	return ranges
}

// itemFoldingRanges creates folding ranges for a test or group.
func (s *Server) itemFoldingRanges(item *scaf.TestOrGroup) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	if item.Test != nil {
		ranges = append(ranges, s.testFoldingRanges(item.Test)...)
	}

	if item.Group != nil {
		ranges = append(ranges, s.groupFoldingRanges(item.Group)...)
	}

	return ranges
}

// testFoldingRanges creates folding ranges for a test.
func (s *Server) testFoldingRanges(test *scaf.Test) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the test itself
	ranges = append(ranges, protocol.FoldingRange{
		StartLine: uint32(test.Pos.Line - 1),    //nolint:gosec
		EndLine:   uint32(test.EndPos.Line - 1), //nolint:gosec
		Kind:      protocol.RegionFoldingRange,
	})

	// Add range for test setup if present
	if test.Setup != nil {
		ranges = append(ranges, s.setupFoldingRange(test.Setup))
	}

	// Add ranges for asserts
	for _, assert := range test.Asserts {
		if assert.EndPos.Line > assert.Pos.Line {
			ranges = append(ranges, protocol.FoldingRange{
				StartLine: uint32(assert.Pos.Line - 1),    //nolint:gosec
				EndLine:   uint32(assert.EndPos.Line - 1), //nolint:gosec
				Kind:      protocol.RegionFoldingRange,
			})
		}
	}

	return ranges
}

// groupFoldingRanges creates folding ranges for a group and its contents.
func (s *Server) groupFoldingRanges(group *scaf.Group) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the group itself
	ranges = append(ranges, protocol.FoldingRange{
		StartLine: uint32(group.Pos.Line - 1),    //nolint:gosec
		EndLine:   uint32(group.EndPos.Line - 1), //nolint:gosec
		Kind:      protocol.RegionFoldingRange,
	})

	// Add range for group setup if present
	if group.Setup != nil {
		ranges = append(ranges, s.setupFoldingRange(group.Setup))
	}

	// Add ranges for nested items
	for _, item := range group.Items {
		ranges = append(ranges, s.itemFoldingRanges(item)...)
	}

	return ranges
}
