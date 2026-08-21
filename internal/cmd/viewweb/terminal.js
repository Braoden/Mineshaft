// terminal.js — xterm.js front end for the /api/terminal WebSocket.
//
// Three panes share one page:
//   shell     a PowerShell ms view spawned; the page is its only screen, so it
//             fits freely to the browser and forwards resizes
//   overseer  an attach to the overseer's live pane; WRITABLE
//   agents    a picker over every other live session; READ-ONLY
//
// The two attach panes are sized by the multiplexer, never by us. tmux runs
// with window-size=latest, so forwarding a resize from here would reflow the
// agent's real terminal the moment the browser became the active client. We
// attach at the pane's native size and CSS-scale the result to fit instead.
//
// Write access is decided by the SERVER from the target name (see
// isWritableTarget in view_terminal.go). The `writable` flag below only styles
// the UI and skips sending keys the server would drop anyway.

const THEME = {
    background: '#0d0e20',
    foreground: '#d7fff7',
    cursor: '#d7fff7',
    cursorAccent: '#111227',
    selectionBackground: 'rgba(46,69,125,.65)',
    black: '#111227', brightBlack: '#3a3a63',
    red: '#e35d6a', brightRed: '#ff7d88',
    green: '#5aa053', brightGreen: '#77c46e',
    yellow: '#e8b04b', brightYellow: '#ffc861',
    blue: '#2e457d', brightBlue: '#4d6fbd',
    magenta: '#7a6a92', brightMagenta: '#a58fc4',
    cyan: '#82a0b9', brightCyan: '#a9ccc4',
    white: '#d7fff7', brightWhite: '#ffffff',
};

const el = id => document.getElementById(id);

const TERM_OPTS = {
    fontFamily: 'Cascadia Mono, Consolas, "Courier New", monospace',
    fontSize: 13,
    lineHeight: 1.15,
    cursorBlink: true,
    scrollback: 5000,
    theme: THEME,
    allowProposedApi: true,
};

// ---------------------------------------------------------------- pane

// makePane builds one terminal bound to one WebSocket.
//
// fixed panes (the attaches) never call fit() and never send a resize; they
// are scaled to their box instead. The shell pane is the only one that owns
// its own dimensions.
function makePane({ hostId, statusId, fixed }) {
    const host = el(hostId);
    const status = statusId ? el(statusId) : null;

    const pane = {
        term: null,
        fit: null,
        ws: null,
        target: null,
        writable: false,
        fixed: !!fixed,
        scaler: null,
    };

    function setStatus(text, cls) {
        if (!status) return;
        status.textContent = text;
        status.className = 'chip' + (cls ? ' ' + cls : '');
    }

    function init() {
        if (pane.term) return;

        // A wrapper we can transform without xterm recomputing its own
        // geometry: scaling the mount point directly confuses its mouse math.
        pane.scaler = document.createElement('div');
        pane.scaler.className = 'term-scale';
        host.appendChild(pane.scaler);

        pane.term = new Terminal(TERM_OPTS);
        if (!pane.fixed) {
            pane.fit = new FitAddon.FitAddon();
            pane.term.loadAddon(pane.fit);
        }
        pane.term.open(pane.scaler);
        if (pane.fit) pane.fit.fit();

        pane.term.onData(d => {
            if (!pane.writable) return;
            send({ t: 'i', d });
        });

        // Only the shell may drive its own size; see the header note.
        pane.term.onResize(({ cols, rows }) => {
            if (!pane.fixed) send({ t: 'r', cols, rows });
        });
    }

    function send(obj) {
        if (pane.ws && pane.ws.readyState === WebSocket.OPEN) {
            pane.ws.send(JSON.stringify(obj));
        }
    }

    // rescale fits a fixed-size terminal into its box without resizing it.
    function rescale() {
        if (!pane.fixed || !pane.scaler) return;
        pane.scaler.style.transform = 'scale(1)';
        const w = pane.scaler.offsetWidth, h = pane.scaler.offsetHeight;
        if (!w || !h) return;
        // Never scale up past 1: a small pane blown up is just blurry.
        const s = Math.min(host.clientWidth / w, host.clientHeight / h, 1);
        pane.scaler.style.transform = `scale(${s})`;
    }

    // connect points the pane at a target. cols/rows are the target's native
    // size and are required for fixed panes.
    function connect({ target, writable, cols, rows }) {
        init();
        if (pane.ws) { try { pane.ws.close(); } catch (_) {} pane.ws = null; }

        pane.target = target;
        pane.writable = !!writable;
        host.classList.toggle('is-readonly', !pane.writable);

        if (pane.fixed && cols && rows) {
            pane.term.resize(cols, rows);
            rescale();
        } else if (pane.fit) {
            pane.fit.fit();
        }

        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const url = `${proto}//${location.host}/api/terminal/ws`
            + `?target=${encodeURIComponent(target)}`
            + `&cols=${pane.term.cols}&rows=${pane.term.rows}`;

        setStatus('connecting', '');
        const ws = new WebSocket(url);
        ws.binaryType = 'arraybuffer';
        pane.ws = ws;

        ws.onopen = () => setStatus(pane.writable ? 'live' : 'read-only', pane.writable ? 'on' : '');
        ws.onmessage = ev => {
            if (typeof ev.data === 'string') pane.term.write(ev.data);
            else pane.term.write(new Uint8Array(ev.data));
        };
        ws.onclose = () => { setStatus('disconnected', 'warn'); pane.ws = null; };
        ws.onerror = () => setStatus('error', 'crit');
    }

    function relayout() {
        if (pane.fixed) rescale();
        else if (pane.fit) pane.fit.fit();
    }

    function focus() { if (pane.term) pane.term.focus(); }

    return { connect, relayout, focus, get target() { return pane.target; } };
}

// ---------------------------------------------------------------- page

const MS = {
    shell: null,
    overseer: null,
    agent: null,
    sessions: [],
    started: false,
    available: null,
};

async function loadSessions() {
    try {
        const r = await fetch('/api/terminal/sessions', { cache: 'no-store' });
        if (!r.ok) throw new Error(r.status);
        MS.sessions = await r.json();
        MS.available = true;
    } catch (err) {
        MS.available = false;
        console.error('terminal unavailable', err);
    }
}

// The overseer is whichever session the server marked writable but did not
// spawn. Derived rather than hardcoded so the name stays the server's business.
function overseerEntry() {
    return MS.sessions.find(s => s.kind === 'agent' && s.writable) || null;
}

function readOnlyEntries() {
    return MS.sessions.filter(s => s.kind === 'agent' && !s.writable);
}

function fillAgentPicker() {
    const sel = el('term-agent-target');
    const list = readOnlyEntries();
    const current = sel.value;

    sel.innerHTML = list.map(s =>
        `<option value="${s.id}"${s.attached ? ' data-attached="1"' : ''}>${s.label}</option>`
    ).join('');

    if (current && list.some(s => s.id === current)) sel.value = current;
    return list;
}

function connectAgentPane() {
    const sel = el('term-agent-target');
    const entry = readOnlyEntries().find(s => s.id === sel.value);
    if (!entry) return;
    MS.agent.connect({ target: entry.id, writable: false, cols: entry.cols, rows: entry.rows });
}

function connectAll() {
    MS.shell.connect({ target: 'shell', writable: true });

    const ov = overseerEntry();
    if (ov) {
        el('term-overseer-name').textContent = ov.label;
        MS.overseer.connect({ target: ov.id, writable: true, cols: ov.cols, rows: ov.rows });
    } else {
        el('term-overseer-name').textContent = 'not running';
    }

    if (fillAgentPicker().length) connectAgentPane();
}

function initPage() {
    if (MS.started) return;
    MS.started = true;

    MS.shell = makePane({ hostId: 'term-host-shell', statusId: 'term-status-shell' });
    MS.overseer = makePane({ hostId: 'term-host-overseer', statusId: 'term-status-overseer', fixed: true });
    MS.agent = makePane({ hostId: 'term-host-agent', statusId: 'term-status-agent', fixed: true });

    el('term-agent-target').addEventListener('change', connectAgentPane);
    el('term-reconnect').addEventListener('click', async () => {
        await loadSessions();
        connectAll();
    });

    let rt = null;
    window.addEventListener('resize', () => {
        clearTimeout(rt);
        rt = setTimeout(() => {
            MS.shell.relayout();
            MS.overseer.relayout();
            MS.agent.relayout();
        }, 150);
    });
}

// activate runs the first time the terminal view is shown, so a dashboard user
// never pays for xterm's setup or opens a shell they didn't ask for.
async function activateTerm() {
    await loadSessions();
    if (MS.available === false) {
        el('term-disabled').style.display = 'flex';
        el('term-live').style.display = 'none';
        return;
    }

    el('term-disabled').style.display = 'none';
    el('term-live').style.display = 'flex';

    const first = !MS.started;
    initPage();
    if (first) connectAll();

    MS.shell.relayout();
    MS.overseer.relayout();
    MS.agent.relayout();
    MS.shell.focus();
}

function deactivateTerm() {
    // keep the sockets open: the shell is meant to survive leaving the page
}

window.MSTerminal = { activate: activateTerm, deactivate: deactivateTerm };
