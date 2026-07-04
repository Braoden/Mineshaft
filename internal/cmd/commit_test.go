package cmd

import "testing"

func TestIdentityToEmail(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		domain   string
		want     string
	}{
		{
			name:     "crew member",
			identity: "excavation/crew/jack",
			domain:   "excavation.local",
			want:     "excavation.crew.jack@excavation.local",
		},
		{
			name:     "miner",
			identity: "excavation/miners/max",
			domain:   "excavation.local",
			want:     "excavation.miners.max@excavation.local",
		},
		{
			name:     "witness",
			identity: "excavation/witness",
			domain:   "excavation.local",
			want:     "excavation.witness@excavation.local",
		},
		{
			name:     "refinery",
			identity: "excavation/refinery",
			domain:   "excavation.local",
			want:     "excavation.refinery@excavation.local",
		},
		{
			name:     "overseer with trailing slash",
			identity: "overseer/",
			domain:   "excavation.local",
			want:     "overseer@excavation.local",
		},
		{
			name:     "supervisor with trailing slash",
			identity: "supervisor/",
			domain:   "excavation.local",
			want:     "supervisor@excavation.local",
		},
		{
			name:     "custom domain",
			identity: "myrig/crew/alice",
			domain:   "example.com",
			want:     "myrig.crew.alice@example.com",
		},
		{
			name:     "deeply nested",
			identity: "rig/miners/nested/deep",
			domain:   "test.io",
			want:     "rig.miners.nested.deep@test.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identityToEmail(tt.identity, tt.domain)
			if got != tt.want {
				t.Errorf("identityToEmail(%q, %q) = %q, want %q",
					tt.identity, tt.domain, got, tt.want)
			}
		})
	}
}
