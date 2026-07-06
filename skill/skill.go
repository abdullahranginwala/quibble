// Package skill embeds the Claude Code skill that `quibble init --claude`
// installs into a project's .claude/skills/quibble/ directory.
//
// SKILL.md in this directory is the single source of truth for the skill's
// content; embedding it from here (rather than copying it under internal/)
// keeps the file and the binary in sync by construction. This package carries
// no API promise — it exists only because go:embed cannot cross package
// boundaries (see plan/DECISIONS.md, M6).
package skill

import _ "embed"

// Content is the raw contents of skill/SKILL.md.
//
//go:embed SKILL.md
var Content []byte
