package runner

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

// ExprResult holds the result of evaluating an expression.
type ExprResult struct {
	Expression string // The expression that was evaluated
	Passed     bool   // Whether the expression evaluated to true
	Error      error  // Any error during compilation or evaluation
}

// EvalExpr evaluates a single expression string against an environment.
// Returns the result of the boolean expression, or an error if:
// - The expression fails to compile
// - The expression fails to evaluate
// - The expression doesn't return a boolean.
func EvalExpr(exprStr string, env map[string]any) ExprResult {
	result := ExprResult{Expression: exprStr}

	if strings.TrimSpace(exprStr) == "" {
		result.Passed = true

		return result
	}

	// Compile the expression
	program, err := expr.Compile(exprStr, expr.Env(env), expr.AsBool())
	if err != nil {
		result.Error = fmt.Errorf("compile expression %q: %w", exprStr, err)

		return result
	}

	// Run the expression
	output, err := expr.Run(program, env)
	if err != nil {
		result.Error = fmt.Errorf("evaluate expression %q: %w", exprStr, err)

		return result
	}

	// Check if result is true
	passed, ok := output.(bool)
	if !ok {
		result.Error = fmt.Errorf("%w: %q returned %T", ErrExprNotBool, exprStr, output)

		return result
	}

	result.Passed = passed

	return result
}

// EvalExprs evaluates multiple expressions against an environment.
// Returns results for each expression. Evaluation continues even if some fail.
func EvalExprs(exprs []string, env map[string]any) []ExprResult {
	results := make([]ExprResult, len(exprs))

	for i, e := range exprs {
		results[i] = EvalExpr(e, env)
	}

	return results
}

// EvalExprsStopOnFail evaluates expressions and stops at the first failure.
// Returns the results evaluated so far.
func EvalExprsStopOnFail(exprs []string, env map[string]any) []ExprResult {
	results := make([]ExprResult, 0, len(exprs))

	for _, e := range exprs {
		result := EvalExpr(e, env)
		results = append(results, result)

		if result.Error != nil || !result.Passed {
			break
		}
	}

	return results
}

// AllPassed returns true if all results passed without errors.
func AllPassed(results []ExprResult) bool {
	for _, r := range results {
		if r.Error != nil || !r.Passed {
			return false
		}
	}

	return true
}

// FirstFailure returns the first failed or errored result, or nil if all passed.
func FirstFailure(results []ExprResult) *ExprResult {
	for i := range results {
		if results[i].Error != nil || !results[i].Passed {
			return &results[i]
		}
	}

	return nil
}
