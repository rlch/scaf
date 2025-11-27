package analysis_test

import (
	"testing"

	"github.com/alecthomas/participle/v2/lexer"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

func TestPrevTokenAtPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		line      int // 1-based (participle convention)
		col       int // 1-based
		wantValue string
		wantType  lexer.TokenType
	}{
		{
			name:      "after query keyword",
			input:     "query GetUser `MATCH (u) RETURN u`\n",
			line:      1,
			col:       7, // right after "query " (at position of 'G' in GetUser)
			wantValue: "query",
			wantType:  scaf.TokenQuery,
		},
		{
			name:      "after query name",
			input:     "query GetUser `MATCH (u) RETURN u`\n",
			line:      1,
			col:       15, // right after "GetUser " (at position of backtick)
			wantValue: "GetUser",
			wantType:  scaf.TokenIdent,
		},
		{
			name: "after setup keyword with inline query",
			input: "query Q `Q`\nQ {\n\tsetup `CREATE (n)`\n}\n",
			line:      3,
			col:       8, // right after "setup " (at position of backtick)
			wantValue: "setup",
			wantType:  scaf.TokenSetup,
		},
		{
			name: "after dot in module reference",
			input: "import fixtures \"./fixtures\"\nquery Q `Q`\nQ {\n\tsetup fixtures.CreateUser()\n}\n",
			line:      4,
			col:       17, // right after "fixtures." (at position of 'C')
			wantValue: ".",
			wantType:  scaf.TokenDot,
		},
		{
			name:      "after import keyword",
			input:     "import fixtures \"./fixtures\"\n",
			line:      1,
			col:       8, // right after "import " (at position of 'f')
			wantValue: "import",
			wantType:  scaf.TokenImport,
		},
		{
			name: "after test keyword",
			input: "query Q `Q`\nQ {\n\ttest \"my test\" {\n\t}\n}\n",
			line:      3,
			col:       7, // right after "test " (at position of quote)
			wantValue: "test",
			wantType:  scaf.TokenTest,
		},
		{
			name: "after group keyword",
			input: "query Q `Q`\nQ {\n\tgroup \"my group\" {\n\t}\n}\n",
			line:      3,
			col:       8, // right after "group " (at position of quote)
			wantValue: "group",
			wantType:  scaf.TokenGroup,
		},
		{
			name: "after assert keyword",
			input: "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\tassert { true }\n\t}\n}\n",
			line:      4,
			col:       10, // right after "assert " (at position of '{')
			wantValue: "assert",
			wantType:  scaf.TokenAssert,
		},
		{
			name: "after open brace in test",
			input: "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\t$id: 1\n\t}\n}\n",
			line:      4,
			col:       3, // at start of "$id" line
			wantValue: "{",
			wantType:  scaf.TokenLBrace,
		},
		{
			name: "after colon in statement",
			input: "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\t$id: 1\n\t}\n}\n",
			line:      4,
			col:       8, // right after ": " (at position of '1')
			wantValue: ":",
			wantType:  scaf.TokenColon,
		},
		{
			name: "after parameter name",
			input: "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\t$id: 1\n\t}\n}\n",
			line:      4,
			col:       6, // right after "$id" (at position of ':')
			wantValue: "$id",
			wantType:  scaf.TokenIdent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := analysis.NewAnalyzer(nil)
			f := analyzer.Analyze("test.scaf", []byte(tt.input))

			if f.Suite == nil {
				t.Fatalf("Failed to parse input: %v", f.ParseError)
			}

			pos := lexer.Position{Line: tt.line, Column: tt.col}
			tok := analysis.PrevTokenAtPosition(f, pos)

			if tok == nil {
				t.Fatalf("PrevTokenAtPosition returned nil, want token %q", tt.wantValue)
			}

			if tok.Value != tt.wantValue {
				t.Errorf("PrevTokenAtPosition value = %q, want %q", tok.Value, tt.wantValue)
			}

			if tok.Type != tt.wantType {
				t.Errorf("PrevTokenAtPosition type = %v, want %v", tok.Type, tt.wantType)
			}
		})
	}
}

func TestTokenAtPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		line      int // 1-based
		col       int // 1-based
		wantValue string
		wantType  lexer.TokenType
	}{
		{
			name:      "on query keyword",
			input:     "query GetUser `MATCH (u) RETURN u`\n",
			line:      1,
			col:       3, // middle of "query"
			wantValue: "query",
			wantType:  scaf.TokenQuery,
		},
		{
			name:      "on identifier",
			input:     "query GetUser `MATCH (u) RETURN u`\n",
			line:      1,
			col:       10, // middle of "GetUser"
			wantValue: "GetUser",
			wantType:  scaf.TokenIdent,
		},
		{
			name:      "on setup keyword",
			input:     "query Q `Q`\nQ {\n\tsetup `CREATE (n)`\n}\n",
			line:      3,
			col:       4, // on "setup"
			wantValue: "setup",
			wantType:  scaf.TokenSetup,
		},
		{
			name:      "on dot",
			input:     "import fixtures \"./fixtures\"\nquery Q `Q`\nQ {\n\tsetup fixtures.CreateUser()\n}\n",
			line:      4,
			col:       16, // on the dot
			wantValue: ".",
			wantType:  scaf.TokenDot,
		},
		{
			name:      "on parameter",
			input:     "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\t$userId: 1\n\t}\n}\n",
			line:      4,
			col:       5, // on "$userId"
			wantValue: "$userId",
			wantType:  scaf.TokenIdent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := analysis.NewAnalyzer(nil)
			f := analyzer.Analyze("test.scaf", []byte(tt.input))

			if f.Suite == nil {
				t.Fatalf("Failed to parse input: %v", f.ParseError)
			}

			pos := lexer.Position{Line: tt.line, Column: tt.col}
			tok := analysis.TokenAtPosition(f, pos)

			if tok == nil {
				t.Fatalf("TokenAtPosition returned nil, want token %q", tt.wantValue)
			}

			if tok.Value != tt.wantValue {
				t.Errorf("TokenAtPosition value = %q, want %q", tok.Value, tt.wantValue)
			}

			if tok.Type != tt.wantType {
				t.Errorf("TokenAtPosition type = %v, want %v", tok.Type, tt.wantType)
			}
		})
	}
}

func TestGetTokenContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		line           int // 1-based
		col            int // 1-based
		wantInSetup    bool
		wantInTest     bool
		wantInGroup    bool
		wantInAssert   bool
		wantQueryScope string
		wantPrevValue  string // expected previous token value (empty if nil expected)
	}{
		{
			name:          "top level after query keyword",
			input:         "query GetUser `Q`\n",
			line:          1,
			col:           7, // right after "query "
			wantPrevValue: "query",
		},
		{
			name:           "inside scope with setup",
			input:          "query Q `Q`\nQ {\n\tsetup `CREATE (n)`\n}\n",
			line:           3,
			col:            8, // right after "setup "
			wantInSetup:    true,
			wantQueryScope: "Q",
			wantPrevValue:  "setup",
		},
		{
			name:           "inside test body",
			input:          "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\t$id: 1\n\t}\n}\n",
			line:           4,
			col:            3, // at start of "$id" line
			wantInTest:     true,
			wantQueryScope: "Q",
			wantPrevValue:  "{",
		},
		{
			name:           "inside test setup",
			input:          "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\tsetup `CREATE (n)`\n\t}\n}\n",
			line:           4,
			col:            9, // right after "setup "
			wantInTest:     true,
			wantInSetup:    true,
			wantQueryScope: "Q",
			wantPrevValue:  "setup",
		},
		{
			name:           "inside group but not in test",
			input:          "query Q `Q`\nQ {\n\tgroup \"g\" {\n\t\ttest \"t\" {}\n\t}\n}\n",
			line:           3,
			col:            15, // inside group, on the { after "g"
			wantInGroup:    true,
			wantInTest:     false,
			wantQueryScope: "Q",
		},
		{
			name:           "inside assert",
			input:          "query Q `Q`\nQ {\n\ttest \"t\" {\n\t\tassert { true }\n\t}\n}\n",
			line:           4,
			col:            12, // inside the assert block
			wantInTest:     true,
			wantInAssert:   true,
			wantQueryScope: "Q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := analysis.NewAnalyzer(nil)
			f := analyzer.Analyze("test.scaf", []byte(tt.input))

			if f.Suite == nil {
				t.Fatalf("Failed to parse input: %v", f.ParseError)
			}

			pos := lexer.Position{Line: tt.line, Column: tt.col}
			ctx := analysis.GetTokenContext(f, pos)

			if ctx.InSetup != tt.wantInSetup {
				t.Errorf("InSetup = %v, want %v", ctx.InSetup, tt.wantInSetup)
			}
			if ctx.InTest != tt.wantInTest {
				t.Errorf("InTest = %v, want %v", ctx.InTest, tt.wantInTest)
			}
			if ctx.InGroup != tt.wantInGroup {
				t.Errorf("InGroup = %v, want %v", ctx.InGroup, tt.wantInGroup)
			}
			if ctx.InAssert != tt.wantInAssert {
				t.Errorf("InAssert = %v, want %v", ctx.InAssert, tt.wantInAssert)
			}
			if ctx.QueryScope != tt.wantQueryScope {
				t.Errorf("QueryScope = %q, want %q", ctx.QueryScope, tt.wantQueryScope)
			}

			if tt.wantPrevValue != "" {
				if ctx.PrevToken == nil {
					t.Errorf("PrevToken = nil, want %q", tt.wantPrevValue)
				} else if ctx.PrevToken.Value != tt.wantPrevValue {
					t.Errorf("PrevToken.Value = %q, want %q", ctx.PrevToken.Value, tt.wantPrevValue)
				}
			}
		})
	}
}
