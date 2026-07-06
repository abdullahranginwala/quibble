## What & why

<!-- What changes, and the problem it solves. Link the issue if one exists. -->

## Checklist

- [ ] `make gate` passes locally (gofmt, vet, build, race tests)
- [ ] New behavior has tests; bug fixes have a regression test
- [ ] No changes to frozen surfaces (thread file format, CLI `--json` fields) — or an issue discussed it first
- [ ] Respects DESIGN.md non-goals (never mutates user markdown; frontend stays zero-dep)
