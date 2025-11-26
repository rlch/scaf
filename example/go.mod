module github.com/rlch/scaf/example

go 1.25.1

require (
	github.com/rlch/neogo v0.0.0
	github.com/rlch/scaf v0.0.0
)

require (
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/rlch/neogo => ../../neogo

replace github.com/rlch/scaf => ..
