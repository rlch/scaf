# Refactor: Separate Dialect and Database

## Overview

Split the current `Dialect` interface into two distinct concepts:
- **Dialect**: Query language analysis (parsing, extracting params/returns)
- **Database**: Execution target (connection, query execution)

## Current Architecture

```go
// dialect.go - Mixed concerns
type Dialect interface {
    Name() string
    Execute(ctx, query, params) ([]map[string]any, error)  // execution
    Close() error                                           // connection
}

// Separately registered analyzers
type QueryAnalyzer interface {
    AnalyzeQuery(query string) (*QueryMetadata, error)
}
```

Problems:
1. `Dialect` conflates query language with database driver
2. `cypher` dialect is actually Neo4j-specific
3. Can't have multiple databases using the same query language (e.g., sql → postgres, mysql)
4. Analyzer and Dialect are registered separately but represent the same concept

## Proposed Architecture

```go
// dialect.go - Pure query language
type Dialect interface {
    Name() string  // "cypher", "sql"
    Analyze(query string) (*QueryMetadata, error)
}

// database.go - Execution target  
type Database interface {
    Name() string     // "neo4j", "postgres", "mysql"
    Dialect() Dialect
    Execute(ctx, query, params) ([]map[string]any, error)
    Close() error
}

type Transactional interface {
    Database
    Begin(ctx) (Transaction, error)
}
```

## File Structure

### Before
```
dialects/
  cypher/
    cypher.go      # Neo4j driver + execution (WRONG: this is a database)
    analyzer.go    # Cypher parsing (CORRECT: this is dialect)
    grammar/       # ANTLR grammar
```

### After
```
dialects/
  cypher/
    cypher.go      # Dialect: Analyze() using grammar
    grammar/       # ANTLR grammar
  sql/
    sql.go         # Dialect: Analyze() for SQL

databases/
  neo4j/
    neo4j.go       # Database: uses cypher dialect, neo4j-go-driver
  postgres/
    postgres.go    # Database: uses sql dialect, pgx
  mysql/
    mysql.go       # Database: uses sql dialect, go-sql-driver
```

## Interface Definitions

### dialect.go

```go
package scaf

// Dialect represents a query language (cypher, sql).
// It provides static analysis of queries without database connection.
type Dialect interface {
    // Name returns the dialect identifier (e.g., "cypher", "sql").
    Name() string

    // Analyze extracts metadata from a query string.
    Analyze(query string) (*QueryMetadata, error)
}

// Registration
var dialects = make(map[string]Dialect)

func RegisterDialect(d Dialect) {
    dialects[d.Name()] = d
}

func GetDialect(name string) Dialect {
    return dialects[name]
}
```

### database.go

```go
package scaf

import "context"

// Database represents an execution target (neo4j, postgres).
// It handles connection and query execution.
type Database interface {
    // Name returns the database identifier (e.g., "neo4j", "postgres").
    Name() string

    // Dialect returns the query language this database uses.
    Dialect() Dialect

    // Execute runs a query with parameters and returns results.
    Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

    // Close releases database resources.
    Close() error
}

// Transaction represents an active database transaction.
type Transaction interface {
    Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
}

// Transactional is implemented by databases that support transactions.
type Transactional interface {
    Database
    Begin(ctx context.Context) (Transaction, error)
}

// Database-specific configs
type Neo4jConfig struct {
    URI      string `yaml:"uri"`
    Username string `yaml:"username"`
    Password string `yaml:"password"`
    Database string `yaml:"database,omitempty"`
}

type PostgresConfig struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    Database string `yaml:"database"`
    User     string `yaml:"user"`
    Password string `yaml:"password"`
    SSLMode  string `yaml:"sslmode,omitempty"`
}

// Registration
type DatabaseFactory func(cfg any) (Database, error)

var databases = make(map[string]DatabaseFactory)

func RegisterDatabase(name string, factory DatabaseFactory) {
    databases[name] = factory
}

func NewDatabase(name string, cfg any) (Database, error) {
    factory, ok := databases[name]
    if !ok {
        return nil, fmt.Errorf("unknown database: %s", name)
    }
    return factory(cfg)
}
```

## Implementation: dialects/cypher/cypher.go

```go
package cypher

import "github.com/rlch/scaf"

func init() {
    scaf.RegisterDialect(New())
}

type Dialect struct{}

func New() *Dialect {
    return &Dialect{}
}

func (d *Dialect) Name() string {
    return "cypher"
}

func (d *Dialect) Analyze(query string) (*scaf.QueryMetadata, error) {
    // Move existing analyzer logic here
    tree, err := parseCypherQuery(query)
    if err != nil {
        return nil, err
    }
    
    result := &scaf.QueryMetadata{
        Parameters: []scaf.ParameterInfo{},
        Returns:    []scaf.ReturnInfo{},
    }
    
    extractParameters(tree, result)
    extractReturns(tree, result)
    
    return result, nil
}

var _ scaf.Dialect = (*Dialect)(nil)
```

## Implementation: databases/neo4j/neo4j.go

```go
package neo4j

import (
    "context"
    "github.com/neo4j/neo4j-go-driver/v5/neo4j"
    "github.com/rlch/scaf"
    "github.com/rlch/scaf/dialects/cypher"
)

func init() {
    scaf.RegisterDatabase("neo4j", func(cfg any) (scaf.Database, error) {
        c, ok := cfg.(scaf.Neo4jConfig)
        if !ok {
            return nil, fmt.Errorf("invalid config type for neo4j")
        }
        return New(c)
    })
}

type Database struct {
    driver  neo4j.DriverWithContext
    session neo4j.SessionWithContext
    dialect scaf.Dialect
}

func New(cfg scaf.Neo4jConfig) (*Database, error) {
    auth := neo4j.NoAuth()
    if cfg.Username != "" {
        auth = neo4j.BasicAuth(cfg.Username, cfg.Password, "")
    }
    
    driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
    if err != nil {
        return nil, err
    }
    
    // Verify connectivity...
    
    return &Database{
        driver:  driver,
        session: driver.NewSession(ctx, sessionCfg),
        dialect: cypher.New(),
    }, nil
}

func (d *Database) Name() string {
    return "neo4j"
}

func (d *Database) Dialect() scaf.Dialect {
    return d.dialect
}

func (d *Database) Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
    // Existing execution logic from dialects/cypher/cypher.go
}

func (d *Database) Close() error {
    // Existing close logic
}

func (d *Database) Begin(ctx context.Context) (scaf.Transaction, error) {
    // Existing transaction logic
}

var _ scaf.Database = (*Database)(nil)
var _ scaf.Transactional = (*Database)(nil)
```

## Config Changes

### Before
```yaml
dialect: cypher
connection:
  uri: bolt://localhost:7687
  username: neo4j
  password: password
```

### After
```yaml
neo4j:
  uri: bolt://localhost:7687
  username: neo4j
  password: password

lang: go
# adapter: neogo (inferred from neo4j + go)
```

## Migration Path

### Phase 1: Add new interfaces (non-breaking)
1. Create `database.go` with `Database` interface
2. Create `databases/neo4j/` with new implementation
3. Keep old `Dialect` interface working

### Phase 2: Consolidate Dialect
1. Merge `QueryAnalyzer` into `Dialect` interface
2. Move `analyzer.go` logic into `cypher.go`
3. Remove separate analyzer registration

### Phase 3: Update consumers
1. Update `runner/runner.go`: `Dialect` → `Database`
2. Update `cmd/scaf/test.go`: config loading
3. Update `cmd/scaf/generate.go`: uses `Dialect.Analyze()`

### Phase 4: Cleanup (breaking)
1. Remove old `Dialect` interface (execution methods)
2. Remove `DialectConfig` (replaced by typed configs)
3. Remove `dialects/cypher/cypher.go` execution code

## Affected Files

| File | Change |
|------|--------|
| `dialect.go` | Simplify to just analysis |
| `database.go` | NEW - Database interface |
| `config.go` | Add typed database configs |
| `dialects/cypher/cypher.go` | Remove execution, keep as Dialect |
| `dialects/cypher/analyzer.go` | Merge into cypher.go |
| `databases/neo4j/neo4j.go` | NEW - Neo4j Database impl |
| `runner/runner.go` | Use Database instead of Dialect |
| `cmd/scaf/test.go` | Update config loading |
| `cmd/scaf/generate.go` | Use Dialect.Analyze() |
| `adapters/neogo/binding.go` | No change (uses Dialect for analysis) |

## Inference Chain

```
Config Key  →  Database      →  Dialect  →  Adapter (codegen)
──────────────────────────────────────────────────────────────
neo4j:      →  neo4j         →  cypher   →  neogo
postgres:   →  postgres      →  sql      →  pgx
mysql:      →  mysql         →  sql      →  (future)
sqlite:     →  sqlite        →  sql      →  (future)
```

## Benefits

1. **Clear separation**: Language analysis vs execution
2. **Multiple databases per dialect**: sql → postgres, mysql, sqlite
3. **Typed configs**: Each database has its own config struct
4. **Simpler Dialect**: Just parsing/analysis, no connection state
5. **Testable**: Can test Dialect without database connection
6. **Extensible**: Easy to add new databases for existing dialects
