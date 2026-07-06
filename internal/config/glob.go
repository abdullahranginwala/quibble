package config

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// MatchDocs walks fsys and returns the relative paths (slash-separated,
// sorted) matching any of the globs. Globs support `**` for any number of
// path segments (including zero) plus the usual path.Match syntax per
// segment. The .quibble directory and dot-directories are always skipped.
func MatchDocs(fsys fs.FS, globs []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if p != "." && (name == ".quibble" || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		for _, g := range globs {
			ok, err := matchGlob(g, p)
			if err != nil {
				return err
			}
			if ok && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// matchGlob matches a slash path against a glob whose segments may be `**`.
func matchGlob(glob, name string) (bool, error) {
	return matchSegs(strings.Split(glob, "/"), strings.Split(name, "/"))
}

// MatchGlob reports whether a slash-separated path matches a single glob whose
// segments may be `**` (any number of segments). Exported for the serve watcher,
// which classifies changed paths against the configured doc globs.
func MatchGlob(glob, name string) (bool, error) {
	return matchGlob(glob, name)
}

func matchSegs(pattern, segs []string) (bool, error) {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// `**` matches zero or more leading segments.
			for i := 0; i <= len(segs); i++ {
				ok, err := matchSegs(pattern[1:], segs[i:])
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		if len(segs) == 0 {
			return false, nil
		}
		ok, err := path.Match(pattern[0], segs[0])
		if err != nil || !ok {
			return ok, err
		}
		pattern, segs = pattern[1:], segs[1:]
	}
	return len(segs) == 0, nil
}
