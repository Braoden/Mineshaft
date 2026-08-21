// dashboard.js — wires live workspace data into the cards and the three rooms.
//
// Data sources:
//   GET  /api/state          town, rigs, agents (role + running)
//   GET  /api/usage          current usage windows + per-limit breakdown
//   GET  /api/usage/history  persisted samples, for the chart and burn rate
//   SSE  /api/events         'state' snapshots and 'feed' activity events

const $ = id => document.getElementById(id);

// Which room each role appears in. Anything unrecognised lands in the office
// rather than vanishing — no running agent should be invisible.
const ROOM_OF = {
    miner: 'mineshaft',
    refinery: 'refinery',
    overseer: 'overseer',
    supervisor: 'overseer',
    witness: 'overseer',
};
const roomFor = role => ROOM_OF[role] || 'overseer';

const state = {
    agents: [],
    town: '',
    usage: null,
    history: [],
    rooms: {},
    feed: [],
};

// ---------------------------------------------------------------- helpers

function severityOf(pct) {
    if (pct >= 90) return 'crit';
    if (pct >= 75) return 'warn';
    return 'ok';
}

// "3h 22m" / "48m" / "12s"
function humanDuration(ms) {
    if (!isFinite(ms) || ms <= 0) return '0m';
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`;
    if (m > 0) return `${m}m ${String(s % 60).padStart(2, '0')}s`;
    return `${s}s`;
}

function clockText(d) {
    return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

function greetingFor(d) {
    const h = d.getHours();
    if (h < 12) return 'Good morning';
    if (h < 18) return 'Good afternoon';
    return 'Good evening';
}

// ---------------------------------------------------------------- rooms

async function initRooms() {
    for (const kind of ['mineshaft', 'refinery', 'overseer']) {
        const room = new window.MSRooms.Room($(`cv-${kind}`), kind);
        try {
            await room.init();
            state.rooms[kind] = room;
        } catch (err) {
            console.error(`room ${kind} failed to init`, err);
        }
    }
    applyAgentsToRooms();

    let resizeTimer = null;
    window.addEventListener('resize', () => {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(() => {
            for (const r of Object.values(state.rooms)) r.resize();
            applyAgentsToRooms();
        }, 180);
    });
}

function applyAgentsToRooms() {
    for (const kind of ['mineshaft', 'refinery', 'overseer']) {
        const room = state.rooms[kind];
        const mine = state.agents.filter(a => roomFor(a.role) === kind);
        const running = mine.filter(a => a.running).length;
        if (room) room.setAgents(mine);

        const chip = $(`chip-${kind}`);
        if (chip) {
            chip.textContent = running === 0
                ? 'idle'
                : `${running} active`;
            chip.className = running > 0 ? 'chip on' : 'chip';
        }
        const empty = $(`empty-${kind}`);
        if (empty) empty.classList.toggle('show', running === 0);
    }
}

// ---------------------------------------------------------------- chart

function renderChart() {
    const host = $('chart');
    const pts = state.history;
    if (!pts || pts.length < 2) {
        host.innerHTML = '<div class="chart-empty">Collecting usage history — the chart appears after a few samples.</div>';
        return;
    }

    const W = 600, H = 108, padB = 16;
    const t0 = new Date(pts[0].ts).getTime();
    const t1 = new Date(pts[pts.length - 1].ts).getTime();
    const span = Math.max(1, t1 - t0);
    const maxPct = Math.max(100, ...pts.map(p => p.pct));

    const xy = p => {
        const x = ((new Date(p.ts).getTime() - t0) / span) * W;
        const y = (H - padB) - (p.pct / maxPct) * (H - padB);
        return [x, y];
    };

    const coords = pts.map(xy);
    const line = coords.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ');
    const area = `${line} L${W},${H - padB} L0,${H - padB} Z`;
    const [lx, ly] = coords[coords.length - 1];

    // horizontal guides at 50% and 100% of the window
    const guide = pct => (H - padB) - (pct / maxPct) * (H - padB);

    host.innerHTML = `
      <svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img"
           aria-label="Session usage over time, currently ${Math.round(pts[pts.length - 1].pct)} percent">
        <defs>
          <linearGradient id="fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#2e457d" stop-opacity=".75"/>
            <stop offset="100%" stop-color="#2e457d" stop-opacity=".04"/>
          </linearGradient>
        </defs>
        <line x1="0" y1="${guide(100).toFixed(1)}" x2="${W}" y2="${guide(100).toFixed(1)}"
              stroke="#82a0b9" stroke-opacity=".18" stroke-dasharray="4 5" vector-effect="non-scaling-stroke"/>
        <line x1="0" y1="${guide(50).toFixed(1)}" x2="${W}" y2="${guide(50).toFixed(1)}"
              stroke="#82a0b9" stroke-opacity=".1" stroke-dasharray="4 5" vector-effect="non-scaling-stroke"/>
        <path d="${area}" fill="url(#fill)"/>
        <path d="${line}" fill="none" stroke="#d7fff7" stroke-width="2"
              stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>
        <circle cx="${lx.toFixed(1)}" cy="${ly.toFixed(1)}" r="3.5" fill="#d7fff7" vector-effect="non-scaling-stroke"/>
      </svg>`;
}

// ---------------------------------------------------------------- burn rate

function renderBurn() {
    const pts = state.history;
    $('burn-samples').textContent = pts.length;

    if (pts.length < 2) {
        $('burn-rate').textContent = '—';
        $('burn-proj').textContent = 'needs more samples';
        return;
    }

    // measure over the last hour of samples where possible
    const now = new Date(pts[pts.length - 1].ts).getTime();
    const window = pts.filter(p => now - new Date(p.ts).getTime() <= 60 * 60 * 1000);
    const seg = window.length >= 2 ? window : pts.slice(-2);

    const first = seg[0], last = seg[seg.length - 1];
    const hours = (new Date(last.ts).getTime() - new Date(first.ts).getTime()) / 3.6e6;
    if (hours <= 0) {
        $('burn-rate').textContent = '—';
        $('burn-proj').textContent = 'needs more samples';
        return;
    }

    const burn = (last.pct - first.pct) / hours;
    const rateEl = $('burn-rate');
    rateEl.textContent = `${burn >= 0 ? '' : '−'}${Math.abs(burn).toFixed(1)}%/hr`;
    rateEl.className = 'kv-val' + (burn >= 25 ? ' warn' : '');

    const projEl = $('burn-proj');
    projEl.className = 'kv-val';
    if (burn <= 0.2) {
        projEl.textContent = 'steady';
        return;
    }

    const hoursToCap = (100 - last.pct) / burn;
    const resetMs = state.usage && state.usage.resets_at
        ? new Date(state.usage.resets_at).getTime() - Date.now()
        : null;

    if (resetMs !== null && hoursToCap * 3.6e6 > resetMs) {
        projEl.textContent = 'safe until reset';
    } else {
        projEl.textContent = `cap in ~${humanDuration(hoursToCap * 3.6e6)}`;
        projEl.className = 'kv-val crit';
    }
}

// ---------------------------------------------------------------- usage cards

function renderUsage() {
    const u = state.usage;
    if (!u || !u.ok) {
        $('hero-num').innerHTML = '—<small>remaining</small>';
        $('hero-label').textContent = 'usage unavailable';
        $('chip-usage').textContent = 'offline';
        $('chip-usage').className = 'chip';
        return;
    }

    const used = u.utilization || 0;
    const remaining = Math.max(0, 100 - used);
    $('hero-num').innerHTML = `${Math.round(remaining)}<small>% remaining</small>`;
    $('hero-label').textContent = `${Math.round(used)}% of the 5-hour window used`;

    const sev = severityOf(used);
    const chip = $('chip-usage');
    chip.textContent = sev === 'ok' ? 'normal' : sev === 'warn' ? 'warning' : 'critical';
    chip.className = 'chip' + (sev === 'ok' ? ' on' : sev === 'warn' ? ' warn' : ' crit');

    renderLimits(u);
    renderChart();
    renderBurn();
}

function renderLimits(u) {
    const host = $('limits');
    const limits = (u.limits && u.limits.length)
        ? u.limits
        : [{ kind: 'session', percent: u.utilization, severity: 'normal', resets_at: u.resets_at, is_active: true }];

    const label = l => {
        if (l.kind === 'session') return 'Session <em>5-hour</em>';
        if (l.kind === 'weekly_all') return 'Weekly <em>all models</em>';
        if (l.kind === 'weekly_scoped') return `Weekly <em>${l.scope || 'scoped'}</em>`;
        return l.kind;
    };

    host.innerHTML = limits.map(l => {
        const pct = Math.max(0, Math.min(100, l.percent || 0));
        const sev = l.severity === 'critical' ? 'crit' : l.severity === 'warning' ? 'warn' : severityOf(pct);
        const cls = sev === 'crit' ? 'crit' : sev === 'warn' ? 'warn' : (l.is_active ? 'mint' : '');
        return `
          <div class="limit">
            <div class="limit-top">
              <span class="limit-name">${label(l)}</span>
              <span class="limit-pct">${Math.round(pct)}%</span>
            </div>
            <div class="bar"><i class="${cls}" style="width:${pct}%"></i></div>
          </div>`;
    }).join('');
}

// countdown ticks locally so it stays smooth between usage refreshes
function renderCountdown() {
    const u = state.usage;
    const el = $('reset-time');
    if (!u || !u.resets_at) { el.textContent = '—'; return; }
    const ms = new Date(u.resets_at).getTime() - Date.now();
    el.textContent = ms > 0 ? humanDuration(ms) : 'now';
}

// ---------------------------------------------------------------- agents

function renderAgents() {
    const host = $('agents');
    const running = state.agents.filter(a => a.running).length;
    $('agent-count').textContent = running;
    $('chip-agents').textContent = `${running}/${state.agents.length}`;
    $('chip-agents').className = running ? 'chip on' : 'chip';

    if (!state.agents.length) {
        host.innerHTML = '<div class="feed-empty">No agents found.</div>';
        return;
    }
    host.innerHTML = state.agents.map(a => `
      <div class="agent ${a.running ? 'on' : ''}">
        <i class="dot"></i>
        <div style="min-width:0">
          <div class="agent-name">${a.name || a.role}</div>
          <div class="agent-rig">${a.rig || 'town'}</div>
        </div>
      </div>`).join('');
}

// ---------------------------------------------------------------- feed

// map a feed event type onto the room that should react to it
function reactTo(ev) {
    const t = (ev.type || '').toLowerCase();
    let kind = 'spark', room = 'overseer';
    if (t.includes('escalation')) { kind = 'escalation'; room = 'overseer'; }
    else if (t.includes('mail')) { kind = 'mail'; room = 'overseer'; }
    else if (t.includes('merge') || t.includes('mq') || t.includes('branch')) { kind = 'spark'; room = 'refinery'; }
    else if (t.includes('bead') || t.includes('commit') || t.includes('issue')) { kind = 'spark'; room = 'mineshaft'; }
    const r = state.rooms[room];
    if (r) r.react(kind);
}

function pushFeed(ev) {
    state.feed.unshift(ev);
    if (state.feed.length > 40) state.feed.pop();
    renderFeed();
    reactTo(ev);
}

function renderFeed() {
    const host = $('feed');
    if (!state.feed.length) {
        host.innerHTML = '<div class="feed-empty">Waiting for events…</div>';
        return;
    }
    host.innerHTML = state.feed.map(ev => {
        const when = ev.ts ? clockText(new Date(ev.ts)) : '';
        const text = ev.summary || ev.type || '';
        return `
          <div class="feed-item">
            <div class="feed-time">${when}</div>
            <div class="feed-body">
              <div class="feed-kind">${(ev.type || 'event').replace(/_/g, ' ')}</div>
              <div class="feed-text">${escapeHTML(text)}</div>
            </div>
          </div>`;
    }).join('');
}

function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, c => (
        { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
    ));
}

// ---------------------------------------------------------------- state feed

function applyState(st) {
    if (!st) return;
    state.agents = st.agents || [];
    state.town = st.town || '';
    $('brand-town').textContent = state.town || 'town';
    const rigs = (st.rigs || []).join(', ');
    $('page-sub').textContent = rigs
        ? `${state.town} · ${rigs}`
        : state.town;
    renderAgents();
    applyAgentsToRooms();
}

function setConn(ok) {
    const el = $('conn');
    el.className = 'conn ' + (ok ? 'live' : 'down');
    $('conn-text').textContent = ok ? 'live' : 'reconnecting';
}

function connect() {
    const es = new EventSource('/api/events');
    es.addEventListener('state', e => {
        setConn(true);
        try { applyState(JSON.parse(e.data)); } catch (_) { /* ignore malformed frame */ }
    });
    es.addEventListener('feed', e => {
        try { pushFeed(JSON.parse(e.data)); } catch (_) { /* ignore malformed frame */ }
    });
    es.onopen = () => setConn(true);
    es.onerror = () => {
        setConn(false);
        es.close();
        setTimeout(connect, 4000);   // SSE has no built-in backoff we control
    };
}

// ---------------------------------------------------------------- polling

async function getJSON(url) {
    const r = await fetch(url, { cache: 'no-store' });
    if (!r.ok) throw new Error(`${url}: ${r.status}`);
    return r.json();
}

async function refreshUsage() {
    try {
        state.usage = await getJSON('/api/usage');
        renderUsage();
        renderCountdown();
    } catch (err) {
        console.error('usage refresh failed', err);
    }
}

async function refreshHistory() {
    try {
        state.history = await getJSON('/api/usage/history');
        renderChart();
        renderBurn();
    } catch (err) {
        console.error('history refresh failed', err);
    }
}

function tickClock() {
    const now = new Date();
    $('clock').textContent = clockText(now);
    $('greeting').textContent = greetingFor(now);
}

// ---------------------------------------------------------------- boot

async function boot() {
    tickClock();
    setConn(false);

    // paint what we can before the (async) WebGL init
    try { applyState(await getJSON('/api/state')); } catch (_) { /* SSE will fill in */ }
    await refreshUsage();
    await refreshHistory();

    await initRooms();
    connect();

    setInterval(renderCountdown, 1000);
    setInterval(tickClock, 20000);
    setInterval(refreshUsage, 60000);
    setInterval(refreshHistory, 60000);
}

boot();
