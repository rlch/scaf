package main

import (
	"fmt"
	"path/filepath"

	"github.com/rlch/scaf"
)

// loadConfigWithDir loads config and returns both the config and the directory it was found in.
// Walks up from startDir to find .scaf.yaml.
func loadConfigWithDir(startDir string) (*scaf.Config, string, error) {
	dir := startDir
	for {
		cfg, err := scaf.LoadConfig(dir)
		if err == nil {
			return cfg, dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, no config found
			return nil, startDir, fmt.Errorf("no .scaf.yaml found")
		}
		dir = parent
	}
}
