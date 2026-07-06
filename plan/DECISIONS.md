# Decisions log (append-only)

- 2026-07-07 M0: `go.mod` directive is `1.25`, not `1.24` — `go-internal@v1.15` (testscript) requires 1.25; toolchain installed is 1.26.4. Conventions file said 1.24; bump was forced by the allowed dep set.
- 2026-07-07 M0: `config.Default()` authors are `human`/`agent` (generic); `quibble init` keeps them — users personalize in config. Validate() additionally requires human ≠ agent since the resolve policy gate depends on distinguishing them.
- 2026-07-07 M0: glob matcher also skips all dot-directories (not just `.quibble`) during doc walks — hidden dirs as doc sources are out of scope.
