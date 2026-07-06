package render

import (
	"encoding/json"
	"strings"
	"testing"
)

// Row 15: NormalizeBlocks behaviour is frozen by a golden file. The assertions
// below document what the golden pins so a future edit that changes them is an
// obvious, deliberate act.
func TestRow15_NormalizeBlocksGolden(t *testing.T) {
	src := readFixture(t, "normalize.md")
	text, blocks := NormalizeBlocks(src)

	out, err := json.MarshalIndent(blocksDump{Text: text, Blocks: blocks}, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	goldenBytes(t, "normalize.json", append(out, '\n'))

	// Whitespace runs collapse to single spaces (and heading text is trimmed).
	if blocks[0].Text != "Heading With Spaces" {
		t.Errorf("whitespace not collapsed in heading: %q", blocks[0].Text)
	}
	if strings.Contains(text, "  ") {
		t.Errorf("normalized text still contains double spaces")
	}

	// The two list items are separate blocks.
	var listItems []string
	for _, b := range blocks {
		if b.Text == "first list item" || b.Text == "second list item" {
			listItems = append(listItems, b.Text)
		}
	}
	if len(listItems) != 2 {
		t.Errorf("list items should be separate blocks, found %v", listItems)
	}

	// The code fence normalizes like any block: interior newlines become spaces.
	var code string
	for _, b := range blocks {
		if strings.Contains(b.Text, "func main()") {
			code = b.Text
		}
	}
	if code == "" {
		t.Fatal("code block not found among blocks")
	}
	if strings.Contains(code, "\n") {
		t.Errorf("code block retained newlines: %q", code)
	}
	if !strings.Contains(code, `println("multi") println("line")`) {
		t.Errorf("code newlines not collapsed to spaces: %q", code)
	}
}

// Blocks join with "\n" and offsets are rune-accurate into Doc.Text.
func TestNormalizeBlocks_Offsets(t *testing.T) {
	// Unicode content to prove rune (not byte) offsets.
	src := []byte("café über\n\nनमस्ते 🙂 done\n")
	text, blocks := NormalizeBlocks(src)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	runes := []rune(text)
	for _, b := range blocks {
		got := string(runes[b.Start:b.End])
		if got != b.Text {
			t.Errorf("offset slice %q != block text %q", got, b.Text)
		}
	}
	// Second block starts after the first block's runes plus one "\n".
	wantStart := len([]rune(blocks[0].Text)) + 1
	if blocks[1].Start != wantStart {
		t.Errorf("block 1 Start = %d, want %d", blocks[1].Start, wantStart)
	}
}

// NormalizeBlocks and the renderer agree on Doc.Text and block IDs.
func TestNormalizeBlocks_MatchesRenderer(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	src := readFixture(t, "kitchen-sink.md")
	doc, err := r.RenderDoc(src, "k.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	text, blocks := NormalizeBlocks(src)
	if text != doc.Text {
		t.Errorf("NormalizeBlocks text differs from Doc.Text")
	}
	if len(blocks) != len(doc.Blocks) {
		t.Fatalf("block count differs: %d vs %d", len(blocks), len(doc.Blocks))
	}
	for i := range blocks {
		if blocks[i] != doc.Blocks[i] {
			t.Errorf("block %d differs: %+v vs %+v", i, blocks[i], doc.Blocks[i])
		}
	}
}
