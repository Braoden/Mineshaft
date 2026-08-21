// sprites.js — procedural pixel art for the dashboard room sims.
//
// Every sprite is authored as a grid of single characters and mapped through
// PALETTE to RGBA. Nothing here loads a binary asset, so the whole art set
// ships inside the Go binary's embedded FS as plain text.
//
// Colour discipline: the page palette is entirely cool (navy/indigo/steel/mint).
// Clawd's body orange is the ONLY warm hue in the rooms, so the mascots read
// instantly against the interiors. Amber appears solely as lamp light.

const PALETTE = {
    '.': null,          // transparent

    // clawd — the only warm colours in the scene
    o: '#d9714f',       // body (official reference colour)
    O: '#b3552f',       // body shade

    // structural cool palette
    e: '#111227',       // deepest — outlines, eyes, shadow
    n: '#1a1938',       // near-black navy
    B: '#272652',       // wall / card navy
    b: '#2e457d',       // indigo — machinery, structure
    i: '#3d5a96',       // indigo highlight
    s: '#82a0b9',       // steel — metal, rails, stone
    S: '#5c7286',       // steel shade
    w: '#d7fff7',       // mint — light, paper, glass
    W: '#a9ccc4',       // mint shade

    // accents (severity palette, reused as light + spark)
    y: '#e8b04b',       // lamp light / warning
    Y: '#b8862f',       // lamp shade
    r: '#e35d6a',       // spark / critical

    // ---- interior materials -------------------------------------------
    // Room interiors are deliberately NOT limited to the dashboard palette.
    // The cards around them carry the cool scheme; inside the boxes the
    // scenes get real materials so the rooms read as places, not diagrams.
    t: '#8a5a33',       // timber
    T: '#5f3d22',       // timber dark
    h: '#a8764a',       // timber light
    k: '#4a4550',       // rock
    K: '#332f3d',       // rock dark
    q: '#655f71',       // rock light
    m: '#9a9a9a',       // steel
    M: '#6e6e6e',       // steel dark
    c: '#cdc8bf',       // bright metal / chrome
    u: '#c98b4b',       // copper / brass
    U: '#8f5f2f',       // copper dark
    l: '#ffe9a8',       // lamp glow
    L: '#ffc861',       // lamp core
    g: '#5aa053',       // green (plants, ledger cloth)
    G: '#3d7038',       // green dark
    x: '#b5482f',       // rust / red
    X: '#7e2f1e',       // rust dark
    p: '#efe9db',       // paper
    P: '#c2bcae',       // paper shade
    z: '#2b2b33',       // coal
    Z: '#9db4cc',       // ore glint
    f: '#c95a78',       // flower / pin head
    v: '#7a6a92',       // dim violet (deep shadow tint)
};

// ---------------------------------------------------------------- grid tools

function makeGrid(w, h) {
    return Array.from({ length: h }, () => Array(w).fill('.'));
}

function rect(g, x, y, w, h, ch) {
    for (let yy = y; yy < y + h; yy++) {
        if (yy < 0 || yy >= g.length) continue;
        for (let xx = x; xx < x + w; xx++) {
            if (xx < 0 || xx >= g[yy].length) continue;
            g[yy][xx] = ch;
        }
    }
}

function px(g, x, y, ch) { rect(g, x, y, 1, 1, ch); }

function disc(g, cx, cy, r, ch) {
    for (let y = Math.ceil(cy - r); y <= cy + r; y++) {
        for (let x = Math.ceil(cx - r); x <= cx + r; x++) {
            const dx = x - cx, dy = y - cy;
            if (dx * dx + dy * dy <= r * r) px(g, x, y, ch);
        }
    }
}

// merge paints src over dst, skipping transparent cells. Layering base body ->
// headwear -> tool is what keeps the sprite count linear instead of N x M.
function merge(dst, src) {
    for (let y = 0; y < src.length && y < dst.length; y++) {
        for (let x = 0; x < src[y].length && x < dst[y].length; x++) {
            if (src[y][x] !== '.') dst[y][x] = src[y][x];
        }
    }
    return dst;
}

function clone(g) { return g.map(row => row.slice()); }

// ---------------------------------------------------------------- clawd body
//
// 18x18. Body occupies rows 4-13, legs rows 14-17. Headwear overlays assume
// the crown sits at rows 0-4, tools attach at the body's right edge.

const CW = 18, CH = 18;

function clawdBlob(g, yOff = 0) {
    rect(g, 4, 4 + yOff, 10, 2, 'o');
    rect(g, 2, 6 + yOff, 14, 2, 'o');
    rect(g, 0, 8 + yOff, 18, 4, 'o');
    rect(g, 2, 12 + yOff, 14, 2, 'o');
    // underside shading gives the blob a little volume
    rect(g, 3, 13 + yOff, 12, 1, 'O');
}

function clawdEyes(g, yOff = 0, closed = false) {
    if (closed) {
        rect(g, 5, 9 + yOff, 2, 1, 'e');
        rect(g, 11, 9 + yOff, 2, 1, 'e');
    } else {
        rect(g, 5, 8 + yOff, 2, 2, 'e');
        rect(g, 11, 8 + yOff, 2, 2, 'e');
    }
}

// legs(mode) — walk cycles move the legs rather than bobbing the whole sprite,
// so a walking clawd reads as walking even when standing still in a small box.
function clawdLegs(g, mode) {
    const foot = (x, h) => {
        rect(g, x, 14, 2, h, 'o');
        rect(g, x, 14 + h - 1, 2, 1, 'O');
    };
    switch (mode) {
        case 'walkA': foot(3, 4); foot(12, 3); break;
        case 'walkB': foot(5, 3); foot(11, 4); break;
        case 'wide':  foot(2, 4); foot(13, 4); break;
        case 'none':  break;
        default:      foot(4, 4); foot(12, 4); break;
    }
}

// arms are small nubs — enough to sell carrying and reaching without turning
// the blob into a full articulated figure.
function clawdArms(g, mode) {
    switch (mode) {
        case 'carry':                       // both arms forward
            rect(g, 15, 9, 3, 2, 'o');
            rect(g, 0, 9, 3, 2, 'o');
            break;
        case 'up':                          // both arms raised
            rect(g, 15, 5, 2, 4, 'o');
            rect(g, 1, 5, 2, 4, 'o');
            break;
        case 'reach':                       // right arm out
            rect(g, 15, 8, 3, 2, 'o');
            break;
        default: break;
    }
}

const POSES = {
    stand:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'stand'); return g; },
    walkA:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'walkA'); return g; },
    walkB:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'walkB'); return g; },
    carry:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'stand'); clawdArms(g, 'carry'); return g; },
    reach:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'wide'); clawdArms(g, 'reach'); return g; },
    stretch: () => { const g = makeGrid(CW, CH); clawdBlob(g, -1); clawdEyes(g, -1, true); clawdLegs(g, 'wide'); clawdArms(g, 'up'); return g; },
    // work poses leave the right side clear for the tool overlay
    workUp:  () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g); clawdLegs(g, 'wide'); return g; },
    workHit: () => { const g = makeGrid(CW, CH); clawdBlob(g, 1); clawdEyes(g, 1); clawdLegs(g, 'wide'); return g; },
    sit:     () => {
        const g = makeGrid(CW, CH);
        clawdBlob(g, 2);
        clawdEyes(g, 2);
        rect(g, 3, 16, 12, 2, 'O');       // folded legs under the body
        return g;
    },
    blink:   () => { const g = makeGrid(CW, CH); clawdBlob(g); clawdEyes(g, 0, true); clawdLegs(g, 'stand'); return g; },
    sleep:   () => {
        const g = makeGrid(CW, CH);
        rect(g, 1, 12, 16, 5, 'o');
        rect(g, 1, 16, 16, 1, 'O');
        px(g, 1, 12, '.'); px(g, 16, 12, '.');   // rounded shoulders
        rect(g, 4, 14, 2, 1, 'e');               // one closed eye, lying on its side
        return g;
    },
};

// ---------------------------------------------------------------- headwear
//
// One hat per role. Silhouettes are deliberately distinct at 18px so the role
// is readable even when the tool is hidden mid-animation.

const HATS = {
    // miner — hard hat with a brim and a lit head lamp
    hard: () => {
        const g = makeGrid(CW, CH);
        rect(g, 5, 1, 8, 3, 'y');
        rect(g, 4, 4, 10, 1, 'Y');
        rect(g, 3, 3, 1, 2, 'Y');
        rect(g, 14, 3, 1, 2, 'Y');
        rect(g, 8, 0, 2, 1, 'y');      // crown ridge
        rect(g, 12, 2, 2, 2, 'w');     // lamp lens
        return g;
    },
    // refinery — welding visor flipped UP onto the crown. Flipped down it
    // covers rows 7-9, which masks clawd's eyes entirely and costs the mascot
    // its face; resting it up keeps the welder read and the character.
    visor: () => {
        const g = makeGrid(CW, CH);
        rect(g, 4, 0, 10, 4, 'b');     // raised visor plate
        rect(g, 5, 1, 8, 2, 'i');      // glass catching the light
        rect(g, 3, 4, 12, 2, 's');     // headband across the brow
        rect(g, 3, 5, 12, 1, 'S');
        return g;
    },
    // overseer — top hat, the one who runs the town
    top: () => {
        const g = makeGrid(CW, CH);
        rect(g, 5, 0, 8, 4, 'e');
        rect(g, 5, 1, 8, 1, 'b');      // band
        rect(g, 3, 4, 12, 1, 'e');     // brim
        return g;
    },
    // supervisor — flat cap with a forward peak
    cap: () => {
        const g = makeGrid(CW, CH);
        rect(g, 5, 2, 8, 2, 'b');
        rect(g, 4, 4, 10, 1, 'i');
        rect(g, 13, 4, 3, 1, 'i');     // peak
        return g;
    },
    // witness — green eyeshade, the auditor
    shade: () => {
        const g = makeGrid(CW, CH);
        rect(g, 4, 5, 10, 2, 'S');
        rect(g, 3, 6, 12, 1, 's');
        rect(g, 5, 4, 8, 1, 'w');      // headband
        return g;
    },
};

// ---------------------------------------------------------------- tools
//
// Tools are overlays keyed by phase so a swing is two sprites, not two
// full-body redraws.

const TOOLS = {
    pick: {
        up: () => {
            const g = makeGrid(CW, CH);
            rect(g, 12, 3, 2, 5, 'S');      // handle, hand to overhead
            rect(g, 9, 1, 8, 2, 's');       // steel head
            px(g, 8, 2, 'S'); px(g, 16, 3, 'S');
            return g;
        },
        hit: () => {
            const g = makeGrid(CW, CH);
            rect(g, 12, 6, 2, 2, 'S');
            rect(g, 14, 8, 2, 2, 'S');
            rect(g, 15, 10, 3, 2, 's');     // head driven down into the seam
            px(g, 17, 9, 'S');
            return g;
        },
    },
    wrench: {
        up: () => {
            const g = makeGrid(CW, CH);
            rect(g, 15, 3, 2, 6, 'S');
            rect(g, 14, 1, 1, 3, 's');      // open jaws
            rect(g, 17, 1, 1, 3, 's');
            rect(g, 14, 3, 4, 1, 's');
            return g;
        },
        hit: () => {
            const g = makeGrid(CW, CH);
            rect(g, 15, 7, 2, 5, 'S');
            rect(g, 14, 6, 4, 1, 's');
            rect(g, 14, 5, 1, 2, 's');
            rect(g, 17, 5, 1, 2, 's');
            return g;
        },
    },
    clipboard: {
        up: () => {
            const g = makeGrid(CW, CH);
            rect(g, 14, 7, 4, 6, 'S');
            rect(g, 15, 8, 2, 4, 'w');      // paper
            rect(g, 15, 7, 2, 1, 's');      // clip
            return g;
        },
        hit: () => {
            const g = makeGrid(CW, CH);
            rect(g, 14, 8, 4, 6, 'S');
            rect(g, 15, 9, 2, 4, 'w');
            rect(g, 15, 8, 2, 1, 's');
            return g;
        },
    },
    whistle: {
        up: () => {
            const g = makeGrid(CW, CH);
            rect(g, 15, 7, 3, 2, 's');
            px(g, 14, 8, 'S');
            rect(g, 15, 5, 1, 2, 'w');      // toot
            return g;
        },
        hit: () => {
            const g = makeGrid(CW, CH);
            rect(g, 15, 8, 3, 2, 's');
            px(g, 14, 9, 'S');
            return g;
        },
    },
    ledger: {
        up: () => {
            const g = makeGrid(CW, CH);
            rect(g, 13, 8, 5, 5, 'b');
            rect(g, 14, 9, 3, 3, 'w');      // open pages
            rect(g, 15, 8, 1, 5, 'S');      // spine
            return g;
        },
        hit: () => {
            const g = makeGrid(CW, CH);
            rect(g, 13, 9, 5, 5, 'b');
            rect(g, 14, 10, 3, 3, 'w');
            rect(g, 15, 9, 1, 5, 'S');
            return g;
        },
    },
};

// role -> { hat, tool }
const ROLE_KIT = {
    miner:      { hat: 'hard',  tool: 'pick' },
    refinery:   { hat: 'visor', tool: 'wrench' },
    overseer:   { hat: 'top',   tool: 'clipboard' },
    supervisor: { hat: 'cap',   tool: 'whistle' },
    witness:    { hat: 'shade', tool: 'ledger' },
};

// buildActor composes every pose for one role into a named grid set.
function buildActor(role) {
    const kit = ROLE_KIT[role] || ROLE_KIT.miner;
    const hat = HATS[kit.hat]();
    const tool = TOOLS[kit.tool];
    const out = {};

    for (const [name, make] of Object.entries(POSES)) {
        const g = make();
        // sleeping clawds take the hat off — it rests beside them
        if (name !== 'sleep') merge(g, hat);
        out[name] = g;
    }
    // work poses get the tool layered on top
    out.workUp = merge(out.workUp, tool.up());
    out.workHit = merge(out.workHit, tool.hit());
    out.carry = merge(clone(out.carry), tool.up());
    return out;
}

// ---------------------------------------------------------------- props
//
// Room furniture. Sizes vary; each returns { w, h, g }.

function prop(w, h, paint) {
    const g = makeGrid(w, h);
    paint(g);
    return { w, h, g };
}

const PROPS = {
    // ================================================== mineshaft
    oreCart: () => prop(24, 15, g => {
        rect(g, 1, 2, 22, 8, 't');
        rect(g, 1, 2, 22, 1, 'h');
        rect(g, 1, 9, 22, 1, 'T');
        for (let x = 3; x < 22; x += 5) rect(g, x, 3, 1, 6, 'T');
        rect(g, 0, 10, 24, 2, 'M');
        disc(g, 5, 12, 2.6, 'M'); disc(g, 5, 12, 1.4, 'm'); px(g, 5, 12, 'K');
        disc(g, 18, 12, 2.6, 'M'); disc(g, 18, 12, 1.4, 'm'); px(g, 18, 12, 'K');
    }),
    oreCartFull: () => prop(24, 15, g => {
        rect(g, 2, 0, 5, 3, 'z'); rect(g, 7, 0, 6, 2, 'k'); rect(g, 13, 0, 6, 3, 'z');
        px(g, 4, 0, 'Z'); px(g, 10, 1, 'Z'); px(g, 16, 0, 'Z');
        rect(g, 1, 2, 22, 8, 't');
        rect(g, 1, 2, 22, 1, 'h');
        rect(g, 1, 9, 22, 1, 'T');
        for (let x = 3; x < 22; x += 5) rect(g, x, 3, 1, 6, 'T');
        rect(g, 0, 10, 24, 2, 'M');
        disc(g, 5, 12, 2.6, 'M'); disc(g, 5, 12, 1.4, 'm'); px(g, 5, 12, 'K');
        disc(g, 18, 12, 2.6, 'M'); disc(g, 18, 12, 1.4, 'm'); px(g, 18, 12, 'K');
    }),
    // full timber support frame: two posts plus a header and braces
    frame: () => prop(46, 34, g => {
        rect(g, 0, 0, 46, 4, 't');
        rect(g, 0, 0, 46, 1, 'h');
        rect(g, 0, 3, 46, 1, 'T');
        rect(g, 1, 4, 5, 30, 't'); rect(g, 1, 4, 1, 30, 'h'); rect(g, 5, 4, 1, 30, 'T');
        rect(g, 40, 4, 5, 30, 't'); rect(g, 40, 4, 1, 30, 'h'); rect(g, 44, 4, 1, 30, 'T');
        // corner braces
        for (let i = 0; i < 6; i++) { px(g, 6 + i, 5 + i, 'T'); px(g, 7 + i, 5 + i, 't'); }
        for (let i = 0; i < 6; i++) { px(g, 39 - i, 5 + i, 'T'); px(g, 38 - i, 5 + i, 't'); }
    }),
    strata: () => prop(40, 7, g => {
        rect(g, 0, 0, 40, 3, 'K');
        rect(g, 2, 1, 30, 1, 'q');
        rect(g, 0, 4, 40, 2, 'k');
        px(g, 9, 5, 'Z'); px(g, 24, 4, 'Z');
    }),
    seam: () => prop(18, 26, g => {
        rect(g, 0, 0, 18, 26, 'K');
        rect(g, 1, 2, 6, 3, 'z'); rect(g, 9, 5, 7, 3, 'z');
        rect(g, 2, 10, 6, 3, 'z'); rect(g, 10, 15, 6, 3, 'z');
        rect(g, 1, 20, 7, 3, 'z');
        px(g, 3, 3, 'Z'); px(g, 12, 6, 'Z'); px(g, 5, 11, 'Z');
        px(g, 13, 16, 'Z'); px(g, 4, 21, 'Z'); px(g, 15, 22, 'Z');
        rect(g, 8, 0, 1, 26, 'q');
    }),
    oreChunk: () => prop(6, 5, g => { rect(g, 0, 1, 6, 4, 'k'); rect(g, 1, 0, 4, 1, 'q'); px(g, 2, 2, 'Z'); }),
    coalPile: () => prop(20, 9, g => {
        rect(g, 2, 5, 16, 4, 'z');
        rect(g, 4, 3, 12, 2, 'z');
        rect(g, 7, 1, 6, 2, 'k');
        px(g, 6, 6, 'Z'); px(g, 13, 4, 'Z'); px(g, 9, 2, 'Z');
    }),
    rail: () => prop(28, 4, g => {
        for (let x = 0; x < 28; x += 6) rect(g, x, 2, 4, 2, 'T');
        rect(g, 0, 0, 28, 1, 'm');
        rect(g, 0, 1, 28, 1, 'M');
    }),
    lamp: () => prop(9, 14, g => {
        rect(g, 4, 0, 1, 4, 'M');
        rect(g, 2, 4, 5, 1, 'M');
        rect(g, 1, 5, 7, 6, 'u');
        rect(g, 2, 6, 5, 4, 'l');
        rect(g, 3, 7, 3, 2, 'L');
        rect(g, 1, 11, 7, 1, 'U');
        rect(g, 3, 12, 3, 1, 'M');
    }),
    lampOff: () => prop(9, 14, g => {
        rect(g, 4, 0, 1, 4, 'M');
        rect(g, 2, 4, 5, 1, 'M');
        rect(g, 1, 5, 7, 6, 'U');
        rect(g, 2, 6, 5, 4, 'K');
        rect(g, 1, 11, 7, 1, 'M');
        rect(g, 3, 12, 3, 1, 'M');
    }),
    cable: () => prop(34, 8, g => {
        for (let x = 0; x < 34; x++) {
            const y = Math.round(2 + Math.sin(x / 5) * 1.6 + 1);
            px(g, x, y, 'K');
        }
        rect(g, 10, 4, 1, 2, 'M'); disc(g, 10, 7, 1.6, 'l');
        rect(g, 26, 4, 1, 2, 'M'); disc(g, 26, 7, 1.6, 'l');
    }),
    toolRack: () => prop(16, 20, g => {
        rect(g, 0, 0, 16, 2, 't');
        rect(g, 2, 2, 1, 14, 'T'); rect(g, 2, 15, 5, 2, 'm');   // shovel
        rect(g, 8, 2, 1, 12, 'T'); rect(g, 6, 13, 6, 2, 'm');   // pick
        rect(g, 13, 2, 1, 10, 'T'); rect(g, 12, 11, 3, 3, 'M'); // hammer
    }),
    barrel: () => prop(12, 14, g => {
        rect(g, 1, 0, 10, 14, 't');
        rect(g, 0, 2, 12, 2, 'M');
        rect(g, 0, 9, 12, 2, 'M');
        rect(g, 2, 0, 1, 14, 'h');
        rect(g, 9, 0, 1, 14, 'T');
    }),
    puddle: () => prop(14, 3, g => {
        rect(g, 1, 1, 12, 2, 'v');
        rect(g, 3, 1, 5, 1, 'Z');
    }),

    // ================================================== refinery
    press: () => prop(34, 30, g => {
        rect(g, 0, 8, 34, 18, 'M');
        rect(g, 0, 8, 34, 1, 'm');
        rect(g, 1, 10, 32, 1, 'K');
        rect(g, 6, 0, 8, 9, 'm');            // piston shaft
        rect(g, 5, 0, 10, 2, 'c');
        rect(g, 7, 2, 2, 7, 'c');
        rect(g, 3, 9, 14, 3, 'M');           // ram head
        rect(g, 19, 12, 12, 9, 'K');         // work window
        rect(g, 20, 13, 10, 7, 'x');         // forge glow
        rect(g, 22, 15, 6, 3, 'L');
        rect(g, 0, 26, 34, 4, 'M');
        for (let x = 2; x < 33; x += 7) rect(g, x, 27, 3, 2, 'K');
        disc(g, 28, 5, 3, 'u'); disc(g, 28, 5, 1.6, 'c');   // flywheel
    }),
    boiler: () => prop(20, 26, g => {
        rect(g, 2, 2, 16, 22, 'M');
        rect(g, 2, 2, 16, 1, 'm');
        rect(g, 3, 4, 14, 2, 'K');
        disc(g, 10, 12, 5, 'K');
        disc(g, 10, 12, 4, 'x');
        disc(g, 10, 12, 2.4, 'L');
        rect(g, 0, 0, 20, 2, 'U');
        rect(g, 4, 20, 12, 2, 'u');
        rect(g, 1, 24, 18, 2, 'M');
    }),
    pipe: () => prop(36, 7, g => {
        rect(g, 0, 1, 36, 5, 'M');
        rect(g, 0, 1, 36, 1, 'm');
        rect(g, 0, 5, 36, 1, 'K');
        rect(g, 7, 0, 4, 7, 'u');
        rect(g, 23, 0, 4, 7, 'u');
    }),
    valve: () => prop(10, 12, g => {
        rect(g, 4, 4, 2, 8, 'M');
        disc(g, 5, 4, 4, 'x');
        disc(g, 5, 4, 2, 'X');
        rect(g, 1, 3, 8, 1, 'x');
    }),
    gauge: () => prop(11, 11, g => {
        disc(g, 5, 5, 5, 'U');
        disc(g, 5, 5, 4, 'u');
        disc(g, 5, 5, 3.2, 'p');
        rect(g, 5, 2, 1, 4, 'X');
        px(g, 5, 5, 'K');
        px(g, 8, 3, 'x');
    }),
    crate: () => prop(14, 12, g => {
        rect(g, 0, 0, 14, 12, 't');
        rect(g, 0, 0, 14, 1, 'h');
        rect(g, 0, 11, 14, 1, 'T');
        rect(g, 0, 5, 14, 1, 'T');
        rect(g, 6, 0, 1, 12, 'T');
        rect(g, 2, 2, 3, 2, 'p');
    }),
    chute: () => prop(16, 20, g => {
        rect(g, 0, 0, 4, 20, 'M');
        rect(g, 12, 0, 4, 20, 'M');
        rect(g, 1, 0, 1, 20, 'm');
        rect(g, 4, 15, 8, 3, 'K');
        rect(g, 4, 18, 8, 2, 'M');
    }),
    conveyor: () => prop(30, 8, g => {
        rect(g, 0, 2, 30, 4, 'K');
        rect(g, 0, 2, 30, 1, 'M');
        for (let x = 1; x < 29; x += 4) rect(g, x, 3, 2, 2, 'M');
        disc(g, 3, 4, 3, 'M'); disc(g, 26, 4, 3, 'M');
    }),
    steam: () => prop(10, 10, g => {
        disc(g, 4, 7, 3, 'c');
        disc(g, 6, 4, 2.4, 'p');
        disc(g, 4, 1, 1.6, 'p');
    }),
    warnSign: () => prop(12, 10, g => {
        rect(g, 0, 0, 12, 10, 'y');
        rect(g, 1, 1, 10, 8, 'K');
        rect(g, 5, 2, 2, 4, 'y');
        rect(g, 5, 7, 2, 1, 'y');
    }),

    // ================================================== overseer office
    desk: () => prop(38, 17, g => {
        rect(g, 0, 3, 38, 4, 't');
        rect(g, 0, 3, 38, 1, 'h');
        rect(g, 0, 6, 38, 1, 'T');
        rect(g, 2, 7, 5, 10, 't'); rect(g, 2, 7, 1, 10, 'h');
        rect(g, 31, 7, 5, 10, 't'); rect(g, 35, 7, 1, 10, 'T');
        rect(g, 7, 8, 24, 1, 'T');
        rect(g, 8, 9, 8, 5, 'T');            // drawer bank
        rect(g, 9, 10, 6, 1, 'h');
        rect(g, 9, 12, 6, 1, 'h');
    }),
    terminal: () => prop(16, 14, g => {
        rect(g, 0, 0, 16, 11, 'K');
        rect(g, 1, 1, 14, 9, 'e');
        rect(g, 2, 2, 8, 1, 'g');
        rect(g, 2, 4, 11, 1, 'W');
        rect(g, 2, 6, 6, 1, 'W');
        rect(g, 2, 8, 9, 1, 'g');
        rect(g, 6, 11, 4, 1, 'M');
        rect(g, 3, 12, 10, 2, 'M');
    }),
    deskLamp: () => prop(12, 14, g => {
        rect(g, 3, 12, 7, 2, 'U');
        rect(g, 6, 5, 1, 7, 'u');
        rect(g, 4, 1, 8, 4, 'g');
        rect(g, 5, 5, 6, 1, 'l');
        rect(g, 6, 6, 4, 3, 'l');
    }),
    papers: () => prop(11, 5, g => {
        rect(g, 0, 2, 10, 3, 'p');
        rect(g, 1, 1, 10, 3, 'p');
        rect(g, 2, 0, 9, 2, 'P');
        rect(g, 3, 2, 5, 1, 'M');
    }),
    bookshelf: () => prop(26, 34, g => {
        rect(g, 0, 0, 26, 34, 'T');
        rect(g, 1, 1, 24, 32, 't');
        rect(g, 1, 10, 24, 2, 'T');
        rect(g, 1, 21, 24, 2, 'T');
        const books = ['x', 'b', 'g', 'u', 'v', 'X', 'i', 'G'];
        let bi = 0;
        for (const top of [2, 13, 24]) {
            let x = 2;
            while (x < 23) {
                const w = 2 + (bi % 2);
                const h = 7 + (bi % 2);
                rect(g, x, top + (8 - h), w, h, books[bi % books.length]);
                x += w + 1; bi++;
            }
        }
    }),
    cabinet: () => prop(18, 26, g => {
        rect(g, 0, 0, 18, 26, 'M');
        rect(g, 0, 0, 18, 1, 'm');
        for (const y of [2, 10, 18]) {
            rect(g, 1, y, 16, 7, 'K');
            rect(g, 6, y + 3, 6, 1, 'c');
        }
    }),
    corkboard: () => prop(30, 20, g => {
        rect(g, 0, 0, 30, 20, 'U');
        rect(g, 1, 1, 28, 18, 'u');
        rect(g, 3, 3, 8, 6, 'p'); px(g, 7, 3, 'x');
        rect(g, 13, 2, 7, 5, 'p'); px(g, 16, 2, 'f');
        rect(g, 22, 4, 6, 7, 'p'); px(g, 25, 4, 'g');
        rect(g, 5, 11, 9, 6, 'p'); px(g, 9, 11, 'f');
        rect(g, 17, 10, 10, 7, 'P'); px(g, 22, 10, 'x');
    }),
    wallClock: () => prop(12, 12, g => {
        disc(g, 5, 5, 5.5, 'T');
        disc(g, 5, 5, 4.5, 'p');
        rect(g, 5, 2, 1, 4, 'K');
        rect(g, 5, 5, 3, 1, 'K');
        px(g, 5, 5, 'x');
    }),
    pigeonholes: () => prop(20, 18, g => {
        rect(g, 0, 0, 20, 18, 'T');
        for (let cy = 1; cy < 18; cy += 6) {
            for (let cx = 1; cx < 20; cx += 6) {
                rect(g, cx, cy, 5, 5, 'K');
                if ((cx + cy) % 4 === 0) rect(g, cx + 1, cy + 3, 3, 2, 'p');
            }
        }
    }),
    mug: () => prop(7, 7, g => {
        rect(g, 0, 1, 5, 6, 'p');
        rect(g, 0, 1, 5, 1, 'P');
        rect(g, 1, 0, 3, 1, 'c');
        px(g, 5, 3, 'P'); px(g, 6, 3, 'P'); px(g, 5, 4, 'P');
    }),
    chair: () => prop(12, 17, g => {
        rect(g, 1, 0, 10, 9, 't');
        rect(g, 2, 1, 8, 7, 'x');
        rect(g, 0, 9, 12, 3, 't');
        rect(g, 5, 12, 2, 4, 'M');
        rect(g, 2, 16, 8, 1, 'M');
    }),
    plant: () => prop(14, 18, g => {
        rect(g, 4, 13, 6, 5, 'x');
        rect(g, 4, 13, 6, 1, 'X');
        rect(g, 6, 8, 2, 5, 'G');
        disc(g, 4, 8, 3, 'g'); disc(g, 10, 7, 3, 'g'); disc(g, 7, 4, 3.4, 'g');
        px(g, 4, 7, 'G'); px(g, 10, 6, 'G');
    }),
    rug: () => prop(34, 5, g => {
        rect(g, 0, 1, 34, 4, 'X');
        rect(g, 1, 2, 32, 2, 'x');
        for (let x = 3; x < 32; x += 6) rect(g, x, 2, 2, 2, 'u');
    }),
};

// ---------------------------------------------------------------- rasterise
//
// Grids become canvases, which Pixi wraps as nearest-neighbour textures.

function gridToCanvas(g, scale = 1) {
    const h = g.length, w = g[0].length;
    const c = document.createElement('canvas');
    c.width = w * scale;
    c.height = h * scale;
    const ctx = c.getContext('2d');
    for (let y = 0; y < h; y++) {
        for (let x = 0; x < w; x++) {
            const col = PALETTE[g[y][x]];
            if (!col) continue;
            ctx.fillStyle = col;
            ctx.fillRect(x * scale, y * scale, scale, scale);
        }
    }
    return c;
}

window.MSArt = { PALETTE, makeGrid, rect, px, disc, merge, clone, POSES, HATS, TOOLS, ROLE_KIT, PROPS, buildActor, gridToCanvas };
