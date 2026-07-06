package cli

import (
	"os"
	"regexp"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"quibble": Execute,
	}))
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// readfirst VAR FILE REGEXP: read FILE, find the first regexp match,
			// and bind it to environment variable VAR for later commands. Lets a
			// script capture a freshly minted (random) thread id from `add` output.
			"readfirst": func(ts *testscript.TestScript, neg bool, args []string) {
				if len(args) != 3 {
					ts.Fatalf("usage: readfirst VAR FILE REGEXP")
				}
				data := ts.ReadFile(args[1])
				re := regexp.MustCompile(args[2])
				m := re.FindString(data)
				if neg {
					if m != "" {
						ts.Fatalf("readfirst: unexpected match %q", m)
					}
					return
				}
				if m == "" {
					ts.Fatalf("readfirst: no match for %q in %s", args[2], args[1])
				}
				ts.Setenv(args[0], m)
			},
		},
	})
}
