package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".quibble"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadNotInitialized(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("want ErrNotInitialized, got %v", err)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
docs: ["notes/**/*.md"]
theme:
  name: paper
authors:
  human: abdullah
  agent: claude
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Docs[0] != "notes/**/*.md" || cfg.Theme.Name != "paper" ||
		cfg.Authors.Human != "abdullah" || cfg.Authors.Agent != "claude" {
		t.Fatalf("fields wrong: %+v", cfg)
	}
}

func TestLoadDefaultsForOmitted(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
authors:
  human: abdullah
  agent: claude
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Docs) == 0 || cfg.Theme.Name != "paper" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

func TestLoadUnknownKey(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
docs: ["*.md"]
themee:
  name: paper
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "themee") {
		t.Fatalf("want error naming unknown key, got %v", err)
	}
}

func TestValidateEmptyDocs(t *testing.T) {
	c := Default()
	c.Docs = nil
	if err := c.Validate(); err == nil {
		t.Fatal("want error for empty docs")
	}
}

func TestValidateSameAuthors(t *testing.T) {
	c := Default()
	c.Authors.Human = "x"
	c.Authors.Agent = "x"
	if err := c.Validate(); err == nil {
		t.Fatal("want error for identical authors")
	}
}

func TestMatchDocs(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/a.md":        {},
		"docs/x/y/b.md":    {},
		"docs/a.txt":       {},
		"notes/a.md":       {},
		"README.md":        {},
		".quibble/c.md":    {},
		".hidden/d.md":     {},
		"docs/.hid/e.md":   {},
		"docs/x/y/z/c.txt": {},
	}
	cases := []struct {
		globs []string
		want  []string
	}{
		{[]string{"docs/**/*.md"}, []string{"docs/a.md", "docs/x/y/b.md"}},
		{[]string{"*.md"}, []string{"README.md"}},
		{[]string{"docs/**/*.md", "*.md"}, []string{"README.md", "docs/a.md", "docs/x/y/b.md"}},
		{[]string{"**/*.md"}, []string{"README.md", "docs/a.md", "docs/x/y/b.md", "notes/a.md"}},
	}
	for _, c := range cases {
		got, err := MatchDocs(fsys, c.globs)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("globs %v: got %v want %v", c.globs, got, c.want)
		}
	}
}
