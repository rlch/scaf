//nolint:testpackage // Tests need access to internal types
package runner

import (
	"strings"
	"testing"
)

func TestEvalExpr_Comparisons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		// Numeric comparisons
		{"greater than - true", "age > 18", map[string]any{"age": 30}, true},
		{"greater than - false", "age > 18", map[string]any{"age": 15}, false},
		{"greater than equal - equal", "age >= 18", map[string]any{"age": 18}, true},
		{"greater than equal - greater", "age >= 18", map[string]any{"age": 30}, true},
		{"greater than equal - less", "age >= 18", map[string]any{"age": 17}, false},
		{"less than - true", "age < 30", map[string]any{"age": 25}, true},
		{"less than - false", "age < 30", map[string]any{"age": 35}, false},
		{"less than equal - equal", "age <= 30", map[string]any{"age": 30}, true},
		{"equal - int", "count == 5", map[string]any{"count": 5}, true},
		{"equal - int mismatch", "count == 5", map[string]any{"count": 10}, false},
		{"not equal - true", "count != 5", map[string]any{"count": 10}, true},
		{"not equal - false", "count != 5", map[string]any{"count": 5}, false},

		// String comparisons
		{"string equal - true", `name == "Alice"`, map[string]any{"name": "Alice"}, true},
		{"string equal - false", `name == "Alice"`, map[string]any{"name": "Bob"}, false},
		{"string not equal", `name != "Alice"`, map[string]any{"name": "Bob"}, true},

		// Boolean comparisons
		{"bool equal true", "verified == true", map[string]any{"verified": true}, true},
		{"bool equal false", "verified == false", map[string]any{"verified": false}, true},
		{"bool mismatch", "verified == true", map[string]any{"verified": false}, false},

		// int64 vs float64 (database returns int64, parser uses float64)
		{"int64 comparison", "age > 18", map[string]any{"age": int64(30)}, true},
		{"float64 comparison", "age > 18.5", map[string]any{"age": 30.0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_BooleanOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{"and - both true", "age > 18 && verified", map[string]any{"age": 30, "verified": true}, true},
		{"and - left false", "age > 18 && verified", map[string]any{"age": 15, "verified": true}, false},
		{"and - right false", "age > 18 && verified", map[string]any{"age": 30, "verified": false}, false},
		{"and - both false", "age > 18 && verified", map[string]any{"age": 15, "verified": false}, false},
		{"or - both true", "age > 18 || verified", map[string]any{"age": 30, "verified": true}, true},
		{"or - left true", "age > 18 || verified", map[string]any{"age": 30, "verified": false}, true},
		{"or - right true", "age > 18 || verified", map[string]any{"age": 15, "verified": true}, true},
		{"or - both false", "age > 18 || verified", map[string]any{"age": 15, "verified": false}, false},
		{"not - true becomes false", "!verified", map[string]any{"verified": true}, false},
		{"not - false becomes true", "!verified", map[string]any{"verified": false}, true},
		{"complex - (a && b) || c", "(age > 18 && verified) || admin", map[string]any{"age": 15, "verified": false, "admin": true}, true},
		{"complex - a && (b || c)", "age > 18 && (verified || admin)", map[string]any{"age": 30, "verified": false, "admin": true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_BuiltinFunctions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		// len()
		{"len string > 0", "len(name) > 0", map[string]any{"name": "Alice"}, true},
		{"len string == 0", "len(name) == 0", map[string]any{"name": ""}, true},
		{"len string check", "len(name) == 5", map[string]any{"name": "Alice"}, true},
		{"len array", "len(items) > 0", map[string]any{"items": []any{1, 2, 3}}, true},
		{"len empty array", "len(items) == 0", map[string]any{"items": []any{}}, true},

		// contains - operator syntax: string contains substring
		{"contains op - true", `email contains "@"`, map[string]any{"email": "alice@example.com"}, true},
		{"contains op - false", `email contains "@"`, map[string]any{"email": "invalid"}, false},
		{"contains op string", `name contains "lic"`, map[string]any{"name": "Alice"}, true},

		// startsWith / endsWith - operator syntax
		{"startsWith op - true", `name startsWith "Al"`, map[string]any{"name": "Alice"}, true},
		{"startsWith op - false", `name startsWith "Bo"`, map[string]any{"name": "Alice"}, false},
		{"endsWith op - true", `email endsWith ".com"`, map[string]any{"email": "alice@example.com"}, true},
		{"endsWith op - false", `email endsWith ".org"`, map[string]any{"email": "alice@example.com"}, false},

		// hasPrefix / hasSuffix - function syntax (alternative to startsWith/endsWith)
		{"hasPrefix fn - true", `hasPrefix(name, "Al")`, map[string]any{"name": "Alice"}, true},
		{"hasPrefix fn - false", `hasPrefix(name, "Bo")`, map[string]any{"name": "Alice"}, false},
		{"hasSuffix fn - true", `hasSuffix(email, ".com")`, map[string]any{"email": "alice@example.com"}, true},
		{"hasSuffix fn - false", `hasSuffix(email, ".org")`, map[string]any{"email": "alice@example.com"}, false},

		// upper() / lower() - function syntax
		{"upper comparison", `upper(name) == "ALICE"`, map[string]any{"name": "Alice"}, true},
		{"lower comparison", `lower(name) == "alice"`, map[string]any{"name": "Alice"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_FieldAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{
			"nested map access",
			`user.name == "Alice"`,
			map[string]any{"user": map[string]any{"name": "Alice", "age": 30}},
			true,
		},
		{
			"deep nested access",
			"user.profile.verified == true",
			map[string]any{"user": map[string]any{"profile": map[string]any{"verified": true}}},
			true,
		},
		{
			"nested numeric",
			"user.age > 18",
			map[string]any{"user": map[string]any{"age": 30}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_ArrayAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{
			"array index",
			"items[0] == 1",
			map[string]any{"items": []any{1, 2, 3}},
			true,
		},
		{
			"array last element",
			"items[2] == 3",
			map[string]any{"items": []any{1, 2, 3}},
			true,
		},
		{
			"array string element",
			`names[0] == "Alice"`,
			map[string]any{"names": []any{"Alice", "Bob"}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_NilHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{"nil equal nil", "value == nil", map[string]any{"value": nil}, true},
		{"nil not equal value", "value != nil", map[string]any{"value": "something"}, true},
		{"value not nil", "value != nil", map[string]any{"value": 42}, true},
		{"check nil explicitly", "value == nil", map[string]any{"value": nil}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_EmptyExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tabs and newlines", "\t\n  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, map[string]any{})
			if result.Error != nil {
				t.Errorf("unexpected error for empty expression: %v", result.Error)
			}

			if !result.Passed {
				t.Error("empty expression should pass")
			}
		})
	}
}

func TestEvalExpr_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expr        string
		env         map[string]any
		errContains string
	}{
		{
			"unknown variable",
			"unknown > 0",
			map[string]any{"age": 30},
			"unknown",
		},
		{
			"syntax error",
			"age > > 0",
			map[string]any{"age": 30},
			"",
		},
		{
			"type mismatch - string > int",
			"name > 0",
			map[string]any{"name": "Alice"},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error == nil {
				t.Error("expected error, got nil")

				return
			}

			if tt.errContains != "" && !strings.Contains(result.Error.Error(), tt.errContains) {
				t.Errorf("error %q should contain %q", result.Error.Error(), tt.errContains)
			}
		})
	}
}

func TestEvalExprs(t *testing.T) {
	t.Parallel()

	env := map[string]any{"age": 30, "name": "Alice", "verified": true}

	exprs := []string{
		"age > 18",
		`name == "Alice"`,
		"verified == true",
	}

	results := EvalExprs(exprs, env)

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	for i, r := range results {
		if r.Error != nil {
			t.Errorf("results[%d] unexpected error: %v", i, r.Error)
		}

		if !r.Passed {
			t.Errorf("results[%d] = false, want true", i)
		}
	}
}

func TestEvalExprs_ContinuesOnFailure(t *testing.T) {
	t.Parallel()

	env := map[string]any{"age": 15} // age < 18

	exprs := []string{
		"age > 18", // fails
		"age > 10", // would pass
		"age > 5",  // would pass
	}

	results := EvalExprs(exprs, env)

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3 (should evaluate all)", len(results))
	}

	if results[0].Passed {
		t.Error("first expression should fail")
	}

	if !results[1].Passed {
		t.Error("second expression should pass")
	}

	if !results[2].Passed {
		t.Error("third expression should pass")
	}
}

func TestEvalExprsStopOnFail(t *testing.T) {
	t.Parallel()

	env := map[string]any{"age": 15} // age < 18

	exprs := []string{
		"age > 10", // passes
		"age > 18", // fails
		"age > 5",  // would pass but shouldn't be evaluated
	}

	results := EvalExprsStopOnFail(exprs, env)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (should stop after failure)", len(results))
	}

	if !results[0].Passed {
		t.Error("first expression should pass")
	}

	if results[1].Passed {
		t.Error("second expression should fail")
	}
}

func TestEvalExprsStopOnFail_StopsOnError(t *testing.T) {
	t.Parallel()

	env := map[string]any{"age": 30}

	exprs := []string{
		"age > 10",     // passes
		"unknown > 18", // error - unknown variable
		"age > 5",      // would pass but shouldn't be evaluated
	}

	results := EvalExprsStopOnFail(exprs, env)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (should stop after error)", len(results))
	}

	if !results[0].Passed {
		t.Error("first expression should pass")
	}

	if results[1].Error == nil {
		t.Error("second expression should have error")
	}
}

func TestAllPassed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []ExprResult
		want    bool
	}{
		{
			"all passed",
			[]ExprResult{
				{Passed: true},
				{Passed: true},
				{Passed: true},
			},
			true,
		},
		{
			"one failed",
			[]ExprResult{
				{Passed: true},
				{Passed: false},
				{Passed: true},
			},
			false,
		},
		{
			"one error",
			[]ExprResult{
				{Passed: true},
				{Error: errTestFail},
				{Passed: true},
			},
			false,
		},
		{
			"empty results",
			[]ExprResult{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := AllPassed(tt.results); got != tt.want {
				t.Errorf("AllPassed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		results    []ExprResult
		wantNil    bool
		wantExpr   string
		wantPassed bool
	}{
		{
			"all passed - returns nil",
			[]ExprResult{
				{Expression: "a", Passed: true},
				{Expression: "b", Passed: true},
			},
			true,
			"",
			false,
		},
		{
			"first fails",
			[]ExprResult{
				{Expression: "a > 10", Passed: false},
				{Expression: "b > 10", Passed: true},
			},
			false,
			"a > 10",
			false,
		},
		{
			"second fails",
			[]ExprResult{
				{Expression: "a > 10", Passed: true},
				{Expression: "b > 10", Passed: false},
			},
			false,
			"b > 10",
			false,
		},
		{
			"error takes precedence",
			[]ExprResult{
				{Expression: "a > 10", Passed: true},
				{Expression: "bad", Error: errTestFail},
				{Expression: "c > 10", Passed: false},
			},
			false,
			"bad",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FirstFailure(tt.results)
			if tt.wantNil {
				if got != nil {
					t.Errorf("FirstFailure() = %v, want nil", got)
				}

				return
			}

			if got == nil {
				t.Fatal("FirstFailure() = nil, want non-nil")
			}

			if got.Expression != tt.wantExpr {
				t.Errorf("FirstFailure().Expression = %q, want %q", got.Expression, tt.wantExpr)
			}
		})
	}
}

func TestEvalExpr_Arithmetic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{"addition", "a + b == 10", map[string]any{"a": 3, "b": 7}, true},
		{"subtraction", "a - b == 5", map[string]any{"a": 10, "b": 5}, true},
		{"multiplication", "a * b == 20", map[string]any{"a": 4, "b": 5}, true},
		{"division", "a / b == 2", map[string]any{"a": 10, "b": 5}, true},
		{"modulo", "a % b == 1", map[string]any{"a": 10, "b": 3}, true},
		{"complex arithmetic", "(a + b) * c == 30", map[string]any{"a": 2, "b": 3, "c": 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_InOperator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{
			"in array - found",
			"value in items",
			map[string]any{"value": 2, "items": []any{1, 2, 3}},
			true,
		},
		{
			"in array - not found",
			"value in items",
			map[string]any{"value": 5, "items": []any{1, 2, 3}},
			false,
		},
		{
			"in string array",
			`name in names`,
			map[string]any{"name": "Alice", "names": []any{"Alice", "Bob", "Charlie"}},
			true,
		},
		{
			"not in array",
			"value not in items",
			map[string]any{"value": 5, "items": []any{1, 2, 3}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}

func TestEvalExpr_TernaryOperator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expr   string
		env    map[string]any
		passed bool
	}{
		{
			"ternary true branch",
			"(age > 18 ? true : false) == true",
			map[string]any{"age": 30},
			true,
		},
		{
			"ternary false branch",
			"(age > 18 ? true : false) == false",
			map[string]any{"age": 15},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EvalExpr(tt.expr, tt.env)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}

			if result.Passed != tt.passed {
				t.Errorf("EvalExpr(%q) = %v, want %v", tt.expr, result.Passed, tt.passed)
			}
		})
	}
}
