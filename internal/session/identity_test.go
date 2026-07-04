package session

import (
	"testing"
)

// testRegistry returns a PrefixRegistry populated with test rig prefixes.
func testRegistry() *PrefixRegistry {
	r := NewPrefixRegistry()
	r.Register("gt", "excavation")
	r.Register("bd", "beads")
	r.Register("hop", "hop")
	r.Register("sky", "sky")
	r.Register("mp", "my-project")
	r.Register("hq", "knjn")
	return r
}

func TestParseSessionName(t *testing.T) {
	reg := testRegistry()
	// Also set as default for ParseSessionName (no-registry variant)
	old := DefaultRegistry()
	SetDefaultRegistry(reg)
	defer func() { SetDefaultRegistry(old) }()

	tests := []struct {
		name       string
		session    string
		wantRole   Role
		wantRig    string
		wantName   string
		wantPrefix string
		wantErr    bool
	}{
		// Town-level roles (hq-overseer, hq-supervisor)
		{
			name:     "overseer",
			session:  "hq-overseer",
			wantRole: RoleOverseer,
		},
		{
			name:     "supervisor",
			session:  "hq-supervisor",
			wantRole: RoleSupervisor,
		},
		{
			name:     "boot",
			session:  "hq-boot",
			wantRole: RoleSupervisor,
			wantName: "boot",
		},

		// Dogs (town-level: hq-dog-<name>)
		{
			name:     "dog alpha",
			session:  "hq-dog-alpha",
			wantRole: RoleDog,
			wantName: "alpha",
		},
		{
			name:     "dog hyphenated name",
			session:  "hq-dog-my-dog",
			wantRole: RoleDog,
			wantName: "my-dog",
		},

		// Rig prefix "hq" collision: hq-refinery/hq-witness/hq-<miner>
		// should resolve as rig-level roles when "hq" is a registered prefix.
		{
			name:       "hq prefix witness",
			session:    "hq-witness",
			wantRole:   RoleWitness,
			wantRig:    "knjn",
			wantPrefix: "hq",
		},
		{
			name:       "hq prefix refinery",
			session:    "hq-refinery",
			wantRole:   RoleRefinery,
			wantRig:    "knjn",
			wantPrefix: "hq",
		},
		{
			name:       "hq prefix miner",
			session:    "hq-jasper",
			wantRole:   RoleMiner,
			wantRig:    "knjn",
			wantName:   "jasper",
			wantPrefix: "hq",
		},
		{
			name:       "hq prefix crew",
			session:    "hq-crew-rushd",
			wantRole:   RoleCrew,
			wantRig:    "knjn",
			wantName:   "rushd",
			wantPrefix: "hq",
		},

		// Witness (new format: <prefix>-witness)
		{
			name:       "witness excavation",
			session:    "gt-witness",
			wantRole:   RoleWitness,
			wantRig:    "excavation",
			wantPrefix: "gt",
		},
		{
			name:       "witness beads",
			session:    "bd-witness",
			wantRole:   RoleWitness,
			wantRig:    "beads",
			wantPrefix: "bd",
		},
		{
			name:       "witness hop",
			session:    "hop-witness",
			wantRole:   RoleWitness,
			wantRig:    "hop",
			wantPrefix: "hop",
		},

		// Refinery (new format: <prefix>-refinery)
		{
			name:       "refinery excavation",
			session:    "gt-refinery",
			wantRole:   RoleRefinery,
			wantRig:    "excavation",
			wantPrefix: "gt",
		},
		{
			name:       "refinery multi-word prefix",
			session:    "mp-refinery",
			wantRole:   RoleRefinery,
			wantRig:    "my-project",
			wantPrefix: "mp",
		},

		// Crew (new format: <prefix>-crew-<name>)
		{
			name:       "crew excavation",
			session:    "gt-crew-max",
			wantRole:   RoleCrew,
			wantRig:    "excavation",
			wantName:   "max",
			wantPrefix: "gt",
		},
		{
			name:       "crew beads",
			session:    "bd-crew-alice",
			wantRole:   RoleCrew,
			wantRig:    "beads",
			wantName:   "alice",
			wantPrefix: "bd",
		},
		{
			name:       "crew hyphenated name",
			session:    "gt-crew-my-worker",
			wantRole:   RoleCrew,
			wantRig:    "excavation",
			wantName:   "my-worker",
			wantPrefix: "gt",
		},

		// Miner (new format: <prefix>-<name>)
		{
			name:       "miner excavation",
			session:    "gt-morsov",
			wantRole:   RoleMiner,
			wantRig:    "excavation",
			wantName:   "morsov",
			wantPrefix: "gt",
		},
		{
			name:       "miner beads",
			session:    "bd-worker1",
			wantRole:   RoleMiner,
			wantRig:    "beads",
			wantName:   "worker1",
			wantPrefix: "bd",
		},
		{
			name:       "miner hop",
			session:    "hop-ostrom",
			wantRole:   RoleMiner,
			wantRig:    "hop",
			wantName:   "ostrom",
			wantPrefix: "hop",
		},
		{
			name:       "miner sky",
			session:    "sky-furiosa",
			wantRole:   RoleMiner,
			wantRig:    "sky",
			wantName:   "furiosa",
			wantPrefix: "sky",
		},

		// Error cases: unknown prefixes should fail (not fall back to splitting on dash)
		{
			name:    "unknown prefix miner",
			session: "zz-alpha",
			wantErr: true,
		},
		{
			name:    "unknown prefix witness",
			session: "foo-witness",
			wantErr: true,
		},
		{
			name:    "empty string",
			session: "",
			wantErr: true,
		},
		{
			name:    "no dash",
			session: "gtwitness",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSessionName(tt.session)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSessionName(%q) error = %v, wantErr %v", tt.session, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Role != tt.wantRole {
				t.Errorf("ParseSessionName(%q).Role = %v, want %v", tt.session, got.Role, tt.wantRole)
			}
			if got.Rig != tt.wantRig {
				t.Errorf("ParseSessionName(%q).Rig = %v, want %v", tt.session, got.Rig, tt.wantRig)
			}
			if got.Name != tt.wantName {
				t.Errorf("ParseSessionName(%q).Name = %v, want %v", tt.session, got.Name, tt.wantName)
			}
			if tt.wantPrefix != "" && got.Prefix != tt.wantPrefix {
				t.Errorf("ParseSessionName(%q).Prefix = %v, want %v", tt.session, got.Prefix, tt.wantPrefix)
			}
		})
	}
}

func TestAgentIdentity_SessionName(t *testing.T) {
	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "overseer",
			identity: AgentIdentity{Role: RoleOverseer},
			want:     "hq-overseer",
		},
		{
			name:     "supervisor",
			identity: AgentIdentity{Role: RoleSupervisor},
			want:     "hq-supervisor",
		},
		{
			name:     "boot",
			identity: AgentIdentity{Role: RoleSupervisor, Name: "boot"},
			want:     "hq-boot",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "excavation", Prefix: "gt"},
			want:     "gt-witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "beads", Prefix: "bd"},
			want:     "bd-refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "excavation", Name: "max", Prefix: "gt"},
			want:     "gt-crew-max",
		},
		{
			name:     "miner",
			identity: AgentIdentity{Role: RoleMiner, Rig: "excavation", Name: "morsov", Prefix: "gt"},
			want:     "gt-morsov",
		},
		{
			name:     "miner hop",
			identity: AgentIdentity{Role: RoleMiner, Rig: "hop", Name: "ostrom", Prefix: "hop"},
			want:     "hop-ostrom",
		},
		{
			name:     "dog",
			identity: AgentIdentity{Role: RoleDog, Name: "alpha"},
			want:     "hq-dog-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.SessionName(); got != tt.want {
				t.Errorf("AgentIdentity.SessionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentIdentity_Address(t *testing.T) {
	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "overseer",
			identity: AgentIdentity{Role: RoleOverseer},
			want:     "overseer",
		},
		{
			name:     "supervisor",
			identity: AgentIdentity{Role: RoleSupervisor},
			want:     "supervisor",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "excavation", Prefix: "gt"},
			want:     "excavation/witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project", Prefix: "mp"},
			want:     "my-project/refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "excavation", Name: "max", Prefix: "gt"},
			want:     "excavation/crew/max",
		},
		{
			name:     "miner",
			identity: AgentIdentity{Role: RoleMiner, Rig: "excavation", Name: "Toast", Prefix: "gt"},
			want:     "excavation/miners/Toast",
		},
		{
			name:     "dog",
			identity: AgentIdentity{Role: RoleDog, Name: "alpha"},
			want:     "supervisor/dogs/alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.Address(); got != tt.want {
				t.Errorf("AgentIdentity.Address() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSessionName_RoundTrip(t *testing.T) {
	reg := testRegistry()
	old := DefaultRegistry()
	SetDefaultRegistry(reg)
	defer func() { SetDefaultRegistry(old) }()

	// Test that parsing then reconstructing gives the same result
	sessions := []string{
		"hq-overseer",
		"hq-supervisor",
		"hq-dog-alpha",
		"gt-witness",
		"bd-refinery",
		"gt-crew-max",
		"gt-morsov",
		"hop-ostrom",
		"sky-furiosa",
		"hq-witness",
		"hq-refinery",
		"hq-jasper",
		"hq-crew-rushd",
	}

	for _, sess := range sessions {
		t.Run(sess, func(t *testing.T) {
			identity, err := ParseSessionName(sess)
			if err != nil {
				t.Fatalf("ParseSessionName(%q) error = %v", sess, err)
			}
			if got := identity.SessionName(); got != sess {
				t.Errorf("Round-trip failed: ParseSessionName(%q).SessionName() = %q", sess, got)
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    AgentIdentity
		wantErr bool
	}{
		{
			name:    "overseer",
			address: "overseer/",
			want:    AgentIdentity{Role: RoleOverseer},
		},
		{
			name:    "supervisor",
			address: "supervisor",
			want:    AgentIdentity{Role: RoleSupervisor},
		},
		{
			name:    "witness",
			address: "excavation/witness",
			want:    AgentIdentity{Role: RoleWitness, Rig: "excavation", Prefix: PrefixFor("excavation")},
		},
		{
			name:    "refinery",
			address: "rig-a/refinery",
			want:    AgentIdentity{Role: RoleRefinery, Rig: "rig-a", Prefix: PrefixFor("rig-a")},
		},
		{
			name:    "crew",
			address: "excavation/crew/max",
			want:    AgentIdentity{Role: RoleCrew, Rig: "excavation", Name: "max", Prefix: PrefixFor("excavation")},
		},
		{
			name:    "miner explicit",
			address: "excavation/miners/nux",
			want:    AgentIdentity{Role: RoleMiner, Rig: "excavation", Name: "nux", Prefix: PrefixFor("excavation")},
		},
		{
			name:    "miner canonical",
			address: "excavation/nux",
			want:    AgentIdentity{Role: RoleMiner, Rig: "excavation", Name: "nux", Prefix: PrefixFor("excavation")},
		},
		{
			name:    "invalid",
			address: "excavation/crew",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.address)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q) error = %v", tt.address, err)
			}
			if *got != tt.want {
				t.Fatalf("ParseAddress(%q) = %#v, want %#v", tt.address, *got, tt.want)
			}
		})
	}
}

func TestPrefixRegistry(t *testing.T) {
	r := NewPrefixRegistry()
	r.Register("gt", "excavation")
	r.Register("bd", "beads")

	if got := r.PrefixForRig("excavation"); got != "gt" {
		t.Errorf("PrefixForRig(excavation) = %q, want %q", got, "gt")
	}
	if got := r.RigForPrefix("bd"); got != "beads" {
		t.Errorf("RigForPrefix(bd) = %q, want %q", got, "beads")
	}
	// Unknown rig returns default
	if got := r.PrefixForRig("unknown"); got != DefaultPrefix {
		t.Errorf("PrefixForRig(unknown) = %q, want %q", got, DefaultPrefix)
	}
	// Unknown prefix returns the prefix itself
	if got := r.RigForPrefix("zz"); got != "zz" {
		t.Errorf("RigForPrefix(zz) = %q, want %q", got, "zz")
	}
}
