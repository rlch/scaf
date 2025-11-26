package lsp

import (
	"go.lsp.dev/protocol"

	"github.com/rlch/scaf"
)

// spanToRange converts a scaf.Span to an LSP protocol.Range.
// scaf uses 1-based line/column, LSP uses 0-based.
func spanToRange(span scaf.Span) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(max(0, span.Start.Line-1)),   //nolint:gosec // G115: values are small line numbers
			Character: uint32(max(0, span.Start.Column-1)), //nolint:gosec // G115: values are small column numbers
		},
		End: protocol.Position{
			Line:      uint32(max(0, span.End.Line-1)),   //nolint:gosec // G115: values are small line numbers
			Character: uint32(max(0, span.End.Column-1)), //nolint:gosec // G115: values are small column numbers
		},
	}
}
