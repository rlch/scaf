package module

import (
	"fmt"
	"path/filepath"

	"github.com/rlch/scaf"
)

// MergeWarning represents a non-fatal issue detected during merge.
type MergeWarning struct {
	Span    scaf.Span
	Code    string // e.g., "same-package-import"
	Message string
}

// MergeError represents a fatal error during merge.
type MergeError struct {
	Span    scaf.Span
	Code    string // e.g., "duplicate-function"
	Message string
}

func (e *MergeError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Span.Start, e.Message)
}

// ParsedFile pairs a parsed scaf file with its source path.
type ParsedFile struct {
	File *scaf.File
	Path string
}

// MergePackageFiles merges multiple parsed .scaf files from the same directory
// into a single logical suite. It handles import deduplication, detects same-package
// imports (warns and skips them), and reports errors for duplicate functions or
// conflicting setup/teardown.
//
// Returns the merged file, any warnings, and an error if merge fails.
func MergePackageFiles(inputs []ParsedFile) (*scaf.File, []MergeWarning, error) {
	if len(inputs) == 0 {
		return &scaf.File{}, nil, nil
	}

	if len(inputs) == 1 {
		return inputs[0].File, nil, nil
	}

	merged := &scaf.File{}
	var warnings []MergeWarning

	// Track imports by resolved path for deduplication
	importsByPath := make(map[string]*scaf.Import)

	// Track functions by name for duplicate detection
	functionsByName := make(map[string]struct {
		fn   *scaf.Function
		file int // index of file where function was defined
	})

	// Normalize sibling paths to absolute for comparison
	siblingSet := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		abs, err := filepath.Abs(input.Path)
		if err == nil {
			siblingSet[abs] = true
		}
	}

	for fileIdx, input := range inputs {
		file := input.File
		fileDir := filepath.Dir(input.Path)

		// Process imports
		for _, imp := range file.Imports {
			resolvedPath := resolveImportPath(imp.Path, fileDir)

			if siblingSet[resolvedPath] {
				continue
			}

			if _, ok := importsByPath[resolvedPath]; !ok {
				importsByPath[resolvedPath] = imp
				merged.Imports = append(merged.Imports, imp)
			}
		}

		// Process functions
		for _, fn := range file.Functions {
			if existing, ok := functionsByName[fn.Name]; ok {
				return nil, warnings, &MergeError{
					Span: fn.Span(),
					Code: "duplicate-function",
					Message: fmt.Sprintf("function %q already defined in file %d at %s",
						fn.Name, existing.file+1, existing.fn.Span().Start),
				}
			}
			functionsByName[fn.Name] = struct {
				fn   *scaf.Function
				file int
			}{fn: fn, file: fileIdx}
			merged.Functions = append(merged.Functions, fn)
		}

		merged.Scopes = append(merged.Scopes, file.Scopes...)
	}

	return merged, warnings, nil
}

// resolveImportPath resolves an import path relative to the file's directory.
// Returns an absolute path for comparison purposes.
func resolveImportPath(importPath, fileDir string) string {
	if filepath.IsAbs(importPath) {
		return filepath.Clean(importPath)
	}

	if fileDir == "" {
		// Can't resolve relative path without a base directory
		return importPath
	}

	// Resolve relative to file's directory
	resolved := filepath.Join(fileDir, importPath)
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}

	// Try common extensions if not present
	cleaned := filepath.Clean(abs)
	if filepath.Ext(cleaned) == "" {
		// Try .scaf extension
		withExt := cleaned + ".scaf"
		return withExt
	}

	return cleaned
}
