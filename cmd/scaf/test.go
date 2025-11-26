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
	"github.com/rlch/scaf/module"
	"github.com/rlch/scaf/runner"
	"github.com/urfave/cli/v3"
)

var (
	ErrNoScafFiles     = errors.New("no .scaf files found")
	ErrNoDialect       = errors.New("no dialect specified (use --dialect or .scaf.yaml)")
	ErrNoConnectionURI = errors.New("no connection URI specified (use --uri or .scaf.yaml)")
)

func testCommand() *cli.Command {
	return &cli.Command{
		Name:      "test",
		Usage:     "Run scaf tests",
		ArgsUsage: "[files or directories...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "dialect",
				Aliases: []string{"d"},
				Usage:   "dialect to use (overrides config)",
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

	// Load config or use flags
	dialectName := cmd.String("dialect")
	cfg := scaf.DialectConfig{
		URI:      cmd.String("uri"),
		Username: cmd.String("username"),
		Password: cmd.String("password"),
	}

	// Try to load config file if dialect not specified
	if dialectName == "" {
		configDir := filepath.Dir(files[0])

		loadedCfg, err := scaf.LoadConfig(configDir)
		if err == nil {
			dialectName = loadedCfg.Dialect
			if cfg.URI == "" {
				cfg = loadedCfg.Connection
			}
		}
	}

	if dialectName == "" {
		return ErrNoDialect
	}

	if cfg.URI == "" {
		return ErrNoConnectionURI
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

	// Create dialect
	dialect, err := scaf.NewDialect(dialectName, cfg)
	if err != nil {
		return fmt.Errorf("failed to create dialect: %w", err)
	}

	defer func() { _ = dialect.Close() }()

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
			runner.WithDialect(dialect),
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