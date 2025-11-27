// Package main provides the scaf CLI tool.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	// Register dialects.
	_ "github.com/rlch/scaf/dialects/cypher"
)

var version = "dev"

func main() {
	app := &cli.Command{
		Name:    "scaf",
		Version: version,
		Usage:   "Database test scaffolding DSL tool",
		Commands: []*cli.Command{
			fmtCommand(),
			testCommand(),
			generateCommand(),
		},
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
