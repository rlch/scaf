# Code Completion Design

This document describes the code completion implementation for the scaf LSP.

## Overview

Code completion in scaf needs to be dialect-aware. Different database dialects (Cypher, SQL, etc.) have different query syntaxes, and we need to extract:

1. **Query Parameters** (`$args`) - Variables that need to be provided as inputs
2. **Return Fields** - What columns/fields the query returns, available in child scopes

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        LSP Server                                │
│                   lsp/completion.go                              │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Completion Context                             │
│              Detect where cursor is                              │
│   (scope decl? setup? test param? assertion field?)             │
└───────────────────────────┬─────────────────────────────────────┘
                            │
            ┌───────────────┼───────────────┬───────────────┐
            ▼               ▼               ▼               ▼
      ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐
      │  Query   │   │ Keyword  │   │  Import  │   │  Field   │
      │  Names   │   │Completion│   │  Alias   │   │Completion│
      └──────────┘   └──────────┘   └──────────┘   └────┬─────┘
                                                        │
                                                        ▼
                                          ┌─────────────────────────┐
                                          │    QueryAnalyzer        │
                                          │  (dialect-specific)     │
                                          │                         │
                                          │  - ExtractParameters()  │
                                          │  - ExtractReturns()     │
                                          └─────────────────────────┘
```

## Dialect Query Analyzer

### Interface Extension

Add to `dialect.go`:

```go
// QueryAnalyzer provides static analysis of queries for IDE features.
// This is an optional interface - dialects that don't implement it
// will have limited completion support.
type QueryAnalyzer interface {
    // AnalyzeQuery parses a query and extracts metadata without executing it.
    AnalyzeQuery(query string) (*QueryMetadata, error)
}

// QueryMetadata holds extracted information about a query.
type QueryMetadata struct {
    // Parameters are the $-prefixed variables needed as input.
    // e.g., for "WHERE id = $id AND name = $name" → ["id", "name"]
    Parameters []ParameterInfo

    // Returns are the fields/columns returned by the query.
    // e.g., for "RETURN p.name, p.age" → [{Name: "name", Expr: "p.name"}, {Name: "age", Expr: "p.age"}]
    Returns []ReturnInfo
}

// ParameterInfo describes a query parameter.
type ParameterInfo struct {
    Name     string // Parameter name without $ prefix
    Position int    // Character position in query (for diagnostics)
    Count    int    // How many times it appears
}

// ReturnInfo describes a returned field.
type ReturnInfo struct {
    Name        string // The alias or inferred name (what you'd use to access it)
    Expression  string // The full expression (e.g., "p.name", "count(*)")
    IsAggregate bool   // True for count(), sum(), etc.
    IsWildcard  bool   // True for RETURN * or n.*
}
```

### Cypher Implementation

Move/adapt from `neogo/parser`:

```go
// dialects/cypher/analyzer.go

type Analyzer struct{}

func (a *Analyzer) AnalyzeQuery(query string) (*scaf.QueryMetadata, error) {
    // Parse with ANTLR
    tree, err := parser.ParseCypherQuery(query)
    if err != nil {
        return nil, err
    }

    // Extract parameters (uses argument_tracker logic)
    params := extractParameters(tree)

    // Extract returns (uses return_tracker logic)
    returns := extractReturns(tree)

    return &scaf.QueryMetadata{
        Parameters: params,
        Returns:    returns,
    }, nil
}
```

## Completion Contexts

### 1. Query Name Completion (Scope Declaration)

**Trigger:** Start of line at top-level or after closing brace

**Context Detection:**
```
GetU|         ← cursor here, suggesting "GetUser", "GetUsers"
```

**Source:** All `query` definitions in the file

**Implementation:**
- Check if position is at start of line (accounting for whitespace)
- Check if not inside any existing scope/test/group
- Suggest all query names from `AnalyzedFile.Symbols.Queries`

### 2. Keyword Completion

**Trigger:** Start of line or after specific contexts

**Keywords by context:**
- Top-level: `query`, `import`, `setup`, `teardown`, `<QueryName>`
- Inside scope: `setup`, `teardown`, `"test name"`, `"group name"`
- Inside test: `setup`, `assert`

### 3. Parameter Completion (in tests)

**Trigger:** `$` character inside a test

**Context Detection:**
```
GetUser {
    "finds user" {
        $i|       ← cursor here, suggest "$id" from query
    }
}
```

**Source:** Query parameters extracted via `QueryAnalyzer.AnalyzeQuery()`

**Implementation:**
1. Find enclosing `QueryScope`
2. Look up the query definition
3. Call `QueryAnalyzer.AnalyzeQuery(query.Body)`
4. Suggest parameters from `QueryMetadata.Parameters`

### 4. Return Field Completion (in assertions/child scopes)

**Trigger:** Inside assertion or when referencing parent results

**Context Detection:**
```
GetUser {
    "finds user" {
        $id: 1
        na|       ← cursor here, suggest "name", "email" from query returns
    }
}
```

**Source:** Return fields extracted via `QueryAnalyzer.AnalyzeQuery()`

**Implementation:**
1. Find enclosing `QueryScope`
2. Look up the query definition  
3. Call `QueryAnalyzer.AnalyzeQuery(query.Body)`
4. Suggest fields from `QueryMetadata.Returns`

### 5. Import Alias Completion (in setup)

**Trigger:** After `setup` keyword or inside setup block

**Context Detection:**
```
setup fix|    ← suggest "fixtures" from imports
```

**Source:** Import aliases from `AnalyzedFile.Symbols.Imports`

### 6. Setup Function Completion (after module.)

**Trigger:** `.` after import alias

**Context Detection:**
```
setup fixtures.|    ← suggest available setups from fixtures module
```

**Source:** Queries and setups from the imported module via `FileLoader`

## Implementation Plan

### Phase 1: Basic Completion (No Dialect) ✅

1. **Query name completion** - From symbol table ✅
2. **Keyword completion** - Static list based on context ✅
3. **Import alias completion** - From symbol table ✅

### Phase 2: Dialect-Aware Completion ✅

1. **Add `QueryAnalyzer` interface** to `dialect.go` ✅
2. **Implement Cypher analyzer** - Adapt from `neogo/parser` ✅
3. **Parameter completion** - Using `QueryMetadata.Parameters` ✅
4. **Return field completion** - Using `QueryMetadata.Returns` ✅

### Phase 3: Cross-File Completion ✅

1. **Setup function completion** - Load and analyze imported files ✅
2. **`LSPFileLoader` implementation** - Resolves relative paths, caches loaded files ✅
3. **Shared query completion** - Queries from imported modules ✅

## LSP Capability Configuration

```go
CompletionProvider: &protocol.CompletionOptions{
    TriggerCharacters: []string{"$", "."},
    ResolveProvider:   false, // No lazy resolution needed initially
},
```

## Completion Item Structure

```go
protocol.CompletionItem{
    Label:         "GetUser",                    // What's shown in menu
    Kind:          protocol.FunctionCompletion,  // Icon type
    Detail:        "query",                      // Secondary text
    Documentation: "SELECT * FROM users...",     // Full docs
    InsertText:    "GetUser",                    // What gets inserted
}
```

### Kind Mapping

| Scaf Element | LSP CompletionItemKind |
|--------------|------------------------|
| Query name   | Function               |
| Parameter    | Variable               |
| Return field | Field                  |
| Import alias | Module                 |
| Keyword      | Keyword                |
| Setup func   | Function               |

## Testing Strategy

### Unit Tests

1. **Context detection** - Given cursor position, correctly identify context
2. **Query analysis** - Extract correct params/returns from queries
3. **Completion items** - Correct items for each context

### Integration Tests

1. **LSP protocol** - Send completion request, verify response
2. **Real queries** - Test with actual Cypher/SQL queries

## File Structure

```
scaf/
├── dialect.go              # QueryAnalyzer interface, QueryMetadata types
├── dialects/
│   └── cypher/
│       ├── cypher.go       # Cypher dialect implementation
│       └── analyzer.go     # Cypher query analyzer (ANTLR-based)
├── analysis/
│   ├── analyzer.go         # FileLoader interface, semantic analysis
│   ├── types.go            # AnalyzedFile, SymbolTable, etc.
│   └── position.go         # Position utilities
└── lsp/
    ├── server.go           # LSP server with FileLoader setup
    ├── completion.go       # LSP completion handler
    └── fileloader.go       # LSPFileLoader for cross-file analysis
```

## Example Walkthrough

Given this scaf file:

```scaf
query GetUser `
MATCH (u:User {id: $id})
RETURN u.name, u.email, u.createdAt as created
`

GetUser {
    "finds existing user" {
        $id: 1
        name: "Alice"
        |  ← cursor here
    }
}
```

**Completion flow:**

1. LSP receives `textDocument/completion` at cursor position
2. Context detection: Inside test body, after parameter/field assignments
3. Find enclosing scope: `GetUser`
4. Look up query: `GetUser` → get query body
5. Analyze query: `QueryAnalyzer.AnalyzeQuery(body)`
6. Get returns: `[{name: "name"}, {name: "email"}, {name: "created"}]`
7. Filter already used: `name` is used
8. Return completions: `email`, `created`

## Open Questions

1. **Type inference** - Should we try to infer types from the query? (e.g., `count(*)` returns int)
2. **Validation** - Should completion also validate that used fields exist?
3. **Snippets** - Should parameter completion insert `$param: ${1:value}` snippet?
4. **Fuzzy matching** - How aggressive should fuzzy matching be?

## References

- [LSP Completion Spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#textDocument_completion)
- [gopls completion implementation](https://github.com/golang/tools/tree/master/gopls/internal/golang/completion)
- [neogo parser](../neogo/parser) - Cypher argument/return extraction
