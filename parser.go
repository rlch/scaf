package scaf

import (
	"github.com/alecthomas/participle/v2"
)

// dslLexer is the custom lexer for the scaf DSL.
// Implements lexer.Definition interface for full control over tokenization.
var dslLexer = newDSLLexer()

var parser = participle.MustBuild[Suite](
	participle.Lexer(dslLexer),
	participle.Unquote("RawString", "String"),
	participle.Elide("Whitespace", "Comment"),
)

// defaultRecoveryStrategies returns the recovery strategies for parsing scaf files.
//
// The strategies are tried in order:
// 1. Skip to statement/block delimiters (}, test, group, query, import, setup, teardown, assert)
// 2. Skip past semicolons (for inline statements)
func defaultRecoveryStrategies() []participle.RecoveryStrategy {
	return []participle.RecoveryStrategy{
		// Skip to common statement terminators and keywords that start new constructs
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
	}
}

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
			participle.Recover(defaultRecoveryStrategies()...),
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
