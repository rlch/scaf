// Package scaf provides a DSL parser for database test scaffolding.
package scaf

import (
	"fmt"
	"strconv"
	"strings"
)

// Suite represents a complete test file with queries, setup, teardown, and test scopes.
type Suite struct {
	Imports  []*Import     `parser:"@@*"`
	Queries  []*Query      `parser:"@@*"`
	Setup    *SetupClause  `parser:"('setup' @@)?"`
	Teardown *string       `parser:"('teardown' @RawString)?"`
	Scopes   []*QueryScope `parser:"@@*"`
}

// Import represents a module import statement.
// Examples:
//
//	import "../../setup/lesson_plan_db"
//	import fixtures "../shared/fixtures"
type Import struct {
	Alias *string `parser:"'import' @Ident?"`
	Path  string  `parser:"@String"`
}

// Query defines a named database query.
type Query struct {
	Name string `parser:"'query' @Ident"`
	Body string `parser:"@RawString"`
}

// SetupClause represents either an inline setup query or a named setup reference.
// Examples:
//
//	setup `CREATE (:User)`                              // inline
//	setup lesson_plan_db.SetupLessonPlanDB()            // named, no params
//	setup fixtures.CreatePosts($n: 10, $authorId: 1)    // named with params
type SetupClause struct {
	Inline *string     `parser:"@RawString"`
	Named  *NamedSetup `parser:"| @@"`
}

// NamedSetup references a setup defined elsewhere (local or imported).
// Examples:
//
//	SetupLessonPlanDB()
//	module.SetupName($param: value)
type NamedSetup struct {
	Module *string       `parser:"(@Ident '.')?"`
	Name   string        `parser:"@Ident '('"`
	Params []*SetupParam `parser:"@@? (',' @@)* ')'"`
}

// SetupParam is a parameter passed to a named setup.
type SetupParam struct {
	Name  string `parser:"@Ident ':'"`
	Value *Value `parser:"@@"`
}

// QueryScope groups tests that target a specific query.
type QueryScope struct {
	QueryName string         `parser:"@Ident '{'"`
	Setup     *SetupClause   `parser:"('setup' @@)?"`
	Teardown  *string        `parser:"('teardown' @RawString)?"`
	Items     []*TestOrGroup `parser:"@@* '}'"`
}

// TestOrGroup is a union type - either a Test or a Group.
type TestOrGroup struct {
	Test  *Test  `parser:"@@"`
	Group *Group `parser:"| @@"`
}

// Group organizes related tests with optional shared setup and teardown.
type Group struct {
	Name     string         `parser:"'group' @String '{'"`
	Setup    *SetupClause   `parser:"('setup' @@)?"`
	Teardown *string        `parser:"('teardown' @RawString)?"`
	Items    []*TestOrGroup `parser:"@@* '}'"`
}

// Test defines a single test case with inputs, expected outputs, and optional assertions.
// Tests run in a transaction that rolls back after execution, so no teardown is needed.
type Test struct {
	Name       string       `parser:"'test' @String '{'"`
	Setup      *SetupClause `parser:"('setup' @@)?"`
	Statements []*Statement `parser:"@@*"`
	Assertion  *Assertion   `parser:"('assert' @@)?"`
	Close      string       `parser:"'}'"`
}

// Statement represents a key-value pair for inputs ($var), expected outputs, or computed fields.
// Examples:
//
//	$userId: 1                                    // input parameter
//	u.name: "Alice"                               // expected output (equality)
//	u: { cronLastExecutedAt: u.createdAt + duration("24h") }  // computed field for mocks
type Statement struct {
	Key   string `parser:"@Ident (@'.' @Ident)*"`
	Value *Value `parser:"':' @@"`
}

// Assertion defines a post-execution query and its expected results.
// This is the legacy assertion format using equality checks.
// TODO: Add support for expr-based assertions (assert <expr>;)
type Assertion struct {
	Query        string       `parser:"@RawString '{'"`
	Expectations []*Statement `parser:"@@* '}'"`
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
	Null    bool     `parser:"@'null'"`
	Str     *string  `parser:"| @String"`
	Number  *float64 `parser:"| @Float | @Int"`
	Boolean *Boolean `parser:"| @('true' | 'false')"`
	Map     *Map     `parser:"| @@"`
	List    *List    `parser:"| @@"`
}

// Map represents a key-value map literal.
type Map struct {
	Entries []*MapEntry `parser:"'{' @@? (',' @@)* '}'"`
}

// MapEntry represents a single entry in a map literal.
type MapEntry struct {
	Key   string `parser:"@Ident ':'"`
	Value *Value `parser:"@@"`
}

// List represents an array/list literal.
type List struct {
	Values []*Value `parser:"'[' @@? (',' @@)* ']'"`
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
