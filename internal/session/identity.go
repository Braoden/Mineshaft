// Package session provides miner session lifecycle management.
package session

import (
	"fmt"
	"strings"
)

// Role represents the type of Excavation Site agent.
type Role string

const (
	RoleOverseer    Role = "overseer"
	RoleSupervisor   Role = "supervisor"
	RoleBoss Role = "boss"
	RoleWitness  Role = "witness"
	RoleRefinery Role = "refinery"
	RoleCrew     Role = "crew"
	RoleMiner  Role = "miner"
	RoleDog      Role = "dog"
)

// AgentIdentity represents a parsed Excavation Site agent identity.
type AgentIdentity struct {
	Role   Role   // overseer, supervisor, witness, refinery, crew, miner, dog
	Rig    string // rig name (empty for overseer/supervisor/dog)
	Name   string // crew/miner/dog name (empty for overseer/supervisor/witness/refinery)
	Prefix string // beads prefix for rig-level agents (e.g., "gt", "bd", "hop")
}

// ParseAddress parses a mail-style address into an AgentIdentity.
func ParseAddress(address string) (*AgentIdentity, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("empty address")
	}

	if address == string(RoleOverseer) || address == string(RoleOverseer)+"/" {
		return &AgentIdentity{Role: RoleOverseer}, nil
	}
	if address == string(RoleSupervisor) || address == string(RoleSupervisor)+"/" {
		return &AgentIdentity{Role: RoleSupervisor}, nil
	}
	if address == "boss" {
		return nil, fmt.Errorf("boss has no session")
	}

	address = strings.TrimSuffix(address, "/")
	parts := strings.Split(address, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid address %q", address)
	}

	rig := parts[0]
	prefix := PrefixFor(rig)
	switch len(parts) {
	case 2:
		name := parts[1]
		switch name {
		case string(RoleWitness):
			return &AgentIdentity{Role: RoleWitness, Rig: rig, Prefix: prefix}, nil
		case string(RoleRefinery):
			return &AgentIdentity{Role: RoleRefinery, Rig: rig, Prefix: prefix}, nil
		case string(RoleCrew), "miners":
			return nil, fmt.Errorf("invalid address %q", address)
		default:
			return &AgentIdentity{Role: RoleMiner, Rig: rig, Name: name, Prefix: prefix}, nil
		}
	case 3:
		role := parts[1]
		name := parts[2]
		switch role {
		case string(RoleCrew):
			return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name, Prefix: prefix}, nil
		case "miners":
			return &AgentIdentity{Role: RoleMiner, Rig: rig, Name: name, Prefix: prefix}, nil
		default:
			return nil, fmt.Errorf("invalid address %q", address)
		}
	default:
		return nil, fmt.Errorf("invalid address %q", address)
	}
}

// ParseSessionName parses a tmux session name into an AgentIdentity.
// Uses the default PrefixRegistry to resolve rig-level prefixes to rig names.
//
// Session name formats:
//   - hq-overseer → Role: overseer (town-level, one per machine)
//   - hq-supervisor → Role: supervisor (town-level, one per machine)
//   - hq-boot → Role: supervisor, Name: boot (boot watchdog)
//   - <prefix>-witness → Role: witness (e.g., gt-witness for excavation)
//   - <prefix>-refinery → Role: refinery (e.g., gt-refinery for excavation)
//   - <prefix>-crew-<name> → Role: crew (e.g., gt-crew-max for excavation)
//   - <prefix>-<name> → Role: miner (e.g., gt-furiosa for excavation)
//
// The prefix is the rig's beads prefix (e.g., "gt" for excavation, "dolt" for beads).
// The rig name is resolved from the default PrefixRegistry. If the prefix is
// not in the registry, the prefix itself is used as the rig name.
func ParseSessionName(session string) (*AgentIdentity, error) {
	return ParseSessionNameWithRegistry(session, DefaultRegistry())
}

// ParseSessionNameWithRegistry parses a tmux session name using a specific registry.
// If registry is nil, an empty registry is used (prefix will not resolve to rig name).
func ParseSessionNameWithRegistry(session string, registry *PrefixRegistry) (*AgentIdentity, error) {
	if registry == nil {
		registry = NewPrefixRegistry()
	}

	// Check for town-level roles (hq- prefix).
	// Note: "hq" may also be a registered rig prefix (e.g., knjn uses "hq").
	// Known town-level roles are matched first; unknown suffixes fall through
	// to rig-level parsing so that hq-witness, hq-refinery, hq-<miner> etc.
	// resolve correctly when "hq" is a rig prefix.
	if strings.HasPrefix(session, HQPrefix) {
		suffix := strings.TrimPrefix(session, HQPrefix)
		switch suffix {
		case string(RoleOverseer):
			return &AgentIdentity{Role: RoleOverseer}, nil
		case string(RoleSupervisor):
			return &AgentIdentity{Role: RoleSupervisor}, nil
		case "boot":
			return &AgentIdentity{Role: RoleSupervisor, Name: "boot"}, nil
		case "boss":
			return &AgentIdentity{Role: RoleBoss}, nil
		default:
			// Dogs: hq-dog-<name>
			if strings.HasPrefix(suffix, "dog-") {
				name := suffix[4:] // len("dog-") = 4
				if name == "" {
					return nil, fmt.Errorf("invalid session name %q: empty dog name", session)
				}
				return &AgentIdentity{Role: RoleDog, Name: name}, nil
			}
			// Fall through to rig-level parsing — "hq" may be a rig prefix.
		}
	}

	// Rig-level roles: <prefix>-<rest>
	// Use registry to identify the prefix boundary
	prefix, rest, _ := registry.matchPrefix(session)
	if prefix == "" || rest == "" {
		return nil, fmt.Errorf("invalid session name %q: cannot determine prefix", session)
	}

	rig := registry.RigForPrefix(prefix)

	// Check for witness (suffix marker)
	if rest == string(RoleWitness) {
		return &AgentIdentity{Role: RoleWitness, Rig: rig, Prefix: prefix}, nil
	}

	// Check for refinery (suffix marker)
	if rest == string(RoleRefinery) {
		return &AgentIdentity{Role: RoleRefinery, Rig: rig, Prefix: prefix}, nil
	}

	// Check for crew (marker in rest)
	if strings.HasPrefix(rest, "crew-") {
		name := rest[5:] // len("crew-") = 5
		if name == "" {
			return nil, fmt.Errorf("invalid session name %q: empty crew name", session)
		}
		return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name, Prefix: prefix}, nil
	}

	// Default: miner
	// rest is the miner name (may contain dashes)
	if rest == "" {
		return nil, fmt.Errorf("invalid session name %q: empty miner name", session)
	}
	return &AgentIdentity{Role: RoleMiner, Rig: rig, Name: rest, Prefix: prefix}, nil
}

// SessionName returns the tmux session name for this identity.
func (a *AgentIdentity) SessionName() string {
	switch a.Role {
	case RoleOverseer:
		return OverseerSessionName()
	case RoleSupervisor:
		if a.Name == "boot" {
			return BootSessionName()
		}
		return SupervisorSessionName()
	case RoleBoss:
		return BossSessionName()
	case RoleWitness:
		return WitnessSessionName(a.prefix())
	case RoleRefinery:
		return RefinerySessionName(a.prefix())
	case RoleCrew:
		return CrewSessionName(a.prefix(), a.Name)
	case RoleMiner:
		return MinerSessionName(a.prefix(), a.Name)
	case RoleDog:
		return DogSessionName(a.Name)
	default:
		return ""
	}
}

// prefix returns the rig prefix, falling back to registry lookup or DefaultPrefix.
func (a *AgentIdentity) prefix() string {
	if a.Prefix != "" {
		return a.Prefix
	}
	if a.Rig != "" {
		return PrefixFor(a.Rig)
	}
	return DefaultPrefix
}

// BeaconAddress returns a human-readable, non-path-like address for use in
// startup beacons. Unlike Address(), this format prevents LLMs from
// misinterpreting the recipient as a filesystem path.
// Examples:
//   - overseer → "overseer"
//   - supervisor → "supervisor"
//   - witness → "witness (rig: excavation)"
//   - crew → "crew max (rig: excavation)"
//   - miner → "miner Toast (rig: excavation)"
func (a *AgentIdentity) BeaconAddress() string {
	switch a.Role {
	case RoleOverseer:
		return "overseer"
	case RoleSupervisor:
		return "supervisor"
	case RoleBoss:
		return "boss"
	case RoleWitness:
		return BeaconRecipient("witness", "", a.Rig)
	case RoleRefinery:
		return BeaconRecipient("refinery", "", a.Rig)
	case RoleCrew:
		return BeaconRecipient("crew", a.Name, a.Rig)
	case RoleMiner:
		return BeaconRecipient("miner", a.Name, a.Rig)
	case RoleDog:
		return BeaconRecipient("dog", a.Name, "")
	default:
		return ""
	}
}

// Address returns the mail-style address for this identity.
// Examples:
//   - overseer → "overseer"
//   - supervisor → "supervisor"
//   - witness → "excavation/witness"
//   - refinery → "excavation/refinery"
//   - crew → "excavation/crew/max"
//   - miner → "excavation/miners/Toast"
func (a *AgentIdentity) Address() string {
	switch a.Role {
	case RoleOverseer:
		return "overseer"
	case RoleSupervisor:
		return "supervisor"
	case RoleBoss:
		return "boss"
	case RoleWitness:
		return fmt.Sprintf("%s/witness", a.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s/refinery", a.Rig)
	case RoleCrew:
		return fmt.Sprintf("%s/crew/%s", a.Rig, a.Name)
	case RoleMiner:
		return fmt.Sprintf("%s/miners/%s", a.Rig, a.Name)
	case RoleDog:
		return fmt.Sprintf("supervisor/dogs/%s", a.Name)
	default:
		return ""
	}
}

// GTRole returns the GT_ROLE environment variable format.
// This is the same as Address() for most roles, except boot
// which is a supervisor variant with its own role identity.
func (a *AgentIdentity) GTRole() string {
	if a.Role == RoleSupervisor && a.Name == "boot" {
		return "boot"
	}
	return a.Address()
}
