package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// RenameKind identifies what kind of symbol is being renamed.
type RenameKind int

const (
	RenameKindNone RenameKind = iota
	RenameKindQuery
	RenameKindImport
	RenameKindParameter
	RenameKindReturnField
)

// RenameContext holds information about a rename operation.
type RenameContext struct {
	Kind       RenameKind
	OldName    string
	QueryScope string // For parameters and return fields
	ModuleAlias string // For cross-file query references
}

// PrepareRename handles textDocument/prepareRename requests.
// Validates that rename is possible and returns the range of the symbol to rename.
func (s *Server) PrepareRename(_ context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
	s.logger.Debug("PrepareRename",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil //nolint:nilnil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	ctx := s.getRenameContext(doc, tokenCtx)
	if ctx.Kind == RenameKindNone {
		return nil, nil //nolint:nilnil
	}

	// Return the range of the symbol being renamed
	rng := s.getRenameRange(doc, tokenCtx, ctx)
	return rng, nil
}

// Rename handles textDocument/rename requests.
// Renames the symbol under the cursor and all its references.
func (s *Server) Rename(_ context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	s.logger.Debug("Rename",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character),
		zap.String("newName", params.NewName))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil //nolint:nilnil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	ctx := s.getRenameContext(doc, tokenCtx)
	if ctx.Kind == RenameKindNone {
		return nil, nil //nolint:nilnil
	}

	// Validate new name
	if err := s.validateNewName(params.NewName, ctx); err != nil {
		return nil, err
	}

	// Check for conflicts
	if err := s.checkRenameConflicts(doc, params.NewName, ctx); err != nil {
		return nil, err
	}

	// Generate edits
	edits := s.generateRenameEdits(doc, params.NewName, ctx)
	if len(edits) == 0 {
		return nil, nil //nolint:nilnil
	}

	return &protocol.WorkspaceEdit{
		Changes: edits,
	}, nil
}

// getRenameContext determines what kind of rename operation this is.
func (s *Server) getRenameContext(doc *Document, tokenCtx *analysis.TokenContext) RenameContext {
	ctx := RenameContext{Kind: RenameKindNone}

	switch node := tokenCtx.Node.(type) {
	case *scaf.Query:
		ctx.Kind = RenameKindQuery
		ctx.OldName = node.Name

	case *scaf.QueryScope:
		ctx.Kind = RenameKindQuery
		ctx.OldName = node.QueryName

	case *scaf.Import:
		ctx.Kind = RenameKindImport
		ctx.OldName = baseNameFromImport(node)

	case *scaf.SetupCall:
		if tokenCtx.Token != nil {
			if tokenCtx.Token.Value == node.Module {
				ctx.Kind = RenameKindImport
				ctx.OldName = node.Module
			}
			// Note: Renaming the query name in a setup call would require
			// cross-file rename, which we don't support yet
		}

	case *scaf.SetupClause:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			ctx.Kind = RenameKindImport
			ctx.OldName = *node.Module
		}

	case *scaf.SetupItem:
		if node.Module != nil && tokenCtx.Token != nil && tokenCtx.Token.Value == *node.Module {
			ctx.Kind = RenameKindImport
			ctx.OldName = *node.Module
		}

	case *scaf.AssertQuery:
		if node.QueryName != nil {
			ctx.Kind = RenameKindQuery
			ctx.OldName = *node.QueryName
		}

	case *scaf.Statement:
		if node.KeyParts != nil {
			key := node.Key()
			if len(key) > 0 && key[0] == '$' {
				ctx.Kind = RenameKindParameter
				ctx.OldName = key
				ctx.QueryScope = tokenCtx.QueryScope
			} else if len(key) > 0 {
				ctx.Kind = RenameKindReturnField
				ctx.OldName = key
				ctx.QueryScope = tokenCtx.QueryScope
			}
		}
	}

	return ctx
}

// getRenameRange returns the range of the symbol to rename.
func (s *Server) getRenameRange(doc *Document, tokenCtx *analysis.TokenContext, ctx RenameContext) *protocol.Range {
	switch node := tokenCtx.Node.(type) {
	case *scaf.Query:
		rng := queryNameRange(node)
		return &rng

	case *scaf.QueryScope:
		rng := scopeNameRange(node)
		return &rng

	case *scaf.Import:
		rng := importAliasRange(node)
		return &rng

	case *scaf.SetupCall:
		if tokenCtx.Token != nil && tokenCtx.Token.Value == node.Module {
			rng := setupCallModuleRange(node)
			return &rng
		}

	case *scaf.SetupClause:
		if node.Module != nil {
			rng := setupModuleRange(node)
			return &rng
		}

	case *scaf.SetupItem:
		if node.Module != nil {
			rng := setupItemModuleRange(node)
			return &rng
		}

	case *scaf.AssertQuery:
		if node.QueryName != nil {
			rng := assertQueryNameRange(node)
			return &rng
		}

	case *scaf.Statement:
		rng := statementKeyRange(node)
		return &rng
	}

	return nil
}

// validateNewName validates that the new name is valid for the symbol type.
func (s *Server) validateNewName(newName string, ctx RenameContext) error {
	if newName == "" {
		return fmt.Errorf("new name cannot be empty")
	}

	// Check for valid identifier characters
	for i, r := range newName {
		if i == 0 {
			// Parameters start with $
			if ctx.Kind == RenameKindParameter {
				if r != '$' {
					return fmt.Errorf("parameter name must start with $")
				}
				continue
			}
			// First char must be letter or underscore
			if !isLetter(r) && r != '_' {
				return fmt.Errorf("name must start with a letter or underscore")
			}
		} else {
			// For parameters, skip the $ when validating
			if ctx.Kind == RenameKindParameter && i == 1 {
				if !isLetter(r) && r != '_' {
					return fmt.Errorf("parameter name must start with a letter or underscore after $")
				}
				continue
			}
			// Subsequent chars can be letter, digit, or underscore
			if !isLetter(r) && !isDigit(r) && r != '_' && r != '.' {
				return fmt.Errorf("name can only contain letters, digits, underscores, and dots")
			}
		}
	}

	return nil
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// checkRenameConflicts checks if the new name conflicts with existing symbols.
func (s *Server) checkRenameConflicts(doc *Document, newName string, ctx RenameContext) error {
	switch ctx.Kind {
	case RenameKindQuery:
		// Check if query name already exists
		if _, exists := doc.Analysis.Symbols.Queries[newName]; exists {
			return fmt.Errorf("query %q already exists", newName)
		}

	case RenameKindImport:
		// Check if import alias already exists
		if _, exists := doc.Analysis.Symbols.Imports[newName]; exists {
			return fmt.Errorf("import alias %q already exists", newName)
		}

	case RenameKindParameter:
		// Parameters are scoped to tests, conflicts are less of an issue
		// but we could check if the new param name exists in the query

	case RenameKindReturnField:
		// Return fields are from query, can't really conflict
	}

	return nil
}

// generateRenameEdits generates all the text edits needed to rename a symbol.
func (s *Server) generateRenameEdits(doc *Document, newName string, ctx RenameContext) map[protocol.DocumentURI][]protocol.TextEdit {
	edits := make(map[protocol.DocumentURI][]protocol.TextEdit)

	switch ctx.Kind {
	case RenameKindQuery:
		s.generateQueryRenameEdits(doc, ctx.OldName, newName, edits)

	case RenameKindImport:
		s.generateImportRenameEdits(doc, ctx.OldName, newName, edits)

	case RenameKindParameter:
		s.generateParameterRenameEdits(doc, ctx.QueryScope, ctx.OldName, newName, edits)

	case RenameKindReturnField:
		s.generateReturnFieldRenameEdits(doc, ctx.QueryScope, ctx.OldName, newName, edits)
	}

	return edits
}

// generateQueryRenameEdits generates edits to rename a query.
func (s *Server) generateQueryRenameEdits(doc *Document, oldName, newName string, edits map[protocol.DocumentURI][]protocol.TextEdit) {
	if doc.Analysis.Suite == nil {
		return
	}

	var docEdits []protocol.TextEdit

	// Rename the query definition
	for _, q := range doc.Analysis.Suite.Queries {
		if q.Name == oldName {
			docEdits = append(docEdits, protocol.TextEdit{
				Range:   queryNameRange(q),
				NewText: newName,
			})
			break
		}
	}

	// Rename all query scope references
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName == oldName {
			docEdits = append(docEdits, protocol.TextEdit{
				Range:   scopeNameRange(scope),
				NewText: newName,
			})
		}

		// Rename assert query references
		s.collectAssertQueryEdits(scope.Items, oldName, newName, &docEdits)
	}

	if len(docEdits) > 0 {
		edits[doc.URI] = docEdits
	}
}

// collectAssertQueryEdits recursively collects edits for assert query references.
func (s *Server) collectAssertQueryEdits(items []*scaf.TestOrGroup, oldName, newName string, edits *[]protocol.TextEdit) {
	for _, item := range items {
		if item.Test != nil {
			for _, assert := range item.Test.Asserts {
				if assert.Query != nil && assert.Query.QueryName != nil && *assert.Query.QueryName == oldName {
					*edits = append(*edits, protocol.TextEdit{
						Range:   assertQueryNameRange(assert.Query),
						NewText: newName,
					})
				}
			}
		}
		if item.Group != nil {
			s.collectAssertQueryEdits(item.Group.Items, oldName, newName, edits)
		}
	}
}

// generateImportRenameEdits generates edits to rename an import alias.
func (s *Server) generateImportRenameEdits(doc *Document, oldAlias, newAlias string, edits map[protocol.DocumentURI][]protocol.TextEdit) {
	if doc.Analysis.Suite == nil {
		return
	}

	var docEdits []protocol.TextEdit

	// Rename the import statement
	for _, imp := range doc.Analysis.Suite.Imports {
		if baseNameFromImport(imp) == oldAlias {
			if imp.Alias != nil {
				// Has explicit alias - rename it
				docEdits = append(docEdits, protocol.TextEdit{
					Range:   importAliasRange(imp),
					NewText: newAlias,
				})
			} else {
				// No explicit alias - need to add one
				// Insert "newAlias " before the path
				insertPos := protocol.Position{
					Line:      uint32(imp.Pos.Line - 1),     //nolint:gosec
					Character: uint32(imp.Pos.Column - 1 + 7), //nolint:gosec // After "import "
				}
				docEdits = append(docEdits, protocol.TextEdit{
					Range: protocol.Range{
						Start: insertPos,
						End:   insertPos,
					},
					NewText: newAlias + " ",
				})
			}
			break
		}
	}

	// Rename all usages in setup clauses
	s.collectSetupImportEdits(doc.Analysis.Suite.Setup, oldAlias, newAlias, &docEdits)

	for _, scope := range doc.Analysis.Suite.Scopes {
		s.collectSetupImportEdits(scope.Setup, oldAlias, newAlias, &docEdits)
		s.collectItemSetupImportEdits(scope.Items, oldAlias, newAlias, &docEdits)
	}

	if len(docEdits) > 0 {
		edits[doc.URI] = docEdits
	}
}

// collectSetupImportEdits collects import rename edits in a setup clause.
func (s *Server) collectSetupImportEdits(setup *scaf.SetupClause, oldAlias, newAlias string, edits *[]protocol.TextEdit) {
	if setup == nil {
		return
	}

	if setup.Module != nil && *setup.Module == oldAlias {
		*edits = append(*edits, protocol.TextEdit{
			Range:   setupModuleRange(setup),
			NewText: newAlias,
		})
	}

	if setup.Call != nil && setup.Call.Module == oldAlias {
		*edits = append(*edits, protocol.TextEdit{
			Range:   setupCallModuleRange(setup.Call),
			NewText: newAlias,
		})
	}

	for _, item := range setup.Block {
		if item.Module != nil && *item.Module == oldAlias {
			*edits = append(*edits, protocol.TextEdit{
				Range:   setupItemModuleRange(item),
				NewText: newAlias,
			})
		}
		if item.Call != nil && item.Call.Module == oldAlias {
			*edits = append(*edits, protocol.TextEdit{
				Range:   setupCallModuleRange(item.Call),
				NewText: newAlias,
			})
		}
	}
}

// collectItemSetupImportEdits recursively collects import rename edits.
func (s *Server) collectItemSetupImportEdits(items []*scaf.TestOrGroup, oldAlias, newAlias string, edits *[]protocol.TextEdit) {
	for _, item := range items {
		if item.Test != nil {
			s.collectSetupImportEdits(item.Test.Setup, oldAlias, newAlias, edits)
		}
		if item.Group != nil {
			s.collectSetupImportEdits(item.Group.Setup, oldAlias, newAlias, edits)
			s.collectItemSetupImportEdits(item.Group.Items, oldAlias, newAlias, edits)
		}
	}
}

// generateParameterRenameEdits generates edits to rename a parameter.
func (s *Server) generateParameterRenameEdits(doc *Document, queryScope, oldName, newName string, edits map[protocol.DocumentURI][]protocol.TextEdit) {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return
	}

	var docEdits []protocol.TextEdit

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}
		s.collectParamEdits(scope.Items, oldName, newName, &docEdits)
	}

	if len(docEdits) > 0 {
		edits[doc.URI] = docEdits
	}
}

// collectParamEdits recursively collects parameter rename edits.
func (s *Server) collectParamEdits(items []*scaf.TestOrGroup, oldName, newName string, edits *[]protocol.TextEdit) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				if stmt.Key() == oldName {
					*edits = append(*edits, protocol.TextEdit{
						Range:   statementKeyRange(stmt),
						NewText: newName,
					})
				}
			}
		}
		if item.Group != nil {
			s.collectParamEdits(item.Group.Items, oldName, newName, edits)
		}
	}
}

// generateReturnFieldRenameEdits generates edits to rename a return field.
func (s *Server) generateReturnFieldRenameEdits(doc *Document, queryScope, oldName, newName string, edits map[protocol.DocumentURI][]protocol.TextEdit) {
	if doc.Analysis.Suite == nil || queryScope == "" {
		return
	}

	var docEdits []protocol.TextEdit

	// Find the scope
	for _, scope := range doc.Analysis.Suite.Scopes {
		if scope.QueryName != queryScope {
			continue
		}
		s.collectFieldEdits(scope.Items, oldName, newName, &docEdits)
	}

	if len(docEdits) > 0 {
		edits[doc.URI] = docEdits
	}
}

// collectFieldEdits recursively collects return field rename edits.
func (s *Server) collectFieldEdits(items []*scaf.TestOrGroup, oldName, newName string, edits *[]protocol.TextEdit) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if len(key) > 0 && key[0] != '$' && key == oldName {
					*edits = append(*edits, protocol.TextEdit{
						Range:   statementKeyRange(stmt),
						NewText: newName,
					})
				}
			}
		}
		if item.Group != nil {
			s.collectFieldEdits(item.Group.Items, oldName, newName, edits)
		}
	}
}
