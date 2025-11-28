package scaf

import (
	"strconv"
	"strings"
)

// Format formats a Suite AST back into scaf DSL source code, preserving comments.
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

// writeLeadingComments writes any leading comments.
func (f *formatter) writeLeadingComments(leading []string) {
	for _, comment := range leading {
		f.writeLine(comment)
	}
}

// writeTrailingComment appends a trailing comment to the current line if one exists.
func (f *formatter) writeTrailingComment(trailing string) {
	if trailing != "" {
		f.write(" " + trailing)
	}
}

func (f *formatter) formatSuite(s *Suite) {
	// Leading comments for the whole file
	f.writeLeadingComments(s.LeadingComments)

	// Imports
	for _, imp := range s.Imports {
		f.formatImport(imp)
	}

	// Queries
	for i, q := range s.Queries {
		if i > 0 || len(s.Imports) > 0 {
			f.blankLine()
		}

		f.formatQuery(q)
	}

	// Global setup
	if s.Setup != nil {
		if len(s.Queries) > 0 || len(s.Imports) > 0 {
			f.blankLine()
		}

		f.formatSetupClause(s.Setup)
	}

	// Global teardown
	if s.Teardown != nil {
		f.formatTeardown(*s.Teardown)
	}

	// Scopes
	for i, scope := range s.Scopes {
		if i > 0 || len(s.Queries) > 0 || len(s.Imports) > 0 || s.Setup != nil || s.Teardown != nil {
			f.blankLine()
		}

		f.formatScope(scope)
	}
}

func (f *formatter) formatImport(imp *Import) {
	f.writeLeadingComments(imp.LeadingComments)

	if imp.Alias != nil {
		f.writeIndent()
		f.write("import " + *imp.Alias + " " + f.quotedString(imp.Path))
	} else {
		f.writeIndent()
		f.write("import " + f.quotedString(imp.Path))
	}

	f.writeTrailingComment(imp.TrailingComment)
	f.write("\n")
}

func (f *formatter) formatQuery(q *Query) {
	f.writeLeadingComments(q.LeadingComments)
	f.writeIndent()
	f.write("query " + q.Name + " " + f.rawString(q.Body))
	f.writeTrailingComment(q.TrailingComment)
	f.write("\n")
}

func (f *formatter) formatSetupClause(s *SetupClause) {
	switch {
	case s.Inline != nil:
		f.writeLine("setup " + f.rawString(*s.Inline))
	case s.Module != nil:
		f.writeLine("setup " + *s.Module)
	case s.Call != nil:
		f.writeLine("setup " + f.formatSetupCall(s.Call))
	case len(s.Block) > 0:
		f.formatSetupBlock(s.Block)
	}
}

func (f *formatter) formatSetupBlock(items []*SetupItem) {
	if len(items) == 1 {
		// Single item - inline format
		f.writeLine("setup { " + f.formatSetupItem(items[0]) + " }")

		return
	}

	// Multiple items - block format
	f.writeLine("setup {")
	f.indent++

	for _, item := range items {
		f.writeLine(f.formatSetupItem(item))
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatSetupItem(item *SetupItem) string {
	if item.Inline != nil {
		return f.rawString(*item.Inline)
	}

	if item.Module != nil {
		return *item.Module
	}

	if item.Call != nil {
		return f.formatSetupCall(item.Call)
	}

	return ""
}

func (f *formatter) formatSetupCall(c *SetupCall) string {
	var b strings.Builder

	b.WriteString(c.Module)
	b.WriteString(".")
	b.WriteString(c.Query)
	b.WriteString("(")

	for i, p := range c.Params {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(p.Name)
		b.WriteString(": ")
		b.WriteString(f.formatParamValue(p.Value))
	}

	b.WriteString(")")

	return b.String()
}

func (f *formatter) formatTeardown(body string) {
	f.writeLine("teardown " + f.rawString(body))
}

func (f *formatter) formatScope(s *QueryScope) {
	f.writeLeadingComments(s.LeadingComments)
	f.writeLine(s.QueryName + " {")
	f.indent++

	if s.Setup != nil {
		f.formatSetupClause(s.Setup)
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
	f.writeLeadingComments(g.LeadingComments)
	f.writeLine("group " + f.quotedString(g.Name) + " {")
	f.indent++

	if g.Setup != nil {
		f.formatSetupClause(g.Setup)
	}

	if g.Teardown != nil {
		f.formatTeardown(*g.Teardown)
	}

	f.formatItems(g.Items, g.Setup != nil || g.Teardown != nil)

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatTest(t *Test) {
	f.writeLeadingComments(t.LeadingComments)
	f.writeLine("test " + f.quotedString(t.Name) + " {")
	f.indent++

	if t.Setup != nil {
		f.formatSetupClause(t.Setup)
	}

	// Separate inputs from outputs
	var inputs, outputs []*Statement

	for _, stmt := range t.Statements {
		if strings.HasPrefix(stmt.Key(), "$") {
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

	// Assertions
	for i, a := range t.Asserts {
		if i == 0 && (len(t.Statements) > 0 || t.Setup != nil) {
			f.blankLine()
		}

		f.formatAssert(a)
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatStatement(s *Statement) {
	f.writeLine(s.Key() + ": " + f.formatValue(s.Value))
}

func (f *formatter) formatAssert(a *Assert) {
	var queryPart string
	if a.Query != nil {
		if a.Query.Inline != nil {
			queryPart = f.rawString(*a.Query.Inline) + " "
		} else if a.Query.QueryName != nil {
			queryPart = *a.Query.QueryName
			if len(a.Query.Params) > 0 {
				var params []string
				for _, p := range a.Query.Params {
					params = append(params, p.Name+": "+f.formatParamValue(p.Value))
				}

				queryPart += "(" + strings.Join(params, ", ") + ") "
			} else {
				queryPart += "() "
			}
		}
	}

	if len(a.Conditions) == 0 {
		f.writeLine("assert " + queryPart + "{}")

		return
	}

	if len(a.Conditions) == 1 {
		f.writeLine("assert " + queryPart + "{ " + a.Conditions[0].String() + " }")

		return
	}

	f.writeLine("assert " + queryPart + "{")
	f.indent++

	for i, cond := range a.Conditions {
		if i < len(a.Conditions)-1 {
			f.writeLine(cond.String() + ";")
		} else {
			f.writeLine(cond.String())
		}
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

func (f *formatter) formatParamValue(v *ParamValue) string {
	if v.IsFieldRef() {
		return v.FieldRefString()
	}

	if v.Literal != nil {
		return f.formatValue(v.Literal)
	}

	return "null"
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
