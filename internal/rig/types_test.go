package rig

import (
	"testing"
)

func TestBeadsPath_AlwaysReturnsRigRoot(t *testing.T) {
	t.Parallel()

	// BeadsPath should always return the rig root path, regardless of HasOverseer.
	// The redirect system at <rig>/.beads/redirect handles finding the actual
	// beads location (either local at <rig>/.beads/ or tracked at overseer/rig/.beads/).
	//
	// This ensures:
	// 1. We don't write files to the user's repo clone (overseer/rig/)
	// 2. The redirect architecture is respected
	// 3. All code paths use the same beads resolution logic

	tests := []struct {
		name     string
		rig      Rig
		wantPath string
	}{
		{
			name: "rig with overseer only",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/ms/testrig",
				HasOverseer: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with witness only",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/ms/testrig",
				HasWitness: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with refinery only",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/ms/testrig",
				HasRefinery: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with no agents",
			rig: Rig{
				Name: "testrig",
				Path: "/home/user/ms/testrig",
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with overseer and witness",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/ms/testrig",
				HasOverseer:   true,
				HasWitness: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with overseer and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/ms/testrig",
				HasOverseer:    true,
				HasRefinery: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with witness and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/ms/testrig",
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with all agents",
			rig: Rig{
				Name:        "fullrig",
				Path:        "/tmp/ms/fullrig",
				HasOverseer:    true,
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/tmp/ms/fullrig",
		},
		{
			name: "rig with miners",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/ms/testrig",
				HasOverseer: true,
				Miners: []string{"miner1", "miner2"},
			},
			wantPath: "/home/user/ms/testrig",
		},
		{
			name: "rig with crew",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/ms/testrig",
				HasOverseer: true,
				Crew:     []string{"crew1", "crew2"},
			},
			wantPath: "/home/user/ms/testrig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.rig.BeadsPath()
			if got != tt.wantPath {
				t.Errorf("BeadsPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestDefaultBranch_FallsBackToMain(t *testing.T) {
	t.Parallel()

	// DefaultBranch should return "main" when config cannot be loaded
	rig := Rig{
		Name: "testrig",
		Path: "/nonexistent/path",
	}

	got := rig.DefaultBranch()
	if got != "main" {
		t.Errorf("DefaultBranch() = %q, want %q", got, "main")
	}
}
