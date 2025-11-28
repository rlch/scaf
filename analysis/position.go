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
		// Check for call inside
		if f.Suite.Setup.Call != nil && containsPosition(f.Suite.Setup.Call.Span(), pos) {
			best = f.Suite.Setup.Call
		} else if child := nodeInSetupBlock(f.Suite.Setup, pos); child != nil {
			best = child
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
		// Check for call inside
		if scope.Setup.Call != nil && containsPosition(scope.Setup.Call.Span(), pos) {
			return scope.Setup.Call
		}
		// Check for block items
		if child := nodeInSetupBlock(scope.Setup, pos); child != nil {
			return child
		}
		return scope.Setup
	}

	// Then check items
	if child := nodeInItems(scope.Items, pos); child != nil {
		return child
	}

	return nil
}

// nodeInSetupBlock checks for more specific nodes within a setup block.
//
//nolint:ireturn // Returning interface is intentional for AST node polymorphism.
func nodeInSetupBlock(setup *scaf.SetupClause, pos lexer.Position) scaf.Node {
	for _, item := range setup.Block {
		if containsPosition(item.Span(), pos) {
			// Check for call inside the item
			if item.Call != nil && containsPosition(item.Call.Span(), pos) {
				return item.Call
			}
			return item
		}
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
				if item.Group.Setup.Call != nil && containsPosition(item.Group.Setup.Call.Span(), pos) {
					return item.Group.Setup.Call
				}
				if child := nodeInSetupBlock(item.Group.Setup, pos); child != nil {
					return child
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
		if test.Setup.Call != nil && containsPosition(test.Setup.Call.Span(), pos) {
			return test.Setup.Call
		}
		if child := nodeInSetupBlock(test.Setup, pos); child != nil {
			return child
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

	// Check setup call
	if setup.Call != nil {
		for i := range setup.Call.Tokens {
			tok := &setup.Call.Tokens[i]
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
		if item.Call != nil {
			for i := range item.Call.Tokens {
				tok := &item.Call.Tokens[i]
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

// tokenEndsBefore returns true if the token ends before the given position.
func tokenEndsBefore(tok *lexer.Token, pos lexer.Position) bool {
	endLine := tok.Pos.Line
	endCol := tok.Pos.Column + len(tok.Value)

	if endLine < pos.Line {
		return true
	}
	if endLine == pos.Line && endCol <= pos.Column {
		return true
	}
	return false
}

// PrevTokenAtPosition finds the non-whitespace token immediately before a given position.
// This is useful for completion to know what token precedes the cursor.
// Whitespace and comment tokens are skipped.
func PrevTokenAtPosition(f *AnalyzedFile, pos lexer.Position) *lexer.Token {
	if f.Suite == nil {
		return nil
	}

	var best *lexer.Token
	bestEnd := lexer.Position{}

	// Helper to update best if this token is closer to pos but still before it
	// Skips whitespace and comment tokens
	checkToken := func(tok *lexer.Token) {
		// Skip whitespace and comments
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenComment {
			return
		}
		if !tokenEndsBefore(tok, pos) {
			return
		}
		tokEnd := lexer.Position{
			Line:   tok.Pos.Line,
			Column: tok.Pos.Column + len(tok.Value),
		}
		// Is this token closer to pos than the current best?
		if best == nil ||
			tokEnd.Line > bestEnd.Line ||
			(tokEnd.Line == bestEnd.Line && tokEnd.Column > bestEnd.Column) {
			best = tok
			bestEnd = tokEnd
		}
	}

	// Check all tokens in the suite
	for i := range f.Suite.Tokens {
		checkToken(&f.Suite.Tokens[i])
	}

	// Check imports
	for _, imp := range f.Suite.Imports {
		for i := range imp.Tokens {
			checkToken(&imp.Tokens[i])
		}
	}

	// Check queries
	for _, q := range f.Suite.Queries {
		for i := range q.Tokens {
			checkToken(&q.Tokens[i])
		}
	}

	// Check scopes
	for _, scope := range f.Suite.Scopes {
		findPrevTokenInScope(scope, pos, &best, &bestEnd)
	}

	return best
}

// findPrevTokenInScope searches for previous token in a scope.
func findPrevTokenInScope(scope *scaf.QueryScope, pos lexer.Position, best **lexer.Token, bestEnd *lexer.Position) {
	checkToken := func(tok *lexer.Token) {
		// Skip whitespace and comments
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenComment {
			return
		}
		if !tokenEndsBefore(tok, pos) {
			return
		}
		tokEnd := lexer.Position{
			Line:   tok.Pos.Line,
			Column: tok.Pos.Column + len(tok.Value),
		}
		if *best == nil ||
			tokEnd.Line > bestEnd.Line ||
			(tokEnd.Line == bestEnd.Line && tokEnd.Column > bestEnd.Column) {
			*best = tok
			*bestEnd = tokEnd
		}
	}

	// Check scope tokens
	for i := range scope.Tokens {
		checkToken(&scope.Tokens[i])
	}

	// Check setup
	if scope.Setup != nil {
		findPrevTokenInSetupClause(scope.Setup, pos, best, bestEnd)
	}

	// Check items
	for _, item := range scope.Items {
		findPrevTokenInTestOrGroup(item, pos, best, bestEnd)
	}
}

// findPrevTokenInTestOrGroup searches for previous token in a test or group.
func findPrevTokenInTestOrGroup(item *scaf.TestOrGroup, pos lexer.Position, best **lexer.Token, bestEnd *lexer.Position) {
	if item == nil {
		return
	}

	checkToken := func(tok *lexer.Token) {
		// Skip whitespace and comments
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenComment {
			return
		}
		if !tokenEndsBefore(tok, pos) {
			return
		}
		tokEnd := lexer.Position{
			Line:   tok.Pos.Line,
			Column: tok.Pos.Column + len(tok.Value),
		}
		if *best == nil ||
			tokEnd.Line > bestEnd.Line ||
			(tokEnd.Line == bestEnd.Line && tokEnd.Column > bestEnd.Column) {
			*best = tok
			*bestEnd = tokEnd
		}
	}

	for i := range item.Tokens {
		checkToken(&item.Tokens[i])
	}

	if item.Test != nil {
		for i := range item.Test.Tokens {
			checkToken(&item.Test.Tokens[i])
		}
		if item.Test.Setup != nil {
			findPrevTokenInSetupClause(item.Test.Setup, pos, best, bestEnd)
		}
	}

	if item.Group != nil {
		for i := range item.Group.Tokens {
			checkToken(&item.Group.Tokens[i])
		}
		if item.Group.Setup != nil {
			findPrevTokenInSetupClause(item.Group.Setup, pos, best, bestEnd)
		}
		for _, child := range item.Group.Items {
			findPrevTokenInTestOrGroup(child, pos, best, bestEnd)
		}
	}
}

// findPrevTokenInSetupClause searches for previous token in a setup clause.
func findPrevTokenInSetupClause(setup *scaf.SetupClause, pos lexer.Position, best **lexer.Token, bestEnd *lexer.Position) {
	checkToken := func(tok *lexer.Token) {
		// Skip whitespace and comments
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenComment {
			return
		}
		if !tokenEndsBefore(tok, pos) {
			return
		}
		tokEnd := lexer.Position{
			Line:   tok.Pos.Line,
			Column: tok.Pos.Column + len(tok.Value),
		}
		if *best == nil ||
			tokEnd.Line > bestEnd.Line ||
			(tokEnd.Line == bestEnd.Line && tokEnd.Column > bestEnd.Column) {
			*best = tok
			*bestEnd = tokEnd
		}
	}

	for i := range setup.Tokens {
		checkToken(&setup.Tokens[i])
	}

	if setup.Call != nil {
		for i := range setup.Call.Tokens {
			checkToken(&setup.Call.Tokens[i])
		}
	}

	// Check block items (setup { ... } syntax)
	for _, item := range setup.Block {
		for i := range item.Tokens {
			checkToken(&item.Tokens[i])
		}
		if item.Call != nil {
			for i := range item.Call.Tokens {
				checkToken(&item.Call.Tokens[i])
			}
		}
	}
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

	// Find the previous token (before cursor position)
	ctx.PrevToken = PrevTokenAtPosition(f, pos)

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
