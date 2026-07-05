// ABOUTME: Hidden command for shell hook to detect rigs and update cache.
// ABOUTME: Called by shell integration to set MS_TOWN_ROOT and MS_RIG env vars.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/state"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var rigDetectCache string

var rigDetectCmd = &cobra.Command{
	Use:    "detect [path]",
	Short:  "Detect rig from repository path (internal use)",
	Hidden: true,
	Long: `Detect rig from a repository path and optionally cache the result.

This is an internal command used by shell integration. It checks if the given
path is inside a Mineshaft rig and outputs shell variable assignments.

When --cache is specified, the result is written to ~/.cache/mineshaft/rigs.cache
for fast lookups by the shell hook.

Output format (to stdout):
  export MS_TOWN_ROOT=/path/to/town
  export MS_ROOT=/path/to/town
  export MS_RIG=rigname

Or if not in a rig:
  unset MS_TOWN_ROOT MS_ROOT MS_RIG`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRigDetect,
}

func init() {
	rigCmd.AddCommand(rigDetectCmd)
	rigDetectCmd.Flags().StringVar(&rigDetectCache, "cache", "", "Repository path to cache detection result for")
}

func runRigDetect(cmd *cobra.Command, args []string) error {
	checkPath := "."
	if len(args) > 0 {
		checkPath = args[0]
	}

	absPath, err := filepath.Abs(checkPath)
	if err != nil {
		return outputNotInRig()
	}

	townRoot, err := workspace.Find(absPath)
	if err != nil || townRoot == "" {
		return outputNotInRig()
	}

	rigName := detectRigFromPath(townRoot, absPath)

	if rigName != "" {
		printEnvSet("MS_TOWN_ROOT", townRoot)
		printEnvSet("MS_ROOT", townRoot)
		printEnvSet("MS_RIG", rigName)
	} else {
		printEnvSet("MS_TOWN_ROOT", townRoot)
		printEnvSet("MS_ROOT", townRoot)
		printEnvUnset("MS_RIG")
	}

	if rigDetectCache != "" {
		if err := updateRigCache(rigDetectCache, townRoot, rigName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update cache: %v\n", err)
		}
	}

	return nil
}

func detectRigFromPath(townRoot, absPath string) string {
	rel, err := filepath.Rel(townRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "." {
		return ""
	}

	candidateRig := parts[0]

	switch candidateRig {
	case constants.RoleOverseer, constants.RoleSupervisor, ".beads", ".claude", ".git", "plugins":
		return ""
	}

	rigConfigPath := filepath.Join(townRoot, candidateRig, "config.json")
	if _, err := os.Stat(rigConfigPath); err == nil {
		return candidateRig
	}

	return ""
}

func outputNotInRig() error {
	if runtime.GOOS == "windows" {
		fmt.Println("Remove-Item Env:MS_TOWN_ROOT -ErrorAction SilentlyContinue; Remove-Item Env:MS_ROOT -ErrorAction SilentlyContinue; Remove-Item Env:MS_RIG -ErrorAction SilentlyContinue")
	} else {
		fmt.Println("unset MS_TOWN_ROOT MS_ROOT MS_RIG")
	}
	return nil
}

func updateRigCache(repoRoot, townRoot, rigName string) error {
	cacheDir := state.CacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	cachePath := filepath.Join(cacheDir, "rigs.cache")

	existing := make(map[string]string)
	if data, err := os.ReadFile(cachePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if idx := strings.Index(line, ":"); idx > 0 {
				existing[line[:idx]] = line[idx+1:]
			}
		}
	}

	var value string
	if rigName != "" {
		if runtime.GOOS == "windows" {
			value = fmt.Sprintf("$env:MS_TOWN_ROOT=%q; $env:MS_ROOT=%q; $env:MS_RIG=%q", townRoot, townRoot, rigName)
		} else {
			value = fmt.Sprintf("export MS_TOWN_ROOT=%q; export MS_ROOT=%q; export MS_RIG=%q", townRoot, townRoot, rigName)
		}
	} else if townRoot != "" {
		if runtime.GOOS == "windows" {
			value = fmt.Sprintf("$env:MS_TOWN_ROOT=%q; $env:MS_ROOT=%q; Remove-Item Env:MS_RIG -ErrorAction SilentlyContinue", townRoot, townRoot)
		} else {
			value = fmt.Sprintf("export MS_TOWN_ROOT=%q; export MS_ROOT=%q; unset MS_RIG", townRoot, townRoot)
		}
	} else {
		if runtime.GOOS == "windows" {
			value = "Remove-Item Env:MS_TOWN_ROOT -ErrorAction SilentlyContinue; Remove-Item Env:MS_ROOT -ErrorAction SilentlyContinue; Remove-Item Env:MS_RIG -ErrorAction SilentlyContinue"
		} else {
			value = "unset MS_TOWN_ROOT MS_ROOT MS_RIG"
		}
	}

	existing[repoRoot] = value

	var lines []string
	for k, v := range existing {
		lines = append(lines, k+":"+v)
	}

	return os.WriteFile(cachePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// printEnvSet prints an OS-appropriate env variable assignment for shell eval.
func printEnvSet(key, value string) {
	if runtime.GOOS == "windows" {
		fmt.Printf("$env:%s=%q\n", key, value)
	} else {
		fmt.Printf("export %s=%q\n", key, value)
	}
}

// printEnvUnset prints an OS-appropriate env variable unset for shell eval.
func printEnvUnset(key string) {
	if runtime.GOOS == "windows" {
		fmt.Printf("Remove-Item Env:%s -ErrorAction SilentlyContinue\n", key)
	} else {
		fmt.Printf("unset %s\n", key)
	}
}
