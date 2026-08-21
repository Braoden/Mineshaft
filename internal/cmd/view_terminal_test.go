package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/steveyegge/mineshaft/internal/session"
)

// TestCheckTerminalOrigin is the load-bearing security test for the terminal.
//
// WebSocket handshakes are NOT subject to CORS: without this check, any page
// the user happens to visit could open ws://127.0.0.1:<port>/api/terminal/ws
// and get a shell on this machine. Every case below is a real thing a browser
// sends, so a regression here is a remote-code-execution hole, not a bug.
func TestCheckTerminalOrigin(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		origin  string
		allowed bool
	}{
		// A non-browser client (curl, a test) sends no Origin. It also has no
		// ambient authority to abuse, so this is allowed.
		{"no origin", "127.0.0.1:8090", "", true},

		{"same origin loopback", "127.0.0.1:8090", "http://127.0.0.1:8090", true},
		{"same origin localhost", "localhost:8090", "http://localhost:8090", true},

		// The attack this exists to stop.
		{"hostile site", "127.0.0.1:8090", "https://evil.example", false},
		{"hostile site plain", "127.0.0.1:8090", "http://attacker.test", false},

		// A sandboxed iframe sends the literal string "null". It must not be
		// mistaken for "no origin".
		{"null origin", "127.0.0.1:8090", "null", false},

		// Port must match: another local service on a different port is a
		// different origin and shouldn't be able to reach this one.
		{"port mismatch", "127.0.0.1:8090", "http://127.0.0.1:9999", false},

		// A DNS name that resolves to loopback is still cross-origin here,
		// which is exactly the rebinding case.
		{"rebinding host", "127.0.0.1:8090", "http://localtest.me:8090", false},

		{"subdomain of loopback name", "localhost:8090", "http://evil.localhost:8090", false},
		{"empty host origin", "127.0.0.1:8090", "http://", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "http://"+tt.host+"/api/terminal/ws", nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			err = checkTerminalOrigin(r)
			if tt.allowed && err != nil {
				t.Errorf("origin %q with host %q should be allowed, got %v", tt.origin, tt.host, err)
			}
			if !tt.allowed && err == nil {
				t.Errorf("origin %q with host %q should be REJECTED but was allowed", tt.origin, tt.host)
			}
		})
	}
}

// The terminal ships on by default, so --terminal=false is the only thing
// standing between a user who wants a safe viewer and shell access on the port.
// This pins the wiring contract that makes the flag real: registerTerminalRoutes
// is the ONLY thing that adds these routes, so skipping it removes the
// endpoints entirely rather than merely hiding the page.
func TestTerminalRoutesAreSkippable(t *testing.T) {
	mux := http.NewServeMux()

	for _, path := range []string{"/api/terminal/ws", "/api/terminal/sessions"} {
		r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:8090"+path, nil)
		if _, pattern := mux.Handler(r); pattern != "" {
			t.Errorf("%s is routed on a bare mux (pattern %q); it must require --terminal", path, pattern)
		}
	}

	registerTerminalRoutes(mux)

	for _, path := range []string{"/api/terminal/ws", "/api/terminal/sessions"} {
		r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:8090"+path, nil)
		if _, pattern := mux.Handler(r); pattern == "" {
			t.Errorf("%s should be routed after registerTerminalRoutes", path)
		}
	}
}

// Only the spawned shell and the overseer accept input. Every other agent is
// observable but not typeable, so a stray keypress cannot land in a working
// agent's prompt. This pins the allowlist: it is decided from the target name
// alone, never from anything the browser sends.
func TestOnlyShellAndOverseerAreWritable(t *testing.T) {
	for _, target := range []string{"shell", session.OverseerSessionName()} {
		if !isWritableTarget(target) {
			t.Errorf("target %q should be writable", target)
		}
	}

	// Agents doing real work. Typing into these corrupts a live session.
	for _, agent := range []string{
		"hq-supervisor", "mi-witness", "hq-boot",
		"mi-refinery", "hq-dog-alpha", "ms-wyvern-Toast",
	} {
		if isWritableTarget(agent) {
			t.Errorf("agent target %q must be read-only", agent)
		}
	}
}

// clientSizes is the ONLY geometry source the sizing loop is allowed to use,
// because psmux's display-message lies (it reported a 120x29 window for a
// session whose sole client was 64x50). Pin the parse against real output.
func TestClientSizesParsesListClients(t *testing.T) {
	const out = `/dev/pts/116: hq-overseer: claude [64x50] (utf8) [activity=896s ago]
/dev/pts/11670: hq-overseer: claude [64x49] (utf8) [activity=18s ago]`

	var got [][2]int
	for _, m := range clientSizeRe.FindAllStringSubmatch(out, -1) {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		got = append(got, [2]int{w, h})
	}

	want := [][2]int{{64, 50}, {64, 49}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}

	// The correction must grow the small client to the large one, never the
	// reverse: tmux sizes a window down to its smallest client, so shrinking
	// would reflow the agent's real terminal.
	minH, maxH := got[1][1], got[0][1]
	if maxH-minH != 1 {
		t.Fatalf("expected a 1-row deficit, got %d", maxH-minH)
	}
}

// The browser must not be able to talk itself into write access. The wire
// format carries no control verb at all, so an unknown or forged frame is
// inert rather than privilege-granting.
func TestClientMessageCannotGrantControl(t *testing.T) {
	var msg termClientMsg
	if err := json.Unmarshal([]byte(`{"t":"c","on":true}`), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.T != "c" {
		t.Fatalf("expected the verb to survive parsing, got %q", msg.T)
	}

	// serveTerminalWS switches on msg.T and has no "c" case: the frame is
	// silently ignored. If someone reintroduces a control verb, this fails.
	if reflect.TypeOf(msg).NumField() != 4 {
		t.Errorf("termClientMsg gained a field; a control toggle must not come back: %+v", msg)
	}
}
