// Package config loads and validates .quibble/config.yml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNotInitialized is returned by Load when the project has no .quibble directory.
var ErrNotInitialized = errors.New("project not initialized (run `quibble init` first)")

type ThemeConfig struct {
	Name      string            `yaml:"name"`
	Overrides map[string]string `yaml:"overrides"`
}

type AuthorsConfig struct {
	Human string `yaml:"human"`
	Agent string `yaml:"agent"`
}

type Config struct {
	Docs    []string      `yaml:"docs"`
	Theme   ThemeConfig   `yaml:"theme"`
	Authors AuthorsConfig `yaml:"authors"`
}

// Default is the shape `quibble init` writes.
func Default() *Config {
	return &Config{
		Docs:  []string{"docs/**/*.md", "*.md"},
		Theme: ThemeConfig{Name: "paper", Overrides: map[string]string{}},
		Authors: AuthorsConfig{
			Human: "human",
			Agent: "agent",
		},
	}
}

// Path returns the config file path for a project root.
func Path(projectRoot string) string {
	return filepath.Join(projectRoot, ".quibble", "config.yml")
}

// Load reads .quibble/config.yml under projectRoot. Missing .quibble dir
// yields ErrNotInitialized; unknown keys are errors so typos surface.
func Load(projectRoot string) (*Config, error) {
	if _, err := os.Stat(filepath.Join(projectRoot, ".quibble")); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotInitialized
		}
		return nil, fmt.Errorf("checking .quibble: %w", err)
	}
	raw, err := os.ReadFile(Path(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotInitialized
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg := Default()
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", Path(projectRoot), err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Docs) == 0 {
		return errors.New("docs must list at least one glob")
	}
	if c.Theme.Name == "" {
		return errors.New("theme.name must be set")
	}
	if c.Authors.Human == "" || c.Authors.Agent == "" {
		return errors.New("authors.human and authors.agent must be set")
	}
	if c.Authors.Human == c.Authors.Agent {
		return errors.New("authors.human and authors.agent must differ (the resolve policy depends on it)")
	}
	return nil
}

// Marshal renders the config back to YAML (used by `quibble init`).
func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
