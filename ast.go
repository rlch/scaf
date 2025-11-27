// Package scaf provides a DSL parser for database test scaffolding.
package scaf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// Node is the interface implemented by all AST nodes.
// It provides access to position information for error reporting and formatting.
type Node interface {
	Span() Span
}

// CompletableNode is implemented by AST nodes that can detect incomplete syntax
// for providing intelligent completions during typing.
type CompletableNode interface {
	Node
	// IsComplete returns true if the node is syntactically complete.
	// Incomplete nodes (e.g., "setup fixtures." without function name) are used
	// to provide context-aware completions.
	IsComplete() bool
}

// Suite represents a complete test file with queries, setup, teardown, and test scopes.
type Suite struct {
	Pos      lexer.Position `parser:""`
	EndPos   lexer.Position `parser:""`
	Tokens   []lexer.Token  `parser:""`
	Imports  []*Import      `parser:"@@*"`
	Queries  []*Query       `parser:"@@*"`
	Setup    *SetupClause   `parser:"('setup' @@)?"`
	Teardown *string        `parser:"('teardown' @RawString)?"`
	Scopes   []*QueryScope  `parser:"@@*"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`
}

// Span returns the source span of this node.
func (s *Suite) Span() Span { return Span{Start: s.Pos, End: s.EndPos} }

// Import represents a module import statement.
// Examples:
//
//	import "../../setup/lesson_plan_db"
//	import fixtures "../shared/fixtures"
type Import struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Alias  *string        `parser:"'import' @Ident?"`
	Path   string         `parser:"@String"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`
}

// Span returns the source span of this node.
func (i *Import) Span() Span { return Span{Start: i.Pos, End: i.EndPos} }

// Query defines a named database query.
type Query struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Name   string         `parser:"'query' @Ident"`
	Body   string         `parser:"@RawString"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`
}

// Span returns the source span of this node.
func (q *Query) Span() Span { return Span{Start: q.Pos, End: q.EndPos} }

// SetupClause represents a setup: inline query, named reference, or block of items.
// Examples:
//
//	setup `CREATE (:User)`                              // inline
//	setup lesson_plan_db.SetupLessonPlanDB()            // named, no params
//	setup fixtures.CreatePosts($n: 10, $authorId: 1)    // named with params
//	setup { `CREATE (:User)`; SetupUsers() }            // block with multiple items
type SetupClause struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Inline *string        `parser:"@RawString"`
	Named  *NamedSetup    `parser:"| @@"`
	Block  []*SetupItem   `parser:"| '{' @@* '}'"`

	// Recovery metadata - populated when parsing recovers from an error.
	// These fields are automatically populated by participle when recovery succeeds.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (s *SetupClause) Span() Span { return Span{Start: s.Pos, End: s.EndPos} }

// IsComplete returns true if the setup clause has content.
func (s *SetupClause) IsComplete() bool {
	return s.Inline != nil || s.Named != nil || len(s.Block) > 0
}

// SetupItem represents a single item in a setup block.
// Can be either an inline query or a named setup call.
type SetupItem struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Inline *string        `parser:"@RawString"`
	Named  *NamedSetup    `parser:"| @@"`
}

// NamedSetup references a setup defined elsewhere (local or imported).
// Examples:
//
//	SetupLessonPlanDB()
//	module.SetupName($param: value)
type NamedSetup struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Module *string        `parser:"(@Ident Dot)?"`
	Name   string         `parser:"@Ident '('"`
	Params []*SetupParam  `parser:"(@@ (Comma @@)*)? ')'"`

	// Recovery metadata - populated when parsing recovers from an error.
	// These fields are automatically populated by participle when recovery succeeds.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (n *NamedSetup) Span() Span { return Span{Start: n.Pos, End: n.EndPos} }

// IsComplete returns true if the named setup has all required parts.
// For NamedSetup, if it parsed successfully, it's complete.
func (n *NamedSetup) IsComplete() bool {
	return n.Name != ""
}

// SetupParam is a parameter passed to a named setup.
type SetupParam struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Name   string         `parser:"@Ident Colon"`
	Value  *ParamValue    `parser:"@@"`
}

// ParamValue represents a value in a parameter - either a literal or a field reference.
// Field references allow passing result values to assert queries.
// Examples:
//
//	$userId: 1           // literal
//	$authorId: u.id      // field reference from parent scope
//
// Note: Literal must come first to match keywords (true, false, null) before they're
// captured as identifiers by FieldRef.
type ParamValue struct {
	Pos      lexer.Position `parser:""`
	EndPos   lexer.Position `parser:""`
	Tokens   []lexer.Token  `parser:""`
	Literal  *Value         `parser:"@@"`
	FieldRef *DottedIdent   `parser:"| @@"`
}

// ToGo converts a ParamValue to a native Go type.
// For field refs, returns nil - caller must resolve from scope.
func (p *ParamValue) ToGo() any {
	if p.Literal != nil {
		return p.Literal.ToGo()
	}

	return nil // Field ref - must be resolved by runner
}

// IsFieldRef returns true if this is a field reference.
func (p *ParamValue) IsFieldRef() bool {
	return p.FieldRef != nil && p.Literal == nil
}

// FieldRefString returns the field reference as a string, or empty if not a field ref.
func (p *ParamValue) FieldRefString() string {
	if p.FieldRef != nil {
		return p.FieldRef.String()
	}

	return ""
}

// String returns a string representation of the ParamValue.
func (p *ParamValue) String() string {
	if p.FieldRef != nil {
		return p.FieldRef.String()
	}

	if p.Literal != nil {
		return p.Literal.String()
	}

	return ""
}

// QueryScope groups tests that target a specific query.
type QueryScope struct {
	Pos       lexer.Position `parser:""`
	EndPos    lexer.Position `parser:""`
	Tokens    []lexer.Token  `parser:""`
	QueryName string         `parser:"@Ident '{'"`
	Setup     *SetupClause   `parser:"('setup' @@)?"`
	Teardown  *string        `parser:"('teardown' @RawString)?"`
	Items     []*TestOrGroup `parser:"@@*"`
	Close     string         `parser:"@'}'"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`

	// Recovery metadata - populated when parsing recovers from an error.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (q *QueryScope) Span() Span { return Span{Start: q.Pos, End: q.EndPos} }

// IsComplete returns true if the query scope has a closing brace.
func (q *QueryScope) IsComplete() bool {
	return q.Close != ""
}

// TestOrGroup is a union type - either a Test or a Group.
type TestOrGroup struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Test   *Test          `parser:"@@"`
	Group  *Group         `parser:"| @@"`
}

// Span returns the source span of this node.
func (t *TestOrGroup) Span() Span { return Span{Start: t.Pos, End: t.EndPos} }

// Group organizes related tests with optional shared setup and teardown.
type Group struct {
	Pos      lexer.Position `parser:""`
	EndPos   lexer.Position `parser:""`
	Tokens   []lexer.Token  `parser:""`
	Name     string         `parser:"'group' @String '{'"`
	Setup    *SetupClause   `parser:"('setup' @@)?"`
	Teardown *string        `parser:"('teardown' @RawString)?"`
	Items    []*TestOrGroup `parser:"@@*"`
	Close    string         `parser:"@'}'"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`

	// Recovery metadata - populated when parsing recovers from an error.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (g *Group) Span() Span { return Span{Start: g.Pos, End: g.EndPos} }

// IsComplete returns true if the group has a closing brace.
func (g *Group) IsComplete() bool {
	return g.Close != ""
}

// Test defines a single test case with inputs, expected outputs, and optional assertions.
// Tests run in a transaction that rolls back after execution, so no teardown is needed.
type Test struct {
	Pos        lexer.Position `parser:""`
	EndPos     lexer.Position `parser:""`
	Tokens     []lexer.Token  `parser:""`
	Name       string         `parser:"'test' @String '{'"`
	Setup      *SetupClause   `parser:"('setup' @@)?"`
	Statements []*Statement   `parser:"@@*"`
	Asserts    []*Assert      `parser:"@@*"`
	Close      string         `parser:"@'}'"`

	// Comments attached to this node (populated after parsing).
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`

	// Recovery metadata - populated when parsing recovers from an error.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (t *Test) Span() Span { return Span{Start: t.Pos, End: t.EndPos} }

// IsComplete returns true if the test has a closing brace.
func (t *Test) IsComplete() bool {
	return t.Close != ""
}

// Assert represents an assertion block with optional query.
// Expressions are captured as tokens and reconstructed as strings for expr.Compile().
// Examples:
//
//	assert { u.age > 18 }                                    // standalone expr
//	assert { x > 0; y < 10; z == 5 }                         // multiple exprs
//	assert CreatePost($title: "x") { p.title == "x" }        // named query with conditions
//	assert `MATCH (n) RETURN count(n) as cnt` { cnt > 0 }    // inline query with conditions
type Assert struct {
	Pos        lexer.Position `parser:""`
	EndPos     lexer.Position `parser:""`
	Tokens     []lexer.Token  `parser:""`
	Query      *AssertQuery   `parser:"'assert' @@? '{'"`
	Conditions []*Expr        `parser:"(@@ Semi?)*"`
	Close      string         `parser:"@'}'"`

	// Recovery metadata - populated when parsing recovers from an error.
	Recovered     bool           `parser:""` // True if this node was recovered
	RecoveredSpan lexer.Position `parser:""` // Position where recovery started
	RecoveredEnd  lexer.Position `parser:""` // Position where recovery ended
	SkippedTokens []lexer.Token  `parser:""` // Tokens skipped during recovery
}

// Span returns the source span of this node.
func (a *Assert) Span() Span { return Span{Start: a.Pos, End: a.EndPos} }

// IsComplete returns true if the assert has a closing brace.
func (a *Assert) IsComplete() bool {
	return a.Close != ""
}

// AssertQuery specifies the query to run before evaluating conditions.
// Either an inline raw string query or a named query reference with params.
type AssertQuery struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	// Inline query (raw string)
	Inline *string `parser:"@RawString"`
	// Or named query reference with required parentheses
	QueryName *string       `parser:"| @Ident '('"`
	Params    []*SetupParam `parser:"(@@ (Comma @@)*)? ')'"`
}

// Span returns the source span of this node.
func (a *AssertQuery) Span() Span { return Span{Start: a.Pos, End: a.EndPos} }

// Expr captures tokens for expr-lang evaluation.
// Tokens are reconstructed into a string and parsed by expr.Compile() at runtime.
type Expr struct {
	Pos        lexer.Position `parser:""`
	EndPos     lexer.Position `parser:""`
	RawTokens  []lexer.Token  `parser:""`
	ExprTokens []*ExprToken   `parser:"@@+"`
}

// String reconstructs the expression as a string for expr-lang.
func (e *Expr) String() string {
	if e == nil || len(e.ExprTokens) == 0 {
		return ""
	}

	var b strings.Builder

	for i, tok := range e.ExprTokens {
		if i > 0 {
			prev := e.ExprTokens[i-1]
			// Add space between tokens except:
			// - around dots (u.name)
			// - after open brackets (foo(x), arr[0])
			// - before close brackets (foo(x), arr[0])
			// - between identifier and open bracket (function calls: len(x))
			// - after comma (we add space after comma below)
			needsSpace := !prev.IsDot() && !prev.IsOpenBracket() && !prev.Comma &&
				!tok.IsDot() && !tok.IsCloseBracket() &&
				(!prev.IsIdent() || !tok.IsOpenBracket())
			if needsSpace {
				b.WriteByte(' ')
			}
		}

		b.WriteString(tok.String())
		// Add space after comma
		if tok.Comma {
			b.WriteByte(' ')
		}
	}

	return b.String()
}

// ExprToken captures individual tokens that can appear in expressions.
// Matches expr-lang's token kinds: Identifier, Number, String, Operator, Bracket.
// Note: { } ; are NOT captured as they're expression delimiters.
type ExprToken struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Str    *string        `parser:"@String"`
	Number *string        `parser:"| @Number"`
	Ident  *string        `parser:"| @Ident"`
	Op     *string        `parser:"| @Op"`
	Dot    bool           `parser:"| @Dot"`
	Colon  bool           `parser:"| @Colon"`
	Comma  bool           `parser:"| @Comma"`
	LParen bool           `parser:"| @'('"`
	RParen bool           `parser:"| @')'"`
	LBrack bool           `parser:"| @'['"`
	RBrack bool           `parser:"| @']'"`
}

// String returns the string representation of a token.
func (t *ExprToken) String() string {
	switch {
	case t.Str != nil:
		return fmt.Sprintf("%q", *t.Str)
	case t.Number != nil:
		return *t.Number
	case t.Ident != nil:
		return *t.Ident
	case t.Op != nil:
		return *t.Op
	case t.Dot:
		return "."
	case t.Colon:
		return ":"
	case t.Comma:
		return ","
	case t.LParen:
		return "("
	case t.RParen:
		return ")"
	case t.LBrack:
		return "["
	case t.RBrack:
		return "]"
	default:
		return ""
	}
}

// IsDot returns true if this token is a dot.
func (t *ExprToken) IsDot() bool {
	return t.Dot
}

// IsOpenBracket returns true if this token is an opening bracket.
func (t *ExprToken) IsOpenBracket() bool {
	return t.LParen || t.LBrack
}

// IsCloseBracket returns true if this token is a closing bracket.
func (t *ExprToken) IsCloseBracket() bool {
	return t.RParen || t.RBrack
}

// IsIdent returns true if this token is an identifier.
func (t *ExprToken) IsIdent() bool {
	return t.Ident != nil
}

// DottedIdent represents a dot-separated identifier like "u.name" or "$userId".
type DottedIdent struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Parts  []string       `parser:"@Ident (Dot @Ident)*"`
}

// String returns the dot-joined identifier.
func (d *DottedIdent) String() string {
	return strings.Join(d.Parts, ".")
}

// Statement represents a key-value pair for inputs ($var) or expected outputs.
// Examples:
//
//	$userId: 1                                    // input parameter
//	u.name: "Alice"                               // expected output (equality)
type Statement struct {
	Pos      lexer.Position `parser:""`
	EndPos   lexer.Position `parser:""`
	Tokens   []lexer.Token  `parser:""`
	KeyParts *DottedIdent   `parser:"@@"`
	Value    *Value         `parser:"Colon @@"`
}

// Span returns the source span of this node.
func (s *Statement) Span() Span { return Span{Start: s.Pos, End: s.EndPos} }

// Key returns the statement key as a dot-joined string.
func (s *Statement) Key() string {
	if s.KeyParts == nil {
		return ""
	}

	return s.KeyParts.String()
}

// NewStatement creates a Statement from a dot-separated key string and value.
// This is a convenience constructor for testing and programmatic AST construction.
//
//nolint:funcorder
func NewStatement(key string, value *Value) *Statement {
	parts := strings.Split(key, ".")

	return &Statement{
		KeyParts: &DottedIdent{Parts: parts},
		Value:    value,
	}
}

// Boolean is a bool type that implements participle's Capture interface.
type Boolean bool

// Capture implements participle's Capture interface for Boolean.
func (b *Boolean) Capture(values []string) error {
	*b = values[0] == "true"

	return nil
}

// Value represents a literal value (string, number, bool, null, map, or list).
type Value struct {
	Pos     lexer.Position `parser:""`
	EndPos  lexer.Position `parser:""`
	Tokens  []lexer.Token  `parser:""`
	Null    bool           `parser:"@'null'"`
	Str     *string        `parser:"| @String"`
	Number  *float64       `parser:"| @Number"`
	Boolean *Boolean       `parser:"| @('true' | 'false')"`
	Map     *Map           `parser:"| @@"`
	List    *List          `parser:"| @@"`
}

// Map represents a key-value map literal.
type Map struct {
	Pos     lexer.Position `parser:""`
	EndPos  lexer.Position `parser:""`
	Tokens  []lexer.Token  `parser:""`
	Entries []*MapEntry    `parser:"'{' (@@ (Comma @@)*)? '}'"`
}

// MapEntry represents a single entry in a map literal.
type MapEntry struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Key    string         `parser:"@Ident Colon"`
	Value  *Value         `parser:"@@"`
}

// List represents an array/list literal.
type List struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
	Values []*Value       `parser:"'[' (@@ (Comma @@)*)? ']'"`
}

// ToGo converts a Value to a native Go type.
func (v *Value) ToGo() any {
	switch {
	case v.Null:
		return nil
	case v.Str != nil:
		return *v.Str
	case v.Number != nil:
		return *v.Number
	case v.Boolean != nil:
		return bool(*v.Boolean)
	case v.Map != nil:
		m := make(map[string]any)
		for _, e := range v.Map.Entries {
			m[e.Key] = e.Value.ToGo()
		}

		return m
	case v.List != nil:
		l := make([]any, len(v.List.Values))
		for i, val := range v.List.Values {
			l[i] = val.ToGo()
		}

		return l
	default:
		return nil
	}
}

// String returns a string representation of the Value.
func (v *Value) String() string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return fmt.Sprintf("%q", *v.Str)
	case v.Number != nil:
		return fmt.Sprintf("%v", *v.Number)
	case v.Boolean != nil:
		return strconv.FormatBool(bool(*v.Boolean))
	case v.Map != nil:
		return v.mapString()
	case v.List != nil:
		return v.listString()
	default:
		return "nil"
	}
}

func (v *Value) mapString() string {
	parts := make([]string, len(v.Map.Entries))
	for i, e := range v.Map.Entries {
		parts[i] = fmt.Sprintf("%s: %s", e.Key, e.Value)
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

func (v *Value) listString() string {
	parts := make([]string, len(v.List.Values))
	for i, val := range v.List.Values {
		parts[i] = val.String()
	}

	return "[" + strings.Join(parts, ", ") + "]"
}