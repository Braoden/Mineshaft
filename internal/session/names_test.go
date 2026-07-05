package session

import (
	"testing"
)

func TestOverseerSessionName(t *testing.T) {
	// Overseer session name is now fixed (one per machine), uses HQ prefix
	want := "hq-overseer"
	got := OverseerSessionName()
	if got != want {
		t.Errorf("OverseerSessionName() = %q, want %q", got, want)
	}
}

func TestSupervisorSessionName(t *testing.T) {
	// Supervisor session name is now fixed (one per machine), uses HQ prefix
	want := "hq-supervisor"
	got := SupervisorSessionName()
	if got != want {
		t.Errorf("SupervisorSessionName() = %q, want %q", got, want)
	}
}

func TestBossSessionName(t *testing.T) {
	want := "hq-boss"
	got := BossSessionName()
	if got != want {
		t.Errorf("BossSessionName() = %q, want %q", got, want)
	}
}

func TestWitnessSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		want      string
	}{
		{"ms", "ms-witness"},
		{"bd", "bd-witness"},
		{"hop", "hop-witness"},
		{"sky", "sky-witness"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix, func(t *testing.T) {
			got := WitnessSessionName(tt.rigPrefix)
			if got != tt.want {
				t.Errorf("WitnessSessionName(%q) = %q, want %q", tt.rigPrefix, got, tt.want)
			}
		})
	}
}

func TestRefinerySessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		want      string
	}{
		{"ms", "ms-refinery"},
		{"bd", "bd-refinery"},
		{"hop", "hop-refinery"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix, func(t *testing.T) {
			got := RefinerySessionName(tt.rigPrefix)
			if got != tt.want {
				t.Errorf("RefinerySessionName(%q) = %q, want %q", tt.rigPrefix, got, tt.want)
			}
		})
	}
}

func TestCrewSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		name      string
		want      string
	}{
		{"ms", "max", "ms-crew-max"},
		{"bd", "alice", "bd-crew-alice"},
		{"hop", "bar", "hop-crew-bar"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix+"/"+tt.name, func(t *testing.T) {
			got := CrewSessionName(tt.rigPrefix, tt.name)
			if got != tt.want {
				t.Errorf("CrewSessionName(%q, %q) = %q, want %q", tt.rigPrefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestMinerSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		name      string
		want      string
	}{
		{"ms", "Toast", "ms-Toast"},
		{"ms", "Furiosa", "ms-Furiosa"},
		{"bd", "worker1", "bd-worker1"},
		{"hop", "ostrom", "hop-ostrom"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix+"/"+tt.name, func(t *testing.T) {
			got := MinerSessionName(tt.rigPrefix, tt.name)
			if got != tt.want {
				t.Errorf("MinerSessionName(%q, %q) = %q, want %q", tt.rigPrefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestDefaultPrefix(t *testing.T) {
	want := "ms"
	if DefaultPrefix != want {
		t.Errorf("DefaultPrefix = %q, want %q", DefaultPrefix, want)
	}
}
