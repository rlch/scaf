package scaf

import (
	"io"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"
)

// Token type constants - negative values as per participle convention.
const (
	tEOF        lexer.TokenType = lexer.EOF
	tComment    lexer.TokenType = -(iota + 2) //nolint:mnd // participle convention
	tRawString                                // backtick strings
	tString                                   // quoted strings
	tNumber                                   // all number formats
	tIdent                                    // identifiers including $-prefixed
	tOp                                       // operators
	tDot                                      // .
	tColon                                    // :
	tComma                                    // ,
	tSemi                                     // ;
	tLParen                                   // (
	tRParen                                   // )
	tLBracket                                 // [
	tRBracket                                 // ]
	tLBrace                                   // {
	tRBrace                                   // }
	tWhitespace                               // spaces, tabs, newlines
)

// Lexer errors.
var (
	ErrUnterminatedRawString = &LexerError{msg: "unterminated raw string"}
	ErrUnterminatedString    = &LexerError{msg: "unterminated string"}
	ErrUnexpectedCharacter   = &LexerError{msg: "unexpected character"}
)

// LexerError represents a lexer error with position.
type LexerError struct {
	msg string
	pos lexer.Position
	ch  rune
}

func (e *LexerError) Error() string {
	if e.ch != 0 {
		return e.pos.String() + ": " + e.msg + ": " + string(e.ch)
	}

	return e.pos.String() + ": " + e.msg
}

func (e *LexerError) withPos(pos lexer.Position) *LexerError {
	return &LexerError{msg: e.msg, pos: pos, ch: e.ch}
}

func (e *LexerError) withChar(ch rune) *LexerError {
	return &LexerError{msg: e.msg, pos: e.pos, ch: ch}
}

// dslDefinition implements lexer.Definition for the scaf DSL.
type dslDefinition struct {
	symbols map[string]lexer.TokenType
	// lastTrivia holds trivia from the most recent lex operation.
	lastTrivia *TriviaList
	// mu protects lastTrivia for concurrent access.
	mu sync.Mutex
}

// newDSLLexer creates a new lexer Definition for the scaf DSL.
func newDSLLexer() *dslDefinition {
	return &dslDefinition{
		symbols: map[string]lexer.TokenType{
			"EOF":        tEOF,
			"Comment":    tComment,
			"RawString":  tRawString,
			"String":     tString,
			"Number":     tNumber,
			"Ident":      tIdent,
			"Op":         tOp,
			"Dot":        tDot,
			"Colon":      tColon,
			"Comma":      tComma,
			"Semi":       tSemi,
			"Whitespace": tWhitespace,
			// Individual bracket tokens for grammar rules
			"(": tLParen,
			")": tRParen,
			"[": tLBracket,
			"]": tRBracket,
			"{": tLBrace,
			"}": tRBrace,
		},
	}
}

// Symbols returns the mapping of symbol names to token types.
func (d *dslDefinition) Symbols() map[string]lexer.TokenType {
	return d.symbols
}

// Lex creates a new Lexer for the given reader.
//
//nolint:ireturn // Required by participle's lexer.Definition interface.
func (d *dslDefinition) Lex(filename string, r io.Reader) (lexer.Lexer, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return d.LexBytes(filename, data)
}

// LexBytes implements lexer.BytesDefinition for efficiency.
//
//nolint:ireturn // Required by participle's lexer.BytesDefinition interface.
func (d *dslDefinition) LexBytes(filename string, data []byte) (lexer.Lexer, error) {
	trivia := &TriviaList{}
	d.lastTrivia = trivia

	return newLexerState(filename, string(data), trivia), nil
}

// LexString implements lexer.StringDefinition for efficiency.
//
//nolint:ireturn // Required by participle's lexer.StringDefinition interface.
func (d *dslDefinition) LexString(filename string, input string) (lexer.Lexer, error) {
	trivia := &TriviaList{}
	d.lastTrivia = trivia

	return newLexerState(filename, input, trivia), nil
}

// Trivia returns the collected trivia from the last lex operation.
// Note: For thread-safe access, use Lock/Unlock around Parse and Trivia calls.
func (d *dslDefinition) Trivia() *TriviaList {
	return d.lastTrivia
}

// Lock acquires the mutex for thread-safe parse+trivia access.
func (d *dslDefinition) Lock() {
	d.mu.Lock()
}

// Unlock releases the mutex.
func (d *dslDefinition) Unlock() {
	d.mu.Unlock()
}

// lexerState holds the state for lexing.
type lexerState struct {
	filename      string
	input         string
	offset        int
	line          int
	col           int
	trivia        *TriviaList
	lastWasNewline bool // tracks if we just saw a blank line (for detached comments)
}

func newLexerState(filename, input string, trivia *TriviaList) *lexerState {
	return &lexerState{
		filename: filename,
		input:    input,
		offset:   0,
		line:     1,
		col:      1,
		trivia:   trivia,
	}
}

// Next returns the next token.
func (l *lexerState) Next() (lexer.Token, error) {
	if l.eof() {
		return lexer.EOFToken(l.pos()), nil
	}

	start := l.pos()
	r := l.peek()

	// Whitespace - track blank lines for "detached" comment detection
	if isSpace(r) {
		newlineCount := 0

		for !l.eof() && isSpace(l.peek()) {
			if l.peek() == '\n' {
				newlineCount++
			}

			l.advance()
		}
		// Two or more newlines means there was a blank line
		l.lastWasNewline = newlineCount >= 2

		return l.token(tWhitespace, start), nil
	}

	// Comment - collect as trivia
	if r == '/' && l.peekAt(1) == '/' {
		for !l.eof() && l.peek() != '\n' {
			l.advance()
		}

		tok := l.token(tComment, start)

		// Record in trivia list
		if l.trivia != nil {
			l.trivia.Add(Trivia{
				Type: TriviaComment,
				Text: tok.Value,
				Span: Span{
					Start: start,
					End:   l.pos(),
				},
				HasNewlineBefore: l.lastWasNewline,
			})
		}

		l.lastWasNewline = false

		return tok, nil
	}

	// Reset blank line tracker for non-trivia tokens
	l.lastWasNewline = false

	// Raw string
	if r == '`' {
		return l.scanRawString(start)
	}

	// String
	if r == '"' || r == '\'' {
		return l.scanString(start, r)
	}

	// Number
	if isDigit(r) {
		return l.scanNumber(start), nil
	}

	// Identifier
	if isIdentStart(r) {
		l.advance() // consume first char

		for !l.eof() && isIdentContinue(l.peek()) {
			l.advance()
		}

		return l.token(tIdent, start), nil
	}

	// Multi-character operators (check before single-char)
	if tok, ok := l.scanMultiCharOp(start); ok {
		return tok, nil
	}

	// Single character tokens
	l.advance()

	switch r {
	case '.':
		return l.token(tDot, start), nil
	case ':':
		return l.token(tColon, start), nil
	case ',':
		return l.token(tComma, start), nil
	case ';':
		return l.token(tSemi, start), nil
	case '(':
		return l.token(tLParen, start), nil
	case ')':
		return l.token(tRParen, start), nil
	case '[':
		return l.token(tLBracket, start), nil
	case ']':
		return l.token(tRBracket, start), nil
	case '{':
		return l.token(tLBrace, start), nil
	case '}':
		return l.token(tRBrace, start), nil
	}

	// Single-character operators
	if strings.ContainsRune("+-*/%^&|!<>=?#~", r) {
		return l.token(tOp, start), nil
	}

	return lexer.Token{}, ErrUnexpectedCharacter.withPos(start).withChar(r)
}

func (l *lexerState) pos() lexer.Position {
	return lexer.Position{
		Filename: l.filename,
		Offset:   l.offset,
		Line:     l.line,
		Column:   l.col,
	}
}

func (l *lexerState) eof() bool {
	return l.offset >= len(l.input)
}

func (l *lexerState) peek() rune {
	if l.eof() {
		return 0
	}

	r, _ := utf8.DecodeRuneInString(l.input[l.offset:])

	return r
}

//nolint:unparam // n is always 1 currently but kept for flexibility.
func (l *lexerState) peekAt(n int) rune {
	off := l.offset + n
	if off >= len(l.input) {
		return 0
	}

	r, _ := utf8.DecodeRuneInString(l.input[off:])

	return r
}

//nolint:unparam // Return value useful for debugging.
func (l *lexerState) advance() rune {
	if l.eof() {
		return 0
	}

	r, size := utf8.DecodeRuneInString(l.input[l.offset:])
	l.offset += size

	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}

	return r
}

func (l *lexerState) match(s string) bool {
	return strings.HasPrefix(l.input[l.offset:], s)
}

func (l *lexerState) token(typ lexer.TokenType, start lexer.Position) lexer.Token {
	return lexer.Token{
		Type:  typ,
		Value: l.input[start.Offset:l.offset],
		Pos:   start,
	}
}

func (l *lexerState) scanRawString(start lexer.Position) (lexer.Token, error) {
	l.advance() // opening `

	for !l.eof() {
		if l.peek() == '`' {
			l.advance() // closing `

			return l.token(tRawString, start), nil
		}

		l.advance()
	}

	return lexer.Token{}, ErrUnterminatedRawString.withPos(start)
}

func (l *lexerState) scanString(start lexer.Position, quote rune) (lexer.Token, error) {
	l.advance() // opening quote

	for !l.eof() {
		ch := l.peek()
		if ch == '\\' && l.peekAt(1) != 0 {
			l.advance() // backslash
			l.advance() // escaped char

			continue
		}

		if ch == quote {
			l.advance() // closing quote

			return l.token(tString, start), nil
		}

		if ch == '\n' {
			return lexer.Token{}, ErrUnterminatedString.withPos(start)
		}

		l.advance()
	}

	return lexer.Token{}, ErrUnterminatedString.withPos(start)
}

func (l *lexerState) scanMultiCharOp(start lexer.Position) (lexer.Token, bool) {
	multiOps := []string{"&&", "||", "==", "!=", "<=", ">=", "!~", "?.", "..", "?:", "::", "##"}

	for _, op := range multiOps {
		if l.match(op) {
			for range len(op) {
				l.advance()
			}

			return l.token(tOp, start), true
		}
	}

	return lexer.Token{}, false
}

func (l *lexerState) scanNumber(start lexer.Position) lexer.Token {
	// Check for hex, octal, binary
	if l.peek() == '0' && l.peekAt(1) != 0 {
		next := l.peekAt(1)

		switch next {
		case 'x', 'X':
			l.advance() // 0
			l.advance() // x

			for !l.eof() && (isHexDigit(l.peek()) || l.peek() == '_') {
				l.advance()
			}

			return l.token(tNumber, start)

		case 'o', 'O':
			l.advance() // 0
			l.advance() // o

			for !l.eof() && (isOctalDigit(l.peek()) || l.peek() == '_') {
				l.advance()
			}

			return l.token(tNumber, start)

		case 'b', 'B':
			l.advance() // 0
			l.advance() // b

			for !l.eof() && (l.peek() == '0' || l.peek() == '1' || l.peek() == '_') {
				l.advance()
			}

			return l.token(tNumber, start)
		}
	}

	// Decimal digits
	for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
		l.advance()
	}

	// Fractional part
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		l.advance() // .

		for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
			l.advance()
		}
	}

	// Exponent
	if l.peek() == 'e' || l.peek() == 'E' {
		l.advance() // e/E

		if l.peek() == '+' || l.peek() == '-' {
			l.advance()
		}

		for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
			l.advance()
		}
	}

	return l.token(tNumber, start)
}

// Character helpers.

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isHexDigit(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isOctalDigit(r rune) bool {
	return r >= '0' && r <= '7'
}

func isIdentStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r)
}

func isIdentContinue(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
