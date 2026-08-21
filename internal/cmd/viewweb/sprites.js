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
    // --- mineshaft
    oreCart: () => prop(22, 14, g => {
        rect(g, 1, 2, 20, 7, 'b');
        rect(g, 1, 2, 20, 1, 'i');
        rect(g, 2, 9, 18, 1, 'e');
        disc(g, 5, 11, 2.4, 's'); disc(g, 5, 11, 1, 'e');
        disc(g, 16, 11, 2.4, 's'); disc(g, 16, 11, 1, 'e');
    }),
    oreCartFull: () => prop(22, 14, g => {
        rect(g, 1, 2, 20, 7, 'b');
        rect(g, 1, 2, 20, 1, 'i');
        rect(g, 3, 0, 4, 2, 's'); rect(g, 8, 0, 5, 2, 'S'); rect(g, 14, 0, 4, 2, 's');
        rect(g, 2, 9, 18, 1, 'e');
        disc(g, 5, 11, 2.4, 's'); disc(g, 5, 11, 1, 'e');
        disc(g, 16, 11, 2.4, 's'); disc(g, 16, 11, 1, 'e');
    }),
    timber: () => prop(6, 26, g => {
        rect(g, 0, 0, 6, 26, 'S');
        rect(g, 1, 0, 1, 26, 's');
        rect(g, 0, 0, 6, 2, 's');
    }),
    seam: () => prop(14, 20, g => {
        rect(g, 0, 0, 14, 20, 'n');
        rect(g, 2, 3, 4, 2, 'i'); rect(g, 7, 6, 5, 2, 'b');
        rect(g, 3, 10, 4, 2, 'i'); rect(g, 8, 14, 4, 2, 'b');
        rect(g, 2, 17, 5, 2, 'i');
    }),
    oreChunk: () => prop(5, 4, g => { rect(g, 0, 1, 5, 3, 's'); rect(g, 1, 0, 3, 1, 'i'); }),
    rail: () => prop(24, 3, g => {
        rect(g, 0, 0, 24, 1, 's');
        for (let x = 1; x < 24; x += 4) rect(g, x, 1, 2, 2, 'S');
    }),
    lamp: () => prop(8, 12, g => {
        rect(g, 3, 0, 2, 3, 'S');
        rect(g, 1, 3, 6, 2, 's');
        rect(g, 2, 5, 4, 4, 'y');
        rect(g, 3, 6, 2, 2, 'w');
        rect(g, 1, 9, 6, 1, 'S');
    }),
    lampOff: () => prop(8, 12, g => {
        rect(g, 3, 0, 2, 3, 'S');
        rect(g, 1, 3, 6, 2, 'S');
        rect(g, 2, 5, 4, 4, 'n');
        rect(g, 1, 9, 6, 1, 'S');
    }),

    // --- refinery
    press: () => prop(26, 24, g => {
        rect(g, 0, 6, 26, 14, 'b');
        rect(g, 0, 6, 26, 1, 'i');
        rect(g, 4, 0, 6, 7, 'S');        // ram
        rect(g, 3, 0, 8, 2, 's');
        rect(g, 2, 20, 22, 2, 'e');
        rect(g, 15, 10, 8, 6, 'n');      // work window
        rect(g, 16, 11, 6, 4, 'i');
    }),
    pipe: () => prop(30, 6, g => {
        rect(g, 0, 1, 30, 4, 'S');
        rect(g, 0, 1, 30, 1, 's');
        rect(g, 6, 0, 3, 6, 's');
        rect(g, 20, 0, 3, 6, 's');
    }),
    gauge: () => prop(10, 10, g => {
        disc(g, 4.5, 4.5, 4.5, 'S');
        disc(g, 4.5, 4.5, 3.2, 'w');
        rect(g, 4, 2, 1, 3, 'e');
        px(g, 4, 4, 'e');
    }),
    crate: () => prop(12, 10, g => {
        rect(g, 0, 0, 12, 10, 'b');
        rect(g, 0, 0, 12, 1, 'i');
        rect(g, 0, 4, 12, 1, 'e');
        rect(g, 5, 0, 2, 10, 'e');
    }),
    chute: () => prop(14, 16, g => {
        rect(g, 0, 0, 3, 16, 'S');
        rect(g, 11, 0, 3, 16, 'S');
        rect(g, 3, 12, 8, 2, 's');
    }),

    // --- overseer office
    desk: () => prop(28, 14, g => {
        rect(g, 0, 3, 28, 3, 'b');
        rect(g, 0, 3, 28, 1, 'i');
        rect(g, 2, 6, 3, 8, 'S');
        rect(g, 23, 6, 3, 8, 'S');
        rect(g, 5, 7, 18, 1, 'S');
    }),
    terminal: () => prop(14, 12, g => {
        rect(g, 1, 0, 12, 9, 'e');
        rect(g, 2, 1, 10, 7, 'b');
        rect(g, 3, 2, 6, 1, 'w');
        rect(g, 3, 4, 8, 1, 'W');
        rect(g, 3, 6, 5, 1, 'W');
        rect(g, 5, 9, 4, 2, 'S');
        rect(g, 3, 11, 8, 1, 'S');
    }),
    shelf: () => prop(20, 18, g => {
        rect(g, 0, 0, 20, 18, 'n');
        rect(g, 0, 5, 20, 1, 'S');
        rect(g, 0, 11, 20, 1, 'S');
        rect(g, 1, 1, 2, 4, 'b'); rect(g, 4, 1, 2, 4, 'i'); rect(g, 7, 2, 2, 3, 'b');
        rect(g, 1, 7, 2, 4, 'i'); rect(g, 4, 7, 2, 4, 'b');
        rect(g, 12, 7, 3, 4, 'S');
        rect(g, 1, 13, 2, 4, 'b'); rect(g, 4, 13, 2, 4, 'i');
    }),
    mailSlot: () => prop(12, 14, g => {
        rect(g, 0, 0, 12, 14, 'b');
        rect(g, 0, 0, 12, 1, 'i');
        rect(g, 2, 4, 8, 2, 'e');
        rect(g, 2, 9, 8, 2, 'e');
    }),
    mug: () => prop(6, 6, g => {
        rect(g, 0, 1, 5, 5, 'w');
        rect(g, 0, 1, 5, 1, 'W');
        px(g, 5, 2, 'W'); px(g, 5, 3, 'W');
    }),
    chair: () => prop(10, 14, g => {
        rect(g, 1, 0, 8, 8, 'b');
        rect(g, 1, 8, 8, 2, 'i');
        rect(g, 4, 10, 2, 4, 'S');
        rect(g, 2, 13, 6, 1, 'S');
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
