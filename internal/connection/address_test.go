package connection

import (
	"testing"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Address
		wantErr bool
	}{
		{
			name:  "rig/miner",
			input: "excavation/rictus",
			want:  &Address{Rig: "excavation", Miner: "rictus"},
		},
		{
			name:  "rig/ broadcast",
			input: "excavation/",
			want:  &Address{Rig: "excavation"},
		},
		{
			name:  "machine:rig/miner",
			input: "vm:excavation/rictus",
			want:  &Address{Machine: "vm", Rig: "excavation", Miner: "rictus"},
		},
		{
			name:  "machine:rig/ broadcast",
			input: "vm:excavation/",
			want:  &Address{Machine: "vm", Rig: "excavation"},
		},
		{
			name:  "rig only (no slash)",
			input: "excavation",
			want:  &Address{Rig: "excavation"},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty machine",
			input:   ":excavation/rictus",
			wantErr: true,
		},
		{
			name:    "empty rig",
			input:   "vm:/rictus",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAddress(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseAddress(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Machine != tt.want.Machine {
				t.Errorf("Machine = %q, want %q", got.Machine, tt.want.Machine)
			}
			if got.Rig != tt.want.Rig {
				t.Errorf("Rig = %q, want %q", got.Rig, tt.want.Rig)
			}
			if got.Miner != tt.want.Miner {
				t.Errorf("Miner = %q, want %q", got.Miner, tt.want.Miner)
			}
		})
	}
}

func TestAddressString(t *testing.T) {
	tests := []struct {
		addr *Address
		want string
	}{
		{
			addr: &Address{Rig: "excavation", Miner: "rictus"},
			want: "excavation/rictus",
		},
		{
			addr: &Address{Rig: "excavation"},
			want: "excavation/",
		},
		{
			addr: &Address{Machine: "vm", Rig: "excavation", Miner: "rictus"},
			want: "vm:excavation/rictus",
		},
		{
			addr: &Address{Machine: "vm", Rig: "excavation"},
			want: "vm:excavation/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.addr.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddressIsLocal(t *testing.T) {
	tests := []struct {
		addr *Address
		want bool
	}{
		{&Address{Rig: "excavation"}, true},
		{&Address{Machine: "", Rig: "excavation"}, true},
		{&Address{Machine: "local", Rig: "excavation"}, true},
		{&Address{Machine: "vm", Rig: "excavation"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := tt.addr.IsLocal(); got != tt.want {
				t.Errorf("IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddressIsBroadcast(t *testing.T) {
	tests := []struct {
		addr *Address
		want bool
	}{
		{&Address{Rig: "excavation"}, true},
		{&Address{Rig: "excavation", Miner: ""}, true},
		{&Address{Rig: "excavation", Miner: "rictus"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := tt.addr.IsBroadcast(); got != tt.want {
				t.Errorf("IsBroadcast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddressEqual(t *testing.T) {
	tests := []struct {
		a, b *Address
		want bool
	}{
		{
			&Address{Rig: "excavation", Miner: "rictus"},
			&Address{Rig: "excavation", Miner: "rictus"},
			true,
		},
		{
			&Address{Machine: "", Rig: "excavation"},
			&Address{Machine: "local", Rig: "excavation"},
			true,
		},
		{
			&Address{Rig: "excavation", Miner: "rictus"},
			&Address{Rig: "excavation", Miner: "nux"},
			false,
		},
		{
			&Address{Rig: "excavation"},
			nil,
			false,
		},
	}

	for _, tt := range tests {
		name := "equal"
		if !tt.want {
			name = "not equal"
		}
		t.Run(name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAddress_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Address
		wantErr bool
	}{
		// Malformed: empty/whitespace variations
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			want:    &Address{Rig: "   "},
			wantErr: false, // whitespace-only rig is technically parsed
		},
		{
			name:    "just slash",
			input:   "/",
			wantErr: true,
		},
		{
			name:    "double slash",
			input:   "//",
			wantErr: true,
		},
		{
			name:    "triple slash",
			input:   "///",
			wantErr: true,
		},

		// Malformed: leading/trailing issues
		{
			name:    "leading slash",
			input:   "/miner",
			wantErr: true,
		},
		{
			name:    "leading slash with rig",
			input:   "/rig/miner",
			wantErr: true,
		},
		{
			name:  "trailing slash is broadcast",
			input: "rig/",
			want:  &Address{Rig: "rig"},
		},

		// Machine prefix edge cases
		{
			name:    "colon only",
			input:   ":",
			wantErr: true,
		},
		{
			name:    "colon with trailing slash",
			input:   ":/",
			wantErr: true,
		},
		{
			name:    "empty machine with colon",
			input:   ":rig/miner",
			wantErr: true,
		},
		{
			name:  "multiple colons in machine",
			input: "host:8080:rig/miner",
			want:  &Address{Machine: "host", Rig: "8080:rig", Miner: "miner"},
		},
		{
			name:  "colon in rig name",
			input: "machine:rig:port/miner",
			want:  &Address{Machine: "machine", Rig: "rig:port", Miner: "miner"},
		},

		// Multiple slash handling (SplitN behavior)
		{
			name:  "extra slashes in miner",
			input: "rig/pole/cat/extra",
			want:  &Address{Rig: "rig", Miner: "pole/cat/extra"},
		},
		{
			name:  "many path components",
			input: "a/b/c/d/e",
			want:  &Address{Rig: "a", Miner: "b/c/d/e"},
		},

		// Unicode handling
		{
			name:  "unicode rig name",
			input: "日本語/miner",
			want:  &Address{Rig: "日本語", Miner: "miner"},
		},
		{
			name:  "unicode miner name",
			input: "rig/工作者",
			want:  &Address{Rig: "rig", Miner: "工作者"},
		},
		{
			name:  "emoji in address",
			input: "🔧/🐱",
			want:  &Address{Rig: "🔧", Miner: "🐱"},
		},
		{
			name:  "unicode machine name",
			input: "マシン:rig/miner",
			want:  &Address{Machine: "マシン", Rig: "rig", Miner: "miner"},
		},

		// Long addresses
		{
			name:  "very long rig name",
			input: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789/miner",
			want:  &Address{Rig: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789", Miner: "miner"},
		},
		{
			name:  "very long miner name",
			input: "rig/abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789",
			want:  &Address{Rig: "rig", Miner: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"},
		},

		// Special characters
		{
			name:  "hyphen in names",
			input: "my-rig/my-miner",
			want:  &Address{Rig: "my-rig", Miner: "my-miner"},
		},
		{
			name:  "underscore in names",
			input: "my_rig/my_miner",
			want:  &Address{Rig: "my_rig", Miner: "my_miner"},
		},
		{
			name:  "dots in names",
			input: "my.rig/my.miner",
			want:  &Address{Rig: "my.rig", Miner: "my.miner"},
		},
		{
			name:  "mixed special chars",
			input: "rig-1_v2.0/miner-alpha_1.0",
			want:  &Address{Rig: "rig-1_v2.0", Miner: "miner-alpha_1.0"},
		},

		// Whitespace in components
		{
			name:  "space in rig name",
			input: "my rig/miner",
			want:  &Address{Rig: "my rig", Miner: "miner"},
		},
		{
			name:  "space in miner name",
			input: "rig/my miner",
			want:  &Address{Rig: "rig", Miner: "my miner"},
		},
		{
			name:  "leading space in rig",
			input: " rig/miner",
			want:  &Address{Rig: " rig", Miner: "miner"},
		},
		{
			name:  "trailing space in miner",
			input: "rig/miner ",
			want:  &Address{Rig: "rig", Miner: "miner "},
		},

		// Edge case: machine with no rig after colon
		{
			name:    "machine colon nothing",
			input:   "machine:",
			wantErr: true,
		},
		{
			name:    "machine colon slash",
			input:   "machine:/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAddress(%q) expected error, got %+v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseAddress(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Machine != tt.want.Machine {
				t.Errorf("Machine = %q, want %q", got.Machine, tt.want.Machine)
			}
			if got.Rig != tt.want.Rig {
				t.Errorf("Rig = %q, want %q", got.Rig, tt.want.Rig)
			}
			if got.Miner != tt.want.Miner {
				t.Errorf("Miner = %q, want %q", got.Miner, tt.want.Miner)
			}
		})
	}
}

func TestMustParseAddress_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseAddress with empty string should panic")
		}
	}()
	MustParseAddress("")
}

func TestMustParseAddress_Valid(t *testing.T) {
	// Should not panic
	addr := MustParseAddress("rig/miner")
	if addr.Rig != "rig" || addr.Miner != "miner" {
		t.Errorf("MustParseAddress returned wrong address: %+v", addr)
	}
}

func TestAddressRigPath(t *testing.T) {
	tests := []struct {
		addr *Address
		want string
	}{
		{
			addr: &Address{Rig: "excavation", Miner: "rictus"},
			want: "excavation/rictus",
		},
		{
			addr: &Address{Rig: "excavation"},
			want: "excavation/",
		},
		{
			addr: &Address{Machine: "vm", Rig: "excavation", Miner: "rictus"},
			want: "excavation/rictus",
		},
		{
			addr: &Address{Rig: "a", Miner: "b/c/d"},
			want: "a/b/c/d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.addr.RigPath()
			if got != tt.want {
				t.Errorf("RigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
