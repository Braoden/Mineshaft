// Package cli provides CLI configuration utilities.
package cli

import (
	"os"
	"sync"
)

var (
	name     string
	nameOnce sync.Once
)

// Name returns the Mineshaft CLI command name.
// Defaults to "ms", but can be overridden with MS_COMMAND env var.
// This allows coexistence with other tools that use "ms" (e.g., Graphite).
func Name() string {
	nameOnce.Do(func() {
		name = os.Getenv("MS_COMMAND")
		if name == "" {
			name = "ms"
		}
	})
	return name
}
