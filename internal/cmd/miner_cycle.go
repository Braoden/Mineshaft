package cmd

import (
	"fmt"
	"strings"

	"github.com/steveyegge/excavation/internal/constants"
	"github.com/steveyegge/excavation/internal/session"
)

// cycleMinerSession switches to the next or previous miner session in the same rig.
// direction: 1 for next, -1 for previous
// sessionOverride: if non-empty, use this instead of detecting current session
func cycleMinerSession(direction int, sessionOverride string) error {
	currentSession, err := resolveCurrentSession(sessionOverride)
	if err != nil {
		return fmt.Errorf("not in a tmux session: %w", err)
	}
	if currentSession == "" {
		return fmt.Errorf("not in a tmux session")
	}

	rigName, _, ok := parseMinerSessionName(currentSession)
	if !ok {
		return nil
	}

	sessions, err := findRigMinerSessions(rigName)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	return cycleInGroup(direction, currentSession, sessions)
}

// parseMinerSessionName extracts rig and miner name from a tmux session name.
// Format: gt-<rig>-<name> where name is NOT crew-*, witness, refinery, overseer, or supervisor.
// Returns empty strings and false if the format doesn't match.
//
// Delegates to session.ParseSessionName for consistent parsing of hyphenated
// rig names (e.g., gt-my-rig-Toast correctly yields rig="my-rig", name="Toast").
func parseMinerSessionName(sessionName string) (rigName, minerName string, ok bool) { //nolint:unparam // minerName kept for API consistency
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		return "", "", false
	}
	if identity.Role != session.RoleMiner {
		return "", "", false
	}
	if identity.Rig == "" || identity.Name == "" {
		return "", "", false
	}
	// Exclude names that are reserved for other session types.
	// Overseer/supervisor use hq- prefix in practice, but gt-<rig>-overseer/supervisor
	// patterns should still be excluded defensively.
	switch identity.Name {
	case constants.RoleOverseer, constants.RoleSupervisor:
		return "", "", false
	}
	return identity.Rig, identity.Name, true
}

// findRigMinerSessions returns all miner sessions for a given rig.
// Finds sessions matching gt-<rig>-<name> pattern, excluding crew, witness,
// and refinery sessions.
func findRigMinerSessions(rigName string) ([]string, error) { //nolint:unparam // error return kept for future use
	allSessions, err := listTmuxSessions()
	if err != nil {
		return nil, nil
	}

	prefix := session.PrefixFor(rigName) + "-"
	var sessions []string

	for _, s := range allSessions {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		if _, _, ok := parseMinerSessionName(s); ok {
			sessions = append(sessions, s)
		}
	}

	return sessions, nil
}
