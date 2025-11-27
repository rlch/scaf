package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/analysis"
)

// LSPFileLoader implements analysis.FileLoader for the LSP server.
// It resolves relative import paths based on document URIs and caches loaded files.
type LSPFileLoader struct {
	logger *zap.Logger

	// workspaceRoot is the root directory of the workspace (from LSP initialize).
	workspaceRoot string

	// mu protects the cache.
	mu sync.RWMutex

	// cache maps absolute file paths to their content.
	cache map[string][]byte

	// analyzed maps absolute file paths to their analyzed files.
	analyzed map[string]*analysis.AnalyzedFile
}

// NewLSPFileLoader creates a new file loader for the LSP server.
func NewLSPFileLoader(logger *zap.Logger, workspaceRoot string) *LSPFileLoader {
	return &LSPFileLoader{
		logger:        logger,
		workspaceRoot: workspaceRoot,
		cache:         make(map[string][]byte),
		analyzed:      make(map[string]*analysis.AnalyzedFile),
	}
}

// Load implements analysis.FileLoader.
// It loads the content of a file at the given path.
// The path may be absolute or relative to some base (resolved by caller).
func (l *LSPFileLoader) Load(path string) ([]byte, error) {
	// Check cache first
	l.mu.RLock()
	if content, ok := l.cache[path]; ok {
		l.mu.RUnlock()
		return content, nil
	}
	l.mu.RUnlock()

	// Load from disk
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Cache it
	l.mu.Lock()
	l.cache[path] = content
	l.mu.Unlock()

	return content, nil
}

// ResolveImportPath resolves a relative import path to an absolute file path.
// basePath is the path of the file containing the import (from document URI).
// importPath is the relative path from the import statement (e.g., "../shared/fixtures").
func (l *LSPFileLoader) ResolveImportPath(basePath, importPath string) string {
	// Get the directory of the base file
	baseDir := filepath.Dir(basePath)

	// Resolve the import path relative to the base directory
	resolved := filepath.Join(baseDir, importPath)

	// Clean the path to resolve .. and .
	resolved = filepath.Clean(resolved)

	// Try the path as-is first (for absolute paths or paths with extension)
	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}

	// If no .scaf extension, try finding the file with extensions
	if !strings.HasSuffix(resolved, ".scaf") {
		// Try .scaf first
		withScaf := resolved + ".scaf"
		if _, err := os.Stat(withScaf); err == nil {
			return withScaf
		}

		// Try dialect-specific extensions (e.g., .cypher.scaf, .sql.scaf)
		// by globbing for any *.scaf file matching the base name
		dir := filepath.Dir(resolved)
		base := filepath.Base(resolved)
		pattern := filepath.Join(dir, base+"*.scaf")

		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) == 1 {
			return matches[0]
		}

		// Fall back to adding .scaf extension (even if file doesn't exist)
		return withScaf
	}

	return resolved
}

// URIToPath converts a document URI to a file system path.
func URIToPath(uri protocol.DocumentURI) string {
	// Parse the URI
	u, err := url.Parse(string(uri))
	if err != nil {
		// Fallback: strip file:// prefix
		return strings.TrimPrefix(string(uri), "file://")
	}

	// For file:// URIs, return the path
	if u.Scheme == "file" {
		return u.Path
	}

	return string(uri)
}

// PathToURI converts a file system path to a document URI.
func PathToURI(path string) protocol.DocumentURI {
	return protocol.DocumentURI("file://" + path)
}

// LoadAndAnalyze loads a file and returns its analysis.
// This is used for cross-file completion to get symbols from imported modules.
func (l *LSPFileLoader) LoadAndAnalyze(path string) (*analysis.AnalyzedFile, error) {
	// Check cache first
	l.mu.RLock()
	if analyzed, ok := l.analyzed[path]; ok {
		l.mu.RUnlock()
		return analyzed, nil
	}
	l.mu.RUnlock()

	// Load the file
	content, err := l.Load(path)
	if err != nil {
		return nil, err
	}

	// Create a temporary analyzer (without loader to avoid infinite recursion for now)
	// TODO: Support recursive imports with cycle detection
	analyzer := analysis.NewAnalyzer(nil)
	result := analyzer.Analyze(path, content)

	// Cache the analysis
	l.mu.Lock()
	l.analyzed[path] = result
	l.mu.Unlock()

	return result, nil
}

// InvalidatePath removes a file from the cache.
// Called when a file is modified.
func (l *LSPFileLoader) InvalidatePath(path string) {
	l.mu.Lock()
	delete(l.cache, path)
	delete(l.analyzed, path)
	l.mu.Unlock()
}

// InvalidateAll clears all cached data.
func (l *LSPFileLoader) InvalidateAll() {
	l.mu.Lock()
	l.cache = make(map[string][]byte)
	l.analyzed = make(map[string]*analysis.AnalyzedFile)
	l.mu.Unlock()
}

// SetWorkspaceRoot updates the workspace root directory.
func (l *LSPFileLoader) SetWorkspaceRoot(root string) {
	l.workspaceRoot = root
}

// Ensure LSPFileLoader implements analysis.FileLoader.
var _ analysis.FileLoader = (*LSPFileLoader)(nil)
