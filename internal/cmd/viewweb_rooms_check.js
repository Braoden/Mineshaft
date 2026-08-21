// viewweb_rooms_check.js — runnable check for room territory assignment.
//
//   node internal/cmd/viewweb_rooms_check.js
//
// Guards the thing that actually went wrong: three office agents all walked to
// the same desk coordinate and rendered on top of each other. The layout maths
// is pure, so it can be exercised without a browser — rooms.js only touches
// PIXI and window.MSArt at construction time, never at module load.

const fs = require('fs');
const path = require('path');
const assert = require('assert');

// Pixi stubs: just enough scene-graph surface for Actor to construct and
// animate. Nothing here renders — we only care which pose gets shown.
class FakeDisplay {
    constructor() { this.x = 0; this.y = 0; this.visible = true; this.scale = { x: 1, y: 1, set() {} }; this.anchor = { set() {} }; this.children = []; }
    addChild(...c) { this.children.push(...c); return c[0]; }
}
global.PIXI = {
    Container: FakeDisplay,
    Sprite: class extends FakeDisplay { constructor(t) { super(); this.texture = t; } },
    Texture: { from: () => ({ source: {} }) },
};

const POSE_NAMES = ['stand', 'walkA', 'walkB', 'carry', 'carryA', 'carryB',
                    'reach', 'stretch', 'workUp', 'workHit', 'sit', 'blink', 'sleep'];
global.window = {
    MSArt: {
        PALETTE: {}, PROPS: {},
        gridToCanvas: () => ({}),
        // each pose gets a distinct grid so identical-frame bugs are visible
        buildActor: () => Object.fromEntries(POSE_NAMES.map(n => [n, [[n]]])),
    },
};

const src = fs.readFileSync(path.join(__dirname, 'viewweb', 'rooms.js'), 'utf8');
new Function(src)();
const { Room, Actor } = global.window.MSRooms;
assert.ok(Room, 'rooms.js should export window.MSRooms.Room');
assert.ok(Actor, 'rooms.js should export window.MSRooms.Actor');

let checks = 0;
const ok = (cond, msg) => { assert.ok(cond, msg); checks++; };

// a Room stand-in carrying only what assignTerritories reads
function fakeRoom({ kind, worldW, anchors, zone, actors }) {
    const room = Object.create(Room.prototype);
    room.kind = kind;
    room.worldW = worldW;
    room.anchors = anchors;
    room.zone = zone;
    room.stationA = Math.round(worldW / 2 - 6);
    room.stationB = Math.round(worldW / 2 + 6);
    room.actors = actors.map(role => ({ role, x: room.stationA }));
    return room;
}

const SPRITE_W = 18;

function assertNoStacking(room, label) {
    const xs = room.actors.map(a => a.station).sort((a, b) => a - b);
    for (const a of room.actors) {
        ok(Number.isFinite(a.station), `${label}: ${a.role} has no station`);
        ok(Number.isFinite(a.homeA) && Number.isFinite(a.homeB),
            `${label}: ${a.role} has no idle band`);
        ok(a.homeB > a.homeA, `${label}: ${a.role} idle band is inverted`);
        ok(a.station >= a.homeA - 1 && a.station <= a.homeB + 1,
            `${label}: ${a.role} station sits outside its own idle band`);
    }
    for (let i = 1; i < xs.length; i++) {
        ok(xs[i] !== xs[i - 1], `${label}: two actors share station x=${xs[i]}`);
    }
    return xs;
}

// ---------------------------------------------------------------- office

{
    const W = 114;
    const third = W / 3;
    const dx = Math.round(third + (third - 38) / 2);
    const room = fakeRoom({
        kind: 'overseer',
        worldW: W,
        anchors: { witness: Math.round(third * 0.5), overseer: dx + 14, supervisor: W - 14 },
        zone: [10, W - 10],
        actors: ['overseer', 'supervisor', 'witness'],
    });
    room.assignTerritories();
    const xs = assertNoStacking(room, 'office');

    // the whole point: three agents must be visibly apart, not merely unequal
    for (let i = 1; i < xs.length; i++) {
        ok(xs[i] - xs[i - 1] >= SPRITE_W,
            `office: stations ${xs[i - 1]} and ${xs[i]} are ${xs[i] - xs[i - 1]}px apart, closer than one sprite`);
    }

    // each role should land on its own anchor, not a generic slice
    const byRole = Object.fromEntries(room.actors.map(a => [a.role, a.station]));
    ok(byRole.overseer === dx + 14, 'office: overseer should be anchored at the desk');
    ok(byRole.supervisor === W - 14, 'office: supervisor should be anchored at the pigeonholes');
    ok(byRole.witness === Math.round(third * 0.5), 'office: witness should be anchored at the bookshelf');
}

// the tight case: a ~1400px window yields a much narrower world than a wide
// monitor, and that is where three anchors are most likely to collide
for (const W of [78, 89, 100, 133]) {
    const third = W / 3;
    const deskW = Math.min(38, Math.round(third + 6));
    const dx = Math.round(third + (third - deskW) / 2);
    const room = fakeRoom({
        kind: 'overseer',
        worldW: W,
        anchors: { witness: Math.round(third * 0.5), overseer: dx + 14, supervisor: W - 14 },
        zone: [10, W - 10],
        actors: ['overseer', 'supervisor', 'witness'],
    });
    room.assignTerritories();
    const xs = assertNoStacking(room, `office@${W}`);
    for (let i = 1; i < xs.length; i++) {
        ok(xs[i] - xs[i - 1] >= SPRITE_W,
            `office@${W}: stations ${xs[i - 1]}/${xs[i]} only ${xs[i] - xs[i - 1]}px apart`);
    }
    // and nobody may be anchored off the edge of the room
    for (const a of room.actors) {
        ok(a.station > 6 && a.station < W - 6,
            `office@${W}: ${a.role} anchored at ${a.station}, off the room`);
    }
}

// a duplicate role must fall through to a slice instead of stealing the anchor
{
    const W = 114;
    const room = fakeRoom({
        kind: 'overseer',
        worldW: W,
        anchors: { overseer: 60 },
        zone: [10, W - 10],
        actors: ['overseer', 'overseer'],
    });
    room.assignTerritories();
    assertNoStacking(room, 'office/duplicate-role');
    ok(room.actors.filter(a => a.station === 60).length === 1,
        'office: only the first of a duplicated role may take the anchor');
}

// ---------------------------------------------------------------- mineshaft

for (const n of [1, 2, 3, 6]) {
    const W = 114;
    const room = fakeRoom({
        kind: 'mineshaft',
        worldW: W,
        anchors: undefined,
        zone: [30, W - 16],
        actors: Array(n).fill('miner'),
    });
    room.assignTerritories();
    const xs = assertNoStacking(room, `mineshaft/${n}`);

    // idle bands must not overlap, or miners drift into each other
    const bands = room.actors.map(a => [a.homeA, a.homeB]).sort((p, q) => p[0] - q[0]);
    for (let i = 1; i < bands.length; i++) {
        ok(bands[i][0] >= bands[i - 1][1],
            `mineshaft/${n}: idle bands [${bands[i - 1]}] and [${bands[i]}] overlap`);
    }
    ok(xs.length === n, `mineshaft/${n}: expected ${n} stations`);
}

// ---------------------------------------------------------------- edges

{
    const room = fakeRoom({ kind: 'refinery', worldW: 90, zone: [44, 76], actors: [] });
    room.assignTerritories();               // must not throw on an empty room
    ok(room.actors.length === 0, 'refinery: empty room stays empty');
}

// an actor stranded far outside its new territory should be pulled back in
{
    const room = fakeRoom({ kind: 'mineshaft', worldW: 114, zone: [30, 98], actors: ['miner'] });
    room.actors[0].x = -500;
    room.assignTerritories();
    ok(room.actors[0].x === room.actors[0].station,
        'a stranded actor should be snapped to its station');
}

// ---------------------------------------------------------------- walk cycles
//
// The regression this guards: a carrying clawd held ONE static pose, so it slid
// across the floor with frozen legs. It showed up as "the refinery sprite does
// not animate walking right", because the refinery's carrying leg happens to be
// the rightward one — the mine carries leftward, which masked the same bug.

function walkPoses({ carry, dir }) {
    const room = fakeRoom({ kind: 'refinery', worldW: 114, zone: [44, 100], actors: [] });
    room.floorY = 40;
    const a = new Actor(room, { id: 'x', role: 'refinery', running: true });
    a.x = dir > 0 ? 0 : 200;
    a.queue = [{ to: dir > 0 ? 200 : 0, carry }];
    const seen = new Set();
    for (let i = 0; i < 40; i++) { a.update(40); seen.add(a.pose); }
    return seen;
}

for (const dir of [1, -1]) {
    const label = dir > 0 ? 'right' : 'left';

    const plain = walkPoses({ carry: false, dir });
    ok(plain.size > 1, `walking ${label}: pose never changed — animation frozen`);
    ok(plain.has('walkA') && plain.has('walkB'),
        `walking ${label}: expected both walk frames, saw ${[...plain]}`);

    const hauling = walkPoses({ carry: true, dir });
    ok(hauling.size > 1, `carrying ${label}: pose never changed — animation frozen`);
    ok(hauling.has('carryA') && hauling.has('carryB'),
        `carrying ${label}: expected both carry frames, saw ${[...hauling]}`);
    ok(!hauling.has('carry'),
        `carrying ${label}: fell back to the static carry pose`);
}

// facing must not affect which frames play
{
    const r = walkPoses({ carry: true, dir: 1 });
    const l = walkPoses({ carry: true, dir: -1 });
    ok(r.size === l.size, 'carry animation differs by direction — it should not');
}

console.log(`rooms check: ${checks} assertions passed`);
