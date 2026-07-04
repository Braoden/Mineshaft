package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/excavation/internal/atomicfile"
)

// TownConfigExistsCheck verifies overseer/town.json exists.
type TownConfigExistsCheck struct {
	BaseCheck
}

// NewTownConfigExistsCheck creates a new town config exists check.
func NewTownConfigExistsCheck() *TownConfigExistsCheck {
	return &TownConfigExistsCheck{
		BaseCheck: BaseCheck{
			CheckName:        "town-config-exists",
			CheckDescription: "Check that overseer/town.json exists",
			CheckCategory:    CategoryCore,
		},
	}
}

// Run checks if overseer/town.json exists.
func (c *TownConfigExistsCheck) Run(ctx *CheckContext) *CheckResult {
	configPath := filepath.Join(ctx.TownRoot, "overseer", "town.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer/town.json not found",
			FixHint: "Run 'gt install' to initialize workspace",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "overseer/town.json exists",
	}
}

// TownConfigValidCheck verifies overseer/town.json is valid JSON with required fields.
type TownConfigValidCheck struct {
	BaseCheck
}

// NewTownConfigValidCheck creates a new town config validation check.
func NewTownConfigValidCheck() *TownConfigValidCheck {
	return &TownConfigValidCheck{
		BaseCheck: BaseCheck{
			CheckName:        "town-config-valid",
			CheckDescription: "Check that overseer/town.json is valid with required fields",
			CheckCategory:    CategoryCore,
		},
	}
}

// townConfig represents the structure of overseer/town.json.
type townConfig struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// Run validates overseer/town.json contents.
func (c *TownConfigValidCheck) Run(ctx *CheckContext) *CheckResult {
	configPath := filepath.Join(ctx.TownRoot, "overseer", "town.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Cannot read overseer/town.json",
			Details: []string{err.Error()},
		}
	}

	var config townConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer/town.json is not valid JSON",
			Details: []string{err.Error()},
			FixHint: "Fix JSON syntax in overseer/town.json",
		}
	}

	var issues []string

	if config.Type != "town" {
		issues = append(issues, fmt.Sprintf("type should be 'town', got '%s'", config.Type))
	}
	if config.Version == 0 {
		issues = append(issues, "version field is missing or zero")
	}
	if config.Name == "" {
		issues = append(issues, "name field is missing or empty")
	}

	if len(issues) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer/town.json has invalid fields",
			Details: issues,
			FixHint: "Fix the field values in overseer/town.json",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("overseer/town.json valid (name=%s, version=%d)", config.Name, config.Version),
	}
}

// RigsRegistryExistsCheck verifies overseer/rigs.json exists.
type RigsRegistryExistsCheck struct {
	FixableCheck
}

// NewRigsRegistryExistsCheck creates a new rigs registry exists check.
func NewRigsRegistryExistsCheck() *RigsRegistryExistsCheck {
	return &RigsRegistryExistsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "rigs-registry-exists",
				CheckDescription: "Check that overseer/rigs.json exists",
				CheckCategory:    CategoryCore,
			},
		},
	}
}

// Run checks if overseer/rigs.json exists.
func (c *RigsRegistryExistsCheck) Run(ctx *CheckContext) *CheckResult {
	rigsPath := filepath.Join(ctx.TownRoot, "overseer", "rigs.json")

	if _, err := os.Stat(rigsPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "overseer/rigs.json not found (no rigs registered)",
			FixHint: "Run 'gt doctor --fix' to create empty rigs.json",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "overseer/rigs.json exists",
	}
}

// Fix creates an empty rigs.json file.
func (c *RigsRegistryExistsCheck) Fix(ctx *CheckContext) error {
	rigsPath := filepath.Join(ctx.TownRoot, "overseer", "rigs.json")

	emptyRigs := struct {
		Version int                    `json:"version"`
		Rigs    map[string]interface{} `json:"rigs"`
	}{
		Version: 1,
		Rigs:    make(map[string]interface{}),
	}

	data, err := json.MarshalIndent(emptyRigs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling empty rigs.json: %w", err)
	}

	return atomicfile.WriteFile(rigsPath, data, 0644)
}

// RigsRegistryValidCheck verifies overseer/rigs.json is valid and rigs exist.
type RigsRegistryValidCheck struct {
	FixableCheck
	missingRigs []string // Cached for Fix
}

// NewRigsRegistryValidCheck creates a new rigs registry validation check.
func NewRigsRegistryValidCheck() *RigsRegistryValidCheck {
	return &RigsRegistryValidCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "rigs-registry-valid",
				CheckDescription: "Check that registered rigs exist on disk",
				CheckCategory:    CategoryCore,
			},
		},
	}
}

// rigsConfig represents the structure of overseer/rigs.json.
type rigsConfig struct {
	Version int                    `json:"version"`
	Rigs    map[string]interface{} `json:"rigs"`
}

// Run validates overseer/rigs.json and checks that registered rigs exist.
func (c *RigsRegistryValidCheck) Run(ctx *CheckContext) *CheckResult {
	rigsPath := filepath.Join(ctx.TownRoot, "overseer", "rigs.json")

	data, err := os.ReadFile(rigsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "No rigs.json (skipping validation)",
			}
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Cannot read overseer/rigs.json",
			Details: []string{err.Error()},
		}
	}

	var config rigsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer/rigs.json is not valid JSON",
			Details: []string{err.Error()},
			FixHint: "Fix JSON syntax in overseer/rigs.json",
		}
	}

	if len(config.Rigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs registered",
		}
	}

	// Check each registered rig exists
	var missing []string
	var found int

	for rigName := range config.Rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		if _, err := os.Stat(rigPath); os.IsNotExist(err) {
			missing = append(missing, rigName)
		} else {
			found++
		}
	}

	// Cache for Fix
	c.missingRigs = missing

	if len(missing) > 0 {
		details := make([]string, len(missing))
		for i, m := range missing {
			details[i] = fmt.Sprintf("Missing rig directory: %s/", m)
		}

		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d of %d registered rig(s) missing", len(missing), len(config.Rigs)),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to remove missing rigs from registry",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("All %d registered rig(s) exist", found),
	}
}

// Fix removes missing rigs from the registry.
func (c *RigsRegistryValidCheck) Fix(ctx *CheckContext) error {
	if len(c.missingRigs) == 0 {
		return nil
	}

	rigsPath := filepath.Join(ctx.TownRoot, "overseer", "rigs.json")

	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return fmt.Errorf("reading rigs.json: %w", err)
	}

	var config rigsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing rigs.json: %w", err)
	}

	// Remove missing rigs
	for _, rig := range c.missingRigs {
		delete(config.Rigs, rig)
	}

	// Write back
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling rigs.json: %w", err)
	}

	return atomicfile.WriteFile(rigsPath, newData, 0644)
}

// OverseerExistsCheck verifies the overseer/ directory structure.
type OverseerExistsCheck struct {
	BaseCheck
}

// NewOverseerExistsCheck creates a new overseer directory check.
func NewOverseerExistsCheck() *OverseerExistsCheck {
	return &OverseerExistsCheck{
		BaseCheck: BaseCheck{
			CheckName:        "overseer-exists",
			CheckDescription: "Check that overseer/ directory exists with required files",
			CheckCategory:    CategoryCore,
		},
	}
}

// Run checks if overseer/ directory exists with expected contents.
func (c *OverseerExistsCheck) Run(ctx *CheckContext) *CheckResult {
	overseerPath := filepath.Join(ctx.TownRoot, "overseer")

	info, err := os.Stat(overseerPath)
	if os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer/ directory not found",
			FixHint: "Run 'gt install' to initialize workspace",
		}
	}
	if !info.IsDir() {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "overseer exists but is not a directory",
			FixHint: "Remove overseer file and run 'gt install'",
		}
	}

	// Check for expected files
	var missing []string
	expectedFiles := []string{"town.json"}

	for _, f := range expectedFiles {
		path := filepath.Join(overseerPath, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "overseer/ exists but missing expected files",
			Details: missing,
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "overseer/ directory exists with required files",
	}
}

// WorkspaceChecks returns all workspace-level health checks.
func WorkspaceChecks() []Check {
	return []Check{
		NewTownConfigExistsCheck(),
		NewTownConfigValidCheck(),
		NewRigsRegistryExistsCheck(),
		NewRigsRegistryValidCheck(),
		NewOverseerExistsCheck(),
	}
}
