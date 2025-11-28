// Package module provides module loading and dependency resolution for scaf test suites.
package module

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for module operations.
var (
	// ErrModuleNotFound is returned when a module cannot be found at the specified path.
	ErrModuleNotFound = errors.New("module: not found")

	// ErrCyclicDependency is returned when a cycle is detected in the import graph.
	ErrCyclicDependency = errors.New("module: cyclic dependency detected")

	// ErrUnknownModule is returned when a module alias cannot be resolved.
	ErrUnknownModule = errors.New("module: unknown module alias")

	// ErrUnknownQuery is returned when a query cannot be found in a module.
	ErrUnknownQuery = errors.New("module: unknown query")

	// ErrNoSetup is returned when a module has no setup clause.
	ErrNoSetup = errors.New("module: no setup clause")

	// ErrParseError is returned when a module fails to parse.
	ErrParseError = errors.New("module: parse error")
)

// CycleError provides details about a cyclic dependency.
type CycleError struct {
	// Path shows the cycle: [A, B, C, A] means A imports B, B imports C, C imports A.
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("%v: %s", ErrCyclicDependency, strings.Join(e.Path, " -> "))
}

func (e *CycleError) Unwrap() error {
	return ErrCyclicDependency
}

// ResolveError provides details about a failed setup resolution.
type ResolveError struct {
	// Module is the module alias (empty for local reference).
	Module string
	// Name is the setup name being resolved.
	Name string
	// AvailablePath is the path of the module where lookup failed.
	AvailablePath string
	// Cause is the underlying error.
	Cause error
}

func (e *ResolveError) Error() string {
	ref := e.Name
	if e.Module != "" {
		ref = e.Module + "." + e.Name
	}

	if e.AvailablePath != "" {
		return fmt.Sprintf("%v: %s (in %s)", e.Cause, ref, e.AvailablePath)
	}

	return fmt.Sprintf("%v: %s", e.Cause, ref)
}

func (e *ResolveError) Unwrap() error {
	return e.Cause
}

// LoadError provides details about a failed module load.
type LoadError struct {
	// Path is the filesystem path that failed to load.
	Path string
	// ImportedFrom is the module that imported this path (empty for root).
	ImportedFrom string
	// Cause is the underlying error.
	Cause error
}

func (e *LoadError) Error() string {
	if e.ImportedFrom != "" {
		return fmt.Sprintf("failed to load %q (imported from %s): %v", e.Path, e.ImportedFrom, e.Cause)
	}

	return fmt.Sprintf("failed to load %q: %v", e.Path, e.Cause)
}

func (e *LoadError) Unwrap() error {
	return e.Cause
}
