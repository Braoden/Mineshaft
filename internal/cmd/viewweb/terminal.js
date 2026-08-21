// terminal.js — xterm.js front end for the /api/terminal WebSocket.
//
// Two kinds of target share one view:
//   shell   a PowerShell we spawned; fully interactive, persists across reloads
//   agent   an attachment to a live agent pane; READ-ONLY until unlocked, and
//           never resized from here (the multiplexer would reflow the agent's
//           own terminal to match this browser)

const T = {
    term: null,
    fit: null,
    ws: null,
    target: 'shell',
    kind: 'shell',
    control: false,
    started: false,
    available: null,     // null = unknown, false = --terminal not passed
};

const tEl = id => document.getElementById(id);

// xterm theme built from the dashboard palette so the terminal belongs to the
// page rather than looking like a pasted-in widget.
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

function setStatus(text, cls) {
    const el = tEl('term-status');
    if (!el) return;
    el.textContent = text;
    el.className = 'chip' + (cls ? ' ' + cls : '');
}

// ---------------------------------------------------------------- sessions

async function loadSessions() {
    const sel = tEl('term-target');
    try {
        const r = await fetch('/api/terminal/sessions', { cache: 'no-store' });
        if (r.status === 404) { T.available = false; return; }
        if (!r.ok) throw new Error(r.status);
        const list = await r.json();
        T.available = true;

        const current = sel.value;
        sel.innerHTML = list.map(s => {
            const mark = s.attached ? ' •' : '';
            return `<option value="${s.id}" data-kind="${s.kind}">${s.label}${mark}</option>`;
        }).join('');
        if (current && list.some(s => s.id === current)) sel.value = current;
    } catch (err) {
        T.available = false;
        console.error('terminal sessions unavailable', err);
    }
}

function showDisabled() {
    tEl('term-disabled').style.display = 'flex';
    tEl('term-live').style.display = 'none';
    setStatus('disabled', '');
}

// ---------------------------------------------------------------- connection

function connectTerm() {
    if (T.ws) { try { T.ws.close(); } catch (_) {} T.ws = null; }

    const sel = tEl('term-target');
    T.target = sel.value || 'shell';
    T.kind = sel.selectedOptions[0] ? sel.selectedOptions[0].dataset.kind : 'shell';
    T.control = false;
    updateControlUI();

    const cols = T.term.cols, rows = T.term.rows;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/api/terminal/ws`
        + `?target=${encodeURIComponent(T.target)}&cols=${cols}&rows=${rows}`;

    setStatus('connecting', '');
    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';
    T.ws = ws;

    ws.onopen = () => {
        setStatus(T.kind === 'agent' ? 'watching' : 'live', 'on');
        T.term.focus();
    };
    ws.onmessage = ev => {
        if (typeof ev.data === 'string') T.term.write(ev.data);
        else T.term.write(new Uint8Array(ev.data));
    };
    ws.onclose = () => {
        setStatus('disconnected', 'warn');
        T.ws = null;
    };
    ws.onerror = () => setStatus('error', 'crit');
}

function send(obj) {
    if (T.ws && T.ws.readyState === WebSocket.OPEN) T.ws.send(JSON.stringify(obj));
}

function updateControlUI() {
    const btn = tEl('term-control');
    if (!btn) return;
    const isAgent = T.kind === 'agent';
    btn.style.display = isAgent ? 'inline-flex' : 'none';
    btn.textContent = T.control ? 'release control' : 'take control';
    btn.className = 'term-btn' + (T.control ? ' active' : '');
    if (isAgent) setStatus(T.control ? 'controlling' : 'watching', T.control ? 'warn' : 'on');
}

// ---------------------------------------------------------------- lifecycle

function initTerm() {
    if (T.started) return;
    T.started = true;

    T.term = new Terminal({
        fontFamily: 'Cascadia Mono, Consolas, "Courier New", monospace',
        fontSize: 13,
        lineHeight: 1.15,
        cursorBlink: true,
        scrollback: 5000,
        theme: THEME,
        allowProposedApi: true,
    });
    T.fit = new FitAddon.FitAddon();
    T.term.loadAddon(T.fit);
    T.term.open(tEl('term-host'));
    T.fit.fit();

    // keystrokes: the server also enforces read-only, this is just the UI half
    T.term.onData(d => {
        if (T.kind === 'agent' && !T.control) return;
        send({ t: 'i', d });
    });

    // Only a spawned shell may be resized. Resizing an attached agent pane
    // would reflow the agent's real terminal, so it is deliberately not sent.
    T.term.onResize(({ cols, rows }) => {
        if (T.kind !== 'agent') send({ t: 'r', cols, rows });
    });

    tEl('term-target').addEventListener('change', connectTerm);
    tEl('term-reconnect').addEventListener('click', connectTerm);
    tEl('term-control').addEventListener('click', () => {
        T.control = !T.control;
        send({ t: 'c', on: T.control });
        updateControlUI();
        T.term.focus();
    });

    let rt = null;
    window.addEventListener('resize', () => {
        clearTimeout(rt);
        rt = setTimeout(() => { if (T.fit) T.fit.fit(); }, 150);
    });
}

// activate is called the first time the terminal view is shown, so a dashboard
// user never pays for xterm's setup or opens a shell they didn't ask for.
async function activateTerm() {
    await loadSessions();
    if (T.available === false) { showDisabled(); return; }

    tEl('term-disabled').style.display = 'none';
    tEl('term-live').style.display = 'flex';

    initTerm();
    T.fit.fit();
    if (!T.ws) connectTerm();
    T.term.focus();
}

function deactivateTerm() {
    // keep the socket open: the shell is meant to survive leaving the page
}

window.MSTerminal = { activate: activateTerm, deactivate: deactivateTerm };
