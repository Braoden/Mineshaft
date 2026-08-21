package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"
	"github.com/coder/websocket"
	"github.com/steveyegge/mineshaft/internal/tmux"
)

// The terminal page runs real shells, so it is opt-in: without --terminal the
// routes below are never registered and `ms view` has exactly the surface it
// had before. See registerTerminalRoutes.

// termScrollback is how much recent output is replayed to a reattaching
// client. Enough to restore context after a page refresh without holding a
// build's entire log in memory.
const termScrollback = 256 * 1024

// termIdleTimeout bounds how long an agent attachment lingers with no client.
// Shell sessions are exempt: a long build must survive a closed tab.
const termIdleTimeout = 30 * time.Second

type termKind string

const (
	termShell termKind = "shell" // a PowerShell we spawned
	termAgent termKind = "agent" // an attachment to an existing agent pane
)

// termSession is one PTY plus the fan-out to every attached browser.
type termSession struct {
	id   string
	kind termKind

	mu      sync.Mutex
	pty     pty.Pty
	cmd     *pty.Cmd
	buf     []byte                  // ring of recent output, capped at termScrollback
	subs    map[chan []byte]struct{} // one channel per attached browser
	closed  bool
	lastUse time.Time
}

type termManager struct {
	mu       sync.Mutex
	sessions map[string]*termSession
	townSock string
}

var terminals = &termManager{sessions: map[string]*termSession{}}

// ---------------------------------------------------------------- session

func (s *termSession) publish(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = append(s.buf, b...)
	if len(s.buf) > termScrollback {
		s.buf = append([]byte(nil), s.buf[len(s.buf)-termScrollback:]...)
	}
	for ch := range s.subs {
		// Never block the PTY reader on a slow browser: drop for that
		// subscriber instead, since a stalled read would back up the shell.
		select {
		case ch <- append([]byte(nil), b...):
		default:
		}
	}
}

func (s *termSession) subscribe() (<-chan []byte, []byte, func()) {
	ch := make(chan []byte, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	history := append([]byte(nil), s.buf...)
	s.lastUse = time.Now()
	s.mu.Unlock()

	return ch, history, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.lastUse = time.Now()
		s.mu.Unlock()
		close(ch)
	}
}

func (s *termSession) subscriberCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *termSession) write(b []byte) error {
	s.mu.Lock()
	p := s.pty
	closed := s.closed
	s.mu.Unlock()
	if closed || p == nil {
		return fmt.Errorf("session closed")
	}
	_, err := p.Write(b)
	return err
}

func (s *termSession) resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.Lock()
	p := s.pty
	s.mu.Unlock()
	if p != nil {
		_ = p.Resize(cols, rows)
	}
}

func (s *termSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	p, c := s.pty, s.cmd
	s.mu.Unlock()

	if p != nil {
		_ = p.Close()
	}
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}

// pump copies PTY output to subscribers until the process exits.
func (s *termSession) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.publish(buf[:n])
		}
		if err != nil {
			s.publish([]byte("\r\n\x1b[2m[session ended]\x1b[0m\r\n"))
			s.close()
			return
		}
	}
}

// ---------------------------------------------------------------- manager

// shellCommand picks the best available PowerShell. pwsh (7+) is preferred;
// Windows PowerShell 5.1 is the fallback and is all that exists on many boxes.
func shellCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pwsh"); err == nil {
			return p, []string{"-NoLogo"}
		}
		return "powershell.exe", []string{"-NoLogo"}
	}
	if p, err := exec.LookPath("pwsh"); err == nil {
		return p, []string{"-NoLogo"}
	}
	return "/bin/sh", nil
}

func (m *termManager) get(id string) *termSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s != nil && !s.closed {
		return s
	}
	return nil
}

// openShell returns the persistent shell, creating it on first use. It lives
// for the life of `ms view` so a reload reattaches instead of restarting.
func (m *termManager) openShell(cols, rows int) (*termSession, error) {
	if s := m.get("shell"); s != nil {
		return s, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions["shell"]; s != nil && !s.closed {
		return s, nil
	}

	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("allocating pty: %w", err)
	}
	if cols > 0 && rows > 0 {
		_ = p.Resize(cols, rows)
	}
	name, args := shellCommand()
	c := p.Command(name, args...)
	if err := c.Start(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("starting %s: %w", name, err)
	}

	s := &termSession{id: "shell", kind: termShell, pty: p, cmd: c,
		subs: map[chan []byte]struct{}{}, lastUse: time.Now()}
	m.sessions["shell"] = s
	go s.pump()
	return s, nil
}

// openAgent attaches to an existing agent pane by running the multiplexer's
// attach inside a PTY — that yields the real rendered pane, colours and cursor
// included, which capture-pane cannot do.
//
// The PTY is sized to the session's CURRENT size rather than the browser's.
// A multiplexer sizes a window to its smallest attached client, so attaching at
// an arbitrary size would visibly reflow the agent's terminal underneath it.
func (m *termManager) openAgent(name string) (*termSession, error) {
	if s := m.get("agent:" + name); s != nil {
		return s, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions["agent:"+name]; s != nil && !s.closed {
		return s, nil
	}

	cols, rows := agentPaneSize(name)

	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("allocating pty: %w", err)
	}
	_ = p.Resize(cols, rows)

	args := []string{"-u"}
	if sock := tmux.GetDefaultSocket(); sock != "" {
		args = append(args, "-L", sock)
	}
	args = append(args, "attach-session", "-t", name)

	c := p.Command("tmux", args...)
	// Strip the nesting variables: `ms view` is often itself running inside a
	// pane, and the multiplexer refuses to attach from within a session
	// ("sessions should be nested with care") unless they are cleared.
	c.Env = tmux.SanitizedEnv()
	if err := c.Start(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("attaching to %s: %w", name, err)
	}

	s := &termSession{id: "agent:" + name, kind: termAgent, pty: p, cmd: c,
		subs: map[chan []byte]struct{}{}, lastUse: time.Now()}
	m.sessions["agent:"+name] = s
	go s.pump()
	go m.reapWhenIdle(s)
	return s, nil
}

// reapWhenIdle detaches an agent attachment once no browser is watching, so we
// don't leave phantom clients hanging off the agent's session.
func (m *termManager) reapWhenIdle(s *termSession) {
	for {
		time.Sleep(5 * time.Second)
		s.mu.Lock()
		closed, subs, idle := s.closed, len(s.subs), time.Since(s.lastUse)
		s.mu.Unlock()
		if closed {
			return
		}
		if subs == 0 && idle > termIdleTimeout {
			s.close()
			m.mu.Lock()
			delete(m.sessions, s.id)
			m.mu.Unlock()
			return
		}
	}
}

// agentPaneSize reports a session's current dimensions, defaulting to a
// conventional size when the query fails.
func agentPaneSize(name string) (int, int) {
	args := []string{"-u"}
	if sock := tmux.GetDefaultSocket(); sock != "" {
		args = append(args, "-L", sock)
	}
	args = append(args, "display-message", "-p", "-t", name, "#{window_width}x#{window_height}")
	sizeCmd := exec.Command("tmux", args...)
	sizeCmd.Env = tmux.SanitizedEnv()
	out, err := sizeCmd.Output()
	if err != nil {
		return 120, 30
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "x", 2)
	if len(parts) != 2 {
		return 120, 30
	}
	c, err1 := strconv.Atoi(parts[0])
	r, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || c <= 0 || r <= 0 {
		return 120, 30
	}
	return c, r
}

// listAgentSessions enumerates live panes on the town socket. Discovered
// dynamically rather than derived from the known roles, because the socket
// carries sessions the dashboard roster never shows (boot, dogs).
func listAgentSessions() []string {
	t := tmux.NewTmux()
	names, err := t.ListSessions()
	if err != nil {
		return nil
	}
	return names
}

// ---------------------------------------------------------------- transport

// checkTerminalOrigin rejects cross-origin WebSocket handshakes.
//
// This is the load-bearing protection. WebSockets are NOT subject to CORS, so
// without it any page the user visits could open ws://127.0.0.1:<port> and get
// a shell on this machine. A same-origin or absent Origin is required.
func checkTerminalOrigin(r *http.Request) error {
	host := r.Host
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil // non-browser client (curl, tests); no ambient authority to abuse
	}
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("unparseable Origin")
	}
	if !strings.EqualFold(u.Host, host) {
		return fmt.Errorf("cross-origin request from %q rejected", origin)
	}
	if h, _, err := net.SplitHostPort(u.Host); err == nil {
		if h != "127.0.0.1" && h != "localhost" && h != "::1" {
			return fmt.Errorf("non-loopback Origin %q rejected", origin)
		}
	}
	return nil
}

// control frames from the browser
type termClientMsg struct {
	T    string `json:"t"`              // "i" input, "r" resize, "c" control
	D    string `json:"d,omitempty"`    // input payload
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	On   bool   `json:"on,omitempty"`   // take-control toggle
}

func serveTerminalWS(w http.ResponseWriter, r *http.Request) {
	if err := checkTerminalOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = "shell"
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))

	var (
		sess *termSession
		err  error
		// Agent panes start read-only: a stray keypress must not land in a
		// working agent's prompt. The browser asks for control explicitly.
		writable = target == "shell"
	)
	if target == "shell" {
		sess, err = terminals.openShell(cols, rows)
	} else {
		if !isKnownAgentSession(target) {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}
		sess, err = terminals.openAgent(target)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  []string{"127.0.0.1:*", "localhost:*", "[::1]:*"},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	out, history, unsubscribe := sess.subscribe()
	defer unsubscribe()

	// replay scrollback so a reattaching browser sees where it left off
	if len(history) > 0 {
		_ = conn.Write(ctx, websocket.MessageBinary, history)
	}

	// writer: PTY output -> browser
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case b, ok := <-out:
				if !ok {
					return
				}
				if err := conn.Write(ctx, websocket.MessageBinary, b); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// reader: browser -> PTY
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg termClientMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.T {
		case "i":
			if writable {
				_ = sess.write([]byte(msg.D))
			}
		case "r":
			// Never resize an agent pane from the browser: the multiplexer
			// would reflow the agent's own terminal to match.
			if sess.kind == termShell {
				sess.resize(msg.Cols, msg.Rows)
			}
		case "c":
			if sess.kind == termAgent {
				writable = msg.On
			}
		}
	}
}

func isKnownAgentSession(name string) bool {
	for _, s := range listAgentSessions() {
		if s == name {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- routes

// registerTerminalRoutes wires the terminal endpoints. Called only when
// --terminal is passed, so the default `ms view` exposes no shell at all.
func registerTerminalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/terminal/sessions", func(w http.ResponseWriter, r *http.Request) {
		type entry struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			Kind     string `json:"kind"`
			Attached bool   `json:"attached"`
		}
		list := []entry{{ID: "shell", Label: "New shell", Kind: "shell",
			Attached: terminals.get("shell") != nil}}
		for _, name := range listAgentSessions() {
			s := terminals.get("agent:" + name)
			list = append(list, entry{ID: name, Label: name, Kind: "agent",
				Attached: s != nil && s.subscriberCount() > 0})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/terminal/ws", serveTerminalWS)
}
