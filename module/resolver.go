package module

import "github.com/rlch/scaf"

// Resolver handles module dependency resolution and cycle detection.
type Resolver struct {
	loader *Loader
}

// NewResolver creates a new module resolver with the given loader.
func NewResolver(loader *Loader) *Resolver {
	return &Resolver{
		loader: loader,
	}
}

// Resolve loads a module and all its transitive dependencies.
// Returns a ResolvedContext containing all modules needed for execution.
// Detects and reports cyclic dependencies.
func (r *Resolver) Resolve(rootPath string) (*ResolvedContext, error) {
	// Load the root module
	root, err := r.loader.Load(rootPath)
	if err != nil {
		return nil, err
	}

	ctx := NewResolvedContext(root)

	// Track visited modules for cycle detection
	// visiting = currently in the DFS stack (gray nodes)
	// visited = fully processed (black nodes)
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	// Resolve all imports starting from root.
	err = r.resolveImports(root, ctx, visiting, visited, []string{root.Path})
	if err != nil {
		return nil, err
	}

	return ctx, nil
}

// resolveImports recursively loads and resolves imports for a module.
func (r *Resolver) resolveImports( //nolint:funcorder
	mod *Module,
	ctx *ResolvedContext,
	visiting, visited map[string]bool,
	path []string,
) error {
	// Mark as currently visiting
	visiting[mod.Path] = true

	// Process each import
	for _, imp := range mod.Suite.Imports {
		// Load the imported module
		imported, err := r.loader.LoadFrom(imp.Path, mod)
		if err != nil {
			return err
		}

		// Check for cycle
		if visiting[imported.Path] {
			// Found a cycle - construct the cycle path
			cyclePath := append(path, imported.Path) //nolint:gocritic // intentional append to new slice

			return &CycleError{Path: cyclePath}
		}

		// Add to context with the appropriate alias
		var alias string
		if imp.Alias != nil {
			alias = *imp.Alias
		} else {
			// Use the base name if no alias specified
			alias = imported.BaseName()
		}

		// Check for alias collision
		if existing, ok := ctx.Imports[alias]; ok && existing.Path != imported.Path {
			// Same alias pointing to different modules - error
			return &ResolveError{
				Module: alias,
				Name:   "",
				Cause:  ErrDuplicateSetup, // TODO: more specific error
			}
		}

		ctx.Imports[alias] = imported
		ctx.AllModules[imported.Path] = imported

		// Recursively resolve if not already visited
		if !visited[imported.Path] {
			newPath := append(path, imported.Path) //nolint:gocritic // intentional append to new slice

			err := r.resolveImports(imported, ctx, visiting, visited, newPath)
			if err != nil {
				return err
			}
		}
	}

	// Mark as fully visited
	visiting[mod.Path] = false
	visited[mod.Path] = true

	return nil
}

// ResolveFromSuite creates a ResolvedContext for an already-parsed suite.
// This is useful when the suite is parsed separately (e.g., in tests).
func (r *Resolver) ResolveFromSuite(path string, suite *scaf.Suite) (*ResolvedContext, error) {
	// Create the root module from the provided suite
	root := NewModule(path, suite)

	// Cache it in the loader so imports can find it
	r.loader.cache[root.Path] = root

	ctx := NewResolvedContext(root)

	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	err := r.resolveImports(root, ctx, visiting, visited, []string{root.Path})
	if err != nil {
		return nil, err
	}

	return ctx, nil
}
