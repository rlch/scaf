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
	"github.com/rlch/scaf/language"
	"github.com/rlch/scaf/language/go"
	"github.com/urfave/cli/v3"

	// Register bindings.
	_ "github.com/rlch/scaf/adapters/neogo"
)

var ErrNoScafFilesForGenerate = errors.New("no .scaf files found")

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
		},
		Action: runGenerate,
	}
}

func runGenerate(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	// Collect scaf files
	files, err := collectScafFiles(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return ErrNoScafFilesForGenerate
	}

	// Load config from the first file's directory
	configDir := filepath.Dir(files[0])

	var cfg *scaf.Config

	loadedCfg, err := scaf.LoadConfig(configDir)
	if err == nil {
		cfg = loadedCfg
	}

	// Get settings from flags, falling back to config
	langName := cmd.String("lang")
	if langName == "" && cfg != nil && cfg.Generate.Lang != "" {
		langName = cfg.Generate.Lang
	}

	if langName == "" {
		langName = "go" // default
	}

	adapterName := cmd.String("adapter")
	if adapterName == "" && cfg != nil && cfg.Generate.Adapter != "" {
		adapterName = cfg.Generate.Adapter
	}

	dialectName := cmd.String("dialect")
	if dialectName == "" && cfg != nil && cfg.Dialect != "" {
		dialectName = cfg.Dialect
	}

	if dialectName == "" {
		dialectName = "cypher" // default
	}

	// Infer adapter from dialect if not specified
	if adapterName == "" {
		switch dialectName {
		case "cypher":
			adapterName = "neogo"
		}
	}

	outputDir := cmd.String("out")
	if outputDir == "" && cfg != nil && cfg.Generate.Out != "" {
		outputDir = cfg.Generate.Out
	}

	packageName := cmd.String("package")
	if packageName == "" && cfg != nil && cfg.Generate.Package != "" {
		packageName = cfg.Generate.Package
	}

	// Get language
	lang := language.Get(langName)
	if lang == nil {
		return fmt.Errorf("unknown language: %s (available: %v)", langName, language.RegisteredLanguages())
	}

	// Validate adapter support for the language
	goLang, ok := lang.(*golang.GoLanguage)
	if !ok {
		return fmt.Errorf("language %q does not support code generation with adapters", langName)
	}

	var binding golang.Binding
	if adapterName != "" {
		binding = golang.GetBinding(adapterName)
		if binding == nil {
			return fmt.Errorf("unknown adapter %q for language %q (available: %v)", adapterName, langName, golang.RegisteredBindings())
		}
	}

	analyzer := scaf.GetAnalyzer(dialectName)

	// Process each file
	for _, inputFile := range files {
		err := generateFile(inputFile, goLang, analyzer, binding, outputDir, packageName)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateFile(
	inputFile string,
	goLang *golang.GoLanguage,
	analyzer scaf.QueryAnalyzer,
	binding golang.Binding,
	outputDir string,
	packageName string,
) error {
	// Read and parse the scaf file
	data, err := os.ReadFile(inputFile) //nolint:gosec // G304: file path from user input is expected
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputFile, err)
	}

	suite, err := scaf.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", inputFile, err)
	}

	// Determine output directory (default: same as input file)
	fileOutputDir := outputDir
	if fileOutputDir == "" {
		fileOutputDir = filepath.Dir(inputFile)
	}

	// Determine package name (default: directory name)
	filePackageName := packageName
	if filePackageName == "" {
		filePackageName = filepath.Base(fileOutputDir)
		// Clean up package name (remove invalid characters)
		filePackageName = strings.ReplaceAll(filePackageName, "-", "")
		filePackageName = strings.ReplaceAll(filePackageName, ".", "")
		if filePackageName == "" {
			filePackageName = "main"
		}
	}

	goCtx := &golang.Context{
		GenerateContext: language.GenerateContext{
			Suite:         suite,
			QueryAnalyzer: analyzer,
			OutputDir:     fileOutputDir,
		},
		PackageName: filePackageName,
		Binding:     binding,
	}

	files, err := goLang.GenerateWithContext(goCtx)
	if err != nil {
		return fmt.Errorf("generating code for %s: %w", inputFile, err)
	}

	// Write output files
	for filename, content := range files {
		if content == nil {
			continue
		}

		outPath := filepath.Join(fileOutputDir, filename)

		err := os.WriteFile(outPath, content, 0o644) //nolint:gosec // G306: output file permissions are fine
		if err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}

		fmt.Printf("wrote %s\n", outPath)
	}

	return nil
}

func collectScafFiles(args []string) ([]string, error) {
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
