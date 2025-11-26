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

	// Setups contains all setup clauses defined in this module, keyed by name.
	// Setup names are derived from query scopes that contain only setup (no tests).
	// For example:
	//   SetupDB { setup `CREATE ...` }
	// registers a setup named "SetupDB".
	Setups map[string]*Setup
}

// Setup represents a reusable setup defined in a module.
type Setup struct {
	// Name is the setup identifier.
	Name string

	// Query is the Cypher/SQL query to execute.
	// This is the body from the setup clause.
	Query string

	// Params are the parameter names this setup accepts (with $ prefix stripped).
	Params []string

	// Module is the module this setup belongs to.
	Module *Module
}

// NewModule creates a Module from a parsed Suite.
func NewModule(path string, suite *scaf.Suite) *Module {
	m := &Module{
		Path:   path,
		Suite:  suite,
		Setups: make(map[string]*Setup),
	}

	// Extract setups from query definitions that are meant to be reusable.
	// Convention: A query scope with only a setup clause (no tests) defines a reusable setup.
	// The scope name becomes the setup name.
	//
	// Example:
	//   query SetupTestDB `CREATE (:TestNode)`
	//   SetupTestDB {
	//     setup SetupTestDB()  // Self-referential - this is the setup definition
	//   }
	//
	// OR simpler - just look for queries that are named like setups and have
	// scope-level setup that references them inline:
	//   query SetupDB `CREATE (:User)`
	//   SetupDB { setup `CREATE (:User)` }

	// For now, we use a simpler convention:
	// Any query whose name starts with "Setup" or ends with "Setup" is a reusable setup.
	// The query body IS the setup query.
	for _, q := range suite.Queries {
		if isSetupQuery(q.Name) {
			m.Setups[q.Name] = &Setup{
				Name:   q.Name,
				Query:  q.Body,
				Params: extractQueryParams(q.Body),
				Module: m,
			}
		}
	}

	return m
}

// isSetupQuery returns true if the query name indicates it's a reusable setup.
func isSetupQuery(name string) bool {
	lower := strings.ToLower(name)

	return strings.HasPrefix(lower, "setup") || strings.HasSuffix(lower, "setup")
}

// extractQueryParams finds $param references in a query string.
// This is a simple implementation that looks for $identifier patterns.
func extractQueryParams(query string) []string {
	var params []string

	seen := make(map[string]bool)

	// Simple state machine to find $identifier
	inParam := false

	var current strings.Builder

	for _, r := range query {
		if r == '$' {
			inParam = true

			current.Reset()

			continue
		}

		if inParam {
			if isIdentChar(r) {
				current.WriteRune(r)
			} else {
				if current.Len() > 0 {
					name := current.String()
					if !seen[name] {
						params = append(params, name)
						seen[name] = true
					}
				}

				inParam = false
			}
		}
	}

	// Handle param at end of string
	if inParam && current.Len() > 0 {
		name := current.String()
		if !seen[name] {
			params = append(params, name)
		}
	}

	return params
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_'
}

// BaseName returns the module name derived from the file path.
// For "/path/to/fixtures.scaf", returns "fixtures".
func (m *Module) BaseName() string {
	base := filepath.Base(m.Path)

	return strings.TrimSuffix(base, filepath.Ext(base))
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

// ResolveSetup looks up a setup by module alias and name.
// If moduleAlias is empty, it searches the root module.
func (rc *ResolvedContext) ResolveSetup(moduleAlias, name string) (*Setup, error) {
	var target *Module

	if moduleAlias == "" {
		// Local reference - search root module
		target = rc.Root
	} else {
		// Import reference
		var ok bool

		target, ok = rc.Imports[moduleAlias]
		if !ok {
			return nil, &ResolveError{
				Module: moduleAlias,
				Name:   name,
				Cause:  ErrUnknownModule,
			}
		}
	}

	setup, ok := target.Setups[name]
	if !ok {
		return nil, &ResolveError{
			Module:        moduleAlias,
			Name:          name,
			AvailablePath: target.Path,
			Cause:         ErrUnknownSetup,
		}
	}

	return setup, nil
}

// GetQueries returns a combined query map with all queries from root and imports.
// Import queries are prefixed with their alias: "alias.QueryName".
func (rc *ResolvedContext) GetQueries() map[string]string {
	queries := make(map[string]string)

	// Add root queries
	for _, q := range rc.Root.Suite.Queries {
		queries[q.Name] = q.Body
	}

	// Add imported queries with prefix
	for alias, mod := range rc.Imports {
		for _, q := range mod.Suite.Queries {
			queries[alias+"."+q.Name] = q.Body
		}
	}

	return queries
}
