package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/analysis"
)

// SignatureHelp handles textDocument/signatureHelp requests.
// Shows parameter hints when typing setup calls like fixtures.CreateUser(.
func (s *Server) SignatureHelp(_ context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	s.logger.Debug("SignatureHelp",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Get line text to analyze
	lines := strings.Split(doc.Content, "\n")
	if int(params.Position.Line) >= len(lines) {
		return nil, nil //nolint:nilnil
	}
	lineText := lines[params.Position.Line]
	col := int(params.Position.Character)
	if col > len(lineText) {
		col = len(lineText)
	}
	textBeforeCursor := lineText[:col]

	// Check if we're inside a setup call's parameter list
	callInfo := s.parseSetupCall(textBeforeCursor)
	if callInfo == nil {
		return nil, nil //nolint:nilnil
	}

	// Get the analysis for symbol lookup
	af := doc.Analysis
	if af == nil {
		return nil, nil //nolint:nilnil
	}
	if af.ParseError != nil && doc.LastValidAnalysis != nil {
		af = doc.LastValidAnalysis
	}

	// Look up the import to find the module
	imp, ok := af.Symbols.Imports[callInfo.module]
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Resolve and load the imported file
	docPath := URIToPath(params.TextDocument.URI)
	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)
	importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
	if err != nil || importedFile.Symbols == nil {
		return nil, nil //nolint:nilnil
	}

	// Find the query in the imported file
	query, ok := importedFile.Symbols.Queries[callInfo.query]
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Build signature information
	sig := s.buildSignatureInfo(callInfo.module, query, callInfo.activeParam)

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{sig},
		ActiveSignature: 0,
		ActiveParameter: uint32(callInfo.activeParam), //nolint:gosec
	}, nil
}

// setupCallInfo holds parsed information about a setup call being typed.
type setupCallInfo struct {
	module      string
	query       string
	activeParam int
}

// parseSetupCall parses text to extract setup call information.
// Returns nil if not inside a setup call.
func (s *Server) parseSetupCall(text string) *setupCallInfo {
	// Look for pattern: module.Query(
	// We need to find the opening paren and count commas to determine active param

	// Find the last opening paren that's part of a setup call
	parenIdx := strings.LastIndex(text, "(")
	if parenIdx < 0 {
		return nil
	}

	// Find the module.query before the paren
	beforeParen := strings.TrimSpace(text[:parenIdx])

	// Check if this looks like a setup call (module.Query pattern)
	dotIdx := strings.LastIndex(beforeParen, ".")
	if dotIdx < 0 {
		return nil
	}

	// Extract module and query names
	// Module could be preceded by "setup " or whitespace
	moduleStart := dotIdx
	for moduleStart > 0 && (isIdentChar(rune(beforeParen[moduleStart-1]))) {
		moduleStart--
	}
	module := beforeParen[moduleStart:dotIdx]
	query := beforeParen[dotIdx+1:]

	if module == "" || query == "" {
		return nil
	}

	// Check if there's "setup" before the module (allowing for whitespace)
	prefix := strings.TrimSpace(beforeParen[:moduleStart])
	if !strings.HasSuffix(prefix, "setup") && prefix != "" {
		// Could also be inside a setup block without the keyword prefix
		// Allow it if we're clearly in a function call pattern
		if !isIdentChar(rune(prefix[len(prefix)-1])) {
			return nil
		}
	}

	// Count commas after the opening paren to determine active parameter
	afterParen := text[parenIdx+1:]
	activeParam := 0
	depth := 0
	for _, ch := range afterParen {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil // Past the closing paren
			}
		case ',':
			if depth == 0 {
				activeParam++
			}
		}
	}

	return &setupCallInfo{
		module:      module,
		query:       query,
		activeParam: activeParam,
	}
}

// isIdentChar returns true if the character can be part of an identifier.
func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// buildSignatureInfo creates a SignatureInformation for a query.
func (s *Server) buildSignatureInfo(module string, query *analysis.QuerySymbol, activeParam int) protocol.SignatureInformation {
	// Build parameter list
	var params []protocol.ParameterInformation
	var paramLabels []string

	for _, p := range query.Params {
		paramLabel := "$" + p
		paramLabels = append(paramLabels, paramLabel)
		params = append(params, protocol.ParameterInformation{
			Label: paramLabel,
		})
	}

	// Try to get more detailed parameter info from query analyzer
	if s.queryAnalyzer != nil && query.Body != "" {
		metadata, err := s.queryAnalyzer.AnalyzeQuery(query.Body)
		if err == nil {
			// Rebuild with type information if available
			params = nil
			paramLabels = nil
			for _, p := range metadata.Parameters {
				paramLabel := "$" + p.Name
				if p.Type != "" {
					paramLabel += ": " + p.Type
				}
				paramLabels = append(paramLabels, paramLabel)
				params = append(params, protocol.ParameterInformation{
					Label: "$" + p.Name,
				})
			}
		}
	}

	// Build label: module.Query($param1, $param2)
	label := module + "." + query.Name + "(" + strings.Join(paramLabels, ", ") + ")"

	// Build documentation with query body preview
	var doc *protocol.MarkupContent
	if query.Body != "" {
		preview := strings.TrimSpace(query.Body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		doc = &protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "```cypher\n" + preview + "\n```",
		}
	}

	return protocol.SignatureInformation{
		Label:         label,
		Documentation: doc,
		Parameters:    params,
	}
}
