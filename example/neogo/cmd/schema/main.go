// Command schema generates a .scaf-schema.json file from neogo models.
//
// Usage:
//
//	go run ./cmd/schema > .scaf-schema.json
package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/rlch/scaf/adapters/neogo"
	"github.com/rlch/scaf/example/neogo/models"
)

func main() {
	adapter := neogo.NewAdapter(
		&models.Person{},
		&models.Movie{},
		&models.ActedIn{},
		&models.Directed{},
		&models.Review{},
		&models.Follows{},
	)

	schema, err := adapter.ExtractSchema()
	if err != nil {
		log.Fatalf("failed to extract schema: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		log.Fatalf("failed to encode schema: %v", err)
	}
}
