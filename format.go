package scaf

import (
	"strconv"
	"strings"
)

// Format formats a Suite AST back into scaf DSL source code.
func Format(s *Suite) string {
	var b strings.Builder

	f := &formatter{b: &b, indent: 0}
	f.formatSuite(s)

	return strings.TrimSpace(b.String()) + "\n"
}

type formatter struct {
	b      *strings.Builder
	indent int
}

func (f *formatter) write(s string) {
	f.b.WriteString(s)
}

func (f *formatter) writeLine(s string) {
	f.writeIndent()
	f.write(s)
	f.write("\n")
}

func (f *formatter) writeIndent() {
	for range f.indent {
		f.write("\t")
	}
}

func (f *formatter) blankLine() {
	f.write("\n")
}

func (f *formatter) formatSuite(s *Suite) {
	// Queries
	for i, q := range s.Queries {
		if i > 0 {
			f.blankLine()
		}

		f.formatQuery(q)
	}

	// Global setup
	if s.Setup != nil {
		if len(s.Queries) > 0 {
			f.blankLine()
		}

		f.formatSetup(*s.Setup)
	}

	// Global teardown
	if s.Teardown != nil {
		f.formatTeardown(*s.Teardown)
	}

	// Scopes
	for i, scope := range s.Scopes {
		if i > 0 || len(s.Queries) > 0 || s.Setup != nil || s.Teardown != nil {
			f.blankLine()
		}

		f.formatScope(scope)
	}
}

func (f *formatter) formatQuery(q *Query) {
	f.writeLine("query " + q.Name + " " + f.rawString(q.Body))
}

func (f *formatter) formatSetup(body string) {
	f.writeLine("setup " + f.rawString(body))
}

func (f *formatter) formatTeardown(body string) {
	f.writeLine("teardown " + f.rawString(body))
}

func (f *formatter) formatScope(s *QueryScope) {
	f.writeLine(s.QueryName + " {")
	f.indent++

	if s.Setup != nil {
		f.formatSetup(*s.Setup)
	}

	if s.Teardown != nil {
		f.formatTeardown(*s.Teardown)
	}

	f.formatItems(s.Items, s.Setup != nil || s.Teardown != nil)

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatItems(items []*TestOrGroup, hasSetupOrTeardown bool) {
	for i, item := range items {
		needsBlank := i > 0 || hasSetupOrTeardown

		if item.Test != nil {
			if needsBlank {
				f.blankLine()
			}

			f.formatTest(item.Test)
		} else if item.Group != nil {
			if needsBlank {
				f.blankLine()
			}

			f.formatGroup(item.Group)
		}
	}
}

func (f *formatter) formatGroup(g *Group) {
	f.writeLine("group " + f.quotedString(g.Name) + " {")
	f.indent++

	if g.Setup != nil {
		f.formatSetup(*g.Setup)
	}

	if g.Teardown != nil {
		f.formatTeardown(*g.Teardown)
	}

	f.formatItems(g.Items, g.Setup != nil || g.Teardown != nil)

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatTest(t *Test) {
	f.writeLine("test " + f.quotedString(t.Name) + " {")
	f.indent++

	if t.Setup != nil {
		f.formatSetup(*t.Setup)
	}

	// Separate inputs from outputs
	var inputs, outputs []*Statement

	for _, stmt := range t.Statements {
		if strings.HasPrefix(stmt.Key, "$") {
			inputs = append(inputs, stmt)
		} else {
			outputs = append(outputs, stmt)
		}
	}

	// Format inputs
	for i, stmt := range inputs {
		if i == 0 && t.Setup != nil {
			f.blankLine()
		}

		f.formatStatement(stmt)
	}

	// Format outputs with blank line separator from inputs
	for i, stmt := range outputs {
		if i == 0 && len(inputs) > 0 {
			f.blankLine()
		}

		f.formatStatement(stmt)
	}

	// Assertion
	if t.Assertion != nil {
		if len(t.Statements) > 0 || t.Setup != nil {
			f.blankLine()
		}

		f.formatAssertion(t.Assertion)
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatStatement(s *Statement) {
	f.writeLine(s.Key + ": " + f.formatValue(s.Value))
}

func (f *formatter) formatAssertion(a *Assertion) {
	f.writeLine("assert " + f.rawString(a.Query) + " {")
	f.indent++

	for _, exp := range a.Expectations {
		f.formatStatement(exp)
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatValue(v *Value) string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return f.quotedString(*v.Str)
	case v.Number != nil:
		return f.formatNumber(*v.Number)
	case v.Boolean != nil:
		return strconv.FormatBool(bool(*v.Boolean))
	case v.Map != nil:
		return f.formatMap(v.Map)
	case v.List != nil:
		return f.formatList(v.List)
	default:
		return "null"
	}
}

func (f *formatter) formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}

	return strconv.FormatFloat(n, 'f', -1, 64)
}

func (f *formatter) formatMap(m *Map) string {
	if len(m.Entries) == 0 {
		return "{}"
	}

	parts := make([]string, len(m.Entries))
	for i, e := range m.Entries {
		parts[i] = e.Key + ": " + f.formatValue(e.Value)
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

func (f *formatter) formatList(l *List) string {
	if len(l.Values) == 0 {
		return "[]"
	}

	parts := make([]string, len(l.Values))
	for i, v := range l.Values {
		parts[i] = f.formatValue(v)
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

func (f *formatter) rawString(s string) string {
	return "`" + s + "`"
}

func (f *formatter) quotedString(s string) string {
	return `"` + s + `"`
}
