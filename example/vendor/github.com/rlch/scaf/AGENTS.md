# scaf

Database test scaffolding DSL. Write declarative tests for database queries with inputs, expected outputs, and assertions.

## DSL Syntax

```scaf
import fixtures "./fixtures"

query GetUser `MATCH (u:User {id: $userId}) RETURN u.name, u.age`

setup fixtures                    // run module's setup clause
teardown `MATCH (n) DETACH DELETE n`

GetUser {
  setup fixtures.CreateUsers()    // call query from module
  
  test "finds user" {
    $userId: 1                    // input param
    u.name: "Alice"               // expected output
    assert { u.age > 18 }
  }
}
```

### Setup Syntax

- `setup fixtures` - run imported module's setup clause
- `setup fixtures.Query($arg: 1)` - call a query from imported module
- `setup `inline query`` - inline raw query
- `setup { fixtures; fixtures.Query() }` - block with multiple items

## Project Structure

```
/                   # Root package: parser, lexer, AST, config
├── cmd/scaf/       # CLI: fmt, test, generate commands
├── cmd/scaf-lsp/   # LSP server binary
├── runner/         # Test execution engine, TUI, result handling
├── lsp/            # LSP server: completion, diagnostics, hover, go-to-def
├── analysis/       # Semantic analysis, type schema, rules
├── module/         # Import resolution, module loading
├── language/       # Code generation interface
├── language/go/    # Go code generator
├── dialects/       # Query dialect implementations
├── dialects/cypher/ # Cypher (Neo4j) dialect + ANTLR grammar
├── databases/neo4j/ # Neo4j database adapter
├── adapters/neogo/  # neogo ORM adapter
├── schemas/        # JSON schemas for config
├── example/        # Example scaf files and tests
```

## Key Files

- `ast.go` - AST node definitions (Suite, Query, Test, Assert, etc.)
- `parser.go` - Participle parser with error recovery
- `lexer.go` - Custom lexer (raw strings, comments, operators)
- `config.go` - `.scaf.yaml` config loading
- `runner/runner.go` - Test execution logic
- `lsp/server.go` - LSP server implementation

## Commands

```bash
scaf test [files...]     # Run tests
scaf fmt [files...]      # Format files
scaf generate [files...] # Generate code
```

## Config (`.scaf.yaml`)

```yaml
dialect: cypher
database: neo4j
neo4j:
  uri: bolt://localhost:7687
  user: neo4j
  password: password
```

## Testing

```bash
go test ./...                    # All tests
go test ./runner -run TestRunner # Runner tests
```
