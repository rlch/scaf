package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	_ "github.com/rlch/scaf/databases/neo4j"
	_ "github.com/rlch/scaf/dialects/cypher"
	"github.com/rlch/scaf/module"
	"github.com/rlch/scaf/runner"
	"github.com/urfave/cli/v3"
)

// Test command errors.
var (
	ErrNoScafFiles         = errors.New("no .scaf files found")
	ErrNoDatabase          = errors.New("no database specified (use neo4j config in .scaf.yaml)")
	ErrNoConnectionURI     = errors.New("no connection URI specified (use --uri or .scaf.yaml)")
	ErrUnsupportedDatabase = errors.New("unsupported database")
	ErrDiagnosticErrors    = errors.New("scaf files contain errors")
)

func testCommand() *cli.Command {
	return &cli.Command{
		Name:      "test",
		Usage:     "Run scaf tests",
		ArgsUsage: "[files or directories...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "database",
				Aliases: []string{"d"},
				Usage:   "database to use (overrides config)",
			},
			&cli.StringFlag{
				Name:    "uri",
				Usage:   "database connection URI",
				Sources: cli.EnvVars("SCAF_URI"),
			},
			&cli.StringFlag{
				Name:    "username",
				Aliases: []string{"u"},
				Usage:   "database username",
				Sources: cli.EnvVars("SCAF_USER"),
			},
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "database password",
				Sources: cli.EnvVars("SCAF_PASS"),
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "output results as JSON",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "verbose output",
			},
			&cli.BoolFlag{
				Name:  "fail-fast",
				Usage: "stop on first failure",
			},
			&cli.StringFlag{
				Name:  "run",
				Usage: "run only tests matching pattern",
			},
			&cli.BoolFlag{
				Name:   "lag",
				Usage:  "add artificial lag (500ms-1.5s) for TUI testing",
				Hidden: true,
			},
		},
		Action: runTest,
	}
}

// parsedSuite holds a parsed suite with its source path and resolved modules.
type parsedSuite struct {
	suite    *scaf.Suite
	path     string
	data     []byte
	resolved *module.ResolvedContext
}

func runTest(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	// Collect test files
	files, err := collectTestFiles(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return ErrNoScafFiles
	}

	// Load config (walk up from cwd to find .scaf.yaml)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	loadedCfg, _, configErr := loadConfigWithDir(cwd)

	// Determine database name (flag > config)
	databaseName := cmd.String("database")
	if databaseName == "" && configErr == nil {
		databaseName = loadedCfg.DatabaseName()
	}

	if databaseName == "" {
		return ErrNoDatabase
	}

	// Build database config based on database type
	var dbCfg any

	switch databaseName {
	case scaf.DatabaseNeo4j:
		neo4jCfg := &scaf.Neo4jConfig{}
		if configErr == nil && loadedCfg.Neo4j != nil {
			neo4jCfg = loadedCfg.Neo4j
		}
		// Override with flags if provided
		if uri := cmd.String("uri"); uri != "" {
			neo4jCfg.URI = uri
		}
		if username := cmd.String("username"); username != "" {
			neo4jCfg.Username = username
		}
		if password := cmd.String("password"); password != "" {
			neo4jCfg.Password = password
		}
		if neo4jCfg.URI == "" {
			return ErrNoConnectionURI
		}
		dbCfg = neo4jCfg
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedDatabase, databaseName)
	}

	// Parse all suites upfront and resolve modules (needed for TUI tree and named setups)
	suites := make([]parsedSuite, 0, len(files))

	loader := module.NewLoader()
	resolver := module.NewResolver(loader)

	for _, file := range files {
		absPath, err := filepath.Abs(file)
		if err != nil {
			return fmt.Errorf("resolving path %s: %w", file, err)
		}

		// Resolve module dependencies (this also parses the file)
		resolved, err := resolver.Resolve(absPath)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", file, err)
		}

		// Read raw data for display purposes
		data, err := os.ReadFile(file) //nolint:gosec // G304: file path from user input is expected
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}

		suites = append(suites, parsedSuite{
			suite:    resolved.Root.Suite,
			path:     file,
			data:     data,
			resolved: resolved,
		})
	}

	// Run analysis and check for errors before proceeding
	// Get the dialect from config to use proper query analyzer
	dialectName := scaf.DialectCypher // default
	if configErr == nil {
		if d := loadedCfg.DialectName(); d != "" {
			dialectName = d
		}
	}

	queryAnalyzer := scaf.GetAnalyzer(dialectName)
	analyzer := analysis.NewAnalyzerWithQueryAnalyzer(nil, nil, queryAnalyzer)

	var hasErrors bool
	for _, ps := range suites {
		result := analyzer.Analyze(ps.path, ps.data)
		if result.HasErrors() {
			hasErrors = true
			// Print errors
			for _, diag := range result.Errors() {
				loc := ""
				if diag.Span.Start.Line > 0 {
					loc = fmt.Sprintf("%s:%d:%d: ", ps.path, diag.Span.Start.Line, diag.Span.Start.Column)
				} else {
					loc = fmt.Sprintf("%s: ", ps.path)
				}
				fmt.Fprintf(os.Stderr, "%serror: %s\n", loc, diag.Message)
			}
		}
	}

	if hasErrors {
		return ErrDiagnosticErrors
	}

	// Create database
	database, err := scaf.NewDatabase(databaseName, dbCfg)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer func() { _ = database.Close() }()

	// Create formatter/handler
	verbose := cmd.Bool("verbose")

	var formatHandler runner.Handler

	switch {
	case cmd.Bool("json"):
		formatter := runner.NewJSONFormatter(os.Stdout)
		formatHandler = runner.NewFormatHandler(formatter, os.Stderr)
	case verbose:
		formatter := runner.NewVerboseFormatter(os.Stdout)
		formatHandler = runner.NewFormatHandler(formatter, os.Stderr)
	default:
		// Build suite trees for TUI
		trees := make([]runner.SuiteTree, len(suites))
		for i, ps := range suites {
			trees[i] = runner.BuildSuiteTree(ps.suite, ps.path)
		}

		// Use animated TUI with tree view
		tuiHandler := runner.NewTUIHandler(os.Stdout, os.Stderr)
		tuiHandler.SetSuites(trees)

		err := tuiHandler.Start()
		if err != nil {
			return fmt.Errorf("failed to start TUI: %w", err)
		}

		formatHandler = tuiHandler
	}

	// Run all test files
	var totalResult *runner.Result

	for _, ps := range suites {
		// Create runner with module context for this suite
		suiteRunner := runner.New(
			runner.WithDatabase(database),
			runner.WithHandler(formatHandler),
			runner.WithFailFast(cmd.Bool("fail-fast")),
			runner.WithFilter(cmd.String("run")),
			runner.WithModules(ps.resolved),
			runner.WithLag(cmd.Bool("lag")),
		)

		result, err := suiteRunner.Run(ctx, ps.suite, ps.path)
		if err != nil {
			return fmt.Errorf("running %s: %w", ps.path, err)
		}

		if totalResult == nil {
			totalResult = result
		} else {
			totalResult.Merge(result)
		}
	}

	// Print summary
	if totalResult != nil {
		if summarizer, ok := formatHandler.(runner.Summarizer); ok {
			_ = summarizer.Summary(totalResult)
		}

		if !totalResult.Ok() {
			return cli.Exit("", 1)
		}
	}

	return nil
}

func collectTestFiles(args []string) ([]string, error) {
	var files []string

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			err := filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if !d.IsDir() && strings.HasSuffix(path, ".scaf") {
					files = append(files, path)
				}

				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			files = append(files, arg)
		}
	}

	return files, nil
}
