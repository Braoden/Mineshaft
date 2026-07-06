// Pixel art data for the Mineshaft view. Pure data, no engine deps
// (validated standalone under node by the build check).
'use strict';

const SPRITE_PALETTE = {
    o: '#da7756', // clawd body (claude orange)
    O: '#b3552f', // body shade
    k: '#2a1a12', // dark: eyes, outlines
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
    d: '#241813', // dark opening
    c: '#e8e4d8', // cloud / paper
    m: '#7a7466', // metal
};

// Clawd frames are 16x18 (2 rows of headroom for the pickaxe).
const SPRITE_DATA = {

clawd_idle: [
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..ookkooookkoo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'...oooooooooo...',
'....oooooooo....',
'.....OO..OO.....',
'.....OO..OO.....',
'................',
],

clawd_blink: [
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..oooooooooooo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'...oooooooooo...',
'....oooooooo....',
'.....OO..OO.....',
'.....OO..OO.....',
'................',
],

clawd_walk1: [
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..ookkooookkoo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'...oooooooooo...',
'....oooooooo....',
'....OO....OO....',
'....OO.....OO...',
'................',
],

clawd_walk2: [
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..ookkooookkoo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'...oooooooooo...',
'....oooooooo....',
'......OOOO......',
'.....OO..OO.....',
'................',
],

clawd_mine1: [
'.........ssss...',
'........ssssss..',
'.........Snn....',
'..........nn....',
'....ooooooonn...',
'...ooooooooonn..',
'..oooooooooooo..',
'..ookkooookkoo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'...oooooooooo...',
'....oooooooo....',
'.....OO..OO.....',
'.....OO..OO.....',
'................',
],

clawd_mine2: [
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..ookkooookkoo..',
'oooOkkooookkOooo',
'oo.ookkkkkkoo.oo',
'.o.ooooooooonn..',
'...ooookkooonn..',
'...ooooooooonns.',
'...oooooooonnss.',
'....oooooooosss.',
'.....OO..OO.ss..',
'.....OO..OO.....',
'................',
],

clawd_sleep: [
'................',
'................',
'................',
'................',
'................',
'................',
'....oooooooo....',
'...oooooooooo...',
'..oooooooooooo..',
'..oooooooooooo..',
'oooOkkooookkOooo',
'oo.oooooooooo.oo',
'.o.oooooooooo.o.',
'...ooookkoooo...',
'...oooooooooo...',
'....oooooooo....',
'.....OO..OO.....',
'................',
],

// accessories, 16x18, aligned over clawd frames
hat_hard: [
'................',
'................',
'......yyyy......',
'....yyyyyyyy....',
'...yYyyyyyyYy...',
'...YYYYYYYYYY...',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
],

hat_hard_g: [
'................',
'................',
'......gggg......',
'....gggggggg....',
'...gGggggggGg...',
'...GGGGGGGGGG...',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
],

hat_top: [
'................',
'.....kkkkkk.....',
'.....kkkkkk.....',
'.....kkkkkk.....',
'....kkkkkkkk....',
'...kkkkkkkkkk...',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
],

hat_cap: [
'................',
'................',
'......bbbb......',
'....bbbbbbbb....',
'...bBbbbbbbbbb..',
'...BBBBBBBBBBb..',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
],

lantern: [
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'.mm.............',
'.myy............',
'.myy............',
'.mm.............',
'................',
'................',
'................',
'................',
'................',
],

apron: [
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'................',
'....bbbbbbbb....',
'....bbbbbbbb....',
'....bBBBBBBb....',
'....bbbbbbbb....',
'................',
'................',
'................',
'................',
],

// buildings
hq: [
'............rr............',
'............rr............',
'.....nnnnnnnnnnnnnnnn.....',
'...nnnnnnnnnnnnnnnnnnnn...',
'.nnnnnnnnnnnnnnnnnnnnnnnn.',
'.NNNNNNNNNNNNNNNNNNNNNNNN.',
'..cccccccccccccccccccccc..',
'..cbbccccccccccccccccbbc..',
'..cbbccccccccccccccccbbc..',
'..cccccccccddddccccccccc..',
'..cccccccccddddccccccccc..',
'..cccccccccddddccccccccc..',
'..cccccccccddddccccccccc..',
'..NNNNNNNNNNNNNNNNNNNNNN..',
],

bunkhouse: [
'..........................',
'...nnnnnnnnnnnnnnnnnnnn...',
'..nnnnnnnnnnnnnnnnnnnnnn..',
'.NNNNNNNNNNNNNNNNNNNNNNNN.',
'.nnnnnnnnnnnnnnnnnnnnnnnn.',
'.nn.bb.nnnn.dd.nnnn.bb.nn.',
'.nn.bb.nnn.dddd.nnn.bb.nn.',
'.nnnnnnnnn.dddd.nnnnnnnnn.',
'.nnnnnnnnn.dddd.nnnnnnnnn.',
'.NNNNNNNNNNNNNNNNNNNNNNNN.',
],

mine: [
'......................',
'..nn..............nn..',
'..nnnn..........nnnn..',
'..nnnnnnnnnnnnnnnnnn..',
'..nnNNNNNNNNNNNNNNnn..',
'..nn.ssssssssssss.nn..',
'..nn.sdddddddddds.nn..',
'..nn.sdddddddddds.nn..',
'..nnnsdddddddddssnn...',
'..nn.sdddddddddds.nn..',
'..nn.sdddddddddds.nn..',
'..nnnsddddddddddsnnn..',
'.ssssssssssssssssssss.',
'.ssSSssSSssSSssSSssSS.',
],

refinery: [
'........ss..........',
'........ss..........',
'......ssssss........',
'......ssssss........',
'...ssssssssssss.....',
'..ssssssssssssss....',
'..sSSSSSSSSSSSSs....',
'..ssssssssssssss....',
'..ss.rryyrr..sss....',
'..ss.ryyyyr..sss....',
'..ss.rryyrr..sss....',
'..ssssssssssssss....',
'..sSSssSSssSSssS....',
'.mmmmmmmmmmmmmmmm...',
],

tower: [
'.yy.......',
'yyyy......',
'.yy.......',
'.nnnnnnnn.',
'.nn.dd.nn.',
'.nnnnnnnn.',
'...nn.....',
'...nn.....',
'...nn.....',
'..nnnn....',
'..nnnn....',
'.nnnnnn...',
],

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
if (typeof module !== 'undefined') module.exports = { SPRITE_PALETTE, SPRITE_DATA };
