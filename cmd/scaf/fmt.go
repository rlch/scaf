package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlch/scaf"
	"github.com/urfave/cli/v3"
)

var errNoScafFiles = errors.New("no .scaf files found")

const filePermissions = 0o600

func fmtCommand() *cli.Command {
	return &cli.Command{
		Name:      "fmt",
		Aliases:   []string{"format"},
		Usage:     "Format scaf files",
		ArgsUsage: "[files...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "write",
				Aliases: []string{"w"},
				Usage:   "write result to file instead of stdout",
			},
			&cli.BoolFlag{
				Name:    "check",
				Aliases: []string{"c"},
				Usage:   "check if files are formatted (exit 1 if not)",
			},
			&cli.BoolFlag{
				Name:    "diff",
				Aliases: []string{"d"},
				Usage:   "display diffs instead of rewriting files",
			},
		},
		Action: runFmt,
	}
}

func runFmt(_ context.Context, cmd *cli.Command) error {
	write := cmd.Bool("write")
	check := cmd.Bool("check")
	diff := cmd.Bool("diff")
	args := cmd.Args().Slice()

	if len(args) == 0 {
		// Read from stdin
		return formatStdin(os.Stdout)
	}

	// Collect all files to format
	files, err := collectFiles(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return errNoScafFiles
	}

	var unformatted []string

	for _, file := range files {
		changed, err := formatFile(file, write, diff, os.Stdout)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}

		if changed {
			unformatted = append(unformatted, file)
		}
	}

	if check && len(unformatted) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "The following files are not formatted:\n")

		for _, f := range unformatted {
			_, _ = fmt.Fprintf(os.Stderr, "  %s\n", f)
		}

		return cli.Exit("", 1)
	}

	return nil
}

func collectFiles(args []string) ([]string, error) {
	var files []string

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			// Walk directory for .scaf files
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

func formatStdin(out io.Writer) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	suite, err := scaf.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing: %w", err)
	}

	formatted := scaf.Format(suite)
	_, err = out.Write([]byte(formatted))

	return err
}

func formatFile(path string, write, showDiff bool, out io.Writer) (bool, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- paths come from user args
	if err != nil {
		return false, err
	}

	suite, err := scaf.Parse(data)
	if err != nil {
		return false, err
	}

	formatted := scaf.Format(suite)
	changed := string(data) != formatted

	if !changed {
		return false, nil
	}

	if write {
		writeErr := os.WriteFile(path, []byte(formatted), filePermissions)
		if writeErr != nil {
			return true, writeErr
		}

		_, _ = fmt.Fprintf(out, "%s\n", path)

		return true, nil
	}

	if showDiff {
		printDiff(out, path, string(data), formatted)

		return true, nil
	}

	// Default: print formatted output
	_, err = out.Write([]byte(formatted))

	return true, err
}

func printDiff(out io.Writer, path, original, formatted string) {
	_, _ = fmt.Fprintf(out, "diff %s\n", path)
	_, _ = fmt.Fprintf(out, "--- %s\n", path)
	_, _ = fmt.Fprintf(out, "+++ %s\n", path)

	origLines := strings.Split(original, "\n")
	fmtLines := strings.Split(formatted, "\n")

	// Simple line-by-line diff
	maxLines := max(len(origLines), len(fmtLines))

	for i := range maxLines {
		var origLine, fmtLine string

		if i < len(origLines) {
			origLine = origLines[i]
		}

		if i < len(fmtLines) {
			fmtLine = fmtLines[i]
		}

		if origLine != fmtLine {
			if origLine != "" {
				_, _ = fmt.Fprintf(out, "-%s\n", origLine)
			}

			if fmtLine != "" {
				_, _ = fmt.Fprintf(out, "+%s\n", fmtLine)
			}
		}
	}
}
