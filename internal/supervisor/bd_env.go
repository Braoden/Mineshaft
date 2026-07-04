package supervisor

import (
	"os"
	"path/filepath"

	"github.com/steveyegge/excavation/internal/beads"
)

func supervisorReadOnlyRoutingEnv(townRoot string) []string {
	return beads.BuildReadOnlyRoutingBDEnv(os.Environ(), townBeadsDir(townRoot))
}

func supervisorMutationRoutingEnv(townRoot string) []string {
	return beads.BuildMutationRoutingBDEnv(os.Environ(), townBeadsDir(townRoot))
}

func townBeadsDir(townRoot string) string {
	if townRoot == "" {
		return ""
	}
	return filepath.Join(townRoot, ".beads")
}
