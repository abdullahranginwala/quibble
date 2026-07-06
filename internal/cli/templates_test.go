package cli

import (
	"strings"
	"testing"

	"github.com/abdullahranginwala/quibble/skill"
)

// Row 1 (M6): the AGENTS.md contract written by `init` must keep every
// required section — regressions in the template are product regressions.
func TestAgentsContractSections(t *testing.T) {
	got := string(agentsContract)
	for _, want := range []string{
		"## Discovery",
		".quibble/comments/<doc-slug>/",
		"_resolved/",
		"## The loop",
		"quibble comments reply",
		"quibble comments address",
		"## The two rules",
		"**Never resolve.**",
		"**Never edit thread files by hand.**",
		"## Anchors",
		"quibble doctor",
		"repin",
		"QUIBBLE_AUTHOR",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md template missing %q", want)
		}
	}
}

// The Claude Code skill installed by `init --claude` is embedded straight from
// skill/SKILL.md (the source of truth — no copy to drift). It must carry valid
// skill frontmatter and teach the same contract as AGENTS.md.
func TestClaudeSkillContract(t *testing.T) {
	got := string(skill.Content)
	if !strings.HasPrefix(got, "---\nname: quibble\n") {
		t.Fatal("skill frontmatter must start with name: quibble")
	}
	if !strings.Contains(got, "\ndescription: ") {
		t.Fatal("skill frontmatter must carry a description")
	}
	for _, want := range []string{
		"quibble comments list",
		"quibble comments reply",
		"quibble comments address",
		"resolve",
		"QUIBBLE_AUTHOR",
		"quibble doctor",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
}
