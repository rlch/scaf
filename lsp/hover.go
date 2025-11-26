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

	// Find the node at this position
	node := analysis.NodeAtPosition(doc.Analysis, pos)
	if node == nil {
		return nil, nil //nolint:nilnil
	}

	// Generate hover content based on node type
	content, rng := s.hoverContent(doc.Analysis, node)
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
func (s *Server) hoverContent(f *analysis.AnalyzedFile, node scaf.Node) (string, *protocol.Range) {
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

	default:
		return "", nil
	}
}

// hoverQuery generates hover content for a query definition.
func (s *Server) hoverQuery(q *scaf.Query) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Query:** `%s`\n\n", q.Name))
	b.WriteString("```cypher\n")
	b.WriteString(strings.TrimSpace(q.Body))
	b.WriteString("\n```")

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

	b.WriteString("```cypher\n")
	b.WriteString(strings.TrimSpace(q.Body))
	b.WriteString("\n```")

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
