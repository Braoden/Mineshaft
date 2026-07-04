package cmd

import "testing"

func TestEffectiveMinerDirCap(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		want       int
	}{
		{"negative uses floor", -1, minMinerDirsPerRig},
		{"zero uses floor", 0, minMinerDirsPerRig},
		{"default below floor uses floor", 10, minMinerDirsPerRig},
		{"one below floor uses floor", minMinerDirsPerRig - 1, minMinerDirsPerRig},
		{"floor remains floor", minMinerDirsPerRig, minMinerDirsPerRig},
		{"above floor is honored", 45, 45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMinerDirCap(tt.configured); got != tt.want {
				t.Errorf("effectiveMinerDirCap(%d) = %d, want %d", tt.configured, got, tt.want)
			}
		})
	}
}
