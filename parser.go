package scaf

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// dslLexer is the custom lexer for the scaf DSL.
// Implements lexer.Definition interface for full control over tokenization.
var dslLexer = newDSLLexer()

var parser = participle.MustBuild[Suite](
	participle.Lexer(dslLexer),
	participle.Unquote("RawString", "String"),
	participle.Elide("Whitespace", "Comment"),
)

// Parse parses a scaf DSL file and returns the AST with comments attached to nodes.
// This function is thread-safe.
//
// On parse errors, returns a partial AST containing everything successfully parsed
// up to the error location, along with the error. Callers should use the partial
// AST for features like completion and hover even when errors are present.
func Parse(data []byte) (*Suite, error) {
	return ParseWithRecovery(data, false)
}

// ParseWithRecovery parses a scaf DSL file with optional error recovery.
// When withRecovery is true, the parser will attempt to continue parsing after
// encountering errors, collecting multiple errors and producing a more complete
// partial AST. This is useful for LSP features where you want maximum information
// even from invalid files.
//
// Note: Error recovery is experimental and may not work correctly with all grammar
// constructs. Use Parse() for normal parsing.
func ParseWithRecovery(data []byte, withRecovery bool) (*Suite, error) {
	// Lock to ensure trivia isn't overwritten by concurrent parses
	dslLexer.Lock()
	defer dslLexer.Unlock()

	var suite *Suite
	var err error

	if withRecovery {
		suite, err = parser.ParseBytes("", data,
			participle.Recover(
				// Type-specific recovery strategies (tried first for their target types)
				// These create partial AST nodes with Recovered=true for incomplete syntax
				participle.ViaParser(recoverSetupClause),
				// Note: recoverNamedSetup is NOT registered here because NamedSetup is only
				// parsed as part of SetupClause, and recoverSetupClause handles the parent.

				// Skip to common statement terminators and keywords that start new constructs
				// This is the fallback recovery strategy - skip tokens until we find
				// a synchronization point (keyword or brace)
				participle.SkipUntil(
					"}", // Block closer
					"test",
					"group",
					"query",
					"import",
					"setup",
					"teardown",
					"assert",
				),
				// Handle nested braces in setup blocks, tests, etc.
				participle.NestedDelimiters("{", "}"),
				// Handle parentheses in function calls like fixtures.CreateUser()
				participle.NestedDelimiters("(", ")"),
			),
			participle.MaxRecoveryErrors(50),
		)
	} else {
		suite, err = parser.ParseBytes("", data)
	}

	// Attach comments even to partial ASTs - Participle populates as much
	// of the AST as possible before the error location
	if suite != nil {
		attachComments(suite, dslLexer.Trivia())
	}

	return suite, err
}

// ExportedLexer returns the lexer definition for testing purposes.
//
//nolint:revive // unexported-return: intentionally returns unexported type for internal test use
func ExportedLexer() *dslDefinition {
	return dslLexer
}

// =============================================================================
// ViaParser Recovery Functions
// =============================================================================

// recoverSetupClause attempts to recover from incomplete setup clause syntax.
// Handles patterns like:
//   - "setup " (empty - waiting for content)
//   - "setup }" (empty setup before closing brace)
//   - "setup fixtures." (incomplete module reference)
//   - "setup fixtures.Func(" (incomplete function call)
//
// Returns a partial SetupClause with Recovered=true, or NextMatch if recovery fails.
func recoverSetupClause(lex *lexer.PeekingLexer) (*SetupClause, error) {
	startPos := lex.Peek().Pos

	// We're positioned after "setup" keyword (consumed by grammar before recovery triggered)
	// Check what's next to determine the incomplete pattern

	tok := lex.Peek()

	// Empty setup - next token is EOF, closing brace, or a keyword that starts a new construct
	if tok.EOF() || tok.Type == TokenRBrace || isRecoveryKeyword(tok.Type) {
		// Don't consume the token - let the parent grammar handle it
		return &SetupClause{
			Pos:       startPos,
			EndPos:    startPos,
			Recovered: true,
		}, nil
	}

	// Check for identifier (could be module name or function name)
	if tok.Type == TokenIdent {
		moduleName := tok.Value
		lex.Next()

		// Check for dot (module.function pattern)
		if dot := lex.Peek(); dot.Type == TokenDot {
			lex.Next() // consume dot

			result := &SetupClause{
				Pos:    startPos,
				EndPos: lex.Peek().Pos,
				Named: &NamedSetup{
					Pos:       startPos,
					Module:    &moduleName,
					Recovered: true,
				},
				Recovered: true,
			}

			// Check for partial function name
			if funcTok := lex.Peek(); funcTok.Type == TokenIdent {
				result.Named.Name = funcTok.Value
				lex.Next()

				// Check for open paren (incomplete call)
				if paren := lex.Peek(); paren.Type == TokenLParen {
					lex.Next() // consume (

					// Try to consume any params and close paren
					// Skip until we hit ) or a sync token
					depth := 1
					for depth > 0 && !lex.Peek().EOF() {
						next := lex.Peek()
						if next.Type == TokenLParen {
							depth++
						} else if next.Type == TokenRParen {
							depth--
						}
						if depth > 0 {
							lex.Next()
						}
					}
					if lex.Peek().Type == TokenRParen {
						lex.Next() // consume )
					}
				}
			}
			result.EndPos = lex.Peek().Pos
			return result, nil
		}

		// Just an identifier with no dot - could be a local function call
		// Create partial named setup
		return &SetupClause{
			Pos:    startPos,
			EndPos: lex.Peek().Pos,
			Named: &NamedSetup{
				Pos:       startPos,
				Name:      moduleName,
				Recovered: true,
			},
			Recovered: true,
		}, nil
	}

	// Couldn't parse anything meaningful - signal to try next recovery strategy
	return nil, participle.NextMatch
}

// recoverNamedSetup attempts to recover from incomplete named setup syntax.
// Similar to recoverSetupClause but for the NamedSetup type specifically.
//
// IMPORTANT: This function MUST return NextMatch if it cannot consume any tokens,
// otherwise participle will panic with "branch was accepted but did not progress the lexer".
func recoverNamedSetup(lex *lexer.PeekingLexer) (*NamedSetup, error) {
	tok := lex.Peek()

	// Must be at an identifier to recover - if not, signal no match
	// This is critical to avoid the "did not progress lexer" panic
	if tok.EOF() || tok.Type != TokenIdent {
		return nil, participle.NextMatch
	}

	startPos := tok.Pos
	firstIdent := tok.Value
	lex.Next() // consume the identifier - we MUST progress the lexer from here

	// Check for dot (module.function pattern)
	if dot := lex.Peek(); dot.Type == TokenDot {
		lex.Next() // consume dot

		result := &NamedSetup{
			Pos:       startPos,
			Module:    &firstIdent,
			Recovered: true,
		}

		// Check for function name
		if funcTok := lex.Peek(); funcTok.Type == TokenIdent {
			result.Name = funcTok.Value
			lex.Next()
		}

		result.EndPos = lex.Peek().Pos
		return result, nil
	}

	// No dot - this is a local function name
	result := &NamedSetup{
		Pos:       startPos,
		Name:      firstIdent,
		Recovered: true,
	}

	// Check for open paren
	if paren := lex.Peek(); paren.Type == TokenLParen {
		lex.Next() // consume (

		// Skip params until ) or sync token
		depth := 1
		for depth > 0 && !lex.Peek().EOF() {
			next := lex.Peek()
			if next.Type == TokenLParen {
				depth++
			} else if next.Type == TokenRParen {
				depth--
			}
			if depth > 0 {
				lex.Next()
			}
		}
		if lex.Peek().Type == TokenRParen {
			lex.Next() // consume )
		}
	}

	result.EndPos = lex.Peek().Pos
	return result, nil
}

// isRecoveryKeyword returns true if the token type is a keyword that starts a new construct.
// Used by recovery functions to detect when we've reached the start of a new statement
// and should stop trying to recover the current incomplete construct.
func isRecoveryKeyword(typ lexer.TokenType) bool {
	switch typ {
	case TokenTest, TokenGroup, TokenQuery, TokenImport, TokenSetup, TokenTeardown, TokenAssert:
		return true
	default:
		return false
	}
}
