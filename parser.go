package scaf

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

var dslLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "RawString", Pattern: "`[^`]*`"},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Float", Pattern: `[-+]?\d+\.\d+`},
	{Name: "Int", Pattern: `[-+]?\d+`},
	{Name: "Ident", Pattern: `\$?[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "Punct", Pattern: `[-!()+/*,><>={}:.\[\];]`},
	{Name: "Whitespace", Pattern: `[ \t\n\r]+`},
})

var parser = participle.MustBuild[Suite](
	participle.Lexer(dslLexer),
	participle.Unquote("RawString", "String"),
	participle.Elide("Whitespace", "Comment"),
)

// Parse parses a scaf DSL file and returns the resulting Suite AST.
func Parse(data []byte) (*Suite, error) {
	return parser.ParseBytes("", data)
}
