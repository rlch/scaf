package scaf

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .scaf.yaml configuration file.
type Config struct {
	// Default dialect for all .scaf files
	Dialect string `yaml:"dialect"`

	// Connection config for the default dialect
	Connection DialectConfig `yaml:"connection"`

	// Per-pattern overrides (glob pattern -> dialect name)
	// e.g., "integration/*.scaf": "postgres"
	Files map[string]string `yaml:"files,omitempty"`

	// Generate config for code generation
	Generate GenerateConfig `yaml:"generate,omitempty"`
}

// GenerateConfig holds settings for the generate command.
type GenerateConfig struct {
	// Language target (e.g., "go")
	Lang string `yaml:"lang,omitempty"`

	// Database adapter (e.g., "neogo")
	Adapter string `yaml:"adapter,omitempty"`

	// Output directory for generated files
	Out string `yaml:"out,omitempty"`

	// Package name for generated code (Go-specific)
	Package string `yaml:"package,omitempty"`
}

// DefaultConfigNames are the filenames we search for.
var DefaultConfigNames = []string{".scaf.yaml", ".scaf.yml", "scaf.yaml", "scaf.yml"}

// LoadConfig finds and loads the nearest .scaf.yaml walking up from dir.
func LoadConfig(dir string) (*Config, error) {
	path, err := FindConfig(dir)
	if err != nil {
		return nil, err
	}

	return LoadConfigFile(path)
}

// FindConfig searches for a config file starting from dir and walking up.
func FindConfig(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for dir := absDir; ; {
		for _, name := range DefaultConfigNames {
			path := filepath.Join(dir, name)

			_, err := os.Stat(path)
			if err == nil {
				return path, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrConfigNotFound
		}

		dir = parent
	}
}

// LoadConfigFile loads a config from a specific path.
func LoadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DialectFor returns the dialect name for a given file path.
// It checks file-specific patterns first, then falls back to the default.
func (c *Config) DialectFor(filePath string) string {
	for pattern, dialect := range c.Files {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return dialect
		}
	}

	return c.Dialect
}
