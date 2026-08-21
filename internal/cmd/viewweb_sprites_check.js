// viewweb_sprites_check.js — runnable check for the hand-authored pixel art.
//
//   node internal/cmd/viewweb_sprites_check.js
//
// Lives outside viewweb/ on purpose: that directory is embedded into the Go
// binary, and a test has no business shipping inside it.
//
// Sprite grids are hundreds of hand-placed coordinates, so the failure modes
// are silent: a typo'd palette key paints nothing, an off-by-one drops a limb,
// a bad rect writes outside the grid. None of that throws in a browser — it
// just renders wrong. This asserts the invariants instead.

const fs = require('fs');
const path = require('path');
const assert = require('assert');

// minimal DOM surface: sprites.js only needs createElement for rasterising
global.window = {};
global.document = {
    createElement: () => ({
        width: 0, height: 0,
        getContext: () => ({ fillStyle: '', fillRect() {} }),
    }),
};

const src = fs.readFileSync(path.join(__dirname, 'viewweb', 'sprites.js'), 'utf8');
new Function(src)();
const Art = global.window.MSArt;
assert.ok(Art, 'sprites.js should export window.MSArt');

let checks = 0;
const ok = (cond, msg) => { assert.ok(cond, msg); checks++; };

// every glyph used anywhere must exist in PALETTE, or it paints nothing
function assertGlyphs(grid, label) {
    for (let y = 0; y < grid.length; y++) {
        for (let x = 0; x < grid[y].length; x++) {
            const ch = grid[y][x];
            ok(ch in Art.PALETTE, `${label}: glyph '${ch}' at ${x},${y} is not in PALETTE`);
        }
    }
}

function countGlyph(grid, ch) {
    let n = 0;
    for (const row of grid) for (const c of row) if (c === ch) n++;
    return n;
}

function rectangular(grid, label) {
    const w = grid[0].length;
    for (const row of grid) ok(row.length === w, `${label}: ragged row (${row.length} vs ${w})`);
}

// ---------------------------------------------------------------- actors

const ROLES = Object.keys(Art.ROLE_KIT);
ok(ROLES.length === 5, `expected 5 roles, got ${ROLES.length}`);

for (const role of ROLES) {
    const poses = Art.buildActor(role);
    const names = Object.keys(poses);
    ok(names.length >= 9, `${role}: expected at least 9 poses, got ${names.length}`);

    for (const [name, g] of Object.entries(poses)) {
        const label = `${role}.${name}`;
        ok(g.length === 18, `${label}: height ${g.length}, want 18`);
        rectangular(g, label);
        ok(g[0].length === 18, `${label}: width ${g[0].length}, want 18`);
        assertGlyphs(g, label);

        // every pose must actually draw a clawd body
        ok(countGlyph(g, 'o') > 20, `${label}: only ${countGlyph(g, 'o')} body pixels — body missing?`);

        // headwear on every pose except sleep (the hat comes off)
        if (name !== 'sleep') {
            const crown = g.slice(0, 5).flat().filter(c => c !== '.').length;
            ok(crown > 0, `${label}: no headwear drawn in rows 0-4`);
        }

        // Eyes must survive the headwear overlay. Checked at their actual
        // coordinates rather than by counting dark pixels — the top hat is
        // drawn in the same colour and would mask a missing face.
        if (!['sleep', 'blink', 'stretch'].includes(name)) {
            const eyeAt = (xs) => {
                for (let y = 6; y <= 14; y++) for (const x of xs) if (g[y][x] === 'e') return true;
                return false;
            };
            ok(eyeAt([5, 6]), `${label}: left eye is covered by headwear`);
            ok(eyeAt([11, 12]), `${label}: right eye is covered by headwear`);
        }
    }

    // work poses must differ from each other, or the swing has no motion
    ok(JSON.stringify(poses.workUp) !== JSON.stringify(poses.workHit),
        `${role}: workUp and workHit are identical — no swing`);
    // walk frames must differ, or the walk cycle is static
    ok(JSON.stringify(poses.walkA) !== JSON.stringify(poses.walkB),
        `${role}: walkA and walkB are identical — no walk cycle`);
}

// each role must be visually distinguishable by its hat
const crowns = ROLES.map(r => JSON.stringify(Art.buildActor(r).stand.slice(0, 6)));
ok(new Set(crowns).size === ROLES.length, 'two roles share the same headwear silhouette');

// ---------------------------------------------------------------- props

for (const [name, make] of Object.entries(Art.PROPS)) {
    const p = make();
    ok(p && p.g && p.w && p.h, `prop ${name}: malformed return`);
    ok(p.g.length === p.h, `prop ${name}: declared h=${p.h} but grid has ${p.g.length} rows`);
    ok(p.g[0].length === p.w, `prop ${name}: declared w=${p.w} but grid has ${p.g[0].length} cols`);
    rectangular(p.g, `prop ${name}`);
    assertGlyphs(p.g, `prop ${name}`);
    const painted = p.g.flat().filter(c => c !== '.').length;
    ok(painted > 0, `prop ${name}: draws nothing`);
}

// the cart swap depends on both variants sharing a footprint
const cart = Art.PROPS.oreCart(), cartFull = Art.PROPS.oreCartFull();
ok(cart.w === cartFull.w && cart.h === cartFull.h, 'ore cart variants must share dimensions');
ok(JSON.stringify(cart.g) !== JSON.stringify(cartFull.g), 'ore cart variants are identical');

const lamp = Art.PROPS.lamp(), lampOff = Art.PROPS.lampOff();
ok(lamp.w === lampOff.w && lamp.h === lampOff.h, 'lamp variants must share dimensions');
ok(countGlyph(lamp.g, 'y') > 0, 'lit lamp should emit amber');
ok(countGlyph(lampOff.g, 'y') === 0, 'unlit lamp should emit no amber');

// clawd orange must be the only warm hue in the room props
for (const [name, make] of Object.entries(Art.PROPS)) {
    const g = make().g;
    ok(countGlyph(g, 'o') === 0 && countGlyph(g, 'O') === 0,
        `prop ${name}: uses clawd body orange — reserve it for the mascots`);
}

console.log(`sprites check: ${checks} assertions passed across ${ROLES.length} roles and ${Object.keys(Art.PROPS).length} props`);
