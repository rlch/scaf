package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/boyter/gocodewalker"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/language"
	"github.com/rlch/scaf/module"
	"github.com/urfave/cli/v3"

	// Register bindings and dialects.
	_ "github.com/rlch/scaf/adapters/neogo"
	_ "github.com/rlch/scaf/dialects/cypher"
	_ "github.com/rlch/scaf/language/go"
)

// Generate command errors.
var (
	ErrNoScafFilesForGenerate   = errors.New("no .scaf files found")
	ErrUnknownLanguage          = errors.New("unknown language")
	ErrGenerateDiagnosticErrors = errors.New("scaf files contain errors")
)

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:      "generate",
		Aliases:   []string{"gen"},
		Usage:     "Generate code from scaf files",
		ArgsUsage: "[files or directories...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lang",
				Aliases: []string{"l"},
				Usage:   "target language (go)",
				Value:   "go",
			},
			&cli.StringFlag{
				Name:    "adapter",
				Aliases: []string{"a"},
				Usage:   "database adapter (neogo)",
			},
			&cli.StringFlag{
				Name:    "dialect",
				Aliases: []string{"d"},
				Usage:   "query dialect (cypher)",
			},
			&cli.StringFlag{
				Name:    "out",
				Aliases: []string{"o"},
				Usage:   "output directory (default: same as input file)",
			},
			&cli.StringFlag{
				Name:    "package",
				Aliases: []string{"p"},
				Usage:   "Go package name (default: directory name)",
			},
			&cli.StringFlag{
				Name:    "schema",
				Aliases: []string{"s"},
				Usage:   "path to schema yml file (e.g., .scaf-schema.yml)",
			},
		},
		Action: runGenerate,
	}
}

func runGenerate(ctx context.Context, cmd *cli.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	cfg, configDir, err := loadConfigWithDir(cwd)
	if err != nil {
		cfg = nil
		configDir = cwd
	}

	langName := firstNonEmpty(cmd.String("lang"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Lang }), scaf.LangGo)
	adapterName := firstNonEmpty(cmd.String("adapter"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Adapter }))
	dialectName := firstNonEmpty(cmd.String("dialect"), cfgDialect(cfg), scaf.DialectCypher)
	outputDir := firstNonEmpty(cmd.String("out"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Out }))
	packageName := firstNonEmpty(cmd.String("package"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Package }))
	schemaPath := firstNonEmpty(cmd.String("schema"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Schema }))

	if adapterName == "" {
		if cfg != nil {
			if dbName := cfg.DatabaseName(); dbName != "" {
				adapterName = scaf.AdapterForDatabase(dbName, langName)
			}
		}
		if adapterName == "" && dialectName == scaf.DialectCypher {
			adapterName = scaf.AdapterNeogo
		}
	}

	var schema *analysis.TypeSchema
	if schemaPath != "" {
		schema, err = analysis.LoadSchema(schemaPath, configDir)
		if err != nil {
			return fmt.Errorf("loading schema: %w", err)
		}
	}

	lang := language.Get(langName)
	if lang == nil {
		return fmt.Errorf("%w: %s (available: %v)", ErrUnknownLanguage, langName, language.RegisteredLanguages())
	}

	queryAnalyzer := scaf.GetAnalyzer(dialectName)

	args := cmd.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	packages, err := discoverPackages(args)
	if err != nil {
		return fmt.Errorf("discovering packages: %w", err)
	}

	if len(packages) == 0 {
		return ErrNoScafFilesForGenerate
	}

	// Run analysis on all files first
	semanticAnalyzer := analysis.NewAnalyzerWithQueryAnalyzer(nil, nil, queryAnalyzer)
	var hasErrors bool

	for _, scafFiles := range packages {
		for _, inputFile := range scafFiles {
			data, err := os.ReadFile(inputFile) //nolint:gosec // G304: file path from user input is expected
			if err != nil {
				return fmt.Errorf("reading %s: %w", inputFile, err)
			}

			result := semanticAnalyzer.Analyze(inputFile, data)
			if result.HasErrors() {
				hasErrors = true
				for _, diag := range result.Errors() {
					loc := ""
					if diag.Span.Start.Line > 0 {
						loc = fmt.Sprintf("%s:%d:%d: ", inputFile, diag.Span.Start.Line, diag.Span.Start.Column)
					} else {
						loc = fmt.Sprintf("%s: ", inputFile)
					}
					fmt.Fprintf(os.Stderr, "%serror: %s\n", loc, diag.Message)
				}
			}
		}
	}

	if hasErrors {
		return ErrGenerateDiagnosticErrors
	}

	opts := &generateOptions{
		lang:        lang,
		analyzer:    queryAnalyzer,
		adapterName: adapterName,
		schema:      schema,
		outputDir:   outputDir,
		packageName: packageName,
	}

	for pkgDir, scafFiles := range packages {
		outDir := outputDir
		if outDir == "" {
			outDir = pkgDir
		}

		if err := generateMergedFiles(scafFiles, outDir, opts); err != nil {
			return err
		}
	}

	return nil
}

// discoverPackages finds all .scaf files and groups them by directory.
// Respects .gitignore files.
func discoverPackages(args []string) (map[string][]string, error) {
	packages := make(map[string][]string)
	var mu sync.Mutex

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			if err := walkDir(arg, func(path string) {
				dir := filepath.Dir(path)
				mu.Lock()
				packages[dir] = append(packages[dir], path)
				mu.Unlock()
			}); err != nil {
				return nil, err
			}
		} else if strings.HasSuffix(arg, ".scaf") {
			dir := filepath.Dir(arg)
			packages[dir] = append(packages[dir], arg)
		}
	}

	return packages, nil
}

// walkDir walks a directory for .scaf files, respecting .gitignore.
func walkDir(root string, callback func(path string)) error {
	fileListQueue := make(chan *gocodewalker.File, 100)

	fileWalker := gocodewalker.NewFileWalker(root, fileListQueue)
	fileWalker.AllowListExtensions = []string{"scaf"}

	var walkErr error
	fileWalker.SetErrorHandler(func(e error) bool {
		walkErr = e
		return true
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for f := range fileListQueue {
			callback(f.Location)
		}
	}()

	if err := fileWalker.Start(); err != nil {
		return err
	}

	wg.Wait()
	return walkErr
}

// Helper functions for config access
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cfgString(cfg *scaf.Config, getter func(*scaf.Config) string) string {
	if cfg == nil {
		return ""
	}
	return getter(cfg)
}

func cfgDialect(cfg *scaf.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.DialectName()
}

// generateMergedFiles parses, merges, and generates code for a group of files.
func generateMergedFiles(inputFiles []string, outputDir string, opts *generateOptions) error {
	var inputs []module.ParsedFile
	for _, inputFile := range inputFiles {
		data, err := os.ReadFile(inputFile) //nolint:gosec // G304: file path from user input is expected
		if err != nil {
			return fmt.Errorf("reading %s: %w", inputFile, err)
		}

		suite, err := scaf.Parse(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", inputFile, err)
		}
		inputs = append(inputs, module.ParsedFile{File: suite, Path: inputFile})
	}

	merged, warnings, err := module.MergePackageFiles(inputs)
	if err != nil {
		return fmt.Errorf("merging files: %w", err)
	}

	for _, w := range warnings {
		loc := ""
		if w.Span.Start.Line > 0 {
			loc = fmt.Sprintf("%d:%d: ", w.Span.Start.Line, w.Span.Start.Column)
		}
		fmt.Fprintf(os.Stderr, "%swarning: %s\n", loc, w.Message)
	}

	genCtx := &language.GenerateContext{
		Suite:         merged,
		QueryAnalyzer: opts.analyzer,
		Schema:        opts.schema,
		OutputDir:     outputDir,
		PackageName:   opts.packageName,
		AdapterName:   opts.adapterName,
	}

	files, err := opts.lang.Generate(genCtx)
	if err != nil {
		return fmt.Errorf("generating code for %v: %w", inputFiles, err)
	}

	for filename, content := range files {
		if content == nil {
			continue
		}

		outPath := filepath.Join(outputDir, filename)

		err := os.WriteFile(outPath, content, 0o644) //nolint:gosec // G306: output file permissions are fine
		if err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}

		fmt.Printf("wrote %s\n", outPath)
	}

	return nil
}

// generateOptions holds configuration for file generation.
type generateOptions struct {
	lang        language.Language
	analyzer    scaf.QueryAnalyzer
	adapterName string
	schema      *analysis.TypeSchema
	outputDir   string
	packageName string
}
