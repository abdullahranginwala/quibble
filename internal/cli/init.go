package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/abdullahranginwala/quibble/skill"
	"github.com/spf13/cobra"
)

//go:embed templates/AGENTS.md
var agentsContract []byte

// claudeMarker guards the CLAUDE.md pointer block against double-appending.
const claudeMarker = "<!-- quibble -->"

// claudePointer is the block appended to an existing CLAUDE.md. It leads with
// the marker so re-running init never appends it twice.
const claudePointer = claudeMarker + `
This repo uses quibble for doc review — open comment threads live in
` + "`.quibble/comments/`" + `. See ` + "`.quibble/AGENTS.md`" + ` for the agent workflow.
`

func newInitCmd() *cobra.Command {
	var claude bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize quibble in the current repository",
		Long: `Creates .quibble/ with a default config, an empty comments store, and the
agent contract. Idempotent: re-running on an initialized repo changes nothing.
Refuses to run outside a git work tree, since comments live in git.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			return runInit(cmd, root, claude)
		},
	}
	cmd.Flags().BoolVar(&claude, "claude", false, "also install the Claude Code skill into .claude/skills/quibble/ (when .claude/ exists)")
	return cmd
}

func runInit(cmd *cobra.Command, root string, claude bool) error {
	if !inGitWorkTree(root) {
		return withExitCode(1, fmt.Errorf(
			"%s is not inside a git work tree; run `git init` first (quibble stores comments in git)", root))
	}

	qdir := filepath.Join(root, ".quibble")
	cfgPath := config.Path(root)
	if fileExists(cfgPath) {
		fmt.Fprintln(cmd.OutOrStdout(), "already initialized")
		if claude {
			return installClaudeSkill(cmd, root)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Join(qdir, "comments"), 0o755); err != nil {
		return withExitCode(1, fmt.Errorf("creating .quibble/comments: %w", err))
	}

	cfgYAML, err := config.Default().Marshal()
	if err != nil {
		return withExitCode(1, fmt.Errorf("rendering default config: %w", err))
	}
	if err := os.WriteFile(cfgPath, cfgYAML, 0o644); err != nil {
		return withExitCode(1, fmt.Errorf("writing %s: %w", cfgPath, err))
	}
	if err := os.WriteFile(filepath.Join(qdir, "comments", ".gitkeep"), nil, 0o644); err != nil {
		return withExitCode(1, fmt.Errorf("writing .gitkeep: %w", err))
	}
	if err := os.WriteFile(filepath.Join(qdir, "AGENTS.md"), agentsContract, 0o644); err != nil {
		return withExitCode(1, fmt.Errorf("writing AGENTS.md: %w", err))
	}

	appended, err := maybeAppendClaudePointer(root)
	if err != nil {
		return withExitCode(1, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "initialized quibble in %s\n", qdir)
	if appended {
		fmt.Fprintln(out, "appended a quibble pointer to CLAUDE.md")
	}
	if claude {
		return installClaudeSkill(cmd, root)
	}
	return nil
}

// installClaudeSkill copies the embedded Claude Code skill (skill/SKILL.md,
// embedded via the skill package) into .claude/skills/quibble/ when the repo
// already uses Claude (.claude/ exists); otherwise it skips with a message
// rather than creating .claude/ uninvited.
func installClaudeSkill(cmd *cobra.Command, root string) error {
	out := cmd.OutOrStdout()
	if fi, err := os.Stat(filepath.Join(root, ".claude")); err != nil || !fi.IsDir() {
		fmt.Fprintln(out, "no .claude/ directory here — skipping Claude skill install")
		return nil
	}
	dir := filepath.Join(root, ".claude", "skills", "quibble")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return withExitCode(1, fmt.Errorf("creating %s: %w", dir, err))
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), skill.Content, 0o644); err != nil {
		return withExitCode(1, fmt.Errorf("writing skill: %w", err))
	}
	fmt.Fprintf(out, "installed Claude Code skill at %s\n", filepath.Join(dir, "SKILL.md"))
	return nil
}

// maybeAppendClaudePointer appends the pointer block to an existing CLAUDE.md
// that lacks the marker. It never creates the file.
func maybeAppendClaudePointer(root string) (bool, error) {
	p := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading CLAUDE.md: %w", err)
	}
	if strings.Contains(string(data), claudeMarker) {
		return false, nil
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false, fmt.Errorf("opening CLAUDE.md: %w", err)
	}
	defer f.Close()
	block := "\n" + claudePointer
	if !strings.HasSuffix(string(data), "\n") {
		block = "\n" + block
	}
	if _, err := f.WriteString(block); err != nil {
		return false, fmt.Errorf("appending to CLAUDE.md: %w", err)
	}
	return true, nil
}
