package lsp

import (
	"context"
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// markdownQueryBlock wraps a query body in a markdown code block with the appropriate language.
func (s *Server) markdownQueryBlock(queryBody string) string {
	lang := scaf.MarkdownLanguage(s.dialectName)
	return "```" + lang + "\n" + strings.TrimSpace(queryBody) + "\n```"
}

// Hover handles textDocument/hover requests.
func (s *Server) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.logger.Debug("Hover",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil //nolint:nilnil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)

	// Get token context for precise information
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	// Find the node at this position
	node := analysis.NodeAtPosition(doc.Analysis, pos)
	if node == nil {
		return nil, nil //nolint:nilnil
	}

	// Generate hover content based on node type
	content, rng := s.hoverContent(doc, doc.Analysis, node, tokenCtx)
	if content == "" {
		return nil, nil //nolint:nilnil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: content,
		},
		Range: rng,
	}, nil
}

// hoverContent generates hover markdown for a node.
func (s *Server) hoverContent(doc *Document, f *analysis.AnalyzedFile, node scaf.Node, tokenCtx *analysis.TokenContext) (string, *protocol.Range) {
	switch n := node.(type) {
	case *scaf.Query:
		return s.hoverQuery(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Import:
		return s.hoverImport(n), rangePtr(spanToRange(n.Span()))

	case *scaf.QueryScope:
		// When hovering over a scope, show info about the referenced query
		if q, ok := f.Symbols.Queries[n.QueryName]; ok {
			return s.hoverQueryRef(q), rangePtr(spanToRange(n.Span()))
		}

		return fmt.Sprintf("**Query Scope:** `%s` (undefined)", n.QueryName), rangePtr(spanToRange(n.Span()))

	case *scaf.Test:
		return s.hoverTest(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Group:
		return s.hoverGroup(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Statement:
		return s.hoverStatement(f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.SetupCall:
		return s.hoverSetupCall(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.SetupClause:
		return s.hoverSetupClause(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.SetupItem:
		return s.hoverSetupItem(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.AssertQuery:
		return s.hoverAssertQuery(f, n), rangePtr(spanToRange(n.Span()))

	default:
		return "", nil
	}
}

// hoverQuery generates hover content for a query definition.
func (s *Server) hoverQuery(q *scaf.Query) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Query:** `%s`\n\n", q.Name))
	b.WriteString(s.markdownQueryBlock(q.Body))

	return b.String()
}

// hoverQueryRef generates hover content for a query reference (in a scope).
func (s *Server) hoverQueryRef(q *analysis.QuerySymbol) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Query:** `%s`\n\n", q.Name))

	if len(q.Params) > 0 {
		b.WriteString("**Parameters:** ")

		for i, p := range q.Params {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString("`$" + p + "`")
		}

		b.WriteString("\n\n")
	}

	b.WriteString(s.markdownQueryBlock(q.Body))

	return b.String()
}

// hoverImport generates hover content for an import.
func (s *Server) hoverImport(imp *scaf.Import) string {
	var b strings.Builder

	b.WriteString("**Import**\n\n")
	b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))

	if imp.Alias != nil {
		b.WriteString(fmt.Sprintf("**Alias:** `%s`\n", *imp.Alias))
	}

	return b.String()
}

// hoverTest generates hover content for a test.
func (s *Server) hoverTest(t *scaf.Test) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Test:** `%s`\n\n", t.Name))

	// Count inputs and outputs
	var inputs, outputs int

	for _, stmt := range t.Statements {
		if strings.HasPrefix(stmt.Key(), "$") {
			inputs++
		} else {
			outputs++
		}
	}

	b.WriteString(fmt.Sprintf("- **Inputs:** %d\n", inputs))
	b.WriteString(fmt.Sprintf("- **Outputs:** %d\n", outputs))
	b.WriteString(fmt.Sprintf("- **Assertions:** %d\n", len(t.Asserts)))

	if t.Setup != nil {
		b.WriteString("- **Has Setup:** yes\n")
	}

	return b.String()
}

// hoverGroup generates hover content for a group.
func (s *Server) hoverGroup(g *scaf.Group) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Group:** `%s`\n\n", g.Name))

	// Count items
	tests, groups := countItems(g.Items)
	b.WriteString(fmt.Sprintf("- **Tests:** %d\n", tests))
	b.WriteString(fmt.Sprintf("- **Nested Groups:** %d\n", groups))

	if g.Setup != nil {
		b.WriteString("- **Has Setup:** yes\n")
	}

	if g.Teardown != nil {
		b.WriteString("- **Has Teardown:** yes\n")
	}

	return b.String()
}

// countItems counts tests and groups in an item list.
func countItems(items []*scaf.TestOrGroup) (int, int) {
	var tests, groups int

	for _, item := range items {
		if item.Test != nil {
			tests++
		}

		if item.Group != nil {
			groups++
		}
	}

	return tests, groups
}

// rangePtr returns a pointer to a Range.
func rangePtr(r protocol.Range) *protocol.Range {
	return &r
}

// hoverStatement generates hover content for a statement ($param or return field).
func (s *Server) hoverStatement(f *analysis.AnalyzedFile, stmt *scaf.Statement, tokenCtx *analysis.TokenContext) string {
	key := stmt.Key()
	if key == "" {
		return ""
	}

	// Find the enclosing query for context
	var querySymbol *analysis.QuerySymbol
	if tokenCtx.QueryScope != "" {
		if q, ok := f.Symbols.Queries[tokenCtx.QueryScope]; ok {
			querySymbol = q
		}
	}

	// Check if this is a parameter ($param)
	if strings.HasPrefix(key, "$") {
		return s.hoverParameter(key, stmt, querySymbol)
	}

	// Otherwise it's a return field (e.g., u.name)
	return s.hoverReturnField(key, stmt, querySymbol)
}

// hoverParameter generates hover content for a parameter statement.
func (s *Server) hoverParameter(key string, stmt *scaf.Statement, q *analysis.QuerySymbol) string {
	var b strings.Builder

	paramName := key[1:] // Remove $ prefix
	b.WriteString(fmt.Sprintf("**Parameter:** `%s`\n\n", key))

	// Show the value being passed
	if stmt.Value != nil {
		b.WriteString(fmt.Sprintf("**Value:** `%s`\n\n", stmt.Value.String()))
	}

	// If we have the query, check if this param exists and show context
	if q != nil {
		found := false
		for _, p := range q.Params {
			if p == paramName {
				found = true
				break
			}
		}

		if found {
			b.WriteString(fmt.Sprintf("Used in query `%s`\n", q.Name))
		} else {
			b.WriteString(fmt.Sprintf("⚠️ Parameter not found in query `%s`\n", q.Name))
		}

		// Try to get more info from query analyzer
		if s.queryAnalyzer != nil && q.Body != "" {
			metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
			if err == nil {
				for _, p := range metadata.Parameters {
					if p.Name == paramName {
						if p.Type != "" {
							b.WriteString(fmt.Sprintf("\n**Type:** `%s`\n", p.Type))
						}
						if p.Count > 1 {
							b.WriteString(fmt.Sprintf("Referenced %d times in query\n", p.Count))
						}
						break
					}
				}
			}
		}
	}

	return b.String()
}

// hoverReturnField generates hover content for a return field statement.
func (s *Server) hoverReturnField(key string, stmt *scaf.Statement, q *analysis.QuerySymbol) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Return Field:** `%s`\n\n", key))

	// Show the expected value
	if stmt.Value != nil {
		b.WriteString(fmt.Sprintf("**Expected:** `%s`\n\n", stmt.Value.String()))
	}

	// If we have the query and analyzer, show where this field comes from
	if q != nil && s.queryAnalyzer != nil && q.Body != "" {
		metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
		if err == nil {
			found := false
			for _, ret := range metadata.Returns {
				// Match against Name, Expression, or Alias
				if ret.Name == key || ret.Expression == key || ret.Alias == key {
					found = true

					if ret.Alias != "" && ret.Expression != ret.Alias {
						b.WriteString(fmt.Sprintf("**Expression:** `%s`\n", ret.Expression))
					}
					if ret.IsAggregate {
						b.WriteString("**Type:** aggregate\n")
					}
					break
				}
			}

			if !found {
				b.WriteString(fmt.Sprintf("⚠️ Field not found in query `%s` RETURN clause\n", q.Name))
			}
		}
	}

	return b.String()
}

// hoverSetupCall generates hover content for a setup call (module.Query()).
func (s *Server) hoverSetupCall(doc *Document, f *analysis.AnalyzedFile, call *scaf.SetupCall, tokenCtx *analysis.TokenContext) string {
	var b strings.Builder

	// Check if hovering on the module name or query name
	if tokenCtx.Token != nil {
		if tokenCtx.Token.Value == call.Module {
			// Hovering on module name - show import info
			return s.hoverModuleRef(f, call.Module)
		}
	}

	// Show info about the query being called
	b.WriteString(fmt.Sprintf("**Setup Call:** `%s.%s`\n\n", call.Module, call.Query))

	// Try to load the imported module and get query info
	if s.fileLoader != nil {
		if imp, ok := f.Symbols.Imports[call.Module]; ok {
			docPath := URIToPath(doc.URI)
			importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

			// Check if the imported file is currently open in the editor
			// If so, use the in-memory version instead of the disk version
			importedURI := PathToURI(importedPath)
			if openDoc, ok := s.getDocument(importedURI); ok && openDoc.Analysis != nil {
				// Use the open document's analysis
				return s.hoverSetupCallWithAnalysis(call, openDoc.Analysis, &b)
			}

			// Otherwise load from disk
			importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)

			if err != nil {
				s.logger.Debug("Failed to load imported file for hover",
					zap.String("path", importedPath),
					zap.Error(err))
				b.WriteString(fmt.Sprintf("⚠️ Could not load module `%s`\n\n", call.Module))
				b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))
				b.WriteString("\n_Tip: Make sure the imported file exists and is saved._\n")
				return b.String()
			}

			return s.hoverSetupCallWithAnalysis(call, importedFile, &b)
		} else {
			s.logger.Debug("Import not found for module",
				zap.String("module", call.Module))
			b.WriteString(fmt.Sprintf("⚠️ Module `%s` not found in imports\n", call.Module))
		}
	} else {
		s.logger.Debug("FileLoader not available for hover")
	}

	return b.String()
}

// hoverSetupCallWithAnalysis generates hover content using a loaded/analyzed file.
func (s *Server) hoverSetupCallWithAnalysis(call *scaf.SetupCall, importedFile *analysis.AnalyzedFile, b *strings.Builder) string {
	if importedFile.Symbols == nil {
		b.WriteString(fmt.Sprintf("⚠️ Module `%s` could not be analyzed\n", call.Module))
		return b.String()
	}

	if q, ok := importedFile.Symbols.Queries[call.Query]; ok {
		if len(q.Params) > 0 {
			b.WriteString("**Parameters:** ")
			for i, p := range q.Params {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("`$" + p + "`")
			}
			b.WriteString("\n\n")
		}
		b.WriteString(s.markdownQueryBlock(q.Body))
		return b.String()
	}

	// Query not found - provide helpful diagnostics
	var queryNames []string
	for name := range importedFile.Symbols.Queries {
		queryNames = append(queryNames, name)
	}
	s.logger.Debug("Query not found in imported file",
		zap.String("queryName", call.Query),
		zap.Strings("availableQueries", queryNames))

	b.WriteString(fmt.Sprintf("⚠️ Query `%s` not found in module `%s`\n\n", call.Query, call.Module))

	if len(queryNames) > 0 {
		b.WriteString("**Available queries:**\n")
		for _, name := range queryNames {
			b.WriteString(fmt.Sprintf("- `%s`\n", name))
		}
	} else {
		b.WriteString("_No queries found in this module._\n")
	}

	return b.String()
}

// hoverSetupClause generates hover content for a setup clause.
func (s *Server) hoverSetupClause(doc *Document, f *analysis.AnalyzedFile, clause *scaf.SetupClause, tokenCtx *analysis.TokenContext) string {
	// If it's a module reference (setup fixtures), show module info
	if clause.Module != nil {
		return s.hoverModuleRef(f, *clause.Module)
	}

	// If it's an inline query, show it
	if clause.Inline != nil {
		var b strings.Builder
		b.WriteString("**Inline Setup Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*clause.Inline))
		return b.String()
	}

	// If it's a block, show count
	if len(clause.Block) > 0 {
		return fmt.Sprintf("**Setup Block:** %d items", len(clause.Block))
	}

	return ""
}

// hoverSetupItem generates hover content for a setup item in a block.
func (s *Server) hoverSetupItem(doc *Document, f *analysis.AnalyzedFile, item *scaf.SetupItem, tokenCtx *analysis.TokenContext) string {
	// If it's a module reference, show module info
	if item.Module != nil {
		return s.hoverModuleRef(f, *item.Module)
	}

	// If it's an inline query, show it
	if item.Inline != nil {
		var b strings.Builder
		b.WriteString("**Inline Setup Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*item.Inline))
		return b.String()
	}

	return ""
}

// hoverModuleRef generates hover content for a module reference.
func (s *Server) hoverModuleRef(f *analysis.AnalyzedFile, moduleName string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Module:** `%s`\n\n", moduleName))

	if imp, ok := f.Symbols.Imports[moduleName]; ok {
		b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))

		// If we can load the module, show its contents
		if s.fileLoader != nil {
			// Note: we'd need the document path here for proper resolution
			// For now just show the import path
		}
	} else {
		b.WriteString("⚠️ Module not found in imports\n")
	}

	return b.String()
}

// hoverAssertQuery generates hover content for an assert query reference.
func (s *Server) hoverAssertQuery(f *analysis.AnalyzedFile, aq *scaf.AssertQuery) string {
	var b strings.Builder

	// Check if it's a named query reference or inline
	if aq.QueryName != nil {
		b.WriteString(fmt.Sprintf("**Assert Query:** `%s`\n\n", *aq.QueryName))

		if q, ok := f.Symbols.Queries[*aq.QueryName]; ok {
			if len(q.Params) > 0 {
				b.WriteString("**Parameters:** ")
				for i, p := range q.Params {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString("`$" + p + "`")
				}
				b.WriteString("\n\n")
			}
			b.WriteString(s.markdownQueryBlock(q.Body))
		} else {
			b.WriteString("⚠️ Query not found\n")
		}
	} else if aq.Inline != nil {
		b.WriteString("**Inline Assert Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*aq.Inline))
	}

	return b.String()
}
