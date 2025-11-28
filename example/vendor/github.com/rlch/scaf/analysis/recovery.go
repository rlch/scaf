// Package analysis provides semantic analysis for scaf DSL files.
package analysis

import (
	"errors"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

// RecoveryCompletionContext provides completion context extracted from recovered AST nodes
// or parse error positions. This is the primary mechanism for understanding what the user
// is typing when parsing fails.
type RecoveryCompletionContext struct {
	// Kind indicates what kind of completion is needed based on recovery analysis.
	Kind RecoveryCompletionKind
	// RecoveredNode is the node that was recovered (has WasRecovered() == true).
	RecoveredNode scaf.Node
	// RecoveredTokens are the tokens that were skipped during recovery.
	RecoveredTokens []lexer.Token
	// Prefix is the text being typed (extracted from recovered tokens).
	Prefix string
	// ModuleAlias is set when completing after "module." pattern.
	ModuleAlias string
	// ParentNode is the parent of the recovered node (provides context).
	ParentNode scaf.Node
	// InSetup is true if inside a setup clause.
	InSetup bool
	// InTest is true if inside a test body.
	InTest bool
	// InAssert is true if inside an assert block.
	InAssert bool
	// QueryScope is the name of the enclosing query scope.
	QueryScope string
	// ErrorPos is the position of the parse error, if any.
	ErrorPos lexer.Position
	// PrevToken is the token before the error/cursor position.
	PrevToken *lexer.Token
}

// RecoveryCompletionKind indicates what kind of completion the recovery suggests.
type RecoveryCompletionKind int

const (
	// RecoveryCompletionNone indicates no recovery-based completion context.
	RecoveryCompletionNone RecoveryCompletionKind = iota
	// RecoveryCompletionSetupAlias indicates completing an import alias after "setup".
	RecoveryCompletionSetupAlias
	// RecoveryCompletionSetupFunction indicates completing a function after "module.".
	RecoveryCompletionSetupFunction
	// RecoveryCompletionParameter indicates completing a $parameter.
	RecoveryCompletionParameter
	// RecoveryCompletionReturnField indicates completing a return field.
	RecoveryCompletionReturnField
	// RecoveryCompletionKeyword indicates completing a keyword.
	RecoveryCompletionKeyword
)

// GetRecoveryCompletionContext analyzes the AST and parse errors to find completion context.
// This is the main entry point for recovery-based completions.
//
// Strategy:
// 1. Look for nodes with RecoveredTokens in RecoverySuite (parser recovery) 
// 2. Look at parse error position and preceding tokens
// 3. Use token context at cursor position
func GetRecoveryCompletionContext(f *AnalyzedFile, pos lexer.Position) *RecoveryCompletionContext {
	if f == nil {
		return nil
	}

	ctx := &RecoveryCompletionContext{}

	// Strategy 1: Check for recovered nodes at position using RecoverySuite
	// RecoverySuite is parsed with recovery enabled, so it may have RecoveredTokens
	if f.RecoverySuite != nil {
		findRecoveredContext(f.RecoverySuite, pos, ctx)
	}

	// If we found a recovered node with tokens, analyze them
	if ctx.RecoveredNode != nil && len(ctx.RecoveredTokens) > 0 {
		analyzeRecoveredTokens(ctx, f.Symbols)
		if ctx.Kind != RecoveryCompletionNone {
			return ctx
		}
	}

	// Strategy 2: Check parse error position from recovery parse
	// This is the main mechanism for completion - we look at where the parse failed
	// and use the tokens before that position to determine context.
	recoveryErr := f.RecoveryError
	if recoveryErr == nil {
		recoveryErr = f.ParseError
	}
	if recoveryErr != nil {
		errorPos := getErrorPosition(recoveryErr)
		if errorPos.Line > 0 {
			ctx.ErrorPos = errorPos
			// Get the token before the error position - use RecoverySuite if available
			if f.RecoverySuite != nil {
				ctx.PrevToken = prevTokenFromSuite(f.RecoverySuite, errorPos)
			} else {
				ctx.PrevToken = PrevTokenAtPosition(f, errorPos)
			}
			
			// If cursor is at or near error position, use error context
			if isNearPosition(pos, errorPos) {
				analyzeErrorContext(ctx, f.Symbols)
				if ctx.Kind != RecoveryCompletionNone {
					return ctx
				}
			}
		}
	}

	// Strategy 3: Use token context at cursor position
	// This handles cases where the file parses correctly but we still need context
	// Prefer RecoverySuite for token lookup since it has more parsed content
	suite := f.RecoverySuite
	if suite == nil {
		suite = f.Suite
	}
	if suite != nil {
		ctx.PrevToken = prevTokenFromSuite(suite, pos)
		analyzeTokenContext(ctx, pos, suite, f.Symbols)
		if ctx.Kind != RecoveryCompletionNone {
			return ctx
		}
	}

	return nil
}

// prevTokenFromSuite finds the previous token in a suite at a given position.
// This is a helper that works directly with a Suite instead of AnalyzedFile.
func prevTokenFromSuite(suite *scaf.Suite, pos lexer.Position) *lexer.Token {
	if suite == nil {
		return nil
	}

	var best *lexer.Token
	bestEnd := lexer.Position{}

	// Helper to update best if this token is closer to pos but still before it
	checkToken := func(tok *lexer.Token) {
		// Skip whitespace and comments
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenComment {
			return
		}
		endCol := tok.Pos.Column + len(tok.Value)
		if tok.Pos.Line < pos.Line || (tok.Pos.Line == pos.Line && endCol <= pos.Column) {
			tokEnd := lexer.Position{Line: tok.Pos.Line, Column: endCol}
			if best == nil || tokEnd.Line > bestEnd.Line || (tokEnd.Line == bestEnd.Line && tokEnd.Column > bestEnd.Column) {
				best = tok
				bestEnd = tokEnd
			}
		}
	}

	// Check all tokens in the suite
	for i := range suite.Tokens {
		checkToken(&suite.Tokens[i])
	}

	// Check imports, queries, scopes...
	for _, imp := range suite.Imports {
		for i := range imp.Tokens {
			checkToken(&imp.Tokens[i])
		}
	}

	for _, q := range suite.Queries {
		for i := range q.Tokens {
			checkToken(&q.Tokens[i])
		}
	}

	for _, scope := range suite.Scopes {
		for i := range scope.Tokens {
			checkToken(&scope.Tokens[i])
		}
		// Check items in scope
		for _, item := range scope.Items {
			for i := range item.Tokens {
				checkToken(&item.Tokens[i])
			}
			if item.Test != nil {
				for i := range item.Test.Tokens {
					checkToken(&item.Test.Tokens[i])
				}
			}
			if item.Group != nil {
				for i := range item.Group.Tokens {
					checkToken(&item.Group.Tokens[i])
				}
			}
		}
	}

	return best
}

// getErrorPosition extracts the position from a parse error.
func getErrorPosition(err error) lexer.Position {
	// Check for RecoveryError first
	var recoveryErr *participle.RecoveryError
	if errors.As(err, &recoveryErr) && len(recoveryErr.Errors) > 0 {
		// Get position from first error
		if perr, ok := recoveryErr.Errors[0].(participle.Error); ok {
			return perr.Position()
		}
	}
	
	// Check for single participle error
	if perr, ok := err.(participle.Error); ok {
		return perr.Position()
	}
	
	return lexer.Position{}
}

// isNearPosition checks if two positions are close enough to be related.
func isNearPosition(pos1, pos2 lexer.Position) bool {
	// Same line, within a few columns
	if pos1.Line == pos2.Line {
		diff := pos1.Column - pos2.Column
		if diff < 0 {
			diff = -diff
		}
		return diff <= 10
	}
	// Adjacent lines
	diff := pos1.Line - pos2.Line
	if diff < 0 {
		diff = -diff
	}
	return diff <= 1
}

// analyzeErrorContext determines completion kind from error position context.
// This is called when there's a parse error and the cursor is near the error position.
// We use the token before the error to determine what completion to offer.
func analyzeErrorContext(ctx *RecoveryCompletionContext, symbols *SymbolTable) {
	if ctx.PrevToken == nil {
		return
	}
	
	// Check what token precedes the error
	switch ctx.PrevToken.Type {
	case scaf.TokenSetup:
		// Error after "setup" - offer import aliases
		ctx.Kind = RecoveryCompletionSetupAlias
		return
	case scaf.TokenDot:
		// Error after a dot - we need to find what's before the dot
		// The dot token doesn't tell us what module it belongs to, so we can't
		// determine ModuleAlias here. This case is better handled by
		// analyzeRecoveredTokens which has access to the full token sequence.
		// For now, signal that we need setup function completion but let the
		// caller figure out the module alias from recovered tokens or text analysis.
		ctx.Kind = RecoveryCompletionSetupFunction
		// Note: ModuleAlias is NOT set here - caller must extract it from context
		return
	case scaf.TokenIdent:
		// If the previous token is an identifier, check if it's an import alias
		if symbols != nil {
			if _, ok := symbols.Imports[ctx.PrevToken.Value]; ok {
				// This is an import alias - user might be typing the dot or function name
				ctx.ModuleAlias = ctx.PrevToken.Value
				ctx.Kind = RecoveryCompletionSetupFunction
				ctx.Prefix = ""
				return
			}
		}
		// Could be typing a setup alias after "setup"
		ctx.Prefix = ctx.PrevToken.Value
		ctx.Kind = RecoveryCompletionSetupAlias
		return
	}
}

// analyzeTokenContext determines completion from token context when no recovery/error.
// This is called when the file parses correctly (or recovery didn't find anything).
func analyzeTokenContext(ctx *RecoveryCompletionContext, pos lexer.Position, suite *scaf.Suite, symbols *SymbolTable) {
	if ctx.PrevToken == nil {
		return
	}
	
	// Determine context from surrounding structure
	for _, scope := range suite.Scopes {
		if containsPosition(scope.Span(), pos) {
			ctx.QueryScope = scope.QueryName
			
			// Check if we're in setup context (after "setup" keyword)
			if ctx.PrevToken.Type == scaf.TokenSetup {
				ctx.InSetup = true
				ctx.Kind = RecoveryCompletionSetupAlias
				return
			}
			
			// Check if previous token is a DOT - need to find module alias before it
			if ctx.PrevToken.Type == scaf.TokenDot {
				// Find the token before the dot
				tokenBeforeDot := prevTokenFromSuite(suite, ctx.PrevToken.Pos)
				if tokenBeforeDot != nil && tokenBeforeDot.Type == scaf.TokenIdent && symbols != nil {
					if _, ok := symbols.Imports[tokenBeforeDot.Value]; ok {
						ctx.ModuleAlias = tokenBeforeDot.Value
						ctx.Kind = RecoveryCompletionSetupFunction
						return
					}
				}
			}
			
			// Check if previous token is an import alias (cursor might be ON the dot)
			if ctx.PrevToken.Type == scaf.TokenIdent && symbols != nil {
				if _, ok := symbols.Imports[ctx.PrevToken.Value]; ok {
					ctx.ModuleAlias = ctx.PrevToken.Value
					ctx.Kind = RecoveryCompletionSetupFunction
					return
				}
			}
		}
	}
}

// findRecoveredContext walks the AST to find recovered nodes at or near the position.
func findRecoveredContext(suite *scaf.Suite, pos lexer.Position, ctx *RecoveryCompletionContext) {
	// Check suite-level recovery
	if suite.WasRecovered() && positionInRecoverySpan(suite.RecoveredSpan, suite.RecoveredEnd, pos) {
		ctx.RecoveredNode = suite
		ctx.RecoveredTokens = suite.RecoveredTokens
	}

	// Check imports
	for _, imp := range suite.Imports {
		if imp.WasRecovered() && positionInRecoverySpan(imp.RecoveredSpan, imp.RecoveredEnd, pos) {
			ctx.RecoveredNode = imp
			ctx.RecoveredTokens = imp.RecoveredTokens
		}
	}

	// Check queries
	for _, q := range suite.Queries {
		if q.WasRecovered() && positionInRecoverySpan(q.RecoveredSpan, q.RecoveredEnd, pos) {
			ctx.RecoveredNode = q
			ctx.RecoveredTokens = q.RecoveredTokens
		}
	}

	// Check suite-level setup
	if suite.Setup != nil {
		findRecoveredInSetup(suite.Setup, pos, ctx)
	}

	// Check scopes
	for _, scope := range suite.Scopes {
		findRecoveredInScope(scope, pos, ctx)
	}
}

// findRecoveredInScope checks a query scope for recovered nodes.
func findRecoveredInScope(scope *scaf.QueryScope, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if scope == nil {
		return
	}

	// Check if scope itself was recovered
	if scope.WasRecovered() && positionInRecoverySpan(scope.RecoveredSpan, scope.RecoveredEnd, pos) {
		ctx.RecoveredNode = scope
		ctx.RecoveredTokens = scope.RecoveredTokens
		ctx.QueryScope = scope.QueryName
	}

	// Track query scope context
	if containsPosition(scope.Span(), pos) {
		ctx.QueryScope = scope.QueryName
	}

	// Check setup in scope
	if scope.Setup != nil {
		findRecoveredInSetup(scope.Setup, pos, ctx)
		if containsPosition(scope.Setup.Span(), pos) {
			ctx.InSetup = true
		}
	}

	// Check items
	for _, item := range scope.Items {
		findRecoveredInTestOrGroup(item, pos, ctx)
	}
}

// findRecoveredInTestOrGroup checks a test or group for recovered nodes.
func findRecoveredInTestOrGroup(item *scaf.TestOrGroup, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if item == nil {
		return
	}

	// Check item-level recovery
	if item.WasRecovered() && positionInRecoverySpan(item.RecoveredSpan, item.RecoveredEnd, pos) {
		ctx.RecoveredNode = item
		ctx.RecoveredTokens = item.RecoveredTokens
	}

	if item.Test != nil {
		findRecoveredInTest(item.Test, pos, ctx)
	}

	if item.Group != nil {
		findRecoveredInGroup(item.Group, pos, ctx)
	}
}

// findRecoveredInTest checks a test for recovered nodes.
func findRecoveredInTest(test *scaf.Test, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if test == nil {
		return
	}

	// Track test context
	if containsPosition(test.Span(), pos) {
		ctx.InTest = true
	}

	// Check test-level recovery
	if test.WasRecovered() && positionInRecoverySpan(test.RecoveredSpan, test.RecoveredEnd, pos) {
		ctx.RecoveredNode = test
		ctx.RecoveredTokens = test.RecoveredTokens
		ctx.ParentNode = test
	}

	// Check setup
	if test.Setup != nil {
		findRecoveredInSetup(test.Setup, pos, ctx)
		if containsPosition(test.Setup.Span(), pos) {
			ctx.InSetup = true
		}
	}

	// Check statements
	for _, stmt := range test.Statements {
		if stmt.WasRecovered() && positionInRecoverySpan(stmt.RecoveredSpan, stmt.RecoveredEnd, pos) {
			ctx.RecoveredNode = stmt
			ctx.RecoveredTokens = stmt.RecoveredTokens
		}
	}

	// Check asserts
	for _, assert := range test.Asserts {
		findRecoveredInAssert(assert, pos, ctx)
	}
}

// findRecoveredInGroup checks a group for recovered nodes.
func findRecoveredInGroup(group *scaf.Group, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if group == nil {
		return
	}

	// Check group-level recovery
	if group.WasRecovered() && positionInRecoverySpan(group.RecoveredSpan, group.RecoveredEnd, pos) {
		ctx.RecoveredNode = group
		ctx.RecoveredTokens = group.RecoveredTokens
	}

	// Check setup
	if group.Setup != nil {
		findRecoveredInSetup(group.Setup, pos, ctx)
		if containsPosition(group.Setup.Span(), pos) {
			ctx.InSetup = true
		}
	}

	// Check items
	for _, item := range group.Items {
		findRecoveredInTestOrGroup(item, pos, ctx)
	}
}

// findRecoveredInSetup checks a setup clause for recovered nodes.
func findRecoveredInSetup(setup *scaf.SetupClause, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if setup == nil {
		return
	}

	// Check setup-level recovery
	if setup.WasRecovered() && positionInRecoverySpan(setup.RecoveredSpan, setup.RecoveredEnd, pos) {
		ctx.RecoveredNode = setup
		ctx.RecoveredTokens = setup.RecoveredTokens
		ctx.InSetup = true
	}

	// Check setup call
	if setup.Call != nil && setup.Call.WasRecovered() {
		if positionInRecoverySpan(setup.Call.RecoveredSpan, setup.Call.RecoveredEnd, pos) {
			ctx.RecoveredNode = setup.Call
			ctx.RecoveredTokens = setup.Call.RecoveredTokens
			ctx.InSetup = true
		}
	}

	// Check block items
	for _, item := range setup.Block {
		if item.WasRecovered() && positionInRecoverySpan(item.RecoveredSpan, item.RecoveredEnd, pos) {
			ctx.RecoveredNode = item
			ctx.RecoveredTokens = item.RecoveredTokens
			ctx.InSetup = true
		}
		if item.Call != nil && item.Call.WasRecovered() {
			if positionInRecoverySpan(item.Call.RecoveredSpan, item.Call.RecoveredEnd, pos) {
				ctx.RecoveredNode = item.Call
				ctx.RecoveredTokens = item.Call.RecoveredTokens
				ctx.InSetup = true
			}
		}
	}
}

// findRecoveredInAssert checks an assert for recovered nodes.
func findRecoveredInAssert(assert *scaf.Assert, pos lexer.Position, ctx *RecoveryCompletionContext) {
	if assert == nil {
		return
	}

	// Track assert context
	if containsPosition(assert.Span(), pos) {
		ctx.InAssert = true
	}

	// Check assert-level recovery
	if assert.WasRecovered() && positionInRecoverySpan(assert.RecoveredSpan, assert.RecoveredEnd, pos) {
		ctx.RecoveredNode = assert
		ctx.RecoveredTokens = assert.RecoveredTokens
		ctx.InAssert = true
	}

	// Check assert query
	if assert.Query != nil && assert.Query.WasRecovered() {
		if positionInRecoverySpan(assert.Query.RecoveredSpan, assert.Query.RecoveredEnd, pos) {
			ctx.RecoveredNode = assert.Query
			ctx.RecoveredTokens = assert.Query.RecoveredTokens
			ctx.InAssert = true
		}
	}
}

// analyzeRecoveredTokens determines completion kind from the recovered tokens.
func analyzeRecoveredTokens(ctx *RecoveryCompletionContext, symbols *SymbolTable) {
	if len(ctx.RecoveredTokens) == 0 {
		// No recovered tokens - check node type for context
		ctx.Kind = determineKindFromNode(ctx)
		return
	}

	// Analyze the pattern of recovered tokens
	tokens := ctx.RecoveredTokens

	// Check for "setup <identifier>." pattern (module.function completion)
	if hasSetupDotPattern(tokens, symbols) {
		ctx.Kind = RecoveryCompletionSetupFunction
		ctx.ModuleAlias = extractModuleAlias(tokens)
		ctx.Prefix = extractPrefixAfterDot(tokens)
		return
	}

	// Check for "setup <identifier>" pattern (module alias completion)
	if hasSetupIdentPattern(tokens) {
		ctx.Kind = RecoveryCompletionSetupAlias
		ctx.Prefix = extractSetupPrefix(tokens)
		return
	}

	// Check for "$" or "$<identifier>" pattern (parameter completion)
	if hasParameterPattern(tokens) {
		ctx.Kind = RecoveryCompletionParameter
		ctx.Prefix = extractParameterPrefix(tokens)
		return
	}

	// Check for identifier at start of line in test (return field completion)
	if ctx.InTest && hasReturnFieldPattern(tokens) {
		ctx.Kind = RecoveryCompletionReturnField
		ctx.Prefix = extractIdentPrefix(tokens)
		return
	}

	// Default: keyword completion
	ctx.Kind = RecoveryCompletionKeyword
	ctx.Prefix = extractIdentPrefix(tokens)
}

// determineKindFromNode determines completion kind when no recovered tokens are available.
func determineKindFromNode(ctx *RecoveryCompletionContext) RecoveryCompletionKind {
	switch ctx.RecoveredNode.(type) {
	case *scaf.SetupClause, *scaf.SetupCall:
		return RecoveryCompletionSetupAlias
	case *scaf.Statement:
		if ctx.InTest {
			return RecoveryCompletionReturnField
		}
	}

	if ctx.InTest {
		return RecoveryCompletionKeyword
	}

	return RecoveryCompletionNone
}

// hasSetupDotPattern checks if tokens match "setup <ident>." or just "<ident>." in setup context.
func hasSetupDotPattern(tokens []lexer.Token, symbols *SymbolTable) bool {
	// Look for pattern: [setup?] <ident> <dot>
	for i, tok := range tokens {
		if tok.Type == scaf.TokenDot && i > 0 {
			prevTok := tokens[i-1]
			if prevTok.Type == scaf.TokenIdent {
				// Check if the identifier is an import alias
				if symbols != nil {
					if _, ok := symbols.Imports[prevTok.Value]; ok {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasSetupIdentPattern checks if tokens match "setup <ident>" pattern.
func hasSetupIdentPattern(tokens []lexer.Token) bool {
	for i, tok := range tokens {
		if tok.Type == scaf.TokenSetup {
			// Check if followed by identifier (or we're at the end after setup)
			if i == len(tokens)-1 {
				return true // Just "setup"
			}
			if i+1 < len(tokens) && tokens[i+1].Type == scaf.TokenIdent {
				return true // "setup <ident>"
			}
		}
	}
	return false
}

// hasParameterPattern checks if tokens contain a $ parameter pattern.
func hasParameterPattern(tokens []lexer.Token) bool {
	for _, tok := range tokens {
		if tok.Type == scaf.TokenIdent && len(tok.Value) > 0 && tok.Value[0] == '$' {
			return true
		}
	}
	return false
}

// hasReturnFieldPattern checks if tokens look like a return field (identifier at start).
func hasReturnFieldPattern(tokens []lexer.Token) bool {
	if len(tokens) == 0 {
		return false
	}
	// First non-whitespace token is an identifier (not starting with $)
	for _, tok := range tokens {
		if tok.Type == scaf.TokenWhitespace {
			continue
		}
		if tok.Type == scaf.TokenIdent && (len(tok.Value) == 0 || tok.Value[0] != '$') {
			return true
		}
		return false
	}
	return false
}

// extractModuleAlias extracts the module alias from tokens containing a dot pattern.
func extractModuleAlias(tokens []lexer.Token) string {
	for i, tok := range tokens {
		if tok.Type == scaf.TokenDot && i > 0 {
			prevTok := tokens[i-1]
			if prevTok.Type == scaf.TokenIdent {
				return prevTok.Value
			}
		}
	}
	return ""
}

// extractPrefixAfterDot extracts any identifier typed after the dot.
func extractPrefixAfterDot(tokens []lexer.Token) string {
	foundDot := false
	for _, tok := range tokens {
		if tok.Type == scaf.TokenDot {
			foundDot = true
			continue
		}
		if foundDot && tok.Type == scaf.TokenIdent {
			return tok.Value
		}
	}
	return ""
}

// extractSetupPrefix extracts the identifier prefix after "setup".
func extractSetupPrefix(tokens []lexer.Token) string {
	foundSetup := false
	for _, tok := range tokens {
		if tok.Type == scaf.TokenSetup {
			foundSetup = true
			continue
		}
		if foundSetup && tok.Type == scaf.TokenIdent {
			return tok.Value
		}
	}
	return ""
}

// extractParameterPrefix extracts the $-prefixed identifier.
func extractParameterPrefix(tokens []lexer.Token) string {
	for _, tok := range tokens {
		if tok.Type == scaf.TokenIdent && len(tok.Value) > 0 && tok.Value[0] == '$' {
			return tok.Value
		}
	}
	return ""
}

// extractIdentPrefix extracts the last identifier from tokens.
func extractIdentPrefix(tokens []lexer.Token) string {
	var last string
	for _, tok := range tokens {
		if tok.Type == scaf.TokenIdent {
			last = tok.Value
		}
	}
	return last
}

// positionInRecoverySpan checks if a position falls within a recovery span.
// Returns true if the position is at or after the start and at or before the end.
func positionInRecoverySpan(start, end, pos lexer.Position) bool {
	// Zero spans don't count
	if start.Line == 0 && start.Column == 0 {
		return false
	}

	// Check if pos is after or at start
	if pos.Line < start.Line {
		return false
	}
	if pos.Line == start.Line && pos.Column < start.Column {
		return false
	}

	// If end is zero, only check start
	if end.Line == 0 && end.Column == 0 {
		// Position is at or after start - we're in the recovery area
		return true
	}

	// Check if pos is before or at end
	if pos.Line > end.Line {
		return false
	}
	if pos.Line == end.Line && pos.Column > end.Column {
		return false
	}

	return true
}
