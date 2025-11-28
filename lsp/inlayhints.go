package lsp

// InlayHints support requires LSP 3.17+ protocol types which are not available
// in go.lsp.dev/protocol v0.12.0.
//
// To enable inlay hints, upgrade to a newer version of go.lsp.dev/protocol
// that includes protocol.InlayHint, protocol.InlayHintParams, etc.
//
// When available, implement:
// - func (s *Server) InlayHint(ctx, params) ([]protocol.InlayHint, error)
// - func (s *Server) InlayHintResolve(ctx, hint) (*protocol.InlayHint, error)
// - func (s *Server) InlayHintRefresh(ctx) error
//
// Inlay hints would show:
// - Parameter types in setup calls (e.g., fixtures.CreateUser($name: "Alice" : string))
// - Return field types in test assertions (e.g., u.name: "Alice" : string)
