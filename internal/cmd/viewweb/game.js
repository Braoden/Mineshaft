// Mineshaft view: real-time pixel-art visualization of the town.
// Every agent is a Clawd. Data arrives over SSE from /api/events.
'use strict';

// ---------------------------------------------------------------- spritesheet

// Bake all sprites onto one canvas; record px rects for TileInfo lookup.
const SHEET_SIZE = 512;
const SPRITES = {}; // name -> {x, y, w, h}
const sheetDataURL = (() => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = SHEET_SIZE;
    const ctx = canvas.getContext('2d');
    let cx = 0, cy = 0, rowH = 0;
    for (const [name, rows] of Object.entries(SPRITE_DATA)) {
        const w = rows[0].length, h = rows.length;
        if (cx + w + 2 > SHEET_SIZE) { cx = 0; cy += rowH; rowH = 0; }
        rows.forEach((row, py) => {
            for (let px = 0; px < w; px++) {
                const c = SPRITE_PALETTE[row[px]];
                if (!c) continue;
                ctx.fillStyle = c;
                ctx.fillRect(cx + px, cy + py, 1, 1);
            }
        });
        SPRITES[name] = { x: cx, y: cy, w, h };
        cx += w + 2;
        rowH = Math.max(rowH, h + 2);
    }
    return canvas.toDataURL();
})();

function T(name) {
    const s = SPRITES[name];
    return new TileInfo(vec2(s.x, s.y), vec2(s.w, s.h));
}
// world size of a sprite at 16px per unit
function spriteSize(name) {
    const s = SPRITES[name];
    return vec2(s.w / 16, s.h / 16);
}

// ---------------------------------------------------------------- constants

const FOOT = 18 / 32;      // standing y for a 16x18 clawd centered on pos
const WALK_SPEED = 2.8;    // units/sec
const HQ_X = -7, BUNK_X = -16, HQ_DOOR = vec2(-7, FOOT);
const RIG_X0 = 2, RIG_W = 20;
const CAM_Y = 3.4, VIEW_H = 13;

const ROLE_ACCESSORY = {
    overseer: 'hat_top', supervisor: 'hat_cap', witness: 'lantern',
    refinery: 'apron', crew: 'hat_hard_g', miner: 'hat_hard',
};
const ROLE_LABEL_COLOR = {
    overseer: '#f2c94c', supervisor: '#4a78b5', witness: '#5ad0d0',
    refinery: '#9a9aff', crew: '#5aa053', miner: '#e8e4d8',
};

// ---------------------------------------------------------------- world state

const agents = new Map(); // id -> Agent
let rigNames = [];
let rigLayout = new Map(); // name -> {towerX, mineX, refX, benchX}
let townName = '';
let firstSnapshot = true;
let hovered = null;
let pinned = null;   // agent shown in the detail panel
let camX = 0, camMin = -20, camMax = 10;
let dragging = false, dragDist = 0, lastScreenX = 0;

function recomputeLayout() {
    rigLayout = new Map();
    rigNames.forEach((name, i) => {
        const base = RIG_X0 + i * RIG_W;
        rigLayout.set(name, {
            towerX: base + 1.5, mineX: base + 7,
            refX: base + 14, benchX: base + 17.5,
        });
    });
    camMax = Math.max(4, RIG_X0 + rigNames.length * RIG_W - 8);
}

function rigOf(agent) {
    return rigLayout.get(agent.rig) || rigLayout.values().next().value;
}

// station position for a running agent; slot disambiguates miners/crew
function stationFor(a) {
    const rl = rigOf(a);
    switch (a.role) {
        case 'overseer':   return vec2(HQ_X + 3, FOOT);
        case 'supervisor': return vec2(HQ_X - 3.2, FOOT);
        case 'witness':    return rl ? vec2(rl.towerX + 1.2, FOOT) : HQ_DOOR.copy();
        case 'refinery':   return rl ? vec2(rl.refX - 1.8, FOOT) : HQ_DOOR.copy();
        case 'crew':       return rl ? vec2(rl.benchX + a.slot * 1.2, FOOT) : HQ_DOOR.copy();
        case 'miner':      return rl ? vec2(rl.mineX - 2.4 + a.slot * 1.2, FOOT) : HQ_DOOR.copy();
    }
    return HQ_DOOR.copy();
}

function bunkFor(a) {
    // stable-ish slot from id hash
    let h = 0;
    for (const ch of a.id) h = (h * 31 + ch.charCodeAt(0)) | 0;
    return vec2(BUNK_X - 4 + (Math.abs(h) % 7) * 1.3, FOOT);
}

// ---------------------------------------------------------------- effects

function burst(pos, colorA, colorB, count = 14, speed = 0.08) {
    new ParticleEmitter(
        pos, 0, 0.2, 0.08, count / 0.08, PI, undefined,
        colorA, colorB, colorA.scale(1, 0), colorB.scale(1, 0),
        0.5, 0.14, 0.02, speed, 0, 0.95, 1, 0, PI, 0.3, 0.4);
}
const sparkle = pos => burst(pos, hsl(0.13, 1, 0.7), hsl(0.1, 1, 0.6));
const puff = pos => burst(pos, hsl(0, 0, 0.8), hsl(0, 0, 0.5));
const debris = pos => burst(pos, hsl(0.08, 0.3, 0.4), hsl(0, 0, 0.55), 8, 0.06);

function heart(pos) {
    new ParticleEmitter(
        pos.add(vec2(0, 0.8)), 0, 0.1, 0.1, 30, 0.5, T('heart'),
        WHITE, WHITE, CLEAR_WHITE, CLEAR_WHITE,
        0.9, 0.4, 0.2, 0.04, 0, 0.98, 1, 0, 0.2, 0.3, 0.5);
}

function smoke(pos) {
    new ParticleEmitter(
        pos, 0, 0.15, 0.05, 40, 0.4,  undefined,
        hsl(0, 0, 0.55, 0.6), hsl(0, 0, 0.4, 0.6), hsl(0, 0, 0.5, 0), hsl(0, 0, 0.35, 0),
        1.6, 0.2, 0.5, 0.025, 0, 0.99, 1, 0, PI / 8, 0.4, 0.6);
}

// bead thrown from one spot to another (sling events)
class Bead extends EngineObject {
    constructor(from, to) {
        super(from.copy(), vec2(5 / 16), T('bead'));
        this.from = from.copy(); this.to = to.copy();
        this.t = 0; this.renderOrder = 20;
    }
    update() {
        this.t += timeDelta / 1.2;
        if (this.t >= 1) { sparkle(this.to); this.destroy(); return; }
        const p = this.t;
        this.pos = vec2(
            lerp(this.from.x, this.to.x, p),
            lerp(this.from.y, this.to.y, p) + 3 * p * (1 - p)); // parabolic arc
    }
}

// ---------------------------------------------------------------- agents

class Agent extends EngineObject {
    constructor(info) {
        super(vec2(0, FOOT), vec2(1, 18 / 16));
        this.id = info.id; this.role = info.role;
        this.rig = info.rig || ''; this.name = info.name;
        this.running = null; this.slot = 0;
        this.target = null; this.departing = false;
        this.phase = rand(10); this.hopT = 0;
        this.bubble = null; this.recent = [];
        this.facing = 1; this.patrolT = rand(4);
        this.renderOrder = 10;
    }

    setRunning(running, immediate) {
        const dest = running ? stationFor(this) : bunkFor(this);
        if (this.running === running && this.target === null
            && Math.abs(this.pos.x - dest.x) < 0.6) return;
        this.running = running;
        if (immediate) { this.pos = dest.copy(); this.target = null; }
        else if (!this.target || Math.abs(this.target.x - dest.x) > 0.6)
            this.target = dest;
    }

    depart() {
        this.departing = true;
        this.target = HQ_DOOR.copy();
    }

    say(text) {
        this.bubble = { text: text.length > 48 ? text.slice(0, 46) + '…' : text, until: time + 6 };
    }

    addEvent(ev) {
        this.recent.unshift(ev);
        if (this.recent.length > 8) this.recent.pop();
    }

    get walking() { return !!this.target; }
    get sleeping() { return !this.running && !this.target; }

    update() {
        if (this.hopT > 0) this.hopT -= timeDelta;

        if (this.target) {
            const dx = this.target.x - this.pos.x;
            this.facing = dx < 0 ? -1 : 1;
            const step = WALK_SPEED * timeDelta;
            if (Math.abs(dx) <= step) {
                this.pos.x = this.target.x;
                this.target = null;
                if (this.departing) { puff(this.pos); this.destroy(); }
            } else
                this.pos.x += Math.sign(dx) * step;
        } else if (this.running) {
            // idle-at-station behaviors
            if (this.role === 'witness') {
                // patrol around the tower
                this.patrolT -= timeDelta;
                if (this.patrolT <= 0) {
                    this.patrolT = rand(6, 3);
                    const rl = rigOf(this);
                    if (rl) this.target = vec2(rl.towerX + rand(4, -1), FOOT);
                }
            } else if (this.role === 'miner' || this.role === 'crew') {
                // occasional debris while mining
                if (rand() < timeDelta / 1.8)
                    debris(this.pos.add(vec2(this.facing * 0.8, 0)));
            } else if (this.role === 'overseer' && rand() < timeDelta / 9) {
                this.target = vec2(HQ_X + rand(6, 1), FOOT);
            }
        }
    }

    frameName() {
        const t = time + this.phase;
        if (this.sleeping) return 'clawd_sleep';
        if (this.walking) return (t * 6 | 0) % 2 ? 'clawd_walk1' : 'clawd_walk2';
        if (this.running && (this.role === 'miner' || this.role === 'crew'))
            return (t * 3 | 0) % 2 ? 'clawd_mine1' : 'clawd_mine2';
        if (this.running && this.role === 'refinery')
            return (t * 2 | 0) % 2 ? 'clawd_idle' : 'clawd_mine2';
        return (t % 4) < 0.2 ? 'clawd_blink' : 'clawd_idle';
    }

    render() {
        const hop = this.hopT > 0 ? Math.sin(this.hopT * PI / 0.4) * 0.35 : 0;
        const bob = this.walking ? Math.abs(Math.sin(time * 9 + this.phase)) * 0.06 : 0;
        const pos = this.pos.add(vec2(0, hop + bob));
        const mirror = this.facing < 0;
        const frame = this.frameName();
        drawTile(pos, spriteSize(frame), T(frame), WHITE, 0, mirror);
        const acc = ROLE_ACCESSORY[this.role];
        if (acc && frame !== 'clawd_sleep')
            drawTile(pos, spriteSize(acc), T(acc), WHITE, 0, mirror);

        // name tag
        drawText(this.name, pos.add(vec2(0, -0.85)), 0.32,
            new Color().setHex(ROLE_LABEL_COLOR[this.role] || '#fff'), 0.05, BLACK);

        if (this.sleeping) {
            const zt = (time + this.phase) % 2;
            drawText('z', this.pos.add(vec2(0.7 + zt * 0.2, 0.9 + zt * 0.5)),
                0.3 + zt * 0.15, hsl(0, 0, 1, 1 - zt / 2));
        }

        if (this.bubble) {
            if (time > this.bubble.until) this.bubble = null;
            else {
                const w = this.bubble.text.length * 0.19 + 0.4;
                const bp = pos.add(vec2(0, 1.6));
                drawRect(bp, vec2(w, 0.62), hsl(0.1, 0.3, 0.95));
                drawRect(bp.add(vec2(0, -0.36)), vec2(0.18, 0.18), hsl(0.1, 0.3, 0.95));
                drawText(this.bubble.text, bp, 0.34, hsl(0, 0, 0.1));
            }
        }
    }
}

// ---------------------------------------------------------------- data feed

function applyState(st) {
    townName = st.town;
    document.getElementById('header').childNodes[0].textContent =
        `⛏ MINESHAFT — ${st.town} `;
    rigNames = st.rigs || [];
    recomputeLayout();

    // slot indexes for miners/crew, per rig
    const slots = {};
    const seen = new Set();
    for (const info of st.agents) {
        seen.add(info.id);
        let a = agents.get(info.id);
        if (!a) {
            a = new Agent(info);
            agents.set(info.id, a);
            a.pos = firstSnapshot
                ? (info.running ? stationFor(a) : bunkFor(a)).copy()
                : HQ_DOOR.copy(); // newcomers walk in from HQ
        }
        if (info.role === 'miner' || info.role === 'crew') {
            const key = info.rig + '/' + info.role;
            a.slot = slots[key] = (slots[key] ?? -1) + 1;
        }
        a.setRunning(info.running, firstSnapshot);
    }
    for (const [id, a] of agents)
        if (!seen.has(id)) { agents.delete(id); a.depart(); }
    firstSnapshot = false;
}

function findAgent(s) {
    if (!s) return null;
    s = String(s).toLowerCase();
    for (const a of agents.values())
        if (a.id.toLowerCase() === s || a.name.toLowerCase() === s) return a;
    for (const a of agents.values()) {
        const id = a.id.toLowerCase(), name = a.name.toLowerCase();
        if (s.includes(name) || id.includes(s) || s.includes(id)) return a;
    }
    return null;
}

function handleFeed(ev) {
    const tstr = (ev.ts || '').slice(11, 16);
    const line = document.createElement('div');
    line.textContent = `[${tstr}] ${ev.summary || ev.type}`;
    const ticker = document.getElementById('ticker');
    ticker.prepend(line);
    while (ticker.children.length > 4) ticker.lastChild.remove();

    const actor = findAgent(ev.actor) ||
        findAgent(ev.payload && (ev.payload.agent || ev.payload.session));
    if (actor) actor.addEvent(`[${tstr}] ${ev.summary || ev.type}`);

    if (ev.type === 'sling') {
        const target = findAgent((ev.payload && ev.payload.target || '').split('/')[0]);
        const from = actor ? actor.pos.copy() : HQ_DOOR.add(vec2(0, 1));
        const to = target ? target.pos.copy() : from.add(vec2(4, 0));
        new Bead(from, to);
        if (actor) actor.say(ev.summary || 'sling!');
    } else if (ev.type === 'session_death') {
        if (actor) {
            puff(actor.pos);
            if (actor.role === 'miner' || actor.role === 'crew') {
                agents.delete(actor.id);
                actor.destroy();
            }
        }
    } else if (actor)
        actor.say(ev.summary || ev.type);
}

function connect() {
    const es = new EventSource('/api/events');
    const conn = document.getElementById('conn');
    es.onopen = () => conn.classList.remove('down');
    es.onerror = () => conn.classList.add('down'); // EventSource auto-reconnects
    es.addEventListener('state', e => applyState(JSON.parse(e.data)));
    es.addEventListener('feed', e => {
        try { handleFeed(JSON.parse(e.data)); } catch { /* ignore bad lines */ }
    });
}

// ---------------------------------------------------------------- hud

function screenScale() {
    const r = mainCanvas.getBoundingClientRect();
    return r.width / mainCanvas.width;
}

function updateTooltip() {
    const el = document.getElementById('tooltip');
    if (!hovered || dragging) { el.style.display = 'none'; return; }
    const st = hovered.sleeping ? 'sleeping' : hovered.walking ? 'walking' : 'working';
    el.innerHTML = `<span class="t-name"></span><br>`;
    el.querySelector('.t-name').textContent = `${hovered.name} (${hovered.role})`;
    el.append(hovered.rig ? `rig: ${hovered.rig} — ${st}` : st);
    if (hovered.recent[0]) {
        el.append(document.createElement('br'));
        el.append(hovered.recent[0]);
    }
    const s = screenScale();
    el.style.left = mousePosScreen.x * s + 16 + 'px';
    el.style.top = mousePosScreen.y * s + 8 + 'px';
    el.style.display = 'block';
}

function showPanel(a) {
    pinned = a;
    const el = document.getElementById('panel');
    el.innerHTML = '<span class="p-close">x</span><h3></h3><div class="p-meta"></div><ul></ul>';
    el.querySelector('h3').textContent = `${a.name} (${a.role})`;
    el.querySelector('.p-meta').textContent =
        (a.rig ? `rig: ${a.rig} ` : '') + `session: ${a.id}`;
    const ul = el.querySelector('ul');
    if (!a.recent.length) {
        const li = document.createElement('li');
        li.textContent = 'no recent events';
        ul.append(li);
    }
    for (const ev of a.recent) {
        const li = document.createElement('li');
        li.textContent = ev;
        ul.append(li);
    }
    el.querySelector('.p-close').onclick = hidePanel;
    el.style.display = 'block';
}
function hidePanel() {
    pinned = null;
    document.getElementById('panel').style.display = 'none';
}

// ---------------------------------------------------------------- engine

function gameInit() {
    setTilesPixelated(true);
    setCanvasPixelated(true);
    setFontDefault('Courier New');
    camX = 0;
    connect();
}

function gameUpdate() {
    // camera: fit height, clamp pan
    setCameraScale(mainCanvasSize.y / VIEW_H);
    camX = clamp(camX, camMin, camMax);
    setCameraPos(vec2(camX, CAM_Y));

    // hover
    hovered = null;
    let best = 0.9;
    for (const a of agents.values()) {
        const d = mousePos.distance(a.pos);
        if (d < best) { best = d; hovered = a; }
    }

    // drag to pan / click
    if (mouseWasPressed(0)) { dragDist = 0; lastScreenX = mousePosScreen.x; }
    if (mouseIsDown(0)) {
        const dx = mousePosScreen.x - lastScreenX;
        dragDist += Math.abs(dx);
        if (dragDist > 8) dragging = true;
        if (dragging) camX -= dx / cameraScale;
        lastScreenX = mousePosScreen.x;
    }
    if (mouseWasReleased(0)) {
        if (!dragging) {
            if (hovered) {
                hovered.hopT = 0.4;
                heart(hovered.pos);
                showPanel(hovered);
            } else {
                sparkle(mousePos);
                if (pinned) hidePanel();
            }
        }
        dragging = false;
    }

    updateTooltip();
}

function gameUpdatePost() {}

function drawBuilding(name, x, label, labelColor) {
    const size = spriteSize(name);
    drawTile(vec2(x, size.y / 2), size, T(name));
    if (label)
        drawText(label, vec2(x, size.y + 0.35), 0.38,
            labelColor || hsl(0, 0, 0.85), 0.05, BLACK);
}

function gameRender() {
    // sky
    drawRectGradient(vec2(camX, CAM_Y + 2), vec2(200, VIEW_H + 8),
        new Color().setHex('#241b3d'), new Color().setHex('#6b4a63'));
    // ground
    drawRect(vec2(camX, -3), vec2(200, 6), new Color().setHex('#4a3628'));
    drawRect(vec2(camX, 0.06), vec2(200, 0.24), new Color().setHex('#3d7038'));
    // pebbles (deterministic scatter)
    for (let i = 0; i < 90; i++) {
        const x = -34 + i * 1.63 + Math.sin(i * 7.3) * 0.7;
        drawRect(vec2(x, -0.7 - (i % 4) * 0.55), vec2(0.14, 0.1), hsl(0, 0, 0.45, 0.5));
    }
    // clouds
    for (let i = 0; i < 5; i++) {
        const span = camMax - camMin + 60;
        const x = camMin - 30 + ((i * 37 + time * (0.25 + i * 0.06)) % span);
        drawTile(vec2(x, 6.8 + (i % 3) * 1.1), spriteSize('cloud').scale(1.4),
            T('cloud'), hsl(0, 0, 1, 0.75));
    }

    // town buildings
    drawBuilding('bunkhouse', BUNK_X, 'bunkhouse');
    drawBuilding('hq', HQ_X, 'HQ', hsl(0.13, 0.8, 0.7));

    // rig blocks
    for (const name of rigNames) {
        const rl = rigLayout.get(name);
        drawBuilding('tower', rl.towerX);
        drawBuilding('mine', rl.mineX, name, hsl(0.13, 0.8, 0.7));
        drawBuilding('refinery', rl.refX);
        // decorations
        drawTile(vec2(rl.mineX + 4.5, spriteSize('tree').y / 2), spriteSize('tree'), T('tree'));
        drawTile(vec2(rl.towerX - 1.6, spriteSize('rock').y / 2), spriteSize('rock'), T('rock'));

        // refinery smoke while running
        const ref = [...agents.values()].find(a => a.rig === name && a.role === 'refinery');
        if (ref && ref.running && rand() < timeDelta / 0.5)
            smoke(vec2(rl.refX - 0.4, spriteSize('refinery').y + 0.2));
    }
    // town trees
    drawTile(vec2(BUNK_X + 5, spriteSize('tree').y / 2), spriteSize('tree'), T('tree'));
    drawTile(vec2(HQ_X - 5.5, spriteSize('rock').y / 2), spriteSize('rock'), T('rock'));
}

function gameRenderPost() {
    if (hovered && !dragging)
        drawRect(hovered.pos.add(vec2(0, -0.68)), vec2(1.3, 0.08), hsl(0.13, 1, 0.7, 0.8));
}

engineInit(gameInit, gameUpdate, gameUpdatePost, gameRender, gameRenderPost, [sheetDataURL]);
