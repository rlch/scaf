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

	// Check global setup clause (Suite.Setup).
	if f.Suite.Setup != nil && containsPosition(f.Suite.Setup.Span(), pos) {
		// Check for named setup inside
		if f.Suite.Setup.Named != nil && containsPosition(f.Suite.Setup.Named.Span(), pos) {
			best = f.Suite.Setup.Named
		} else {
			best = f.Suite.Setup
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
	// Check setup clause first (more specific)
	if scope.Setup != nil && containsPosition(scope.Setup.Span(), pos) {
		// Check for named setup inside
		if scope.Setup.Named != nil && containsPosition(scope.Setup.Named.Span(), pos) {
			return scope.Setup.Named
		}
		return scope.Setup
	}

	// Then check items
	if child := nodeInItems(scope.Items, pos); child != nil {
		return child
	}

	return nil
}

//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func nodeInItems(items []*scaf.TestOrGroup, pos lexer.Position) scaf.Node {
	for _, item := range items {
		if item.Test != nil && containsPosition(item.Test.Span(), pos) {
			// Check for more specific nodes inside test
			if child := nodeInTest(item.Test, pos); child != nil {
				return child
			}
			return item.Test
		}

		if item.Group != nil && containsPosition(item.Group.Span(), pos) {
			// Check setup in group
			if item.Group.Setup != nil && containsPosition(item.Group.Setup.Span(), pos) {
				if item.Group.Setup.Named != nil && containsPosition(item.Group.Setup.Named.Span(), pos) {
					return item.Group.Setup.Named
				}
				return item.Group.Setup
			}

			// Check children first.
			if child := nodeInItems(item.Group.Items, pos); child != nil {
				return child
			}

			return item.Group
		}
	}

	return nil
}

//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func nodeInTest(test *scaf.Test, pos lexer.Position) scaf.Node {
	// Check setup
	if test.Setup != nil && containsPosition(test.Setup.Span(), pos) {
		if test.Setup.Named != nil && containsPosition(test.Setup.Named.Span(), pos) {
			return test.Setup.Named
		}
		return test.Setup
	}

	// Check statements
	for _, stmt := range test.Statements {
		if containsPosition(stmt.Span(), pos) {
			return stmt
		}
	}

	// Check asserts
	for _, assert := range test.Asserts {
		if containsPosition(assert.Span(), pos) {
			// Check if we're on the AssertQuery (more specific)
			if assert.Query != nil && containsPosition(assert.Query.Span(), pos) {
				return assert.Query
			}
			return assert
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

// TokenAtPosition finds the exact token at a given position by scanning node tokens.
// This is more precise than span-based detection because it returns the specific token
// the cursor is on, not just the containing node.
func TokenAtPosition(f *AnalyzedFile, pos lexer.Position) *lexer.Token {
	if f.Suite == nil {
		return nil
	}

	// Check all tokens in the suite
	for i := range f.Suite.Tokens {
		tok := &f.Suite.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	// Check imports
	for _, imp := range f.Suite.Imports {
		for i := range imp.Tokens {
			tok := &imp.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
	}

	// Check queries
	for _, q := range f.Suite.Queries {
		for i := range q.Tokens {
			tok := &q.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
	}

	// Check scopes
	for _, scope := range f.Suite.Scopes {
		if tok := findTokenInScope(scope, pos); tok != nil {
			return tok
		}
	}

	return nil
}

// findTokenInScope searches for a token in a scope and its children.
func findTokenInScope(scope *scaf.QueryScope, pos lexer.Position) *lexer.Token {
	// Check scope tokens
	for i := range scope.Tokens {
		tok := &scope.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	// Check setup clause
	if scope.Setup != nil {
		if tok := findTokenInSetup(scope.Setup, pos); tok != nil {
			return tok
		}
	}

	// Check items
	for _, item := range scope.Items {
		if tok := findTokenInTestOrGroup(item, pos); tok != nil {
			return tok
		}
	}

	return nil
}

// findTokenInTestOrGroup searches for a token in a test or group.
func findTokenInTestOrGroup(item *scaf.TestOrGroup, pos lexer.Position) *lexer.Token {
	if item == nil {
		return nil
	}

	// Check item tokens
	for i := range item.Tokens {
		tok := &item.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	if item.Test != nil {
		if tok := findTokenInTest(item.Test, pos); tok != nil {
			return tok
		}
	}

	if item.Group != nil {
		if tok := findTokenInGroup(item.Group, pos); tok != nil {
			return tok
		}
	}

	return nil
}

// findTokenInTest searches for a token in a test.
func findTokenInTest(test *scaf.Test, pos lexer.Position) *lexer.Token {
	// Check test tokens
	for i := range test.Tokens {
		tok := &test.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	// Check setup
	if test.Setup != nil {
		if tok := findTokenInSetup(test.Setup, pos); tok != nil {
			return tok
		}
	}

	// Check statements
	for _, stmt := range test.Statements {
		for i := range stmt.Tokens {
			tok := &stmt.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
	}

	// Check asserts
	for _, assert := range test.Asserts {
		for i := range assert.Tokens {
			tok := &assert.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
	}

	return nil
}

// findTokenInGroup searches for a token in a group.
func findTokenInGroup(group *scaf.Group, pos lexer.Position) *lexer.Token {
	// Check group tokens
	for i := range group.Tokens {
		tok := &group.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	// Check setup
	if group.Setup != nil {
		if tok := findTokenInSetup(group.Setup, pos); tok != nil {
			return tok
		}
	}

	// Check items
	for _, item := range group.Items {
		if tok := findTokenInTestOrGroup(item, pos); tok != nil {
			return tok
		}
	}

	return nil
}

// findTokenInSetup searches for a token in a setup clause.
func findTokenInSetup(setup *scaf.SetupClause, pos lexer.Position) *lexer.Token {
	// Check setup tokens
	for i := range setup.Tokens {
		tok := &setup.Tokens[i]
		if tokenContainsPosition(tok, pos) {
			return tok
		}
	}

	// Check named setup
	if setup.Named != nil {
		for i := range setup.Named.Tokens {
			tok := &setup.Named.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
	}

	// Check block items
	for _, item := range setup.Block {
		for i := range item.Tokens {
			tok := &item.Tokens[i]
			if tokenContainsPosition(tok, pos) {
				return tok
			}
		}
		if item.Named != nil {
			for i := range item.Named.Tokens {
				tok := &item.Named.Tokens[i]
				if tokenContainsPosition(tok, pos) {
					return tok
				}
			}
		}
	}

	return nil
}

// tokenContainsPosition checks if a token contains a given position.
// Tokens span from Pos to Pos + len(Value).
func tokenContainsPosition(tok *lexer.Token, pos lexer.Position) bool {
	// Token must be on the same line (most tokens are single-line)
	if tok.Pos.Line != pos.Line {
		return false
	}

	// Check column bounds: Pos.Column <= pos.Column < Pos.Column + len(Value)
	startCol := tok.Pos.Column
	endCol := tok.Pos.Column + len(tok.Value)

	return pos.Column >= startCol && pos.Column < endCol
}

// TokenContext provides context about what kind of position we're at.
type TokenContext struct {
	// Token is the token at the cursor position (nil if between tokens).
	Token *lexer.Token
	// PrevToken is the token before the cursor (nil if at start).
	PrevToken *lexer.Token
	// Node is the AST node containing this position.
	Node scaf.Node
	// InSetup is true if inside a setup clause.
	InSetup bool
	// InTest is true if inside a test body.
	InTest bool
	// InGroup is true if inside a group body.
	InGroup bool
	// InAssert is true if inside an assert block.
	InAssert bool
	// QueryScope is the name of the enclosing query scope (empty if top-level).
	QueryScope string
}

// GetTokenContext returns detailed context about a cursor position.
// This is useful for intelligent completions based on exact position.
func GetTokenContext(f *AnalyzedFile, pos lexer.Position) *TokenContext {
	ctx := &TokenContext{}

	if f.Suite == nil {
		return ctx
	}

	// Find the token at position
	ctx.Token = TokenAtPosition(f, pos)

	// Find the containing node
	ctx.Node = NodeAtPosition(f, pos)

	// Determine context from node hierarchy
	for _, scope := range f.Suite.Scopes {
		if containsPosition(scope.Span(), pos) {
			ctx.QueryScope = scope.QueryName

			// Check if in setup
			if scope.Setup != nil && containsPosition(scope.Setup.Span(), pos) {
				ctx.InSetup = true
			}

			// Check items
			for _, item := range scope.Items {
				if item.Test != nil && containsPosition(item.Test.Span(), pos) {
					ctx.InTest = true
					// Check setup in test
					if item.Test.Setup != nil && containsPosition(item.Test.Setup.Span(), pos) {
						ctx.InSetup = true
					}
					// Check asserts
					for _, assert := range item.Test.Asserts {
						if containsPosition(assert.Span(), pos) {
							ctx.InAssert = true
						}
					}
				}

				if item.Group != nil {
					ctx.InGroup = checkInGroup(item.Group, pos, ctx)
				}
			}
		}
	}

	return ctx
}

// checkInGroup recursively checks if position is in a group.
func checkInGroup(group *scaf.Group, pos lexer.Position, ctx *TokenContext) bool {
	if !containsPosition(group.Span(), pos) {
		return false
	}

	ctx.InGroup = true

	if group.Setup != nil && containsPosition(group.Setup.Span(), pos) {
		ctx.InSetup = true
	}

	for _, item := range group.Items {
		if item.Test != nil && containsPosition(item.Test.Span(), pos) {
			ctx.InTest = true
			if item.Test.Setup != nil && containsPosition(item.Test.Setup.Span(), pos) {
				ctx.InSetup = true
			}
			for _, assert := range item.Test.Asserts {
				if containsPosition(assert.Span(), pos) {
					ctx.InAssert = true
				}
			}
		}

		if item.Group != nil {
			checkInGroup(item.Group, pos, ctx)
		}
	}

	return true
}
