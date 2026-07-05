// ms is the Mineshaft CLI for managing multi-agent workspaces.
package main

import (
	"os"

	"github.com/steveyegge/mineshaft/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
