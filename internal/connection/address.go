package connection

import (
	"fmt"
	"strings"
)

// Address represents a parsed agent or rig address.
// Format: [machine:]rig[/miner]
//
// Examples:
//   - "mineshaft/rictus"        -> local machine, mineshaft rig, rictus miner
//   - "vm:mineshaft/rictus"     -> vm machine, mineshaft rig, rictus miner
//   - "mineshaft/"              -> local machine, mineshaft rig, broadcast
//   - "vm:mineshaft/"           -> vm machine, mineshaft rig, broadcast
type Address struct {
	Machine string // Machine name (empty = local)
	Rig     string // Rig name (required)
	Miner string // Miner name (empty = broadcast to rig)
}

// ParseAddress parses an address string into its components.
// Valid formats:
//   - rig/miner
//   - rig/
//   - machine:rig/miner
//   - machine:rig/
func ParseAddress(s string) (*Address, error) {
	if s == "" {
		return nil, fmt.Errorf("empty address")
	}

	addr := &Address{}

	// Check for machine prefix (machine:)
	if idx := strings.Index(s, ":"); idx >= 0 {
		addr.Machine = s[:idx]
		s = s[idx+1:]
		if addr.Machine == "" {
			return nil, fmt.Errorf("empty machine name before ':'")
		}
	}

	// Parse rig/miner
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return nil, fmt.Errorf("missing rig name in address")
	}

	addr.Rig = parts[0]

	if len(parts) == 2 {
		addr.Miner = parts[1] // May be empty for broadcast
	}

	return addr, nil
}

// String returns the address in canonical form.
func (a *Address) String() string {
	var sb strings.Builder

	if a.Machine != "" {
		sb.WriteString(a.Machine)
		sb.WriteString(":")
	}

	sb.WriteString(a.Rig)
	sb.WriteString("/")

	if a.Miner != "" {
		sb.WriteString(a.Miner)
	}

	return sb.String()
}

// IsLocal returns true if the address targets the local machine.
func (a *Address) IsLocal() bool {
	return a.Machine == "" || a.Machine == "local"
}

// IsBroadcast returns true if the address targets a rig (no specific miner).
func (a *Address) IsBroadcast() bool {
	return a.Miner == ""
}

// RigPath returns the rig/miner portion without machine prefix.
func (a *Address) RigPath() string {
	if a.Miner != "" {
		return a.Rig + "/" + a.Miner
	}
	return a.Rig + "/"
}

// Validate checks if the address is valid against the registry.
// Returns nil if valid, otherwise an error describing the issue.
func (a *Address) Validate(registry *MachineRegistry) error {
	// Check machine exists (if specified)
	if a.Machine != "" && a.Machine != "local" {
		if _, err := registry.Get(a.Machine); err != nil {
			return fmt.Errorf("unknown machine: %s", a.Machine)
		}
	}

	// Rig validation would require connection to target machine
	// to check if rig exists - defer to caller for now

	return nil
}

// Equal returns true if two addresses are equivalent.
func (a *Address) Equal(other *Address) bool {
	if other == nil {
		return false
	}

	// Normalize local machine comparisons
	m1, m2 := a.Machine, other.Machine
	if m1 == "" || m1 == "local" {
		m1 = "local"
	}
	if m2 == "" || m2 == "local" {
		m2 = "local"
	}

	return m1 == m2 && a.Rig == other.Rig && a.Miner == other.Miner
}

// MustParseAddress parses an address and panics on error.
// Only use for known-good addresses (e.g., constants).
func MustParseAddress(s string) *Address {
	addr, err := ParseAddress(s)
	if err != nil {
		panic(fmt.Sprintf("invalid address %q: %v", s, err))
	}
	return addr
}
