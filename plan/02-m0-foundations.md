# M0 — Foundations

**Goal:** a repo where every later milestone can drop code and immediately have build, test, lint, and CI. No product behavior yet beyond `quibble version`.

**Depends on:** nothing. **Blocks:** everything.

## Deliverables

```
go.mod / go.sum          # deps from 01-conventions.md added as used
cmd/quibble/main.go      # thin: calls internal/cli.Execute()
internal/cli/root.go     # cobra root: name, short/long desc, global flags
internal/cli/version.go  # `quibble version` — version string, set via ldflags, default "dev"
internal/config/config.go
internal/config/config_test.go
.github/workflows/ci.yml
Makefile                 # build / test / gate / install targets mirroring 01-conventions §Gate
```

## Spec

### CLI root

- Root command `quibble`, silence cobra's default error+usage spam on runtime errors (`SilenceUsage: true, SilenceErrors: true`; print errors once in `Execute()` to stderr, exit 1).
- Global persistent flags: `--json` (bool, machine output — commands that support it check it; others ignore), `--dir` (string, default `.`: the target repo/project root every command operates on).
- `quibble version` prints `quibble <version> (<commit>)`; both injected via `-ldflags "-X ...`" in the Makefile, defaulting to `dev`/`unknown`.

### Config (`internal/config`)

`.quibble/config.yml` shape (v0.1 fields only — unknown fields error loudly so typos surface):

```yaml
docs: ["docs/**/*.md", "*.md"]   # doublestar globs relative to project root
theme:
  name: paper                     # built-in name; paths are M-later
  overrides: {}                   # map[string]string of --qbl-* token → value
authors:
  human: abdullah                 # default attribution for CLI/web actions
  agent: claude                   # attribution the agent uses
```

API:

```go
type Config struct {
    Docs    []string      `yaml:"docs"`
    Theme   ThemeConfig   `yaml:"theme"`
    Authors AuthorsConfig `yaml:"authors"`
}
func Load(projectRoot string) (*Config, error)  // reads .quibble/config.yml
func Default() *Config                          // the shape `quibble init` writes
func (c *Config) Validate() error
```

- `Load` errors distinguishably when `.quibble/` is missing (`ErrNotInitialized`) — CLI commands other than `init`/`version`/`build` will surface "run `quibble init` first".
- Glob matching: implement with `path.Match` over a walked tree supporting `**` (write a small matcher; ~40 lines; test it — do not add a dep for this).

### CI

Single workflow, jobs on `ubuntu-latest` and `macos-latest`: checkout, setup-go (from go.mod), run the four gate commands from `01-conventions.md`.

## Tests

| # | Case | Expect |
|---|------|--------|
| 1 | `Load` on dir without `.quibble/` | `ErrNotInitialized` |
| 2 | `Load` on valid config | fields populated, defaults applied for omitted keys |
| 3 | Config with unknown top-level key | error naming the key |
| 4 | `Validate` with empty `docs` | error |
| 5 | Glob `docs/**/*.md` | matches `docs/a.md`, `docs/x/y/b.md`; not `docs/a.txt`, `notes/a.md` |
| 6 | Glob `*.md` | matches root `README.md`; not `docs/a.md` |
| 7 | `quibble version` (testscript) | exits 0, output contains version string |
| 8 | `quibble nonsense` (testscript) | exits 1, helpful error, no panic |

## Acceptance

- `make gate` green locally; CI green on push.
- `go run ./cmd/quibble version` works from a fresh clone with only Go installed.
