package module

import (
	"path/filepath"
	"strings"

	"github.com/rlch/scaf"
)

// Module represents a loaded scaf module with resolved metadata.
type Module struct {
	// Path is the absolute filesystem path to the .scaf file.
	Path string

	// Suite is the parsed AST.
	Suite *scaf.Suite

	// Queries maps query name to query body for fast lookup.
	Queries map[string]string
}

// NewModule creates a Module from a parsed Suite.
func NewModule(path string, suite *scaf.Suite) *Module {
	m := &Module{
		Path:    path,
		Suite:   suite,
		Queries: make(map[string]string),
	}

	// Index queries for fast lookup
	for _, q := range suite.Queries {
		m.Queries[q.Name] = q.Body
	}

	return m
}

// HasSetup returns true if this module has a setup clause.
func (m *Module) HasSetup() bool {
	return m.Suite != nil && m.Suite.Setup != nil
}

// GetSetup returns the module's setup clause, or nil if none.
func (m *Module) GetSetup() *scaf.SetupClause {
	if m.Suite == nil {
		return nil
	}
	return m.Suite.Setup
}

// GetQuery returns a query by name, or empty string if not found.
func (m *Module) GetQuery(name string) (string, bool) {
	q, ok := m.Queries[name]
	return q, ok
}

// BaseName returns the module name derived from the file path.
// For "/path/to/fixtures.scaf", returns "fixtures".
// For "/path/to/fixtures.cypher.scaf", returns "fixtures".
func (m *Module) BaseName() string {
	base := filepath.Base(m.Path)
	// Strip all extensions (handles .cypher.scaf, .sql.scaf, etc.)
	for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// ResolvedContext holds all modules needed to execute a test suite.
type ResolvedContext struct {
	// Root is the main module being executed.
	Root *Module

	// Imports maps alias -> module for all imported modules.
	// If an import has no alias, the module's base name is used.
	Imports map[string]*Module

	// AllModules contains all loaded modules by absolute path.
	AllModules map[string]*Module
}

// NewResolvedContext creates a new resolution context.
func NewResolvedContext(root *Module) *ResolvedContext {
	return &ResolvedContext{
		Root:       root,
		Imports:    make(map[string]*Module),
		AllModules: map[string]*Module{root.Path: root},
	}
}

// ResolveModule looks up a module by alias.
// Returns an error if the module is not found.
func (rc *ResolvedContext) ResolveModule(alias string) (*Module, error) {
	mod, ok := rc.Imports[alias]
	if !ok {
		return nil, &ResolveError{
			Module: alias,
			Cause:  ErrUnknownModule,
		}
	}
	return mod, nil
}

// ResolveQuery looks up a query by module alias and query name.
// Returns the query body and any error.
func (rc *ResolvedContext) ResolveQuery(moduleAlias, queryName string) (string, error) {
	mod, ok := rc.Imports[moduleAlias]
	if !ok {
		return "", &ResolveError{
			Module: moduleAlias,
			Name:   queryName,
			Cause:  ErrUnknownModule,
		}
	}

	query, ok := mod.GetQuery(queryName)
	if !ok {
		return "", &ResolveError{
			Module:        moduleAlias,
			Name:          queryName,
			AvailablePath: mod.Path,
			Cause:         ErrUnknownQuery,
		}
	}

	return query, nil
}

// GetQueries returns a combined query map with all queries from root and imports.
// Import queries are prefixed with their alias: "alias.QueryName".
func (rc *ResolvedContext) GetQueries() map[string]string {
	queries := make(map[string]string)

	// Add root queries
	for name, body := range rc.Root.Queries {
		queries[name] = body
	}

	// Add imported queries with prefix
	for alias, mod := range rc.Imports {
		for name, body := range mod.Queries {
			queries[alias+"."+name] = body
		}
	}

	return queries
}
