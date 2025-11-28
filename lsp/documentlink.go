package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
)

// DocumentLink handles textDocument/documentLink requests.
// Returns links for import paths that can be clicked to open the imported file.
func (s *Server) DocumentLink(_ context.Context, params *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	s.logger.Debug("DocumentLink",
		zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	var links []protocol.DocumentLink

	for _, imp := range doc.Analysis.Suite.Imports {
		// Calculate the range for just the path string (excluding quotes)
		// Import format: import [alias] "path"
		// We want to link the path part
		pathRange := s.importPathRange(imp)

		// Resolve the import path to a file URI
		docPath := URIToPath(params.TextDocument.URI)
		resolvedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)
		targetURI := PathToURI(resolvedPath)

		links = append(links, protocol.DocumentLink{
			Range:   pathRange,
			Target:  targetURI,
			Tooltip: "Open " + imp.Path,
		})
	}

	return links, nil
}

// importPathRange calculates the range for just the path string in an import.
// The import format is: import [alias] "path"
// We want to return the range of "path" (including quotes for click target).
func (s *Server) importPathRange(imp *scaf.Import) protocol.Range {
	// The path is the last part of the import statement
	// Import.Pos points to "import", and the path is at the end
	// We need to find where the quoted path starts

	// Calculate based on import structure:
	// "import " = 7 chars
	// If alias present: "alias " = len(alias) + 1
	// Then the path starts with quote

	startCol := imp.Pos.Column + 7 // After "import "
	if imp.Alias != nil {
		startCol += len(*imp.Alias) + 1 // After "alias "
	}

	// The path includes quotes in the source, so we want to link the whole thing
	// Path length + 2 for quotes
	pathLen := len(imp.Path) + 2

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(imp.Pos.Line - 1), //nolint:gosec
			Character: uint32(startCol - 1),     //nolint:gosec
		},
		End: protocol.Position{
			Line:      uint32(imp.Pos.Line - 1),       //nolint:gosec
			Character: uint32(startCol - 1 + pathLen), //nolint:gosec
		},
	}
}
