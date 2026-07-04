package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

)

// BossConfig represents the human operator's identity (overseer/boss.json).
// The boss is the human who controls Excavation Site, distinct from AI agents.
type BossConfig struct {
	Type     string `json:"type"`               // "boss"
	Version  int    `json:"version"`            // schema version
	Name     string `json:"name"`               // display name
	Email    string `json:"email,omitempty"`    // email address
	Username string `json:"username,omitempty"` // username/handle
	Source   string `json:"source"`             // how identity was detected
}

// CurrentBossVersion is the current schema version for BossConfig.
const CurrentBossVersion = 1

// BossConfigPath returns the standard path for boss config in a town.
func BossConfigPath(townRoot string) string {
	return filepath.Join(townRoot, "overseer", "boss.json")
}

// LoadBossConfig loads and validates an boss configuration file.
func LoadBossConfig(path string) (*BossConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading boss config: %w", err)
	}

	var config BossConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing boss config: %w", err)
	}

	if err := validateBossConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveBossConfig saves an boss configuration to a file.
func SaveBossConfig(path string, config *BossConfig) error {
	if err := validateBossConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding boss config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: boss config doesn't contain secrets
		return fmt.Errorf("writing boss config: %w", err)
	}

	return nil
}

// validateBossConfig validates an BossConfig.
func validateBossConfig(c *BossConfig) error {
	// Type must be "boss" (allow empty for backwards compat on load, set on save)
	if c.Type != "boss" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'boss', got '%s'", ErrInvalidType, c.Type)
	}
	// Ensure type is set for saving
	if c.Type == "" {
		c.Type = "boss"
	}
	if c.Version > CurrentBossVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentBossVersion)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingField)
	}
	return nil
}

// DetectBoss attempts to detect the boss's identity from available sources.
// Priority order:
//  1. Existing config file (if path provided and exists)
//  2. Git config (user.name + user.email)
//  3. GitHub CLI (gh api user)
//  4. Environment ($USER or whoami)
func DetectBoss(townRoot string) (*BossConfig, error) {
	configPath := BossConfigPath(townRoot)

	// Priority 1: Check existing config
	if existing, err := LoadBossConfig(configPath); err == nil {
		return existing, nil
	}

	// Priority 2: Try git config
	if config := detectFromGitConfig(townRoot); config != nil {
		return config, nil
	}

	// Priority 3: Try GitHub CLI
	if config := detectFromGitHub(); config != nil {
		return config, nil
	}

	// Priority 4: Fall back to environment
	return detectFromEnvironment(), nil
}

// detectFromGitConfig attempts to get identity from git config.
func detectFromGitConfig(dir string) *BossConfig {
	// Try to get user.name
	nameCmd := exec.Command("git", "config", "user.name")
	nameCmd.Dir = dir

	nameOut, err := nameCmd.Output()
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(nameOut))
	if name == "" {
		return nil
	}

	config := &BossConfig{
		Type:    "boss",
		Version: CurrentBossVersion,
		Name:    name,
		Source:  "git-config",
	}

	// Try to get user.email (optional)
	emailCmd := exec.Command("git", "config", "user.email")
	emailCmd.Dir = dir

	if emailOut, err := emailCmd.Output(); err == nil {
		config.Email = strings.TrimSpace(string(emailOut))
	}

	// Extract username from email if available
	if config.Email != "" {
		if idx := strings.Index(config.Email, "@"); idx > 0 {
			config.Username = config.Email[:idx]
		}
	}

	return config
}

// detectFromGitHub attempts to get identity from GitHub CLI.
func detectFromGitHub() *BossConfig {
	cmd := exec.Command("gh", "api", "user", "--jq", ".login + \"|\" + .name + \"|\" + .email")

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) < 1 || parts[0] == "" {
		return nil
	}

	config := &BossConfig{
		Type:     "boss",
		Version:  CurrentBossVersion,
		Username: parts[0],
		Source:   "github-cli",
	}

	// Use name if available, otherwise username
	if len(parts) >= 2 && parts[1] != "" {
		config.Name = parts[1]
	} else {
		config.Name = parts[0]
	}

	// Add email if available
	if len(parts) >= 3 && parts[2] != "" {
		config.Email = parts[2]
	}

	return config
}

// detectFromEnvironment falls back to environment variables.
func detectFromEnvironment() *BossConfig {
	username := os.Getenv("USER")
	if username == "" {
		// Try whoami as last resort
		cmd := exec.Command("whoami")
	
		if out, err := cmd.Output(); err == nil {
			username = strings.TrimSpace(string(out))
		}
	}
	if username == "" {
		username = "boss"
	}

	return &BossConfig{
		Type:     "boss",
		Version:  CurrentBossVersion,
		Name:     username,
		Username: username,
		Source:   "environment",
	}
}

// LoadOrDetectBoss loads existing config or detects and saves a new one.
func LoadOrDetectBoss(townRoot string) (*BossConfig, error) {
	configPath := BossConfigPath(townRoot)

	// Try loading existing
	if config, err := LoadBossConfig(configPath); err == nil {
		return config, nil
	}

	// Detect new
	config, err := DetectBoss(townRoot)
	if err != nil {
		return nil, err
	}

	// Save for next time
	if err := SaveBossConfig(configPath, config); err != nil {
		// Non-fatal - we can still use the detected config
		fmt.Fprintf(os.Stderr, "warning: could not save boss config: %v\n", err)
	}

	return config, nil
}

// FormatBossIdentity returns a formatted string for display.
// Example: "Steve Yegge <stevey@example.com>"
func (c *BossConfig) FormatBossIdentity() string {
	if c.Email != "" {
		return fmt.Sprintf("%s <%s>", c.Name, c.Email)
	}
	if c.Username != "" && c.Username != c.Name {
		return fmt.Sprintf("%s (@%s)", c.Name, c.Username)
	}
	return c.Name
}
