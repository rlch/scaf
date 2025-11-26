// Package cyphergrammar provides an ANTLR-generated parser for Cypher queries.
//
// This package contains the lexer and parser generated from CypherLexer.g4 and
// CypherParser.g4, which implement the openCypher query language grammar.
//
// # Regenerating the Parser
//
// To regenerate the Go files from the grammar files, you need ANTLR 4.13.2+:
//
//	cd dialects/cypher/grammar
//	java -jar antlr-4.13.2-complete.jar -Dlanguage=Go -package cyphergrammar CypherLexer.g4 CypherParser.g4
//
// Note: After regeneration, ensure the package declaration is `cyphergrammar`
// (not `cypher`) in all generated .go files.
//
// # Grammar Origin
//
// The grammar is based on the openCypher grammar specification, originally from:
// https://github.com/zhguchev/cypher-antlr4
// Licensed under BSD-3-Clause license.
package cyphergrammar
