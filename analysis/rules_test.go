package analysis_test

import (
	"testing"

	"github.com/rlch/scaf/analysis"
)

func TestRule_UndefinedQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

UndefinedQuery {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "undefined-query")
}

func TestRule_UndefinedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

setup undefined.Setup()

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "undefined-import")
}

func TestRule_UnusedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"

query Q `+"`Q`"+`

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "unused-import")
}

func TestRule_UsedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"

query Q `+"`Q`"+`

setup fixtures.Setup()

Q {
	test "t" {}
}
`)

	assertNoDiagnostic(t, result, "unused-import")
}

func TestRule_DuplicateQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query GetUser `+"`Q1`"+`
query GetUser `+"`Q2`"+`

GetUser {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "duplicate-query")
}

func TestRule_DuplicateImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"
import fixtures "./other"

query Q `+"`Q`"+`

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "duplicate-import")
}

func TestRule_UnknownParameter(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query GetUser `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 1
		$unknownParam: "test"
	}
}
`)

	assertHasDiagnostic(t, result, "unknown-parameter")
}

func TestRule_EmptyTest(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

Q {
	test "empty" {}
}
`)

	assertHasDiagnostic(t, result, "empty-test")
}

func TestRule_DuplicateTestName(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

Q {
	test "same name" {
		$x: 1
	}
	test "same name" {
		$x: 2
	}
}
`)

	assertHasDiagnostic(t, result, "duplicate-test")
}

func TestRule_DuplicateGroupName(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

Q {
	group "mygroup" {
		test "t1" { $x: 1 }
	}
	group "mygroup" {
		test "t2" { $x: 2 }
	}
}
`)

	assertHasDiagnostic(t, result, "duplicate-group")
}

func TestRule_UndefinedAssertQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

Q {
	test "t" {
		$x: 1
		assert UndefinedQuery() { result > 0 }
	}
}
`)

	assertHasDiagnostic(t, result, "undefined-assert-query")
}

func TestRule_MissingRequiredParams(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query GetUser `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "missing name" {
		$id: 1
	}
}
`)

	assertHasDiagnostic(t, result, "missing-required-params")
}

func TestRule_AllParamsProvided(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query GetUser `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "has all params" {
		$id: 1
		$name: "Alice"
	}
}
`)

	assertNoDiagnostic(t, result, "missing-required-params")
}

func TestRule_EmptyGroup(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
query Q `+"`Q`"+`

Q {
	group "empty group" {}
}
`)

	assertHasDiagnostic(t, result, "empty-group")
}

// Test helpers

func analyze(t *testing.T, input string) *analysis.AnalyzedFile {
	t.Helper()

	analyzer := analysis.NewAnalyzer(nil)

	return analyzer.Analyze("test.scaf", []byte(input))
}

func assertHasDiagnostic(t *testing.T, result *analysis.AnalyzedFile, code string) {
	t.Helper()

	for _, d := range result.Diagnostics {
		if d.Code == code {
			return
		}
	}

	t.Errorf("expected diagnostic %q, got:", code)

	for _, d := range result.Diagnostics {
		t.Logf("  %s: %s", d.Code, d.Message)
	}
}

func assertNoDiagnostic(t *testing.T, result *analysis.AnalyzedFile, code string) {
	t.Helper()

	for _, d := range result.Diagnostics {
		if d.Code == code {
			t.Errorf("unexpected diagnostic %q: %s", code, d.Message)
		}
	}
}
