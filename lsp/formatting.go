package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
)

// Formatting handles textDocument/formatting requests.
func (s *Server) Formatting(_ context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	s.logger.Debug("Formatting", zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	// Need a valid parse to format (no parse errors)
	if doc.Analysis == nil || doc.Analysis.Suite == nil || doc.Analysis.ParseError != nil {
		return nil, nil
	}

	// Use the existing formatter
	formatted := scaf.Format(doc.Analysis.Suite)

	// If no change, return empty edits
	if formatted == doc.Content {
		return []protocol.TextEdit{}, nil
	}

	// Return a single edit that replaces the entire document
	lines := strings.Count(doc.Content, "\n")
	lastLineLen := len(doc.Content) - strings.LastIndex(doc.Content, "\n") - 1
	if lastLineLen < 0 {
		lastLineLen = len(doc.Content)
	}

	return []protocol.TextEdit{
		{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: uint32(lines), Character: uint32(lastLineLen)}, //nolint:gosec
			},
			NewText: formatted,
		},
	}, nil
}
