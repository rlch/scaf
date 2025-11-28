package runner

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/module"
)

var (
	// ErrFieldNotFound is returned when a field reference cannot be resolved.
	ErrFieldNotFound = errors.New("field not found in scope")
	// ErrFieldNotMap is returned when attempting to access a field on a non-map value.
	ErrFieldNotMap = errors.New("cannot access field on non-map value")
)

// Runner executes scaf test suites.
type Runner struct {
	database scaf.Database
	handler  Handler
	failFast bool
	filter   *regexp.Regexp
	modules  *module.ResolvedContext
	lag      bool // artificial lag for TUI testing
}

// Option configures a Runner.
type Option func(*Runner)

// WithDatabase sets the database for query execution.
func WithDatabase(db scaf.Database) Option {
	return func(r *Runner) {
		r.database = db
	}
}

// WithDialect sets the database dialect.
// Deprecated: Use WithDatabase instead.
func WithDialect(d scaf.Database) Option {
	return WithDatabase(d)
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

// WithModules sets the resolved module context for named setup resolution.
func WithModules(ctx *module.ResolvedContext) Option {
	return func(r *Runner) {
		r.modules = ctx
	}
}

// WithLag enables artificial lag (500ms-1.5s) for TUI testing.
func WithLag(enabled bool) Option {
	return func(r *Runner) {
		r.lag = enabled
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

// RunFile loads, resolves, and runs a scaf file with full module resolution.
// This is a convenience method that handles parsing and import resolution.
func (r *Runner) RunFile(ctx context.Context, path string) (*Result, error) {
	if r.database == nil {
		return nil, ErrNoDatabase
	}

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	resolved, err := resolver.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve modules: %w", err)
	}

	// Set the module context for this run
	r.modules = resolved

	return r.Run(ctx, resolved.Root.Suite, path)
}

// Run executes a parsed Suite and returns the results.
func (r *Runner) Run(ctx context.Context, suite *scaf.Suite, suitePath string) (*Result, error) {
	if r.database == nil {
		return nil, ErrNoDatabase
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
		err := r.executeSetup(ctx, r.database, suite.Setup)
		if err != nil {
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
				_ = r.executeQuery(ctx, r.database, *suite.Teardown, nil)
			}

			return result, err
		}
	}

	// Execute suite teardown
	if suite.Teardown != nil {
		err := r.executeQuery(ctx, r.database, *suite.Teardown, nil)
		if err != nil {
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
		return fmt.Errorf("%w: %s", ErrUnknownQuery, scope.QueryName)
	}

	// Execute scope setup
	if scope.Setup != nil {
		err := r.executeSetup(ctx, r.database, scope.Setup)
		if err != nil {
			return fmt.Errorf("scope %s setup: %w", scope.QueryName, err)
		}
	}

	// Run all items
	for _, item := range scope.Items {
		path := []string{scope.QueryName}

		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, queryBody, queries, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, queryBody, queries, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			// Run scope teardown before returning
			if scope.Teardown != nil {
				_ = r.executeQuery(ctx, r.database, *scope.Teardown, nil)
			}

			return err
		}
	}

	// Execute scope teardown
	if scope.Teardown != nil {
		err := r.executeQuery(ctx, r.database, *scope.Teardown, nil)
		if err != nil {
			return fmt.Errorf("scope %s teardown: %w", scope.QueryName, err)
		}
	}

	return nil
}

func (r *Runner) runGroup(
	ctx context.Context,
	group *scaf.Group,
	queryBody string,
	queries map[string]string,
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
		err := r.executeSetup(ctx, r.database, group.Setup)
		if err != nil {
			return fmt.Errorf("group %s setup: %w", group.Name, err)
		}
	}

	// Run all items
	for _, item := range group.Items {
		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, queryBody, queries, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, queryBody, queries, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			// Run group teardown before returning
			if group.Teardown != nil {
				_ = r.executeQuery(ctx, r.database, *group.Teardown, nil)
			}

			return err
		}
	}

	// Execute group teardown
	if group.Teardown != nil {
		err := r.executeQuery(ctx, r.database, *group.Teardown, nil)
		if err != nil {
			return fmt.Errorf("group %s teardown: %w", group.Name, err)
		}
	}

	return nil
}

func (r *Runner) runTest(
	ctx context.Context,
	test *scaf.Test,
	queryBody string,
	queries map[string]string,
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

	// Artificial lag for TUI testing
	if r.lag {
		time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond) //nolint:gosec // G404: weak random is fine for artificial lag
	}

	// Try to run test in a transaction for isolation
	txDB, canTx := r.database.(scaf.TransactionalDatabase)
	if canTx {
		return r.runTestInTransaction(ctx, txDB, test, queryBody, queries, path, suitePath, start, handler, result)
	}

	// Fallback: run without transaction isolation
	return r.runTestDirect(ctx, r.database, test, queryBody, queries, path, suitePath, start, handler, result)
}

func (r *Runner) runTestInTransaction(
	ctx context.Context,
	txDB scaf.TransactionalDatabase,
	test *scaf.Test,
	queryBody string,
	queries map[string]string,
	path []string,
	suitePath string,
	start time.Time,
	handler Handler,
	result *Result,
) error {
	tx, err := txDB.Begin(ctx)
	if err != nil {
		return r.emitError(ctx, path, suitePath, start, fmt.Errorf("begin transaction: %w", err), handler, result)
	}

	// Always rollback - tests should not persist changes
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	return r.runTestDirect(ctx, tx, test, queryBody, queries, path, suitePath, start, handler, result)
}

func (r *Runner) runTestDirect(
	ctx context.Context,
	exec executor,
	test *scaf.Test,
	queryBody string,
	queries map[string]string,
	path []string,
	suitePath string,
	start time.Time,
	handler Handler,
	result *Result,
) error {
	// Execute test setup (within transaction if available)
	if test.Setup != nil {
		err := r.executeSetup(ctx, exec, test.Setup)
		if err != nil {
			return r.emitError(ctx, path, suitePath, start, fmt.Errorf("test setup: %w", err), handler, result)
		}
	}

	// Build params and expectations from statements
	params := make(map[string]any)
	expectations := make(map[string]any)

	for _, stmt := range test.Statements {
		key := stmt.Key()
		if len(key) > 0 && key[0] == '$' {
			params[key[1:]] = stmt.Value.ToGo()
		} else {
			expectations[key] = stmt.Value.ToGo()
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

	// Evaluate assert blocks
	for _, assert := range test.Asserts {
		err := r.evaluateAssert(ctx, exec, assert, actual, queries, path, suitePath, start, handler, result)
		if err != nil {
			return err
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

// executeSetup executes a setup clause - inline, module, call, or block.
func (r *Runner) executeSetup(ctx context.Context, exec executor, setup *scaf.SetupClause) error {
	if setup == nil {
		return nil
	}

	if setup.Inline != nil {
		return r.executeQuery(ctx, exec, *setup.Inline, nil)
	}

	if setup.Module != nil {
		return r.executeModuleSetup(ctx, exec, *setup.Module)
	}

	if setup.Call != nil {
		return r.executeSetupCall(ctx, exec, setup.Call)
	}

	// Block setup - execute each item in order
	for _, item := range setup.Block {
		err := r.executeSetupItem(ctx, exec, item)
		if err != nil {
			return err
		}
	}

	return nil
}

// executeSetupItem executes a single setup item (inline, module, or call).
func (r *Runner) executeSetupItem(ctx context.Context, exec executor, item *scaf.SetupItem) error {
	if item.Inline != nil {
		return r.executeQuery(ctx, exec, *item.Inline, nil)
	}

	if item.Module != nil {
		return r.executeModuleSetup(ctx, exec, *item.Module)
	}

	if item.Call != nil {
		return r.executeSetupCall(ctx, exec, item.Call)
	}

	return nil
}

// executeModuleSetup runs an imported module's setup clause.
func (r *Runner) executeModuleSetup(ctx context.Context, exec executor, moduleAlias string) error {
	if r.modules == nil {
		return fmt.Errorf("%w: %s", ErrNoModuleContext, moduleAlias)
	}

	// Resolve the module
	mod, err := r.modules.ResolveModule(moduleAlias)
	if err != nil {
		return fmt.Errorf("failed to resolve module: %w", err)
	}

	// Get and execute the module's setup clause
	modSetup := mod.GetSetup()
	if modSetup == nil {
		return fmt.Errorf("module %q has no setup clause", moduleAlias)
	}

	// Recursively execute the module's setup
	return r.executeSetup(ctx, exec, modSetup)
}

// executeSetupCall executes a query call from a module with parameters.
func (r *Runner) executeSetupCall(ctx context.Context, exec executor, call *scaf.SetupCall) error {
	if r.modules == nil {
		return fmt.Errorf("%w: %s.%s", ErrNoModuleContext, call.Module, call.Query)
	}

	// Resolve the query from the module
	queryBody, err := r.modules.ResolveQuery(call.Module, call.Query)
	if err != nil {
		return fmt.Errorf("failed to resolve query: %w", err)
	}

	// Build params from the call
	params := make(map[string]any)

	for _, p := range call.Params {
		key := p.Name
		// Strip $ prefix if present
		if len(key) > 0 && key[0] == '$' {
			key = key[1:]
		}

		params[key] = p.Value.ToGo()
	}

	// Execute the query with the provided params
	return r.executeQuery(ctx, exec, queryBody, params)
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

// evaluateAssert evaluates an assert block's conditions.
// If the assert has a query, it runs that query first and evaluates conditions against its results.
// Otherwise, it evaluates conditions against the main query results.
func (r *Runner) evaluateAssert(
	ctx context.Context,
	exec executor,
	assert *scaf.Assert,
	mainResult map[string]any,
	queries map[string]string,
	path []string,
	suitePath string,
	start time.Time,
	handler Handler,
	result *Result,
) error {
	// Determine which result to evaluate against
	env := mainResult

	// If assert has a query, run it first
	if assert.Query != nil {
		assertResult, err := r.runAssertQuery(ctx, exec, assert.Query, queries, mainResult)
		if err != nil {
			return r.emitError(ctx, path, suitePath, start, fmt.Errorf("assert query: %w", err), handler, result)
		}

		env = assertResult
	}

	// Evaluate each condition
	for _, condition := range assert.Conditions {
		exprStr := condition.String()

		evalResult := EvalExpr(exprStr, env)
		if evalResult.Error != nil {
			return r.emitError(ctx, path, suitePath, start, evalResult.Error, handler, result)
		}

		if !evalResult.Passed {
			elapsed := time.Since(start)

			return handler.Event(ctx, Event{
				Time:     time.Now(),
				Action:   ActionFail,
				Suite:    suitePath,
				Path:     path,
				Elapsed:  elapsed,
				Field:    exprStr,
				Expected: true,
				Actual:   false,
			}, result)
		}
	}

	return nil
}

// runAssertQuery executes the query specified in an assert block.
// parentScope contains the main query results, used to resolve field references in params.
func (r *Runner) runAssertQuery(
	ctx context.Context,
	exec executor,
	query *scaf.AssertQuery,
	queries map[string]string,
	parentScope map[string]any,
) (map[string]any, error) {
	var queryBody string

	params := make(map[string]any)

	switch {
	case query.Inline != nil:
		// Inline query
		queryBody = *query.Inline
	case query.QueryName != nil:
		// Named query reference
		var ok bool

		queryBody, ok = queries[*query.QueryName]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownQuery, *query.QueryName)
		}

		// Build params from assert query params
		for _, p := range query.Params {
			key := p.Name
			if len(key) > 0 && key[0] == '$' {
				key = key[1:]
			}

			// Resolve field references from parent scope
			if p.Value.IsFieldRef() {
				fieldRef := p.Value.FieldRefString()

				val, err := resolveFieldRef(fieldRef, parentScope)
				if err != nil {
					return nil, fmt.Errorf("param $%s: %w", key, err)
				}

				params[key] = val
			} else {
				params[key] = p.Value.ToGo()
			}
		}
	default:
		return nil, ErrAssertNoQuery
	}

	rows, err := exec.Execute(ctx, queryBody, params)
	if err != nil {
		return nil, err
	}

	// Return first row or empty map
	if len(rows) > 0 {
		return rows[0], nil
	}

	return make(map[string]any), nil
}

// resolveFieldRef resolves a dotted field reference (e.g., "u.id") from a scope.
func resolveFieldRef(ref string, scope map[string]any) (any, error) {
	// First try direct lookup (handles "u.name" as a column name)
	if val, ok := scope[ref]; ok {
		return val, nil
	}

	// Try nested field access (e.g., "u.id" where u is a map)
	parts := strings.Split(ref, ".")

	var current any = scope

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, ref)
			}

			current = val
		default:
			return nil, fmt.Errorf("%w: %s", ErrFieldNotMap, part)
		}
	}

	return current, nil
}
