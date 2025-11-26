package lsp

// This file contains stub implementations for LSP methods we haven't implemented yet.
// All return nil/empty to satisfy the protocol.Server interface.

import (
	"context"

	"go.lsp.dev/protocol"
)

// Lifecycle methods are in server.go

// WorkDoneProgressCancel handles window/workDoneProgress/cancel.
func (s *Server) WorkDoneProgressCancel(_ context.Context, _ *protocol.WorkDoneProgressCancelParams) error {
	return nil
}

// LogTrace handles $/logTrace.
func (s *Server) LogTrace(_ context.Context, _ *protocol.LogTraceParams) error {
	return nil
}

// SetTrace handles $/setTrace.
func (s *Server) SetTrace(_ context.Context, _ *protocol.SetTraceParams) error {
	return nil
}

// CodeAction handles textDocument/codeAction.
func (s *Server) CodeAction(_ context.Context, _ *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	return nil, nil
}

// CodeLens handles textDocument/codeLens.
func (s *Server) CodeLens(_ context.Context, _ *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	return nil, nil
}

// CodeLensResolve handles codeLens/resolve.
func (s *Server) CodeLensResolve(_ context.Context, _ *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// ColorPresentation handles textDocument/colorPresentation.
func (s *Server) ColorPresentation(_ context.Context, _ *protocol.ColorPresentationParams) ([]protocol.ColorPresentation, error) {
	return nil, nil
}

// Completion is implemented in completion.go

// CompletionResolve handles completionItem/resolve.
func (s *Server) CompletionResolve(_ context.Context, _ *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// Declaration handles textDocument/declaration.
func (s *Server) Declaration(_ context.Context, _ *protocol.DeclarationParams) ([]protocol.Location, error) {
	return nil, nil
}

// Definition is implemented in definition.go

// DidChangeConfiguration handles workspace/didChangeConfiguration.
func (s *Server) DidChangeConfiguration(_ context.Context, _ *protocol.DidChangeConfigurationParams) error {
	return nil
}

// DidChangeWatchedFiles handles workspace/didChangeWatchedFiles.
func (s *Server) DidChangeWatchedFiles(_ context.Context, _ *protocol.DidChangeWatchedFilesParams) error {
	return nil
}

// DidChangeWorkspaceFolders handles workspace/didChangeWorkspaceFolders.
func (s *Server) DidChangeWorkspaceFolders(_ context.Context, _ *protocol.DidChangeWorkspaceFoldersParams) error {
	return nil
}

// DocumentColor handles textDocument/documentColor.
func (s *Server) DocumentColor(_ context.Context, _ *protocol.DocumentColorParams) ([]protocol.ColorInformation, error) {
	return nil, nil
}

// DocumentHighlight handles textDocument/documentHighlight.
func (s *Server) DocumentHighlight(_ context.Context, _ *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	return nil, nil
}

// DocumentLink handles textDocument/documentLink.
func (s *Server) DocumentLink(_ context.Context, _ *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	return nil, nil
}

// DocumentLinkResolve handles documentLink/resolve.
func (s *Server) DocumentLinkResolve(_ context.Context, _ *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// DocumentSymbol handles textDocument/documentSymbol.
func (s *Server) DocumentSymbol(_ context.Context, _ *protocol.DocumentSymbolParams) ([]any, error) {
	return nil, nil
}

// ExecuteCommand handles workspace/executeCommand.
func (s *Server) ExecuteCommand(_ context.Context, _ *protocol.ExecuteCommandParams) (any, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// FoldingRanges handles textDocument/foldingRange.
func (s *Server) FoldingRanges(_ context.Context, _ *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	return nil, nil
}

// Formatting handles textDocument/formatting.
func (s *Server) Formatting(_ context.Context, _ *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}

// Implementation handles textDocument/implementation.
func (s *Server) Implementation(_ context.Context, _ *protocol.ImplementationParams) ([]protocol.Location, error) {
	return nil, nil
}

// OnTypeFormatting handles textDocument/onTypeFormatting.
func (s *Server) OnTypeFormatting(_ context.Context, _ *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}

// PrepareRename handles textDocument/prepareRename.
func (s *Server) PrepareRename(_ context.Context, _ *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// RangeFormatting handles textDocument/rangeFormatting.
func (s *Server) RangeFormatting(_ context.Context, _ *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}

// References handles textDocument/references.
func (s *Server) References(_ context.Context, _ *protocol.ReferenceParams) ([]protocol.Location, error) {
	return nil, nil
}

// Rename handles textDocument/rename.
func (s *Server) Rename(_ context.Context, _ *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// SignatureHelp handles textDocument/signatureHelp.
func (s *Server) SignatureHelp(_ context.Context, _ *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// Symbols handles workspace/symbol.
func (s *Server) Symbols(_ context.Context, _ *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	return nil, nil
}

// TypeDefinition handles textDocument/typeDefinition.
func (s *Server) TypeDefinition(_ context.Context, _ *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	return nil, nil
}

// WillSave handles textDocument/willSave.
func (s *Server) WillSave(_ context.Context, _ *protocol.WillSaveTextDocumentParams) error {
	return nil
}

// WillSaveWaitUntil handles textDocument/willSaveWaitUntil.
func (s *Server) WillSaveWaitUntil(_ context.Context, _ *protocol.WillSaveTextDocumentParams) ([]protocol.TextEdit, error) {
	return nil, nil
}

// ShowDocument handles window/showDocument.
func (s *Server) ShowDocument(_ context.Context, _ *protocol.ShowDocumentParams) (*protocol.ShowDocumentResult, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// WillCreateFiles handles workspace/willCreateFiles.
func (s *Server) WillCreateFiles(_ context.Context, _ *protocol.CreateFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// DidCreateFiles handles workspace/didCreateFiles.
func (s *Server) DidCreateFiles(_ context.Context, _ *protocol.CreateFilesParams) error {
	return nil
}

// WillRenameFiles handles workspace/willRenameFiles.
func (s *Server) WillRenameFiles(_ context.Context, _ *protocol.RenameFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// DidRenameFiles handles workspace/didRenameFiles.
func (s *Server) DidRenameFiles(_ context.Context, _ *protocol.RenameFilesParams) error {
	return nil
}

// WillDeleteFiles handles workspace/willDeleteFiles.
func (s *Server) WillDeleteFiles(_ context.Context, _ *protocol.DeleteFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// DidDeleteFiles handles workspace/didDeleteFiles.
func (s *Server) DidDeleteFiles(_ context.Context, _ *protocol.DeleteFilesParams) error {
	return nil
}

// CodeLensRefresh handles workspace/codeLens/refresh.
func (s *Server) CodeLensRefresh(_ context.Context) error {
	return nil
}

// PrepareCallHierarchy handles textDocument/prepareCallHierarchy.
func (s *Server) PrepareCallHierarchy(_ context.Context, _ *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return nil, nil
}

// IncomingCalls handles callHierarchy/incomingCalls.
func (s *Server) IncomingCalls(_ context.Context, _ *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, nil
}

// OutgoingCalls handles callHierarchy/outgoingCalls.
func (s *Server) OutgoingCalls(_ context.Context, _ *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, nil
}

// SemanticTokensFull handles textDocument/semanticTokens/full.
func (s *Server) SemanticTokensFull(_ context.Context, _ *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// SemanticTokensFullDelta handles textDocument/semanticTokens/full/delta.
func (s *Server) SemanticTokensFullDelta(_ context.Context, _ *protocol.SemanticTokensDeltaParams) (any, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// SemanticTokensRange handles textDocument/semanticTokens/range.
func (s *Server) SemanticTokensRange(_ context.Context, _ *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// SemanticTokensRefresh handles workspace/semanticTokens/refresh.
func (s *Server) SemanticTokensRefresh(_ context.Context) error {
	return nil
}

// LinkedEditingRange handles textDocument/linkedEditingRange.
func (s *Server) LinkedEditingRange(_ context.Context, _ *protocol.LinkedEditingRangeParams) (*protocol.LinkedEditingRanges, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}

// Moniker handles textDocument/moniker.
func (s *Server) Moniker(_ context.Context, _ *protocol.MonikerParams) ([]protocol.Moniker, error) {
	return nil, nil
}

// Request handles custom requests.
func (s *Server) Request(_ context.Context, _ string, _ any) (any, error) {
	return nil, nil //nolint:nilnil // LSP stub returns nil for unimplemented features
}
