// Pixel art data for the Mineshaft view. Pure data, no engine deps
// (validated standalone under node by the build check).
//
// Clawds are generated at exactly 2x the official 9x7 mascot reference
// (rounded wide body, two square eyes, two legs, #d9714f). Buildings
// are painted programmatically (cutaway/dollhouse style); interior
// positions are exported via BUILD_META (all in sprite pixels).
'use strict';

const SPRITE_PALETTE = {
    o: '#d9714f', // clawd body (official reference color)
    O: '#b3552f', // body shade
    k: '#000000', // eyes / dark details
    w: '#ffffff', // white
    y: '#f2c94c', // yellow / gold
    Y: '#c9992a', // yellow shade
    g: '#5aa053', // green
    G: '#3d7038', // green shade
    b: '#4a78b5', // blue
    B: '#2f4f7a', // blue shade
    n: '#8a5a33', // wood
    N: '#5f3d22', // wood dark
    h: '#a8764a', // wood light
    s: '#9a9a9a', // stone
    S: '#6e6e6e', // stone dark
    r: '#b5482f', // red
    R: '#7e2f1e', // red dark
    d: '#241813', // dark opening / shaft
    D: '#38291d', // interior back wall (cutaway)
    A: '#2e2118', // interior deep shade
    c: '#e8e4d8', // cloud / paper / mattress
    m: '#7a7466', // metal
    M: '#55504a', // metal dark
    t: '#c9b18a', // parchment / clock face
    T: '#9a8560', // parchment shade
    i: '#9ad0e8', // window glass
    f: '#c95a78', // flower pink
    l: '#ffe9a8', // lamp light
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
// filled pixel circle (bounds-safe)
function disc(g, cx, cy, r, ch) {
    for (let j = -r; j <= r; j++)
        for (let i = -r; i <= r; i++)
            if (i * i + j * j <= r * r + r / 2) {
                const x = cx + i, y = cy + j;
                if (g[y] !== undefined && g[y][x] !== undefined) g[y][x] = ch;
            }
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

// two legs at ref x2,x6; mode 'all' | 'walk1' (left up) | 'walk2' (right up)
function clawdLegs(g, mode) {
    [4, 12].forEach((x, i) => {
        const short = mode === 'walk1' ? i === 0 : mode === 'walk2' ? i === 1 : false;
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

// lying down asleep; breath=1 is the inhale frame (body 1px taller)
function makeClawdSleep(breath) {
    const g = makeGrid(18, 18);
    const top = breath ? 11 : 12;
    rect(g, 1, top, 16, 17 - top, 'o');
    rect(g, 1, 16, 16, 1, 'O');
    g[top][1] = '.'; g[top][16] = '.';       // rounded shoulders
    rect(g, 3, top + 2, 2, 1, 'k');          // one closed eye (lying on its side)
    rect(g, 5, 17, 2, 1, 'O');               // tucked feet
    rect(g, 11, 17, 2, 1, 'O');
    return gridRows(g);
}

// blanket overlay for clawds sleeping in a bunk: covers all but the head
function makeBlanket() {
    const g = makeGrid(18, 18);
    rect(g, 9, 11, 9, 7, 'b');
    rect(g, 9, 11, 9, 1, 'c');   // folded-over sheet
    rect(g, 9, 16, 9, 2, 'B');   // shaded drape
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
        case 'wrench': // open-end wrench held up in the right hand
            rect(g, 14, 5, 1, 2, 'm'); rect(g, 16, 5, 1, 2, 'm');
            rect(g, 14, 7, 3, 1, 'm'); rect(g, 15, 8, 2, 6, 'M');
            break;
    }
    return gridRows(g);
}

// ------------------------------------------------------------ hand sprites

const HAND_SPRITES = {

rock: [
'...sss..',
'..sssss.',
'.sssSSss',
'ssSSssss',
],

rock2: [
'....ss....',
'..ssssss..',
'.sssssSss.',
'ssSSssssss',
],

cloud: [
'.....cccc.........',
'...cccccccc..cc...',
'..cccccccccccccc..',
'.cccccccccccccccc.',
'..cccccccccccccc..',
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

zee: [
'cccccc',
'....cc',
'...cc.',
'..cc..',
'.cc...',
'cccccc',
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

// ------------------------------------------------------------ small props

function buildTree() {
    const g = makeGrid(16, 24);
    rect(g, 3, 0, 10, 3, 'g');
    rect(g, 1, 2, 14, 4, 'g');
    rect(g, 0, 5, 16, 4, 'g');
    rect(g, 2, 9, 12, 3, 'g');
    rect(g, 2, 6, 4, 3, 'G');    // shade patches
    rect(g, 10, 2, 4, 2, 'G');
    rect(g, 6, 10, 5, 2, 'G');
    rect(g, 6, 12, 4, 9, 'n');   // trunk
    rect(g, 6, 12, 1, 9, 'N');
    rect(g, 4, 21, 8, 3, 'N');   // roots
    return gridRows(g);
}

function buildPine() {
    const g = makeGrid(14, 26);
    rect(g, 6, 0, 2, 2, 'G');
    rect(g, 4, 2, 6, 4, 'g');
    rect(g, 3, 6, 8, 4, 'g');
    rect(g, 2, 10, 10, 4, 'g');
    rect(g, 1, 14, 12, 4, 'g');
    rect(g, 0, 18, 14, 2, 'G');
    rect(g, 4, 3, 1, 3, 'G');    // left shading
    rect(g, 3, 7, 1, 3, 'G');
    rect(g, 2, 11, 1, 3, 'G');
    rect(g, 1, 15, 1, 3, 'G');
    rect(g, 6, 20, 3, 4, 'N');   // trunk
    rect(g, 5, 24, 5, 2, 'N');
    return gridRows(g);
}

function buildBush() {
    const g = makeGrid(12, 8);
    rect(g, 2, 1, 8, 3, 'g');
    rect(g, 0, 3, 12, 5, 'g');
    rect(g, 1, 5, 4, 3, 'G');
    rect(g, 8, 4, 3, 2, 'G');
    rect(g, 3, 4, 1, 1, 'r');    // berries
    rect(g, 7, 6, 1, 1, 'r');
    return gridRows(g);
}

function buildGrass() {
    const g = makeGrid(8, 5);
    rect(g, 0, 3, 2, 2, 'g');
    rect(g, 3, 1, 2, 4, 'g');
    rect(g, 3, 1, 1, 4, 'G');
    rect(g, 6, 2, 2, 3, 'G');
    return gridRows(g);
}

function buildGrass2() {
    const g = makeGrid(6, 4);
    rect(g, 0, 2, 2, 2, 'G');
    rect(g, 3, 0, 2, 4, 'g');
    return gridRows(g);
}

function buildFlower(ch, center) {
    const g = makeGrid(6, 8);
    rect(g, 1, 0, 4, 3, ch);
    rect(g, 2, 1, 2, 1, center);
    rect(g, 3, 3, 1, 4, 'g');
    rect(g, 1, 4, 2, 1, 'g');
    return gridRows(g);
}

function buildFence() {
    const g = makeGrid(24, 10);
    for (const x of [1, 11, 21]) rect(g, x, 1, 2, 9, 'N');
    rect(g, 0, 3, 24, 2, 'n');
    rect(g, 0, 3, 24, 1, 'h');
    rect(g, 0, 6, 24, 2, 'n');
    return gridRows(g);
}

function buildBarrel() {
    const g = makeGrid(10, 12);
    rect(g, 1, 1, 8, 10, 'n');
    rect(g, 2, 0, 6, 1, 'N');
    rect(g, 0, 2, 10, 1, 'm');
    rect(g, 0, 8, 10, 1, 'm');
    rect(g, 2, 2, 1, 8, 'h');
    return gridRows(g);
}

function buildCrate() {
    const g = makeGrid(10, 10);
    rect(g, 1, 1, 8, 8, 'n');
    rect(g, 0, 0, 10, 1, 'N');
    rect(g, 0, 9, 10, 1, 'N');
    rect(g, 0, 0, 1, 10, 'N');
    rect(g, 9, 0, 1, 10, 'N');
    for (let i = 0; i < 6; i++) rect(g, 2 + i, 7 - i, 1, 1, 'N'); // diagonal
    return gridRows(g);
}

function buildWagon() {
    const g = makeGrid(26, 16);
    rect(g, 2, 4, 20, 5, 'n');     // bed
    rect(g, 2, 4, 20, 1, 'N');
    for (const x of [7, 12, 17]) rect(g, x, 4, 1, 5, 'N'); // slats
    rect(g, 22, 5, 4, 2, 'N');     // handle
    for (const wx of [4, 14]) {    // wheels
        rect(g, wx + 1, 9, 4, 1, 'k');
        rect(g, wx, 10, 6, 4, 'k');
        rect(g, wx + 1, 14, 4, 1, 'k');
        rect(g, wx + 2, 11, 2, 2, 'S');
    }
    return gridRows(g);
}

function buildOreCart() {
    const g = makeGrid(16, 12);
    rect(g, 3, 1, 3, 2, 's');      // ore heaped above the rim
    rect(g, 6, 0, 4, 3, 'y');
    rect(g, 10, 1, 3, 2, 's');
    rect(g, 1, 3, 14, 6, 'm');
    rect(g, 1, 3, 14, 1, 'M');
    rect(g, 4, 4, 1, 5, 'M');
    rect(g, 11, 4, 1, 5, 'M');
    rect(g, 3, 9, 4, 3, 'k');
    rect(g, 9, 9, 4, 3, 'k');
    return gridRows(g);
}

function buildOrePile() {
    const g = makeGrid(14, 8);
    rect(g, 4, 2, 6, 2, 's');
    rect(g, 2, 4, 10, 2, 's');
    rect(g, 0, 6, 14, 2, 's');
    rect(g, 1, 6, 4, 2, 'S');
    rect(g, 8, 4, 3, 2, 'S');
    for (const [x, y] of [[5, 3], [9, 5], [3, 6], [11, 6]]) rect(g, x, y, 1, 1, 'y');
    return gridRows(g);
}

function buildToolRack() {
    const g = makeGrid(16, 14);
    rect(g, 1, 1, 2, 13, 'N');
    rect(g, 13, 1, 2, 13, 'N');
    rect(g, 1, 2, 14, 2, 'n');
    rect(g, 1, 2, 14, 1, 'h');
    rect(g, 4, 5, 5, 1, 's');      // pickaxe head
    rect(g, 3, 6, 1, 1, 's');
    rect(g, 9, 6, 1, 1, 's');
    rect(g, 6, 5, 1, 8, 'n');      // pickaxe handle
    rect(g, 10, 4, 1, 8, 'n');     // shovel handle
    rect(g, 9, 12, 3, 2, 'm');     // shovel blade
    return gridRows(g);
}

function buildLamp() {
    const g = makeGrid(8, 28);
    rect(g, 1, 0, 6, 2, 'm');      // cap
    rect(g, 2, 2, 4, 4, 'l');      // glass
    rect(g, 1, 6, 6, 1, 'm');      // plate
    rect(g, 3, 7, 2, 17, 'M');     // pole
    rect(g, 1, 24, 6, 4, 'S');     // base
    rect(g, 1, 24, 6, 1, 's');
    return gridRows(g);
}

function buildSign() {
    const g = makeGrid(18, 18);
    rect(g, 3, 10, 2, 8, 'N');     // posts
    rect(g, 13, 10, 2, 8, 'N');
    rect(g, 0, 2, 18, 8, 'h');     // board
    rect(g, 0, 2, 18, 1, 'n');
    rect(g, 0, 9, 18, 1, 'n');
    rect(g, 0, 2, 1, 8, 'n');
    rect(g, 17, 2, 1, 8, 'n');
    for (const [x, y] of [[1, 3], [16, 3], [1, 8], [16, 8]]) rect(g, x, y, 1, 1, 'k');
    return gridRows(g);
}

function buildWell() {
    const g = makeGrid(18, 20);
    rect(g, 2, 0, 14, 2, 'R');     // roof
    rect(g, 1, 2, 16, 2, 'r');
    rect(g, 2, 4, 2, 12, 'n');     // posts
    rect(g, 14, 4, 2, 12, 'n');
    rect(g, 4, 6, 10, 1, 'N');     // axle
    rect(g, 5, 12, 8, 2, 'd');     // opening
    rect(g, 8, 7, 1, 7, 'k');      // rope
    rect(g, 1, 14, 16, 6, 's');    // stone ring
    rect(g, 1, 16, 16, 1, 'S');
    rect(g, 4, 18, 3, 1, 'S');
    rect(g, 11, 15, 3, 1, 'S');
    return gridRows(g);
}

function buildShed() {
    const g = makeGrid(30, 26);
    rect(g, 0, 0, 30, 5, 'N');     // roof
    rect(g, 0, 0, 30, 1, 'h');
    rect(g, 0, 5, 30, 1, 'n');     // eave
    rect(g, 0, 6, 30, 16, 'n');    // wall
    rect(g, 0, 11, 30, 1, 'N');    // plank seams
    rect(g, 0, 16, 30, 1, 'N');
    rect(g, 9, 8, 1, 14, 'N');     // door frame
    rect(g, 20, 8, 1, 14, 'N');
    rect(g, 9, 8, 12, 1, 'N');
    rect(g, 10, 9, 10, 13, 'd');   // door
    rect(g, 11, 14, 8, 1, 'N');    // cross brace
    rect(g, 24, 10, 1, 10, 'n');   // leaning shovel
    rect(g, 23, 20, 3, 2, 'm');
    rect(g, 0, 22, 30, 4, 'N');    // base
    return gridRows(g);
}

function buildWaterTower() {
    const g = makeGrid(26, 52);
    rect(g, 10, 0, 6, 2, 'R');     // stepped cone roof
    rect(g, 6, 2, 14, 2, 'r');
    rect(g, 3, 4, 20, 2, 'R');
    rect(g, 2, 6, 22, 2, 'r');
    rect(g, 2, 8, 22, 18, 'n');    // tank
    for (const x of [6, 11, 16]) rect(g, x, 8, 1, 18, 'N'); // staves
    rect(g, 2, 11, 22, 1, 'm');    // hoops
    rect(g, 2, 21, 22, 1, 'm');
    rect(g, 3, 26, 3, 26, 'n');    // legs
    rect(g, 20, 26, 3, 26, 'n');
    rect(g, 5, 34, 16, 2, 'N');    // braces
    rect(g, 5, 42, 16, 2, 'N');
    rect(g, 11, 26, 4, 3, 'm');    // spout
    rect(g, 12, 29, 2, 5, 'M');
    return gridRows(g);
}

function buildStore() {
    const g = makeGrid(42, 44);
    rect(g, 0, 4, 42, 3, 'N');     // cornice
    rect(g, 0, 4, 42, 1, 'h');
    rect(g, 0, 7, 42, 13, 'n');    // false front
    rect(g, 7, 9, 28, 7, 'h');     // sign board
    rect(g, 7, 9, 28, 1, 'n');
    rect(g, 7, 15, 28, 1, 'n');
    rect(g, 7, 9, 1, 7, 'n');
    rect(g, 34, 9, 1, 7, 'n');
    for (const [x, y] of [[8, 10], [33, 10], [8, 14], [33, 14]]) rect(g, x, y, 1, 1, 'k');
    rect(g, 0, 20, 42, 20, 'n');   // main wall
    rect(g, 0, 26, 42, 1, 'N');    // plank seams
    rect(g, 0, 33, 42, 1, 'N');
    rect(g, 5, 23, 11, 17, 'N');   // door frame
    rect(g, 6, 24, 9, 16, 'd');    // door
    rect(g, 13, 31, 1, 2, 'y');    // handle
    rect(g, 19, 23, 15, 13, 'n');  // window frame
    rect(g, 20, 24, 13, 11, 'i');  // glass
    rect(g, 26, 24, 1, 11, 'n');   // mullions
    rect(g, 20, 29, 13, 1, 'n');
    rect(g, 1, 18, 40, 4, 'r');    // awning
    for (let x = 4; x <= 37; x += 6) rect(g, x, 18, 3, 4, 'c');
    rect(g, 1, 21, 40, 1, 'R');
    rect(g, 36, 33, 5, 7, 'n');    // barrel by the door
    rect(g, 36, 34, 5, 1, 'm');
    rect(g, 36, 38, 5, 1, 'm');
    rect(g, 0, 40, 42, 4, 'N');    // porch base
    rect(g, 0, 40, 42, 1, 'h');
    return gridRows(g);
}

// ------------------------------------------------------------ buildings

// HQ: 64x92. Cutaway first floor (supervisor desk, bookshelf), clock
// tower on the right; the first-floor roof is the overseer's balcony.
function buildHQ() {
    const g = makeGrid(64, 92);
    // tower
    rect(g, 36, 6, 28, 60, 'D');
    rect(g, 36, 6, 2, 60, 'n');
    rect(g, 62, 6, 2, 60, 'n');
    for (const wx of [36, 62])
        for (let y = 12; y < 66; y += 9) rect(g, wx, y, 2, 1, 'N');
    rect(g, 34, 4, 30, 2, 'n');    // eave
    rect(g, 34, 4, 30, 1, 'h');
    rect(g, 33, 1, 31, 3, 'N');    // roof
    rect(g, 33, 1, 31, 1, 'h');
    rect(g, 47, 0, 1, 1, 'k');     // flag
    rect(g, 48, 0, 6, 2, 'r');
    // clock face (hands drawn at runtime)
    disc(g, 50, 19, 10, 'N');
    disc(g, 50, 19, 8, 't');
    for (const [x, y] of [[50, 12], [50, 26], [43, 19], [57, 19]]) rect(g, x, y, 1, 1, 'T');
    rect(g, 50, 19, 1, 1, 'k');
    // tower window
    rect(g, 42, 33, 14, 14, 'n');
    rect(g, 43, 34, 12, 12, 'i');
    rect(g, 48, 34, 2, 12, 'n');
    rect(g, 43, 39, 12, 1, 'n');
    rect(g, 41, 47, 16, 1, 'h');   // sill
    // balcony door in tower left wall
    rect(g, 36, 46, 2, 12, '.');
    // balcony (= first-floor roof) + railing
    rect(g, 4, 58, 34, 4, 'n');
    rect(g, 4, 58, 34, 1, 'h');
    rect(g, 4, 46, 32, 2, 'n');
    rect(g, 4, 46, 32, 1, 'h');
    for (const x of [4, 14, 24, 34]) rect(g, x, 48, 2, 10, 'n');
    // first floor
    rect(g, 0, 62, 64, 4, 'N');    // ceiling band
    rect(g, 0, 66, 64, 26, 'D');
    rect(g, 0, 66, 64, 3, 'A');
    rect(g, 0, 89, 64, 3, 'N');    // floor slab
    rect(g, 0, 66, 2, 26, 'n');
    rect(g, 62, 66, 2, 26, 'n');
    rect(g, 0, 73, 2, 19, '.');    // door opening (left)
    // desk with papers
    rect(g, 40, 79, 16, 2, 'n');
    rect(g, 40, 79, 16, 1, 'h');
    rect(g, 41, 81, 2, 8, 'N');
    rect(g, 53, 81, 2, 8, 'N');
    rect(g, 43, 76, 6, 3, 'c');
    rect(g, 51, 77, 2, 2, 'k');
    // bookshelf
    rect(g, 8, 70, 14, 17, 'N');
    for (const [i, sy] of [71, 76, 81].entries()) {
        rect(g, 9, sy, 12, 4, 'd');
        const books = ['rbgyc', 'gcrby', 'ybcgr'][i];
        for (let bx = 0; bx < 5; bx++)
            rect(g, 10 + bx * 2, sy + 1, 2, 3, books[bx]);
    }
    // framed clawd portrait
    rect(g, 27, 70, 6, 7, 'n');
    rect(g, 28, 71, 4, 5, 'c');
    rect(g, 29, 73, 2, 2, 'o');
    // rug
    rect(g, 26, 89, 16, 1, 'r');
    return gridRows(g);
}

// Bunkhouse: 88x62 cutaway with double-decker bunks (2 levels x 4 beds).
function buildBunkhouse() {
    const g = makeGrid(88, 62);
    rect(g, 70, 0, 6, 3, 's');     // chimney
    rect(g, 70, 0, 6, 1, 'S');
    rect(g, 2, 2, 84, 6, 'n');     // roof
    rect(g, 2, 2, 84, 1, 'h');
    for (let x = 8; x < 86; x += 8) rect(g, x, 3, 1, 5, 'N'); // shingle seams
    rect(g, 0, 8, 88, 4, 'N');     // eave
    rect(g, 0, 12, 88, 46, 'D');   // interior
    rect(g, 0, 12, 88, 3, 'A');
    rect(g, 0, 12, 2, 46, 'n');    // walls
    rect(g, 86, 12, 2, 46, 'n');
    rect(g, 86, 40, 2, 18, '.');   // door opening (right)
    rect(g, 0, 58, 88, 4, 'N');    // floor
    // back-wall windows
    for (const wx of [8, 38]) {
        rect(g, wx, 16, 12, 9, 'n');
        rect(g, wx + 1, 17, 10, 7, 'i');
        rect(g, wx + 5, 17, 2, 7, 'n');
    }
    // hanging lantern
    rect(g, 62, 12, 1, 4, 'm');
    rect(g, 60, 16, 5, 4, 'y');
    rect(g, 60, 16, 5, 1, 'm');
    // upper level platform + ladder
    rect(g, 2, 32, 80, 2, 'n');
    rect(g, 2, 32, 80, 1, 'h');
    rect(g, 80, 32, 2, 26, 'N');
    for (let y = 35; y < 58; y += 4) rect(g, 79, y, 4, 1, 'n');
    // beds on both levels: [level leg row, mattress top row]
    for (const [legRow, topRow] of [[56, 53], [30, 27]]) {
        for (const cx of [13, 33, 53, 73]) {
            rect(g, cx - 7, legRow, 2, 2, 'N');       // legs
            rect(g, cx + 5, legRow, 2, 2, 'N');
            rect(g, cx - 7, topRow, 14, 3, 'c');      // mattress
            rect(g, cx - 7, topRow, 4, 2, 'w');       // pillow
            rect(g, cx - 1, topRow - 1, 8, 4, 'b');   // blanket
            rect(g, cx - 1, topRow + 1, 8, 1, 'B');
        }
    }
    // rug
    rect(g, 28, 57, 14, 1, 'r');
    return gridRows(g);
}

// Mineshaft headframe: 52x60, sloped A-frame legs, spoked sheave wheel,
// cross bracing, timber-lined shaft collar at bottom center.
function buildMine() {
    const g = makeGrid(52, 60);
    // wheel house on top
    rect(g, 10, 0, 32, 3, 'N');    // roof
    rect(g, 10, 0, 32, 1, 'h');
    rect(g, 12, 3, 28, 11, 'n');   // housing
    rect(g, 17, 4, 18, 9, 'd');    // open front
    // sheave wheel with spokes
    disc(g, 26, 8, 5, 's');
    disc(g, 26, 8, 4, 'd');
    rect(g, 22, 8, 9, 1, 's');
    rect(g, 26, 4, 1, 9, 's');
    rect(g, 25, 7, 3, 3, 'S');     // hub
    // sloped legs (batter inward toward the housing)
    for (let j = 0; j < 46; j += 2) {
        const dx = Math.round(12 * j / 46);
        rect(g, 14 - dx, 14 + j, 4, 2, 'n');
        rect(g, 14 - dx, 14 + j, 1, 2, 'h');
        rect(g, 34 + dx, 14 + j, 4, 2, 'n');
        rect(g, 37 + dx, 14 + j, 1, 2, 'N');
    }
    // cross bracing between the legs
    rect(g, 12, 26, 28, 2, 'n');
    rect(g, 12, 26, 28, 1, 'h');
    rect(g, 8, 40, 36, 2, 'n');
    rect(g, 8, 40, 36, 1, 'h');
    for (let i = 0; i < 12; i++) {  // X brace
        rect(g, 14 + i * 2, 28 + i, 2, 1, 'N');
        rect(g, 36 - i * 2, 28 + i, 2, 1, 'N');
    }
    // cable from wheel into the shaft
    rect(g, 25, 13, 1, 33, 'm');
    // warning sign on left leg
    rect(g, 3, 46, 5, 5, 'y');
    rect(g, 4, 47, 2, 3, 'k');
    // shaft collar with hazard-striped lintel
    rect(g, 15, 44, 22, 2, 'n');
    for (let x = 16; x < 36; x += 4) rect(g, x, 44, 2, 2, 'y');
    rect(g, 17, 46, 18, 14, 'd');
    rect(g, 15, 46, 2, 14, 'N');
    rect(g, 35, 46, 2, 14, 'N');
    rect(g, 25, 46, 1, 10, 'm');   // cable continues down
    return gridRows(g);
}

// Refinery: 80x66 industrial cutaway - furnace, pipes, tank, chimneys.
function buildRefinery() {
    const g = makeGrid(80, 66);
    // chimneys first (behind the hall roofline)
    rect(g, 52, 0, 14, 32, 's');
    rect(g, 50, 0, 18, 3, 'S');
    rect(g, 52, 10, 14, 2, 'S');
    rect(g, 52, 20, 14, 2, 'S');
    rect(g, 32, 8, 10, 24, 's');
    rect(g, 31, 6, 12, 3, 'S');
    // main hall
    rect(g, 0, 26, 80, 4, 'S');    // roof band
    rect(g, 0, 26, 80, 1, 'm');
    rect(g, 0, 30, 80, 32, 'D');   // interior
    rect(g, 0, 30, 80, 3, 'A');
    rect(g, 0, 30, 2, 32, 's');
    rect(g, 78, 30, 2, 32, 's');
    rect(g, 78, 44, 2, 18, '.');   // door opening (right)
    rect(g, 0, 62, 80, 4, 'S');    // floor slab
    // furnace with fire
    rect(g, 6, 36, 24, 26, 's');
    rect(g, 6, 36, 24, 2, 'S');
    rect(g, 6, 44, 24, 1, 'S');    // brick seams
    rect(g, 6, 52, 24, 1, 'S');
    rect(g, 11, 46, 14, 13, 'd');
    rect(g, 13, 51, 10, 7, 'r');
    rect(g, 15, 53, 6, 4, 'y');
    rect(g, 16, 54, 3, 2, 'l');
    rect(g, 11, 58, 14, 1, 'k');   // grate
    rect(g, 7, 39, 5, 5, 'c');     // gauge
    rect(g, 9, 41, 1, 1, 'k');
    // pipe from furnace to tank
    rect(g, 30, 33, 26, 3, 'm');
    rect(g, 53, 33, 3, 13, 'm');
    for (const x of [34, 40, 46]) rect(g, x, 34, 1, 1, 'M');
    // tank with rivets + valve
    rect(g, 48, 46, 28, 16, 'm');
    rect(g, 48, 46, 28, 2, 'M');
    for (const y of [50, 56])
        for (let x = 51; x < 74; x += 5) rect(g, x, y, 1, 1, 'M');
    rect(g, 59, 43, 6, 2, 'r');
    rect(g, 61, 45, 2, 1, 'M');
    // crate
    rect(g, 32, 56, 7, 6, 'n');
    rect(g, 32, 56, 7, 1, 'N');
    rect(g, 32, 61, 7, 1, 'N');
    rect(g, 32, 56, 1, 6, 'N');
    rect(g, 38, 56, 1, 6, 'N');
    return gridRows(g);
}

// Witness watchtower: 26x62 with cabin, platform, ladder, lamp.
function buildTower() {
    const g = makeGrid(26, 62);
    // roof + flag
    rect(g, 2, 0, 1, 2, 'k');
    rect(g, 3, 0, 4, 2, 'g');
    rect(g, 0, 2, 26, 4, 'N');
    rect(g, 0, 2, 26, 1, 'h');
    // cabin
    rect(g, 2, 6, 22, 14, 'D');
    rect(g, 2, 6, 2, 14, 'n');
    rect(g, 22, 6, 2, 14, 'n');
    rect(g, 7, 8, 12, 1, 'n');     // window frame
    rect(g, 7, 17, 12, 1, 'n');
    rect(g, 8, 9, 10, 8, 'd');     // window
    rect(g, 6, 9, 2, 8, 'r');      // shutters
    rect(g, 18, 9, 2, 8, 'r');
    // platform
    rect(g, 0, 20, 26, 4, 'n');
    rect(g, 0, 20, 26, 1, 'h');
    // legs + braces
    rect(g, 2, 24, 3, 38, 'n');
    rect(g, 21, 24, 3, 38, 'n');
    rect(g, 4, 40, 18, 2, 'N');
    rect(g, 4, 50, 18, 2, 'N');
    // ladder
    rect(g, 12, 24, 2, 38, 'N');
    for (let y = 26; y < 61; y += 4) rect(g, 10, y, 6, 1, 'n');
    // lamp under platform
    rect(g, 11, 24, 4, 1, 'm');
    rect(g, 11, 25, 4, 3, 'y');
    return gridRows(g);
}

const SPRITE_DATA = {
    clawd_idle: makeClawd('all', true),
    clawd_blink: makeClawd('all', false),
    clawd_walk1: makeClawd('walk1', true),
    clawd_walk2: makeClawd('walk2', true),
    clawd_mine1: makeClawd('all', true, 'up'),
    clawd_mine2: makeClawd('all', true, 'down'),
    clawd_sleep1: makeClawdSleep(0),
    clawd_sleep2: makeClawdSleep(1),
    blanket: makeBlanket(),
    hat_hard: makeAccessory('hat_hard'),
    hat_hard_g: makeAccessory('hat_hard_g'),
    hat_top: makeAccessory('hat_top'),
    hat_cap: makeAccessory('hat_cap'),
    lantern: makeAccessory('lantern'),
    wrench: makeAccessory('wrench'),
    ...HAND_SPRITES,
    tree: buildTree(),
    pine: buildPine(),
    bush: buildBush(),
    grass: buildGrass(),
    grass2: buildGrass2(),
    flower_p: buildFlower('f', 'w'),
    flower_y: buildFlower('y', 'Y'),
    fence: buildFence(),
    barrel: buildBarrel(),
    crate: buildCrate(),
    wagon: buildWagon(),
    orecart: buildOreCart(),
    orepile: buildOrePile(),
    toolrack: buildToolRack(),
    lamp: buildLamp(),
    sign: buildSign(),
    well: buildWell(),
    shed: buildShed(),
    watertower: buildWaterTower(),
    store: buildStore(),
    hq: buildHQ(),
    bunkhouse: buildBunkhouse(),
    mine: buildMine(),
    refinery: buildRefinery(),
    tower: buildTower(),
};

// Interior positions in sprite pixels (x from left, row from top).
const BUILD_META = {
    hq: {
        balcony: { x0: 6, x1: 36, floorRow: 58 }, // overseer paces here
        towerX: 50,                               // climb line inside tower
        deskX: 36,                                // supervisor works here
        clock: { cx: 50, cyRow: 19, r: 8 },       // tower clock face
    },
    bunkhouse: {
        bedCenters: [13, 33, 53, 73],
        bedRows: [53, 27],                        // mattress top row: lower, upper level
        ladderX: 81,                              // climb line to upper bunks
    },
    refinery: { furnaceX: 33, chimneyX: 59 },
    lamp: { lightRow: 4 },                        // glow center for night halos
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
