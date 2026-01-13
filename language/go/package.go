// Package golang provides Go code generation from scaf DSL files.
//
// This file implements package name inference using a ladder of stdlib strategies.

package golang

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// InferPackageName determines the Go package name for a directory.
//
// It uses a ladder of strategies in order of preference:
//  1. go/build.ImportDir - respects build tags and returns the canonical package name
//  2. parser.ParseFile with PackageClauseOnly - catches build-tag-hidden .go files
//  3. SanitizePackageName(filepath.Base(dir)) - fallback using folder name
//
// Returns an error only if the directory cannot be accessed.
func InferPackageName(dir string) (string, error) {
	// Strategy 1: Use go/build which respects build tags
	if pkg, err := build.ImportDir(dir, 0); err == nil && pkg.Name != "" {
		return pkg.Name, nil
	}

	// Strategy 2: Parse any .go file's package clause directly
	// This catches files hidden by build tags
	if name := parseAnyPackageClause(dir); name != "" {
		return name, nil
	}

	// Strategy 3: Fallback to sanitized directory name
	return SanitizePackageName(filepath.Base(dir)), nil
}

// SanitizePackageName converts a string to a valid Go package name.
//
// It applies the following transformations:
//   - Removes invalid characters (hyphens, dots, spaces)
//   - Converts to lowercase
//   - Prefixes with "pkg" if empty or starts with a digit
//   - Suffixes with "pkg" if the result is a Go keyword
//
// Examples:
//
//	"my-package" -> "mypackage"
//	"My.Package" -> "mypackage"
//	"123start"   -> "pkg123start"
//	"type"       -> "typepkg"
func SanitizePackageName(name string) string {
	var b strings.Builder
	for _, r := range name {
		// Keep only letters, digits, and underscores
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(unicode.ToLower(r))
		}
		// Skip hyphens, dots, spaces, and other invalid chars
	}

	result := b.String()

	// Empty result or starts with digit: prefix with "pkg"
	if result == "" || unicode.IsDigit(rune(result[0])) {
		result = "pkg" + result
	}

	// Go keyword: suffix with "pkg"
	if IsKeyword(result) {
		result = result + "pkg"
	}

	return result
}

// IsKeyword returns true if name is a Go keyword.
func IsKeyword(name string) bool {
	return token.Lookup(name).IsKeyword()
}

// parseAnyPackageClause finds any .go file in dir and extracts its package name.
// Returns empty string if no .go files exist or parsing fails.
func parseAnyPackageClause(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		// Skip test files - they might have _test package suffix
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		// Parse only the package clause for efficiency
		f, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
		if err != nil {
			continue
		}
		if f.Name != nil && f.Name.Name != "" {
			return f.Name.Name
		}
	}

	return ""
}
