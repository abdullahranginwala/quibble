package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// Real-browser e2e for the selection → comment flow. The v0.1.0 bug this
// pins: the float button's own click bubbles a mouseup to the document-level
// selection handler, and removing the button there detaches it before its
// click can dispatch — so "select text, press 💬 Comment" silently did
// nothing. That failure mode is invisible to httptest and only reproduces
// under a real event loop, hence headless Chrome.
//
// The test drives native input via chromedp (real mouse drag to select, real
// click on the button), then asserts the panel opens, the POST lands, and the
// thread file appears in .quibble/comments/.
func TestBrowserSelectCommentFlow(t *testing.T) {
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome/Chromium found; skipping browser e2e")
	}

	root := t.TempDir()
	writeFileMk(t, filepath.Join(root, ".quibble", "config.yml"), `
docs: ["docs/**/*.md"]
theme:
  name: paper
authors:
  human: abdullah
  agent: claude
`)
	writeFileMk(t, filepath.Join(root, "docs", "plan.md"), `# Rollout Plan

## Rollback

If error rates exceed one percent we roll back within thirty minutes.
`)
	if err := os.MkdirAll(filepath.Join(root, ".quibble", "comments"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chrome),
		chromedp.Flag("headless", "new"),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 60*time.Second)
	defer cancelT()

	sel := `//p[contains(text(),"roll back within thirty minutes")]`

	// 1. Load the doc page and wait for the comment layer to boot.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL+"/d/docs--plan"),
		chromedp.WaitVisible(sel),
		chromedp.WaitReady("#qbl-comments-root"),
	); err != nil {
		t.Fatalf("loading doc page: %v", err)
	}

	// 2. Select a text range with a real mouse drag across the paragraph, so
	// the document-level mouseup handler runs exactly as it does for a user.
	if err := chromedp.Run(ctx,
		dragSelect(sel, 0.05, 0.60),
		chromedp.WaitVisible(".qbl-float", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("drag-selecting text (float button never appeared): %v", err)
	}

	// 3. Click the float button with real mouse events — the regression: this
	// click's mouseup must not destroy the button before the panel opens.
	if err := chromedp.Run(ctx,
		chromedp.Click(".qbl-float", chromedp.ByQuery),
		chromedp.WaitVisible("#qbl-panel.qbl-open", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("clicking 💬 Comment did not open the panel (the v0.1.0 regression): %v", err)
	}

	// 4. Type a body and submit; the server must accept it (offsets + quote
	// computed by the real frontend against the real DOM).
	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#qbl-panel .qbl-reply-box", "Please clarify rollback timing.", chromedp.ByQuery),
		chromedp.Click(`#qbl-panel [data-act="create"]`, chromedp.ByQuery),
		chromedp.WaitNotPresent("#qbl-panel.qbl-open", chromedp.ByQuery),
		chromedp.WaitVisible("mark.qbl-mark", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("submitting the comment (no mark rendered after create): %v", err)
	}

	// 5. The thread must exist on disk with the selected quote as its anchor.
	dir := filepath.Join(root, ".quibble", "comments", "docs--plan")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want exactly 1 thread file in %s, got %v (err %v)", dir, entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Please clarify rollback timing.", "exact:", "status: open"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("thread file missing %q:\n%s", want, raw)
		}
	}
}

// dragSelect presses the mouse at fracA of the element's width and releases at
// fracB (both on the vertical midline), producing a native text selection and
// a real document-level mouseup — the event the regression lived in.
func dragSelect(xpath string, fracA, fracB float64) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var box struct{ X, Y, W, H float64 }
		if err := chromedp.Evaluate(fmt.Sprintf(`(() => {
			const el = document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
			const r = el.getBoundingClientRect();
			return {X: r.x, Y: r.y, W: r.width, H: r.height};
		})()`, xpath), &box).Do(ctx); err != nil {
			return err
		}
		y := box.Y + box.H/2
		x1 := box.X + box.W*fracA
		x2 := box.X + box.W*fracB
		steps := []struct {
			typ  input.MouseType
			x    float64
			down bool
		}{
			{input.MousePressed, x1, true},
			{input.MouseMoved, (x1 + x2) / 2, true},
			{input.MouseMoved, x2, true},
			{input.MouseReleased, x2, true},
		}
		for _, s := range steps {
			ev := input.DispatchMouseEvent(s.typ, s.x, y).WithButton(input.Left)
			if s.typ == input.MousePressed || s.typ == input.MouseReleased {
				ev = ev.WithClickCount(1)
			}
			if err := ev.Do(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeFileMk(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findChrome() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}
