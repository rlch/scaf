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

// Parse parses a scaf DSL file and returns the AST with comments attached to nodes.
// This function is thread-safe.
func Parse(data []byte) (*Suite, error) {
	// Lock to ensure trivia isn't overwritten by concurrent parses
	dslLexer.Lock()
	defer dslLexer.Unlock()

	suite, err := parser.ParseBytes("", data)
	if err != nil {
		return nil, err
	}

	attachComments(suite, dslLexer.Trivia())

	return suite, nil
}

// ExportedLexer returns the lexer definition for testing purposes.
//
//nolint:revive // unexported-return: intentionally returns unexported type for internal test use
func ExportedLexer() *dslDefinition {
	return dslLexer
}
