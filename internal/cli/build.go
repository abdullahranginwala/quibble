package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing/fstest"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/abdullahranginwala/quibble/pkg/render"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Render the docs to a static HTML site",
		Long: `Globs the configured docs, renders them to self-contained HTML, and writes
the site (index + one page per doc + assets) to the output directory. Runs
without a .quibble directory too, defaulting to **/*.md and the paper theme.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			dest := out
			if dest == "" {
				dest = filepath.Join(root, "dist")
			} else if !filepath.IsAbs(dest) {
				dest = filepath.Join(root, dest)
			}
			return runBuild(cmd, root, dest)
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output directory (default: dist/ under the project root)")
	return cmd
}

// buildConfig loads config, or synthesizes the zero-config defaults when the
// project has no .quibble directory (the "just give me pretty HTML" path).
func buildConfig(root string) (*config.Config, error) {
	cfg, err := config.Load(root)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, config.ErrNotInitialized) {
		return &config.Config{
			Docs:  []string{"**/*.md"},
			Theme: config.ThemeConfig{Name: "paper", Overrides: map[string]string{}},
		}, nil
	}
	return nil, err
}

// rendererFor builds a renderer honoring the config's theme name and overrides.
// Only the built-in paper theme is wired in v0.1.
func rendererFor(cfg *config.Config) (*render.Renderer, error) {
	if cfg.Theme.Name != "paper" {
		return nil, fmt.Errorf("unknown theme %q (only the built-in \"paper\" theme is available)", cfg.Theme.Name)
	}
	return render.New(render.Options{
		Theme:     render.Paper(),
		Overrides: cfg.Theme.Overrides,
		TOC:       true,
		Title:     "Docs",
	})
}

func runBuild(cmd *cobra.Command, root, dest string) error {
	cfg, err := buildConfig(root)
	if err != nil {
		return withExitCode(1, err)
	}
	r, err := rendererFor(cfg)
	if err != nil {
		return withExitCode(1, err)
	}

	paths, err := config.MatchDocs(os.DirFS(root), cfg.Docs)
	if err != nil {
		return withExitCode(1, fmt.Errorf("globbing docs: %w", err))
	}

	// First pass: read + render each doc individually so a single bad doc does
	// not abort the whole build. Good sources feed a filtered FS that RenderDir
	// turns into the authoritative site (index + pages + assets).
	good := fstest.MapFS{}
	var failures []string
	for _, p := range paths {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		if _, err := r.RenderDoc(src, p); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		good[p] = &fstest.MapFile{Data: src}
	}

	site, err := r.RenderDir(good)
	if err != nil {
		return withExitCode(1, fmt.Errorf("assembling site: %w", err))
	}
	if err := site.WriteTo(dest); err != nil {
		return withExitCode(1, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "wrote %d doc(s) to %s\n", len(good), dest)

	if len(failures) > 0 {
		sort.Strings(failures)
		errOut := cmd.ErrOrStderr()
		fmt.Fprintf(errOut, "build: %d doc(s) failed:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(errOut, "  %s\n", f)
		}
		return withExitCode(1, fmt.Errorf("%d doc(s) failed to render", len(failures)))
	}
	return nil
}
