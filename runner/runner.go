package runner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/rlch/scaf"
)

// Runner executes scaf test suites.
type Runner struct {
	dialect  scaf.Dialect
	handler  Handler
	failFast bool
	filter   *regexp.Regexp
}

// Option configures a Runner.
type Option func(*Runner)

// WithDialect sets the database dialect.
func WithDialect(d scaf.Dialect) Option {
	return func(r *Runner) {
		r.dialect = d
	}
}

// WithHandler sets the event handler.
func WithHandler(h Handler) Option {
	return func(r *Runner) {
		r.handler = h
	}
}

// WithFailFast stops on first failure.
func WithFailFast(enabled bool) Option {
	return func(r *Runner) {
		r.failFast = enabled
	}
}

// WithFilter sets a regex pattern to filter which tests run.
// Tests whose path matches the pattern will be executed.
func WithFilter(pattern string) Option {
	return func(r *Runner) {
		if pattern != "" {
			r.filter = regexp.MustCompile(pattern)
		}
	}
}

// New creates a Runner with the given options.
func New(opts ...Option) *Runner {
	r := &Runner{}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// executor abstracts query execution - either direct dialect or transaction.
type executor interface {
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)
}

// Run executes a parsed Suite and returns the results.
func (r *Runner) Run(ctx context.Context, suite *scaf.Suite, suitePath string) (*Result, error) {
	if r.dialect == nil {
		return nil, ErrNoDialect
	}

	result := NewResult()

	handlers := []Handler{NewResultHandler()}
	if r.handler != nil {
		handlers = append(handlers, r.handler)
	}

	if r.failFast {
		handlers = append(handlers, NewStopOnFailHandler(1))
	}

	handler := NewMultiHandler(handlers...)

	// Build query lookup map
	queries := make(map[string]string)
	for _, q := range suite.Queries {
		queries[q.Name] = q.Body
	}

	// Execute suite setup
	if suite.Setup != nil {
		if err := r.executeSetup(ctx, r.dialect, suite.Setup); err != nil {
			return result, fmt.Errorf("suite setup: %w", err)
		}
	}

	// Run all scopes
	for _, scope := range suite.Scopes {
		err := r.runQueryScope(ctx, scope, queries, suitePath, handler, result)
		if errors.Is(err, ErrMaxFailures) {
			break
		}

		if err != nil {
			// Run suite teardown even on error
			if suite.Teardown != nil {
				_ = r.executeQuery(ctx, r.dialect, *suite.Teardown, nil)
			}

			return result, err
		}
	}

	// Execute suite teardown
	if suite.Teardown != nil {
		if err := r.executeQuery(ctx, r.dialect, *suite.Teardown, nil); err != nil {
			return result, fmt.Errorf("suite teardown: %w", err)
		}
	}

	result.Finish()

	return result, nil
}

func (r *Runner) runQueryScope(
	ctx context.Context,
	scope *scaf.QueryScope,
	queries map[string]string,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	queryBody, ok := queries[scope.QueryName]
	if !ok {
		return fmt.Errorf("unknown query: %s", scope.QueryName)
	}

	// Execute scope setup
	if scope.Setup != nil {
		if err := r.executeSetup(ctx, r.dialect, scope.Setup); err != nil {
			return fmt.Errorf("scope %s setup: %w", scope.QueryName, err)
		}
	}

	// Run all items
	for _, item := range scope.Items {
		path := []string{scope.QueryName}

		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, queryBody, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, queryBody, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			// Run scope teardown before returning
			if scope.Teardown != nil {
				_ = r.executeQuery(ctx, r.dialect, *scope.Teardown, nil)
			}

			return err
		}
	}

	// Execute scope teardown
	if scope.Teardown != nil {
		if err := r.executeQuery(ctx, r.dialect, *scope.Teardown, nil); err != nil {
			return fmt.Errorf("scope %s teardown: %w", scope.QueryName, err)
		}
	}

	return nil
}

func (r *Runner) runGroup(
	ctx context.Context,
	group *scaf.Group,
	queryBody string,
	parentPath []string,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	path := make([]string, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = group.Name

	// Execute group setup
	if group.Setup != nil {
		if err := r.executeSetup(ctx, r.dialect, group.Setup); err != nil {
			return fmt.Errorf("group %s setup: %w", group.Name, err)
		}
	}

	// Run all items
	for _, item := range group.Items {
		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, queryBody, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, queryBody, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			// Run group teardown before returning
			if group.Teardown != nil {
				_ = r.executeQuery(ctx, r.dialect, *group.Teardown, nil)
			}

			return err
		}
	}

	// Execute group teardown
	if group.Teardown != nil {
		if err := r.executeQuery(ctx, r.dialect, *group.Teardown, nil); err != nil {
			return fmt.Errorf("group %s teardown: %w", group.Name, err)
		}
	}

	return nil
}

func (r *Runner) runTest(
	ctx context.Context,
	test *scaf.Test,
	queryBody string,
	parentPath []string,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	path := make([]string, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = test.Name

	// Check if test matches filter
	if !r.matchesFilter(path) {
		return nil
	}

	start := time.Now()

	_ = handler.Event(ctx, Event{
		Time:   start,
		Action: ActionRun,
		Suite:  suitePath,
		Path:   path,
	}, result)

	// Try to run test in a transaction for isolation
	txDialect, canTx := r.dialect.(scaf.Transactional)
	if canTx {
		return r.runTestInTransaction(ctx, txDialect, test, queryBody, path, suitePath, start, handler, result)
	}

	// Fallback: run without transaction isolation
	return r.runTestDirect(ctx, r.dialect, test, queryBody, path, suitePath, start, handler, result)
}

func (r *Runner) runTestInTransaction(
	ctx context.Context,
	txDialect scaf.Transactional,
	test *scaf.Test,
	queryBody string,
	path []string,
	suitePath string,
	start time.Time,
	handler Handler,
	result *Result,
) error {
	tx, err := txDialect.Begin(ctx)
	if err != nil {
		return r.emitError(ctx, path, suitePath, start, fmt.Errorf("begin transaction: %w", err), handler, result)
	}

	// Always rollback - tests should not persist changes
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	return r.runTestDirect(ctx, tx, test, queryBody, path, suitePath, start, handler, result)
}

func (r *Runner) runTestDirect(
	ctx context.Context,
	exec executor,
	test *scaf.Test,
	queryBody string,
	path []string,
	suitePath string,
	start time.Time,
	handler Handler,
	result *Result,
) error {
	// Execute test setup (within transaction if available)
	if test.Setup != nil {
		if err := r.executeSetup(ctx, exec, test.Setup); err != nil {
			return r.emitError(ctx, path, suitePath, start, fmt.Errorf("test setup: %w", err), handler, result)
		}
	}

	// Build params and expectations from statements
	params := make(map[string]any)
	expectations := make(map[string]any)

	for _, stmt := range test.Statements {
		if len(stmt.Key) > 0 && stmt.Key[0] == '$' {
			params[stmt.Key[1:]] = stmt.Value.ToGo()
		} else {
			expectations[stmt.Key] = stmt.Value.ToGo()
		}
	}

	// Execute query
	rows, err := exec.Execute(ctx, queryBody, params)
	if err != nil {
		return r.emitError(ctx, path, suitePath, start, err, handler, result)
	}

	// Compare results - check first row against expectations
	var actual map[string]any
	if len(rows) > 0 {
		actual = rows[0]
	} else {
		actual = make(map[string]any)
	}

	// Check each expectation
	for field, expected := range expectations {
		got, exists := actual[field]
		if !exists {
			// Field doesn't exist in result - treat as nil
			got = nil
		}

		if !valuesEqual(expected, got) {
			elapsed := time.Since(start)

			return handler.Event(ctx, Event{
				Time:     time.Now(),
				Action:   ActionFail,
				Suite:    suitePath,
				Path:     path,
				Elapsed:  elapsed,
				Field:    field,
				Expected: expected,
				Actual:   got,
			}, result)
		}
	}

	elapsed := time.Since(start)

	return handler.Event(ctx, Event{
		Time:    time.Now(),
		Action:  ActionPass,
		Suite:   suitePath,
		Path:    path,
		Elapsed: elapsed,
	}, result)
}

// valuesEqual compares expected and actual values for equality.
func valuesEqual(expected, actual any) bool {
	// Handle nil cases
	if expected == nil && actual == nil {
		return true
	}

	if expected == nil || actual == nil {
		return false
	}

	// Handle numeric comparison (int64 from neo4j vs float64 from parser)
	switch e := expected.(type) {
	case float64:
		switch a := actual.(type) {
		case float64:
			return e == a
		case int64:
			return e == float64(a)
		}
	case int64:
		switch a := actual.(type) {
		case float64:
			return float64(e) == a
		case int64:
			return e == a
		}
	}

	// Default: use reflect.DeepEqual
	return reflect.DeepEqual(expected, actual)
}

func (r *Runner) executeQuery(ctx context.Context, exec executor, query string, params map[string]any) error {
	_, err := exec.Execute(ctx, query, params)

	return err
}

// executeSetup executes a setup clause - either inline or named.
// Named setups with module references are not yet implemented.
func (r *Runner) executeSetup(ctx context.Context, exec executor, setup *scaf.SetupClause) error {
	if setup == nil {
		return nil
	}

	if setup.Inline != nil {
		return r.executeQuery(ctx, exec, *setup.Inline, nil)
	}

	if setup.Named != nil {
		// TODO: Implement named setup resolution
		// For now, we'd need to look up the setup in the current suite or imported modules
		return fmt.Errorf("named setup references not yet implemented: %s", setup.Named.Name)
	}

	return nil
}

func (r *Runner) emitError(
	ctx context.Context,
	path []string,
	suitePath string,
	start time.Time,
	err error,
	handler Handler,
	result *Result,
) error {
	return handler.Event(ctx, Event{
		Time:    time.Now(),
		Action:  ActionError,
		Suite:   suitePath,
		Path:    path,
		Elapsed: time.Since(start),
		Error:   err,
	}, result)
}

// matchesFilter returns true if the test path matches the filter pattern.
// If no filter is set, all tests match.
func (r *Runner) matchesFilter(path []string) bool {
	if r.filter == nil {
		return true
	}

	pathStr := strings.Join(path, "/")

	return r.filter.MatchString(pathStr)
}
