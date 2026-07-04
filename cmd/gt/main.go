// gt is the Excavation Site CLI for managing multi-agent workspaces.
package main

import (
	"os"

	"github.com/steveyegge/excavation/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
