// Command quibble renders markdown docs to reviewable HTML and manages
// git-native, anchored comment threads on them. See DESIGN.md.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "quibble: design phase — see DESIGN.md for the plan")
	os.Exit(1)
}
