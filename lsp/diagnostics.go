package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/analysis"
)

// publishDiagnostics converts analysis diagnostics to LSP format and publishes them.
func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	if doc.Analysis == nil {
		return
	}

	diagnostics := make([]protocol.Diagnostic, 0, len(doc.Analysis.Diagnostics))

	for _, d := range doc.Analysis.Diagnostics {
		lspDiag := convertDiagnostic(d)
		s.logger.Debug("Publishing diagnostic",
			zap.Int("span.start.line", d.Span.Start.Line),
			zap.Int("span.start.col", d.Span.Start.Column),
			zap.Uint32("lsp.start.line", lspDiag.Range.Start.Line),
			zap.Uint32("lsp.start.char", lspDiag.Range.Start.Character),
			zap.String("message", d.Message))
		diagnostics = append(diagnostics, lspDiag)
	}

	err := s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Version:     uint32(doc.Version), //nolint:gosec // LSP version numbers are always non-negative
		Diagnostics: diagnostics,
	})
	if err != nil {
		s.logger.Error("Failed to publish diagnostics", zap.Error(err))
	}
}

// convertDiagnostic converts an analysis.Diagnostic to an LSP protocol.Diagnostic.
func convertDiagnostic(d analysis.Diagnostic) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    spanToRange(d.Span),
		Severity: convertSeverity(d.Severity),
		Code:     d.Code,
		Source:   d.Source,
		Message:  d.Message,
	}
}

// convertSeverity converts analysis severity to LSP severity.
func convertSeverity(sev analysis.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {
	case analysis.SeverityError:
		return protocol.DiagnosticSeverityError
	case analysis.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	case analysis.SeverityInformation:
		return protocol.DiagnosticSeverityInformation
	case analysis.SeverityHint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityError
	}
}
