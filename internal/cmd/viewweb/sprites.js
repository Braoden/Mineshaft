// Pixel art data for the Mineshaft view. Pure data, no engine deps
// (validated standalone under node by the build check).
//
// Clawds are generated at exactly 2x the official 9x7 mascot reference
// (rounded wide body, two square eyes, three legs, #d9714f). Buildings
// are painted programmatically (cutaway/dollhouse style); interior
// positions are exported via BUILD_META (all in sprite pixels).
'use strict';

const SPRITE_PALETTE = {
    o: '#d9714f', // clawd body (official reference color)
    O: '#b3552f', // body shade
    k: '#000000', // eyes / dark details
    w: '#ffffff', // white
    y: '#f2c94c', // yellow
    Y: '#c9992a', // yellow shade
    g: '#5aa053', // green
    G: '#3d7038', // green shade
    b: '#4a78b5', // blue
    B: '#2f4f7a', // blue shade
    n: '#8a5a33', // wood
    N: '#5f3d22', // wood dark
    s: '#9a9a9a', // stone
    S: '#6e6e6e', // stone dark
    r: '#b5482f', // red
    d: '#241813', // dark opening / shaft
    D: '#38291d', // interior back wall (cutaway)
    c: '#e8e4d8', // cloud / paper / mattress
    m: '#7a7466', // metal
};

// ------------------------------------------------------------ paint helpers

function makeGrid(w, h) {
    return Array.from({ length: h }, () => Array(w).fill('.'));
}
function rect(g, x, y, w, h, ch) {
    if (x < 0 || y < 0 || y + h > g.length || x + w > g[0].length)
        throw new Error(`rect out of bounds: ${x},${y} ${w}x${h} in ${g[0].length}x${g.length}`);
    for (let j = y; j < y + h; j++)
        for (let i = x; i < x + w; i++)
            g[j][i] = ch;
}
function gridRows(g) { return g.map(r => r.join('')); }

// ------------------------------------------------------------ clawd frames

// 18x18: 4 rows of headroom for tools, body rows 4-13, legs rows 14-17.
// Every frame is the 9x7 reference scaled 2x.
function clawdBody(eyes) {
    const g = makeGrid(18, 18);
    rect(g, 4, 4, 10, 2, 'o');   // ref row 0: x2-6
    rect(g, 2, 6, 14, 2, 'o');   // ref row 1: x1-7
    rect(g, 0, 8, 18, 4, 'o');   // ref rows 2-3: full width
    rect(g, 2, 12, 14, 2, 'o');  // ref row 4: x1-7
    if (eyes) {
        rect(g, 4, 8, 2, 2, 'k');  // ref eye at x2,y2
        rect(g, 10, 8, 2, 2, 'k'); // ref eye at x5,y2
    }
    return g;
}

// legs at ref x2,4,6; mode 'all' | 'walk1' (middle up) | 'walk2' (outer up)
function clawdLegs(g, mode) {
    [4, 8, 12].forEach((x, i) => {
        const short = mode === 'walk1' ? i === 1 : mode === 'walk2' ? i !== 1 : false;
        rect(g, x, 14, 2, short ? 2 : 3, 'o');
        if (!short) rect(g, x, 16, 2, 2, 'O');
    });
}

function makeClawd(mode, eyes = true, tool = null) {
    const g = clawdBody(eyes);
    clawdLegs(g, mode);
    if (tool === 'up') {          // pickaxe raised overhead
        rect(g, 6, 0, 10, 2, 's');
        rect(g, 10, 2, 2, 2, 'n');
    } else if (tool === 'down') { // pickaxe swung down-left
        rect(g, 2, 9, 2, 2, 'n');
        rect(g, 1, 11, 2, 2, 'n');
        rect(g, 0, 13, 2, 4, 's');
    }
    return gridRows(g);
}

function makeClawdSleep() {
    const g = makeGrid(18, 18);
    rect(g, 1, 12, 16, 5, 'o');   // lying flat
    rect(g, 1, 16, 16, 1, 'O');
    rect(g, 4, 13, 2, 1, 'k');    // closed eyes
    rect(g, 10, 13, 2, 1, 'k');
    return gridRows(g);
}

// accessories, 18x18, body top = row 4
function makeAccessory(kind) {
    const g = makeGrid(18, 18);
    switch (kind) {
        case 'hat_hard':   rect(g, 5, 0, 8, 3, 'y'); rect(g, 4, 3, 10, 2, 'Y'); break;
        case 'hat_hard_g': rect(g, 5, 0, 8, 3, 'g'); rect(g, 4, 3, 10, 2, 'G'); break;
        case 'hat_top':    rect(g, 5, 0, 8, 3, 'k'); rect(g, 3, 3, 12, 2, 'k'); break;
        case 'hat_cap':    rect(g, 5, 0, 8, 3, 'b'); rect(g, 9, 3, 7, 2, 'B'); break;
        case 'lantern':
            rect(g, 0, 10, 3, 1, 'm'); rect(g, 0, 11, 3, 2, 'y'); rect(g, 0, 13, 3, 1, 'm');
            break;
        case 'apron':      rect(g, 4, 9, 10, 4, 'b'); rect(g, 5, 12, 8, 1, 'B'); break;
    }
    return gridRows(g);
}

// ------------------------------------------------------------ hand sprites

const HAND_SPRITES = {

tree: [
'....gggg....',
'..gggggggg..',
'.gggGGgggggg',
'.ggggggGGgg.',
'..gGGgggggg.',
'...gggggg...',
'.....nn.....',
'.....nn.....',
'.....nn.....',
'....nnnn....',
],

rock: [
'...sss..',
'..sssss.',
'.sssSSss',
'ssSSssss',
],

cloud: [
'.....cccc.........',
'...cccccccc..cc...',
'..cccccccccccccc..',
'.cccccccccccccccc.',
'..cccccccccccccc..',
],

moon: [
'...cccc..',
'..cccccc.',
'.ccmccccc',
'.cccccmcc',
'.cccccccc',
'.cmcccccc',
'..cccccc.',
'...cccc..',
],

bead: [
'.oo.',
'oooo',
'oooo',
'.oo.',
],

heart: [
'.rr.rr.',
'rrrrrrr',
'rrrrrrr',
'.rrrrr.',
'..rrr..',
'...r...',
],

bench: [
'..kk................',
'..kkkk..mm..........',
'.kkkkk..mm..........',
'nnnnnnnnnnnnnnnnnnnn',
'nnnnnnnnnnnnnnnnnnnn',
'.NN..............NN.',
'.NN..............NN.',
'.NN..............NN.',
],
};

// ------------------------------------------------------------ buildings

// HQ: 64x76. Cutaway first floor (supervisor desk), tower on the right
// with a balcony platform jutting left (overseer paces there).
function buildHQ() {
    const g = makeGrid(64, 76);
    // first floor
    rect(g, 0, 52, 64, 22, 'D');   // interior
    rect(g, 0, 74, 64, 2, 'N');    // floor slab
    rect(g, 0, 52, 2, 22, 'n');    // left wall
    rect(g, 62, 52, 2, 22, 'n');   // right wall
    rect(g, 0, 58, 2, 16, '.');    // door opening (left)
    // desk with papers
    rect(g, 40, 66, 14, 2, 'n');
    rect(g, 41, 68, 2, 7, 'n');
    rect(g, 51, 68, 2, 7, 'n');
    rect(g, 43, 63, 6, 3, 'c');
    // wall clock
    rect(g, 20, 58, 4, 4, 'c');
    // first-floor ceiling
    rect(g, 0, 48, 64, 4, 'N');
    // tower (right side)
    rect(g, 36, 8, 28, 40, 'D');   // interior
    rect(g, 36, 8, 2, 40, 'n');    // tower left wall
    rect(g, 62, 8, 2, 40, 'n');    // tower right wall
    rect(g, 36, 34, 2, 12, '.');   // balcony door opening
    rect(g, 44, 18, 10, 10, 'd');  // window
    rect(g, 44, 18, 10, 1, 'n');
    rect(g, 44, 27, 10, 1, 'n');
    // tower roof + flag
    rect(g, 34, 4, 30, 4, 'N');
    rect(g, 48, 0, 2, 4, 'k');
    rect(g, 50, 0, 6, 3, 'r');
    // balcony platform + railing
    rect(g, 6, 44, 32, 4, 'n');    // platform (walk level = row 44)
    rect(g, 6, 32, 30, 2, 'n');    // top rail
    rect(g, 6, 34, 2, 10, 'n');    // railing posts
    rect(g, 16, 34, 2, 10, 'n');
    rect(g, 26, 34, 2, 10, 'n');
    return gridRows(g);
}

// Bunkhouse: 88x56 cutaway with double-decker bunks (2 levels x 4 beds).
function buildBunkhouse() {
    const g = makeGrid(88, 56);
    rect(g, 0, 10, 88, 44, 'D');   // interior
    rect(g, 0, 54, 88, 2, 'N');    // floor
    rect(g, 0, 10, 2, 44, 'n');    // left wall
    rect(g, 86, 10, 2, 44, 'n');   // right wall
    rect(g, 86, 40, 2, 14, '.');   // door opening (right)
    rect(g, 0, 0, 88, 8, 'n');     // roof
    rect(g, 0, 8, 88, 2, 'N');
    // upper level platform + ladder
    rect(g, 2, 30, 78, 2, 'n');
    rect(g, 80, 30, 2, 24, 'N');
    for (let y = 33; y < 54; y += 4) rect(g, 79, y, 4, 1, 'n');
    // beds on both levels: [level floor row for legs, mattress top row]
    for (const [legRow, topRow] of [[52, 49], [28, 25]]) {
        for (const cx of [13, 33, 53, 73]) {
            rect(g, cx - 7, legRow, 2, 2, 'N');       // legs
            rect(g, cx + 5, legRow, 2, 2, 'N');
            rect(g, cx - 7, topRow, 14, 3, 'c');      // mattress
            rect(g, cx - 7, topRow, 4, 2, 'w');       // pillow
            rect(g, cx - 1, topRow - 1, 8, 4, 'b');   // blanket
        }
    }
    return gridRows(g);
}

// Mineshaft headframe: 48x44, open shaft hole at the bottom center.
function buildMine() {
    const g = makeGrid(48, 44);
    // legs of the headframe
    rect(g, 4, 8, 4, 36, 'n');
    rect(g, 40, 8, 4, 36, 'n');
    rect(g, 8, 16, 32, 3, 'n');    // crossbar
    rect(g, 8, 30, 32, 3, 'n');    // lower crossbar
    // wheel house at top
    rect(g, 16, 0, 16, 10, 'N');
    rect(g, 20, 2, 8, 8, 's');
    rect(g, 23, 4, 2, 4, 'S');     // wheel hub
    // shaft opening below (connects to the underground shaft)
    rect(g, 18, 33, 12, 11, 'd');
    rect(g, 16, 33, 2, 11, 'N');   // shaft frame
    rect(g, 30, 33, 2, 11, 'N');
    return gridRows(g);
}

// Refinery: 72x60 industrial cutaway - furnace, pipes, tank, chimneys.
function buildRefinery() {
    const g = makeGrid(72, 60);
    // chimneys first (behind the hall roofline)
    rect(g, 46, 2, 12, 26, 's');
    rect(g, 44, 0, 16, 3, 'S');    // cap
    rect(g, 46, 8, 12, 2, 'S');    // bands
    rect(g, 46, 16, 12, 2, 'S');
    rect(g, 28, 12, 8, 16, 's');   // small chimney
    rect(g, 27, 10, 10, 2, 'S');
    // main hall
    rect(g, 0, 28, 72, 28, 'D');   // interior
    rect(g, 0, 56, 72, 4, 'S');    // floor slab
    rect(g, 0, 24, 72, 4, 'S');    // roof band
    rect(g, 0, 28, 2, 28, 's');    // left wall
    rect(g, 70, 28, 2, 28, 's');   // right wall
    rect(g, 70, 42, 2, 14, '.');   // door opening (right)
    // furnace with glow
    rect(g, 6, 36, 20, 20, 's');
    rect(g, 10, 42, 12, 10, 'd');
    rect(g, 12, 45, 8, 6, 'r');
    rect(g, 14, 47, 4, 3, 'y');
    rect(g, 6, 36, 20, 2, 'S');
    // pipe from furnace to tank
    rect(g, 26, 32, 26, 3, 'm');
    rect(g, 49, 32, 3, 10, 'm');
    // tank with rivets
    rect(g, 44, 42, 20, 14, 'm');
    rect(g, 44, 42, 20, 2, 'S');
    rect(g, 46, 48, 2, 2, 'S');
    rect(g, 60, 48, 2, 2, 'S');
    return gridRows(g);
}

// Witness watchtower: 24x56 with cabin, platform, ladder, lamp.
function buildTower() {
    const g = makeGrid(24, 56);
    // legs + cross brace
    rect(g, 2, 20, 3, 36, 'n');
    rect(g, 19, 20, 3, 36, 'n');
    rect(g, 4, 34, 16, 2, 'n');
    // ladder
    rect(g, 11, 20, 2, 36, 'N');
    for (let y = 22; y < 54; y += 4) rect(g, 9, y, 6, 1, 'n');
    // platform + cabin
    rect(g, 0, 16, 24, 4, 'n');
    rect(g, 2, 4, 20, 12, 'D');
    rect(g, 2, 4, 2, 12, 'n');
    rect(g, 20, 4, 2, 12, 'n');
    rect(g, 0, 0, 24, 4, 'N');     // roof
    rect(g, 8, 7, 8, 6, 'd');      // window
    rect(g, 10, 18, 4, 4, 'y');    // lamp under platform
    return gridRows(g);
}

const SPRITE_DATA = {
    clawd_idle: makeClawd('all', true),
    clawd_blink: makeClawd('all', false),
    clawd_walk1: makeClawd('walk1', true),
    clawd_walk2: makeClawd('walk2', true),
    clawd_mine1: makeClawd('all', true, 'up'),
    clawd_mine2: makeClawd('all', true, 'down'),
    clawd_sleep: makeClawdSleep(),
    hat_hard: makeAccessory('hat_hard'),
    hat_hard_g: makeAccessory('hat_hard_g'),
    hat_top: makeAccessory('hat_top'),
    hat_cap: makeAccessory('hat_cap'),
    lantern: makeAccessory('lantern'),
    apron: makeAccessory('apron'),
    ...HAND_SPRITES,
    hq: buildHQ(),
    bunkhouse: buildBunkhouse(),
    mine: buildMine(),
    refinery: buildRefinery(),
    tower: buildTower(),
};

// Interior positions in sprite pixels (x from left, row from top).
const BUILD_META = {
    hq: {
        balcony: { x0: 8, x1: 34, floorRow: 44 }, // overseer paces here
        towerX: 50,                               // climb line inside tower
        deskX: 34,                                // supervisor works here
    },
    bunkhouse: {
        bedCenters: [13, 33, 53, 73],
        bedRows: [49, 25],                        // mattress top row: lower, upper level
    },
    refinery: { furnaceX: 30, chimneyX: 52 },
};

// Validate: every row of a sprite must be the same width, and every
// non-'.' char must be in the palette. Throws loudly at load.
(function validateSprites() {
    for (const [name, rows] of Object.entries(SPRITE_DATA)) {
        const w = rows[0].length;
        rows.forEach((row, i) => {
            if (row.length !== w)
                throw new Error(`sprite ${name} row ${i}: width ${row.length} != ${w}`);
            for (const ch of row)
                if (ch !== '.' && !SPRITE_PALETTE[ch])
                    throw new Error(`sprite ${name} row ${i}: unknown palette char '${ch}'`);
        });
    }
})();

// node check support
if (typeof module !== 'undefined') module.exports = { SPRITE_PALETTE, SPRITE_DATA, BUILD_META };
