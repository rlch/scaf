package analysis

import (
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

// NodeAtPosition finds the most specific AST node at a given position.
// Returns nil if no node contains the position.
//
//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func NodeAtPosition(f *AnalyzedFile, pos lexer.Position) scaf.Node {
	if f.Suite == nil {
		return nil
	}

	// Walk the AST to find the most specific node containing the position.
	// We use a simple recursive descent, tracking the most specific match.
	var best scaf.Node

	// Check imports.
	for _, imp := range f.Suite.Imports {
		if containsPosition(imp.Span(), pos) {
			best = imp
		}
	}

	// Check queries.
	for _, q := range f.Suite.Queries {
		if containsPosition(q.Span(), pos) {
			best = q
		}
	}

	// Check scopes.
	for _, scope := range f.Suite.Scopes {
		if containsPosition(scope.Span(), pos) {
			// Check if we're in a more specific child.
			if child := nodeInScope(scope, pos); child != nil {
				best = child
			} else {
				best = scope
			}
		}
	}

	return best
}

//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func nodeInScope(scope *scaf.QueryScope, pos lexer.Position) scaf.Node {
	return nodeInItems(scope.Items, pos)
}

//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func nodeInItems(items []*scaf.TestOrGroup, pos lexer.Position) scaf.Node {
	for _, item := range items {
		if item.Test != nil && containsPosition(item.Test.Span(), pos) {
			return item.Test
		}

		if item.Group != nil && containsPosition(item.Group.Span(), pos) {
			// Check children first.
			if child := nodeInItems(item.Group.Items, pos); child != nil {
				return child
			}

			return item.Group
		}
	}

	return nil
}

// containsPosition checks if a span contains a position.
func containsPosition(span scaf.Span, pos lexer.Position) bool {
	// Check if pos is after start.
	if pos.Line < span.Start.Line {
		return false
	}

	if pos.Line == span.Start.Line && pos.Column < span.Start.Column {
		return false
	}

	// Check if pos is before end.
	if pos.Line > span.End.Line {
		return false
	}

	if pos.Line == span.End.Line && pos.Column > span.End.Column {
		return false
	}

	return true
}

// PositionToLexer converts LSP 0-based line/character to participle's 1-based line/column.
func PositionToLexer(line, character uint32) lexer.Position {
	return lexer.Position{
		Line:   int(line) + 1, // LSP is 0-based, participle is 1-based
		Column: int(character) + 1,
	}
}

// QueryAtPosition returns the query whose name is at the given position.
// Useful for finding the query referenced in a scope declaration.
func QueryAtPosition(f *AnalyzedFile, pos lexer.Position) *QuerySymbol {
	// Check if position is on a query scope's query name.
	if f.Suite == nil {
		return nil
	}

	for _, scope := range f.Suite.Scopes {
		// The query name is at the start of the scope.
		// Check if position is on the first line of the scope, before the '{'.
		if pos.Line == scope.Pos.Line {
			// Position is on the scope declaration line.
			// Return the referenced query.
			if q, ok := f.Symbols.Queries[scope.QueryName]; ok {
				return q
			}
		}
	}

	return nil
}

// SymbolAtPosition returns the symbol at the given position, if any.
func SymbolAtPosition(f *AnalyzedFile, pos lexer.Position) *Symbol {
	// Check queries.
	for _, q := range f.Symbols.Queries {
		if containsPosition(q.Span, pos) {
			return &q.Symbol
		}
	}

	// Check imports.
	for _, imp := range f.Symbols.Imports {
		if containsPosition(imp.Span, pos) {
			return &imp.Symbol
		}
	}

	// Check tests.
	for _, t := range f.Symbols.Tests {
		if containsPosition(t.Span, pos) {
			return &t.Symbol
		}
	}

	return nil
}
