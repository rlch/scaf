package scaf_test

import (
	"strings"
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

func TestLexer_Symbols(t *testing.T) {
	t.Parallel()

	def := scaf.ExportedLexer()
	symbols := def.Symbols()

	expected := []string{
		"EOF", "Comment", "RawString", "String", "Number", "Ident", "Op",
		"Dot", "Colon", "Comma", "Semi", "Whitespace",
		"(", ")", "[", "]", "{", "}",
	}

	for _, name := range expected {
		if _, ok := symbols[name]; !ok {
			t.Errorf("missing symbol: %s", name)
		}
	}
}

type tokenExpect struct {
	typ string
	val string
}

func lexTokens(t *testing.T, input string) []tokenExpect {
	t.Helper()

	def := scaf.ExportedLexer()
	symbols := def.Symbols()

	symbolNames := make(map[lexer.TokenType]string)
	for name, typ := range symbols {
		symbolNames[typ] = name
	}

	lex, err := def.Lex("", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Lex() error: %v", err)
	}

	var tokens []tokenExpect

	for {
		tok, err := lex.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}

		if tok.EOF() {
			break
		}

		// Skip whitespace
		if symbolNames[tok.Type] == "Whitespace" {
			continue
		}

		tokens = append(tokens, tokenExpect{
			typ: symbolNames[tok.Type],
			val: tok.Value,
		})
	}

	return tokens
}

func TestLexer_Identifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"foo", []tokenExpect{{"Ident", "foo"}}},
		{"foo_bar", []tokenExpect{{"Ident", "foo_bar"}}},
		{"foo123", []tokenExpect{{"Ident", "foo123"}}},
		{"$userId", []tokenExpect{{"Ident", "$userId"}}},
		{"_private", []tokenExpect{{"Ident", "_private"}}},
		{"foo bar", []tokenExpect{{"Ident", "foo"}, {"Ident", "bar"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_Numbers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"123", []tokenExpect{{"Number", "123"}}},
		{"123.456", []tokenExpect{{"Number", "123.456"}}},
		{"1e10", []tokenExpect{{"Number", "1e10"}}},
		{"1E10", []tokenExpect{{"Number", "1E10"}}},
		{"1.5e-3", []tokenExpect{{"Number", "1.5e-3"}}},
		{"1.5e+3", []tokenExpect{{"Number", "1.5e+3"}}},
		{"1_000_000", []tokenExpect{{"Number", "1_000_000"}}},
		{"0xFF", []tokenExpect{{"Number", "0xFF"}}},
		{"0XFF", []tokenExpect{{"Number", "0XFF"}}},
		{"0xFF_FF", []tokenExpect{{"Number", "0xFF_FF"}}},
		{"0o755", []tokenExpect{{"Number", "0o755"}}},
		{"0O755", []tokenExpect{{"Number", "0O755"}}},
		{"0b1010", []tokenExpect{{"Number", "0b1010"}}},
		{"0B1010", []tokenExpect{{"Number", "0B1010"}}},
		{"0", []tokenExpect{{"Number", "0"}}},
		{"0.5", []tokenExpect{{"Number", "0.5"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_Strings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{`"hello"`, []tokenExpect{{"String", `"hello"`}}},
		{`'hello'`, []tokenExpect{{"String", `'hello'`}}},
		{`"hello\"world"`, []tokenExpect{{"String", `"hello\"world"`}}},
		{`""`, []tokenExpect{{"String", `""`}}},
		{`"escape\\test"`, []tokenExpect{{"String", `"escape\\test"`}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_RawStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"`hello`", []tokenExpect{{"RawString", "`hello`"}}},
		{"`hello \"world\"`", []tokenExpect{{"RawString", "`hello \"world\"`"}}},
		{"`line1\nline2`", []tokenExpect{{"RawString", "`line1\nline2`"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_MultiCharOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"&&", []tokenExpect{{"Op", "&&"}}},
		{"||", []tokenExpect{{"Op", "||"}}},
		{"==", []tokenExpect{{"Op", "=="}}},
		{"!=", []tokenExpect{{"Op", "!="}}},
		{"<=", []tokenExpect{{"Op", "<="}}},
		{">=", []tokenExpect{{"Op", ">="}}},
		{"!~", []tokenExpect{{"Op", "!~"}}},
		{"?.", []tokenExpect{{"Op", "?."}}},
		{"?:", []tokenExpect{{"Op", "?:"}}},
		{"::", []tokenExpect{{"Op", "::"}}},
		{"##", []tokenExpect{{"Op", "##"}}},
		{"..", []tokenExpect{{"Op", ".."}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_SingleCharOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"+", []tokenExpect{{"Op", "+"}}},
		{"-", []tokenExpect{{"Op", "-"}}},
		{"*", []tokenExpect{{"Op", "*"}}},
		{"/", []tokenExpect{{"Op", "/"}}},
		{"%", []tokenExpect{{"Op", "%"}}},
		{"<", []tokenExpect{{"Op", "<"}}},
		{">", []tokenExpect{{"Op", ">"}}},
		{"!", []tokenExpect{{"Op", "!"}}},
		{"^", []tokenExpect{{"Op", "^"}}},
		{"&", []tokenExpect{{"Op", "&"}}},
		{"|", []tokenExpect{{"Op", "|"}}},
		{"=", []tokenExpect{{"Op", "="}}},
		{"?", []tokenExpect{{"Op", "?"}}},
		{"#", []tokenExpect{{"Op", "#"}}},
		{"~", []tokenExpect{{"Op", "~"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_Punctuation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{".", []tokenExpect{{"Dot", "."}}},
		{":", []tokenExpect{{"Colon", ":"}}},
		{",", []tokenExpect{{"Comma", ","}}},
		{";", []tokenExpect{{"Semi", ";"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_Brackets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []tokenExpect
	}{
		{"()", []tokenExpect{{"(", "("}, {")", ")"}}},
		{"[]", []tokenExpect{{"[", "["}, {"]", "]"}}},
		{"{}", []tokenExpect{{"{", "{"}, {"}", "}"}}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_Comments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []tokenExpect
	}{
		{"comment before token", "// comment\nfoo", []tokenExpect{{"Comment", "// comment"}, {"Ident", "foo"}}},
		{"comment only", "// just a comment", []tokenExpect{{"Comment", "// just a comment"}}},
		{"empty comment", "//\nfoo", []tokenExpect{{"Comment", "//"}, {"Ident", "foo"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_ComplexExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []tokenExpect
	}{
		{
			"dotted identifier",
			"u.name",
			[]tokenExpect{{"Ident", "u"}, {"Dot", "."}, {"Ident", "name"}},
		},
		{
			"key-value pair",
			"$id: 123",
			[]tokenExpect{{"Ident", "$id"}, {"Colon", ":"}, {"Number", "123"}},
		},
		{
			"function call",
			"len(items)",
			[]tokenExpect{{"Ident", "len"}, {"(", "("}, {"Ident", "items"}, {")", ")"}},
		},
		{
			"comparison",
			"x > 0 && y < 10",
			[]tokenExpect{
				{"Ident", "x"}, {"Op", ">"}, {"Number", "0"},
				{"Op", "&&"},
				{"Ident", "y"}, {"Op", "<"}, {"Number", "10"},
			},
		},
		{
			"array access",
			"items[0]",
			[]tokenExpect{{"Ident", "items"}, {"[", "["}, {"Number", "0"}, {"]", "]"}},
		},
		{
			"map literal",
			"{a: 1, b: 2}",
			[]tokenExpect{
				{"{", "{"}, {"Ident", "a"}, {"Colon", ":"}, {"Number", "1"},
				{"Comma", ","},
				{"Ident", "b"}, {"Colon", ":"}, {"Number", "2"}, {"}", "}"},
			},
		},
		{
			"assertion",
			"assert { c == 1 }",
			[]tokenExpect{
				{"assert", "assert"}, {"{", "{"},
				{"Ident", "c"}, {"Op", "=="}, {"Number", "1"},
				{"}", "}"},
			},
		},
		{
			"semicolon separated",
			"x > 0; y < 10",
			[]tokenExpect{
				{"Ident", "x"}, {"Op", ">"}, {"Number", "0"},
				{"Semi", ";"},
				{"Ident", "y"}, {"Op", "<"}, {"Number", "10"},
			},
		},
		{
			"query definition",
			"query GetUser `MATCH (u:User) RETURN u`",
			[]tokenExpect{
				{"query", "query"}, {"Ident", "GetUser"},
				{"RawString", "`MATCH (u:User) RETURN u`"},
			},
		},
		{
			"test definition",
			`test "finds user" { }`,
			[]tokenExpect{
				{"test", "test"}, {"String", `"finds user"`},
				{"{", "{"}, {"}", "}"},
			},
		},
		{
			"chained dots",
			"a.b.c.d",
			[]tokenExpect{
				{"Ident", "a"}, {"Dot", "."},
				{"Ident", "b"}, {"Dot", "."},
				{"Ident", "c"}, {"Dot", "."},
				{"Ident", "d"},
			},
		},
		{
			"optional chain and range",
			"a?.b..c",
			[]tokenExpect{
				{"Ident", "a"}, {"Op", "?."},
				{"Ident", "b"}, {"Op", ".."},
				{"Ident", "c"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lexTokens(t, tt.input)
			assertTokens(t, tt.expected, got)
		})
	}
}

func TestLexer_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		got := lexTokens(t, "")
		if len(got) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(got))
		}
	})

	t.Run("only whitespace", func(t *testing.T) {
		t.Parallel()

		got := lexTokens(t, "   \t\n  ")
		if len(got) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(got))
		}
	})

	t.Run("eof after single char", func(t *testing.T) {
		t.Parallel()

		// Test that advancing at EOF doesn't panic - single char will be consumed, then EOF
		got := lexTokens(t, "x")
		assertTokens(t, []tokenExpect{{"Ident", "x"}}, got)
	})

	t.Run("peek at eof", func(t *testing.T) {
		t.Parallel()

		// Empty input triggers peek() at EOF
		got := lexTokens(t, "")
		if len(got) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(got))
		}
	})
}

func TestLexer_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"unterminated double string", `"hello`},
		{"unterminated single string", `'hello`},
		{"unterminated raw string", "`hello"},
		{"string with newline", "\"hello\nworld\""},
		{"unexpected character", "@"},
	}

	def := scaf.ExportedLexer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lex, err := def.Lex("", strings.NewReader(tt.input))
			if err != nil {
				return // error on Lex is acceptable
			}

			var gotError bool

			for {
				tok, err := lex.Next()
				if err != nil {
					gotError = true

					break
				}

				if tok.EOF() {
					break
				}
			}

			if !gotError {
				t.Errorf("expected error for input %q", tt.input)
			}
		})
	}
}

func TestLexer_Positions(t *testing.T) {
	t.Parallel()

	input := "foo\nbar baz"
	def := scaf.ExportedLexer()

	lex, err := def.Lex("test.scaf", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Lex() error: %v", err)
	}

	expected := []struct {
		val    string
		line   int
		column int
	}{
		{"foo", 1, 1},
		{"bar", 2, 1},
		{"baz", 2, 5},
	}

	symbols := def.Symbols()

	symbolNames := make(map[lexer.TokenType]string)
	for name, typ := range symbols {
		symbolNames[typ] = name
	}

	idx := 0

	for {
		tok, err := lex.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}

		if tok.EOF() {
			break
		}

		if symbolNames[tok.Type] == "Whitespace" {
			continue
		}

		if idx >= len(expected) {
			t.Fatalf("unexpected token: %v", tok)
		}

		exp := expected[idx]
		if tok.Value != exp.val {
			t.Errorf("token[%d].Value = %q, want %q", idx, tok.Value, exp.val)
		}

		if tok.Pos.Line != exp.line {
			t.Errorf("token[%d].Pos.Line = %d, want %d", idx, tok.Pos.Line, exp.line)
		}

		if tok.Pos.Column != exp.column {
			t.Errorf("token[%d].Pos.Column = %d, want %d", idx, tok.Pos.Column, exp.column)
		}

		idx++
	}
}

func TestLexer_LexBytes(t *testing.T) {
	t.Parallel()

	def := scaf.ExportedLexer()

	lex, err := def.LexBytes("test.scaf", []byte("foo bar"))
	if err != nil {
		t.Fatalf("LexBytes() error: %v", err)
	}

	var tokens []string

	symbols := def.Symbols()

	symbolNames := make(map[lexer.TokenType]string)
	for name, typ := range symbols {
		symbolNames[typ] = name
	}

	for {
		tok, err := lex.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}

		if tok.EOF() {
			break
		}

		if symbolNames[tok.Type] != "Whitespace" {
			tokens = append(tokens, tok.Value)
		}
	}

	if len(tokens) != 2 || tokens[0] != "foo" || tokens[1] != "bar" {
		t.Errorf("LexBytes() tokens = %v, want [foo bar]", tokens)
	}
}

func TestLexer_LexString(t *testing.T) {
	t.Parallel()

	def := scaf.ExportedLexer()

	lex, err := def.LexString("test.scaf", "foo bar")
	if err != nil {
		t.Fatalf("LexString() error: %v", err)
	}

	var tokens []string

	symbols := def.Symbols()

	symbolNames := make(map[lexer.TokenType]string)
	for name, typ := range symbols {
		symbolNames[typ] = name
	}

	for {
		tok, err := lex.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}

		if tok.EOF() {
			break
		}

		if symbolNames[tok.Type] != "Whitespace" {
			tokens = append(tokens, tok.Value)
		}
	}

	if len(tokens) != 2 || tokens[0] != "foo" || tokens[1] != "bar" {
		t.Errorf("LexString() tokens = %v, want [foo bar]", tokens)
	}
}

func TestLexerError(t *testing.T) {
	t.Parallel()

	// Test the error formatting with character
	def := scaf.ExportedLexer()

	lex, err := def.Lex("test.scaf", strings.NewReader("@"))
	if err != nil {
		t.Fatalf("Lex() error: %v", err)
	}

	_, err = lex.Next()
	if err == nil {
		t.Fatal("expected error for @")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "unexpected character") {
		t.Errorf("error = %q, want to contain 'unexpected character'", errStr)
	}

	if !strings.Contains(errStr, "@") {
		t.Errorf("error = %q, want to contain '@'", errStr)
	}
}

func TestLexerError_NoChar(t *testing.T) {
	t.Parallel()

	// Test the error formatting without character (unterminated string)
	def := scaf.ExportedLexer()

	lex, err := def.Lex("test.scaf", strings.NewReader(`"unterminated`))
	if err != nil {
		t.Fatalf("Lex() error: %v", err)
	}

	_, err = lex.Next()
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "unterminated string") {
		t.Errorf("error = %q, want to contain 'unterminated string'", errStr)
	}
}

type failingReader struct {
	err error
}

func (f *failingReader) Read(_ []byte) (int, error) {
	return 0, f.err
}

func TestLexer_LexReaderError(t *testing.T) {
	t.Parallel()

	def := scaf.ExportedLexer()

	// Use a reader that returns an error
	testErr := &testError{msg: "test read error"}

	_, err := def.Lex("test.scaf", &failingReader{err: testErr})
	if err == nil {
		t.Error("expected error from failing reader")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func assertTokens(t *testing.T, expected, got []tokenExpect) {
	t.Helper()

	if len(got) != len(expected) {
		t.Errorf("token count = %d, want %d", len(got), len(expected))
		t.Errorf("got: %v", got)

		return
	}

	for i, exp := range expected {
		if got[i].typ != exp.typ {
			t.Errorf("token[%d].typ = %q, want %q", i, got[i].typ, exp.typ)
		}

		if got[i].val != exp.val {
			t.Errorf("token[%d].val = %q, want %q", i, got[i].val, exp.val)
		}
	}
}

func TestLexer_TriviaCollection(t *testing.T) {
	// Not parallel - trivia state is shared across lexer calls
	tests := []struct {
		name             string
		input            string
		expectedComments []string
	}{
		{
			name:             "single comment",
			input:            "// comment\nfoo",
			expectedComments: []string{"// comment"},
		},
		{
			name:             "multiple comments",
			input:            "// first\n// second\nfoo",
			expectedComments: []string{"// first", "// second"},
		},
		{
			name:             "trailing comment",
			input:            "foo // trailing",
			expectedComments: []string{"// trailing"},
		},
		{
			name:             "no comments",
			input:            "foo bar",
			expectedComments: nil,
		},
		{
			name:             "comment with blank line before",
			input:            "foo\n\n// detached comment\nbar",
			expectedComments: []string{"// detached comment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel - trivia state is shared
			def := scaf.ExportedLexer()

			lex, err := def.Lex("", strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Lex() error: %v", err)
			}

			// Consume all tokens
			for {
				tok, err := lex.Next()
				if err != nil {
					t.Fatalf("Next() error: %v", err)
				}

				if tok.EOF() {
					break
				}
			}

			// Check collected trivia
			trivia := def.Trivia()
			comments := trivia.All()

			if len(comments) != len(tt.expectedComments) {
				t.Fatalf("comment count = %d, want %d", len(comments), len(tt.expectedComments))
			}

			for i, exp := range tt.expectedComments {
				if comments[i].Text != exp {
					t.Errorf("comment[%d] = %q, want %q", i, comments[i].Text, exp)
				}

				if comments[i].Type != scaf.TriviaComment {
					t.Errorf("comment[%d].Type = %v, want TriviaComment", i, comments[i].Type)
				}
			}
		})
	}
}

func TestLexer_TriviaDetachedComments(t *testing.T) {
	// Not parallel - trivia state is shared across lexer calls
	input := `foo

// This is detached (blank line before)
bar`

	def := scaf.ExportedLexer()

	lex, err := def.Lex("", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Lex() error: %v", err)
	}

	// Consume all tokens
	for {
		tok, err := lex.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}

		if tok.EOF() {
			break
		}
	}

	trivia := def.Trivia()
	comments := trivia.All()

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if !comments[0].HasNewlineBefore {
		t.Error("expected HasNewlineBefore to be true for detached comment")
	}
}
