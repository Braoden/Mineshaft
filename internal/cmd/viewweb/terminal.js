// terminal.js - xterm.js front end for the /api/terminal WebSocket.
//
// One pane, one shell. `ms view` spawns a PowerShell and this page is its only
// screen, so it fits freely to the browser and forwards resizes upstream.
//
// Write access is still decided by the SERVER from the target name (see
// isWritableTarget in view_terminal.go); "shell" is the writable one.

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

const TERM_OPTS = {
    fontFamily: 'Cascadia Mono, Consolas, "Courier New", monospace',
    fontSize: 13,
    lineHeight: 1.15,
    cursorBlink: true,
    scrollback: 5000,
    theme: THEME,
    allowProposedApi: true,
};

const MS = { term: null, fit: null, ws: null, started: false, available: null, waiting: false };

function send(obj) {
    if (MS.ws && MS.ws.readyState === WebSocket.OPEN) MS.ws.send(JSON.stringify(obj));
}

function connect() {
    if (MS.ws) { try { MS.ws.close(); } catch (_) {} MS.ws = null; }
    MS.fit.fit();

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
        `${proto}//${location.host}/api/terminal/ws`
        + `?target=shell&cols=${MS.term.cols}&rows=${MS.term.rows}`);
    ws.binaryType = 'arraybuffer';
    MS.ws = ws;

    ws.onmessage = ev => {
        if (typeof ev.data === 'string') MS.term.write(ev.data);
        else MS.term.write(new Uint8Array(ev.data));
    };
    // No status chrome on this page, so the terminal reports its own state and
    // any keypress reconnects. Costs no UI and beats a silent dead pane.
    ws.onclose = () => {
        MS.ws = null;
        MS.waiting = true;
        MS.term.write('\r\n\x1b[33m[disconnected - press any key to reconnect]\x1b[0m\r\n');
    };
    ws.onerror = () => { try { ws.close(); } catch (_) {} };
}

function initPage() {
    if (MS.started) return;
    MS.started = true;

    MS.term = new Terminal(TERM_OPTS);
    MS.fit = new FitAddon.FitAddon();
    MS.term.loadAddon(MS.fit);
    MS.term.open(document.getElementById('term-host'));
    MS.fit.fit();

    MS.term.onData(d => {
        if (MS.waiting) { MS.waiting = false; connect(); return; }
        send({ t: 'i', d });
    });
    MS.term.onResize(({ cols, rows }) => send({ t: 'r', cols, rows }));

    let rt = null;
    window.addEventListener('resize', () => {
        clearTimeout(rt);
        rt = setTimeout(() => MS.fit.fit(), 150);
    });
}

// activate runs the first time the terminal view is shown, so a dashboard user
// never pays for xterm's setup or opens a shell they didn't ask for.
async function activateTerm() {
    if (MS.available === null) {
        try {
            const r = await fetch('/api/terminal/sessions', { cache: 'no-store' });
            if (!r.ok) throw new Error(r.status);
            MS.available = true;
        } catch (err) {
            MS.available = false;
            console.error('terminal unavailable', err);
        }
    }

    const host = document.getElementById('term-host');
    if (MS.available === false) {
        document.getElementById('term-disabled').style.display = 'block';
        host.style.display = 'none';
        return;
    }

    const first = !MS.started;
    initPage();
    if (first) connect();

    MS.fit.fit();
    MS.term.focus();
}

function deactivateTerm() {
    // keep the socket open: the shell is meant to survive leaving the page
}

window.MSTerminal = { activate: activateTerm, deactivate: deactivateTerm };
