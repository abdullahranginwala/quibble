package render

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update regenerates golden files instead of comparing against them. Run
//
//	go test ./pkg/render/... -update
//
// and inspect the diff before committing.
var update = flag.Bool("update", false, "update golden files")

// goldenBytes compares got against the golden file at testdata/golden/name,
// writing it instead when -update is set.
func goldenBytes(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", name, err)
	}
	if string(got) != string(want) {
		t.Errorf("golden %s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
