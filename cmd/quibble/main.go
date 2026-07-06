// Command quibble renders markdown docs to reviewable HTML and manages
// git-native, anchored comment threads on them. See DESIGN.md.
package main

import (
	"os"

	"github.com/abdullahranginwala/quibble/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
