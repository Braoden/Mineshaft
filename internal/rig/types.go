// Package rig provides rig management functionality.
package rig

import (
	"github.com/steveyegge/excavation/internal/config"
)

// Rig represents a managed repository in the workspace.
type Rig struct {
	// Name is the rig identifier (directory name).
	Name string `json:"name"`

	// Path is the absolute path to the rig directory.
	Path string `json:"path"`

	// GitURL is the remote repository URL (fetch/pull).
	GitURL string `json:"git_url"`

	// PushURL is an optional push URL for read-only upstreams.
	// When set, miners push here instead of to GitURL (e.g., personal fork).
	PushURL string `json:"push_url,omitempty"`

	// LocalRepo is an optional local repository used for reference clones.
	LocalRepo string `json:"local_repo,omitempty"`

	// Config is the rig-level configuration.
	Config *config.BeadsConfig `json:"config,omitempty"`

	// Miners is the list of miner names in this rig.
	Miners []string `json:"miners,omitempty"`

	// Crew is the list of crew worker names in this rig.
	// Crew workers are user-managed persistent workspaces.
	Crew []string `json:"crew,omitempty"`

	// HasWitness indicates if the rig has a witness agent.
	HasWitness bool `json:"has_witness"`

	// HasRefinery indicates if the rig has a refinery agent.
	HasRefinery bool `json:"has_refinery"`

	// HasOverseer indicates if the rig has a overseer clone.
	HasOverseer bool `json:"has_overseer"`
}

// AgentDirs are the standard agent directories in a rig.
// Note: witness doesn't have a /rig subdirectory (no clone needed).
var AgentDirs = []string{
	"miners",
	"crew",
	"refinery/rig",
	"witness",
	"overseer/rig",
}

// RigSummary provides a concise overview of a rig.
type RigSummary struct {
	Name         string `json:"name"`
	MinerCount int    `json:"miner_count"`
	CrewCount    int    `json:"crew_count"`
	HasWitness   bool   `json:"has_witness"`
	HasRefinery  bool   `json:"has_refinery"`
}

// Summary returns a RigSummary for this rig.
func (r *Rig) Summary() RigSummary {
	return RigSummary{
		Name:         r.Name,
		MinerCount: len(r.Miners),
		CrewCount:    len(r.Crew),
		HasWitness:   r.HasWitness,
		HasRefinery:  r.HasRefinery,
	}
}

// BeadsPath returns the path to use for beads operations.
// Always returns the rig root path where .beads/ contains either:
//   - A local beads database (when repo doesn't track .beads/)
//   - A redirect file pointing to overseer/rig/.beads (when repo tracks .beads/)
//
// The redirect is set up by initBeads() during rig creation and followed
// automatically by the bd CLI and beads.ResolveBeadsDir().
//
// This ensures we never write to the user's repo clone (overseer/rig/) and
// all beads operations go through the redirect system.
func (r *Rig) BeadsPath() string {
	return r.Path
}

// DefaultBranch returns the configured default branch for this rig.
// Falls back to "main" if not configured or if config cannot be loaded.
func (r *Rig) DefaultBranch() string {
	cfg, err := LoadRigConfig(r.Path)
	if err != nil || cfg.DefaultBranch == "" {
		return "main"
	}
	return cfg.DefaultBranch
}
