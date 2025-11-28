// Package lsp implements a Language Server Protocol server for the scaf DSL.
package lsp

import (
	"context"
	"sync"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// Server implements the LSP Server interface for scaf.
type Server struct {
	client protocol.Client
	logger *zap.Logger

	// Document state
	mu        sync.RWMutex
	documents map[protocol.DocumentURI]*Document

	// Analyzer for semantic analysis
	analyzer *analysis.Analyzer

	// FileLoader for cross-file analysis (imports)
	fileLoader *LSPFileLoader

	// Query analysis for dialect-specific completions
	dialectName   string              // e.g., "cypher", "sql"
	queryAnalyzer scaf.QueryAnalyzer  // dialect-specific query analyzer

	// Server state
	initialized   bool
	shutdown      bool
	workspaceRoot string
}

// Document represents an open document in the server.
type Document struct {
	URI      protocol.DocumentURI
	Version  int32
	Content  string
	Analysis *analysis.AnalyzedFile

	// LastValidAnalysis holds the most recent analysis that parsed successfully.
	// Used for completion when the current document has parse errors.
	LastValidAnalysis *analysis.AnalyzedFile
}

// NewServer creates a new LSP server.
// dialectName specifies the query dialect (e.g., "cypher", "sql") for completion/hover.
// If empty, defaults to "cypher".
func NewServer(client protocol.Client, logger *zap.Logger, dialectName string) *Server {
	fileLoader := NewLSPFileLoader(logger, "")
	resolver := NewLSPCrossFileResolver(fileLoader)

	// Default to cypher if not specified
	if dialectName == "" {
		dialectName = "cypher"
	}

	// Get the query analyzer for this dialect
	queryAnalyzer := scaf.GetAnalyzer(dialectName)
	if queryAnalyzer == nil {
		logger.Warn("No query analyzer registered for dialect",
			zap.String("dialect", dialectName),
			zap.Strings("available", scaf.RegisteredAnalyzers()))
	}

	return &Server{
		client:        client,
		logger:        logger,
		documents:     make(map[protocol.DocumentURI]*Document),
		analyzer:      analysis.NewAnalyzerWithResolver(fileLoader, resolver),
		fileLoader:    fileLoader,
		dialectName:   dialectName,
		queryAnalyzer: queryAnalyzer,
	}
}

// Initialize handles the initialize request.
func (s *Server) Initialize(_ context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.logger.Info("Initialize", zap.Any("params", params))

	// Extract workspace root from params
	if params.RootURI != "" {
		s.workspaceRoot = URIToPath(params.RootURI)
		s.fileLoader.SetWorkspaceRoot(s.workspaceRoot)
		s.logger.Info("Workspace root", zap.String("root", s.workspaceRoot))
	} else if params.RootPath != "" {
		s.workspaceRoot = params.RootPath
		s.fileLoader.SetWorkspaceRoot(s.workspaceRoot)
		s.logger.Info("Workspace root (from RootPath)", zap.String("root", s.workspaceRoot))
	}

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			// Full document sync - client sends entire content on change
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
			},
			// Hover support
			HoverProvider: true,
			// Go to definition
			DefinitionProvider: true,
			// Completion support
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"$", "."},
				ResolveProvider:   false,
			},
			// Document symbol support for outline view
			DocumentSymbolProvider: true,
			// Document highlight support
			DocumentHighlightProvider: true,
			// Find all references
			ReferencesProvider: true,
			// Rename support
			RenameProvider: &protocol.RenameOptions{
				PrepareProvider: true,
			},
			// Code actions (quick fixes)
			CodeActionProvider: &protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{
					protocol.QuickFix,
				},
			},
			// Document links (clickable import paths)
			DocumentLinkProvider: &protocol.DocumentLinkOptions{
				ResolveProvider: false,
			},
			// Workspace symbol search
			WorkspaceSymbolProvider: true,
			// Folding ranges for code folding
			FoldingRangeProvider: true,
			// Signature help for setup calls
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ","},
				RetriggerCharacters: []string{","},
			},
			// Document formatting
			DocumentFormattingProvider: true,
			// Code lens for running tests
			CodeLensProvider: &protocol.CodeLensOptions{
				ResolveProvider: false,
			},
			// Note: InlayHintProvider requires LSP 3.17+ protocol types
			// not available in go.lsp.dev/protocol v0.12.0
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "scaf-lsp",
			Version: "0.1.0",
		},
	}, nil
}

// Initialized handles the initialized notification.
func (s *Server) Initialized(_ context.Context, _ *protocol.InitializedParams) error {
	s.logger.Info("Initialized")
	s.initialized = true

	return nil
}

// Shutdown handles the shutdown request.
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("Shutdown")
	s.shutdown = true

	return nil
}

// Exit handles the exit notification.
func (s *Server) Exit(_ context.Context) error {
	s.logger.Info("Exit")
	// The main loop should handle exiting after this
	return nil
}

// DidOpen handles textDocument/didOpen notifications.
func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.logger.Info("DidOpen", zap.String("uri", string(params.TextDocument.URI)))

	s.mu.Lock()
	defer s.mu.Unlock()

	doc := &Document{
		URI:     params.TextDocument.URI,
		Version: params.TextDocument.Version,
		Content: params.TextDocument.Text,
	}

	// Analyze the document
	// Use the file system path (not URI) for proper import resolution
	docPath := URIToPath(params.TextDocument.URI)
	doc.Analysis = s.analyzer.Analyze(docPath, []byte(params.TextDocument.Text))

	// If parsing succeeded, save as last valid analysis for completion fallback
	if doc.Analysis.ParseError == nil {
		doc.LastValidAnalysis = doc.Analysis
	}

	s.documents[params.TextDocument.URI] = doc

	// Publish diagnostics
	s.publishDiagnostics(ctx, doc)

	return nil
}

// DidChange handles textDocument/didChange notifications.
func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.logger.Info("DidChange",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Int32("version", params.TextDocument.Version))

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		s.logger.Warn("DidChange for unknown document", zap.String("uri", string(params.TextDocument.URI)))

		return nil
	}

	// Full sync - take the last content change (should only be one with full sync)
	if len(params.ContentChanges) > 0 {
		doc.Content = params.ContentChanges[len(params.ContentChanges)-1].Text
		doc.Version = params.TextDocument.Version

		// Re-analyze (use file system path for proper import resolution)
		docPath := URIToPath(params.TextDocument.URI)
		doc.Analysis = s.analyzer.Analyze(docPath, []byte(doc.Content))

		// If parsing succeeded, save as last valid analysis for completion fallback
		if doc.Analysis.ParseError == nil {
			doc.LastValidAnalysis = doc.Analysis
		}

		// Publish diagnostics
		s.publishDiagnostics(ctx, doc)
	}

	return nil
}

// DidClose handles textDocument/didClose notifications.
func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.logger.Info("DidClose", zap.String("uri", string(params.TextDocument.URI)))

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.documents, params.TextDocument.URI)

	// Clear diagnostics for closed document
	err := s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	if err != nil {
		s.logger.Error("Failed to clear diagnostics", zap.Error(err))
	}

	return nil
}

// DidSave handles textDocument/didSave notifications.
func (s *Server) DidSave(_ context.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.logger.Info("DidSave", zap.String("uri", string(params.TextDocument.URI)))
	// Could trigger additional validation here if needed
	return nil
}

// getDocument returns a document by URI (read-locked).
func (s *Server) getDocument(uri protocol.DocumentURI) (*Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.documents[uri]

	return doc, ok
}
