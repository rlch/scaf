package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/runner"
	"github.com/urfave/cli/v3"

	// Register dialects
	_ "github.com/rlch/scaf/dialects/cypher"
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
		},
		Action: runTest,
	}
}

// parsedSuite holds a parsed suite with its source path.
type parsedSuite struct {
	suite *scaf.Suite
	path  string
	data  []byte
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
		return fmt.Errorf("no .scaf files found")
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
		if loadedCfg, err := scaf.LoadConfig(configDir); err == nil {
			dialectName = loadedCfg.Dialect
			if cfg.URI == "" {
				cfg = loadedCfg.Connection
			}
		}
	}

	if dialectName == "" {
		return fmt.Errorf("no dialect specified (use --dialect or .scaf.yaml)")
	}

	if cfg.URI == "" {
		return fmt.Errorf("no connection URI specified (use --uri or .scaf.yaml)")
	}

	// Parse all suites upfront (needed for TUI tree)
	var suites []parsedSuite

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}

		suite, err := scaf.Parse(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", file, err)
		}

		suites = append(suites, parsedSuite{
			suite: suite,
			path:  file,
			data:  data,
		})
	}

	// Create dialect
	dialect, err := scaf.NewDialect(dialectName, cfg)
	if err != nil {
		return fmt.Errorf("failed to create dialect: %w", err)
	}
	defer dialect.Close()

	// Create formatter/handler
	var formatHandler runner.Handler
	verbose := cmd.Bool("verbose")

	if cmd.Bool("json") {
		formatter := runner.NewJSONFormatter(os.Stdout)
		formatHandler = runner.NewFormatHandler(formatter, os.Stderr)
	} else if verbose {
		formatter := runner.NewVerboseFormatter(os.Stdout)
		formatHandler = runner.NewFormatHandler(formatter, os.Stderr)
	} else {
		// Build suite trees for TUI
		trees := make([]runner.SuiteTree, len(suites))
		for i, ps := range suites {
			trees[i] = runner.BuildSuiteTree(ps.suite, ps.path)
		}

		// Use animated TUI with tree view
		tuiHandler := runner.NewTUIHandler(os.Stdout, os.Stderr)
		tuiHandler.SetSuites(trees)

		if err := tuiHandler.Start(); err != nil {
			return fmt.Errorf("failed to start TUI: %w", err)
		}

		formatHandler = tuiHandler
	}

	// Create runner
	r := runner.New(
		runner.WithDialect(dialect),
		runner.WithHandler(formatHandler),
		runner.WithFailFast(cmd.Bool("fail-fast")),
		runner.WithFilter(cmd.String("run")),
	)

	// Run all test files
	var totalResult *runner.Result

	for _, ps := range suites {
		result, err := r.Run(ctx, ps.suite, ps.path)
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
			os.Exit(1)
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