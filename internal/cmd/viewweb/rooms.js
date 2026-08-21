// rooms.js — three independent PixiJS room sims.
//
// Each Room owns its own PIXI.Application on its own canvas. That independence
// is the whole reason for Pixi here: the previous engine kept one global world,
// which made three separate boxes impossible without fighting it.
//
// Rooms render at a low world resolution and scale up by an INTEGER factor with
// nearest-neighbour filtering, so pixels stay square. World dimensions are
// derived from the box size rather than fixed, and props anchor to the floor
// line and edges, so a room fills its card at any size.

const Art = window.MSArt;

const FLOOR_H = 11;          // floor band height in world pixels

// Scale is derived from the box HEIGHT, because what matters visually is how
// large a clawd reads against the room — not how wide the card is. An 18px
// mascot at scale 4 stands ~1/3 the height of the stage, which is big enough
// to read expressions and tools at a glance.
const TARGET_WORLD_H = 58;
const MIN_SCALE = 3, MAX_SCALE = 6;

function scaleFor(boxH) {
    return Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.round(boxH / TARGET_WORLD_H)));
}

// texture cache — grids are deterministic, so rasterise each one once.
const texCache = new Map();
function tex(key, grid) {
    if (texCache.has(key)) return texCache.get(key);
    const t = PIXI.Texture.from(Art.gridToCanvas(grid));
    if (t.source) {
        t.source.scaleMode = 'nearest';
        t.source.antialias = false;
    }
    texCache.set(key, t);
    return t;
}

function sprite(key, grid) {
    return new PIXI.Sprite(tex(key, grid));
}

function rnd(a, b) { return a + Math.random() * (b - a); }
function pick(arr) { return arr[Math.floor(Math.random() * arr.length)]; }

// ---------------------------------------------------------------- actor

class Actor {
    constructor(room, agent) {
        this.room = room;
        this.agent = agent;                       // { id, role, name, running }
        this.role = agent.role === 'miner' ? 'miner' : agent.role;
        this.poses = Art.buildActor(this.role);

        this.view = new PIXI.Container();
        this.sprites = {};
        for (const [name, g] of Object.entries(this.poses)) {
            const s = sprite(`${this.role}:${name}`, g);
            s.visible = false;
            s.anchor.set(0.5, 1);                 // feet-anchored
            this.view.addChild(s);
            this.sprites[name] = s;
        }

        this.x = rnd(room.stationA, room.stationB);
        this.facing = 1;
        this.pose = 'stand';
        this.queue = [];
        this.timer = 0;
        this.walkPhase = 0;
        this.show('stand');
    }

    show(name) {
        if (this.sprites[this.pose]) this.sprites[this.pose].visible = false;
        this.pose = this.sprites[name] ? name : 'stand';
        const s = this.sprites[this.pose];
        s.visible = true;
        s.scale.x = this.facing;
    }

    // step kinds: {pose, ms} hold a pose; {to} walk to a world x; {fx} fire an effect
    push(step) { this.queue.push(step); }

    interrupt(steps) {
        this.queue = steps.concat(this.queue.slice(0, 2));
        this.timer = 0;
    }

    update(dtMs) {
        if (this.queue.length === 0) this.room.routine(this);
        const step = this.queue[0];
        if (!step) return;

        if (step.to !== undefined) {
            const dx = step.to - this.x;
            const dist = Math.abs(dx);
            if (dist < 1.2) {
                this.x = step.to;
                this.queue.shift();
                return;
            }
            this.facing = dx > 0 ? 1 : -1;
            const speed = (step.speed || 0.022) * dtMs;
            this.x += Math.sign(dx) * Math.min(speed, dist);
            this.walkPhase += dtMs;
            const carrying = step.carry ? 'carry' : null;
            if (carrying) this.show('carry');
            else this.show(this.walkPhase % 320 < 160 ? 'walkA' : 'walkB');
            return;
        }

        if (this.timer === 0) {
            if (step.fx) this.room.effect(step.fx, this);
            this.show(step.pose || 'stand');
            if (step.face !== undefined) {
                this.facing = step.face;
                this.sprites[this.pose].scale.x = this.facing;
            }
        }
        this.timer += dtMs;
        if (this.timer >= (step.ms || 400)) {
            this.timer = 0;
            this.queue.shift();
        }
    }

    render() {
        this.view.x = Math.round(this.x);
        this.view.y = this.room.floorY;
    }
}

// ---------------------------------------------------------------- effects

class Effect {
    constructor(view, ttl, tick) {
        this.view = view;
        this.ttl = ttl;
        this.age = 0;
        this.tick = tick;
    }
    update(dt) {
        this.age += dt;
        if (this.tick) this.tick(this.age / this.ttl, this.view);
        return this.age < this.ttl;
    }
}

// ---------------------------------------------------------------- room

class Room {
    constructor(canvas, kind) {
        this.canvas = canvas;
        this.kind = kind;
        this.actors = [];
        this.effects = [];
        this.ready = false;
    }

    async init() {
        const box = this.canvas.parentElement.getBoundingClientRect();
        const boxW = Math.max(160, Math.floor(box.width));
        const boxH = Math.max(100, Math.floor(box.height));
        this.scale = scaleFor(boxH);
        this.worldW = Math.ceil(boxW / this.scale);
        this.worldH = Math.ceil(boxH / this.scale);
        this.floorY = this.worldH - FLOOR_H;

        this.app = new PIXI.Application();
        await this.app.init({
            canvas: this.canvas,
            width: this.worldW * this.scale,
            height: this.worldH * this.scale,
            backgroundAlpha: 0,
            antialias: false,
        });

        this.world = new PIXI.Container();
        this.world.scale.set(this.scale);
        this.app.stage.addChild(this.world);

        this.bg = new PIXI.Container();
        this.propLayer = new PIXI.Container();
        this.actorLayer = new PIXI.Container();
        this.fxLayer = new PIXI.Container();
        this.world.addChild(this.bg, this.propLayer, this.actorLayer, this.fxLayer);

        this.buildInterior();

        this.app.ticker.add(() => {
            const dt = this.app.ticker.deltaMS;
            for (const a of this.actors) { a.update(dt); a.render(); }
            this.effects = this.effects.filter(e => {
                const alive = e.update(dt);
                if (!alive) e.view.destroy();
                return alive;
            });
        });
        this.ready = true;
    }

    addProp(name, x, y, opts = {}) {
        const p = Art.PROPS[name]();
        const s = sprite(`prop:${name}`, p.g);
        s.x = Math.round(x);
        s.y = Math.round(y);
        if (opts.anchorBottom) s.y = Math.round(y - p.h);
        if (opts.alpha !== undefined) s.alpha = opts.alpha;
        this.propLayer.addChild(s);
        return s;
    }

    band(x, y, w, h, color, alpha = 1) {
        const g = new PIXI.Graphics();
        g.rect(x, y, w, h).fill({ color, alpha });
        this.bg.addChild(g);
        return g;
    }

    buildInterior() {
        const W = this.worldW, H = this.worldH, fy = this.floorY;
        const P = Art.PALETTE;
        const at = (name, x, yFromFloor, opts) => this.addProp(name, x, fy - yFromFloor, opts);

        if (this.kind === 'mineshaft') {
            // rock face, lit from the lamp side and falling off into the dark
            this.band(0, 0, W, fy, P.K);
            this.band(0, 0, W, 8, P.k, .55);
            for (let i = 0; i < Math.ceil(W / 34); i++) {
                this.addProp('strata', i * 34 + (i % 2 ? -6 : 2), 6 + (i % 3) * 11);
            }
            // dirt floor over bedrock, with sleeper-bearing rails
            this.band(0, fy, W, H - fy, P.T);
            this.band(0, fy, W, 1, P.t);
            this.band(0, fy + 5, W, H - fy - 5, P.K, .5);

            // timber support frames give the shaft its depth
            at('frame', 4, 34);
            at('frame', W - 52, 34);

            at('seam', W - 22, 26);
            at('rail', 2, -3);
            at('rail', 30, -3);
            at('rail', 58, -3);
            this.addProp('cable', Math.round(W * .18), 1);
            this.lamp = this.addProp('lamp', Math.round(W * .33), 2);

            at('toolRack', 20, 32);
            at('coalPile', W - 46, 9);
            at('barrel', W - 66, 14);
            at('puddle', 44, 3);
            at('oreChunk', 62, 5);
            this.cart = at('oreCart', 4, 15);
            this.cartX = 16;
            this.stationA = W - 34;
            this.stationB = W - 26;

        } else if (this.kind === 'refinery') {
            // riveted plate wall
            this.band(0, 0, W, fy, P.B);
            this.band(0, 0, W, 10, P.n);
            for (let x = 3; x < W; x += 9) this.band(x, 11, 1, fy - 12, P.n, .5);
            for (let x = 6; x < W; x += 18) this.band(x, 13, 1, 1, P.S, .8);
            this.band(0, fy, W, H - fy, P.n);
            this.band(0, fy, W, 1, P.M);

            this.addProp('pipe', 2, 2);
            this.addProp('pipe', W - 40, 2);
            at('valve', 44, 46);
            this.addProp('gauge', W - 16, 12);
            at('warnSign', 22, 40);

            at('boiler', 4, 26);
            at('press', W - 44, 30);
            at('chute', W - 18, 20);
            at('conveyor', 26, 2);
            this.crate = at('crate', 26, 12);
            at('crate', 38, 12);
            this.crateX = 32;
            this.stationA = W - 38;
            this.stationB = W - 30;

        } else {
            // panelled office: dark upper wall over a timber wainscot
            this.band(0, 0, W, fy, P.B);
            this.band(0, fy - 14, W, 14, P.T);
            this.band(0, fy - 14, W, 1, P.h);
            for (let x = 4; x < W; x += 12) this.band(x, fy - 13, 1, 12, P.t, .7);

            this.band(0, fy, W, H - fy, P.T);
            this.band(0, fy, W, 1, P.t);
            at('rug', Math.round(W / 2 - 17), 4);

            at('bookshelf', 3, 34);
            at('cabinet', 32, 26);
            this.addProp('corkboard', Math.round(W / 2 + 4), 5);
            this.addProp('wallClock', Math.round(W / 2 - 12), 6);
            at('pigeonholes', W - 24, 32);
            at('plant', W - 44, 18);

            const dx = Math.round(W / 2 - 19);
            this.desk = at('desk', dx, 17);
            at('terminal', dx + 4, 30);
            at('deskLamp', dx + 24, 30);
            at('papers', dx + 21, 17);
            at('mug', dx + 33, 18);
            at('chair', dx + 12, 17);

            this.deskX = dx + 14;
            this.mailX = W - 16;
            this.stationA = Math.round(W / 2 - 24);
            this.stationB = Math.round(W / 2 + 14);
        }

        // contact shadow ties the figures to the ground
        this.band(0, fy - 1, W, 1, P.e, 0.28);
    }

    // ------------------------------------------------------------ routines
    //
    // Each routine is a randomised chain of 6-10 steps. Order and dwell times
    // vary per cycle so no two passes look identical.

    routine(actor) {
        const jitter = () => rnd(0.8, 1.35);
        const fidget = () => pick([
            { pose: 'stretch', ms: 700 * jitter() },
            { pose: 'blink', ms: 200 },
            { pose: 'stand', ms: 600 * jitter() },
        ]);

        if (this.kind === 'mineshaft') {
            const face = rnd(this.stationA, this.stationB);
            actor.push({ to: face });
            const swings = 2 + Math.floor(Math.random() * 3);
            for (let i = 0; i < swings; i++) {
                actor.push({ pose: 'workUp', ms: 190 * jitter(), face: 1 });
                actor.push({ pose: 'workHit', ms: 150 * jitter(), face: 1, fx: 'strike' });
            }
            actor.push({ pose: 'reach', ms: 320, face: 1 });
            actor.push({ to: this.cartX, carry: true, speed: 0.016 });
            actor.push({ pose: 'reach', ms: 300, face: -1, fx: 'dump' });
            actor.push(fidget());
            if (Math.random() < 0.5) actor.push(fidget());

        } else if (this.kind === 'refinery') {
            actor.push({ to: this.crateX });
            actor.push({ pose: 'reach', ms: 300, face: -1 });
            actor.push({ to: rnd(this.stationA, this.stationB), carry: true, speed: 0.016 });
            const pulls = 1 + Math.floor(Math.random() * 3);
            for (let i = 0; i < pulls; i++) {
                actor.push({ pose: 'workUp', ms: 240 * jitter(), face: 1 });
                actor.push({ pose: 'workHit', ms: 200 * jitter(), face: 1, fx: 'spark' });
            }
            actor.push({ pose: 'reach', ms: 380, face: 1 });     // inspect
            actor.push({ pose: 'workHit', ms: 220, face: 1, fx: 'stamp' });
            actor.push(fidget());

        } else {
            const atDesk = Math.random() < 0.6;
            if (atDesk) {
                actor.push({ to: this.deskX });
                const types = 3 + Math.floor(Math.random() * 4);
                for (let i = 0; i < types; i++) {
                    actor.push({ pose: 'sit', ms: 260 * jitter() });
                    actor.push({ pose: 'workUp', ms: 200 * jitter() });
                }
                actor.push({ pose: 'sit', ms: 700 * jitter() });
            } else {
                actor.push({ to: this.mailX });
                actor.push({ pose: 'reach', ms: 350, face: 1, fx: 'mail' });
                actor.push({ pose: 'workUp', ms: 400 });
                actor.push({ to: rnd(this.stationA, this.stationB) });
            }
            actor.push(fidget());
        }
    }

    // ------------------------------------------------------------ effects

    effect(kind, actor) {
        const P = Art.PALETTE;
        const at = actor ? { x: actor.x, y: this.floorY } : { x: this.worldW / 2, y: this.floorY - 20 };

        if (kind === 'strike' || kind === 'spark') {
            const g = new PIXI.Graphics();
            const cx = at.x + (kind === 'spark' ? 10 : 14);
            const cy = at.y - (kind === 'spark' ? 12 : 8);
            const col = kind === 'spark' ? P.y : P.s;
            for (let i = 0; i < 5; i++) {
                const a = rnd(-Math.PI, 0), d = rnd(2, 6);
                g.rect(cx + Math.cos(a) * d, cy + Math.sin(a) * d, 1, 1).fill({ color: col });
            }
            this.fxLayer.addChild(g);
            this.effects.push(new Effect(g, 260, (t, v) => { v.alpha = 1 - t; }));
            return;
        }
        if (kind === 'dump') {
            this.fillCart();
            return;
        }
        if (kind === 'stamp' || kind === 'mail' || kind === 'flash') {
            const g = new PIXI.Graphics();
            const col = kind === 'flash' ? P.r : P.w;
            g.rect(at.x - 3, at.y - 26, 6, 5).fill({ color: col });
            this.fxLayer.addChild(g);
            this.effects.push(new Effect(g, 500, (t, v) => { v.alpha = 1 - t; v.y = -t * 6; }));
            return;
        }
    }

    // cart visibly fills as ore is delivered, then resets when full
    fillCart() {
        if (this.kind !== 'mineshaft' || !this.cart) return;
        this.cartFill = (this.cartFill || 0) + 1;
        if (this.cartFill >= 3) {
            this.cartFill = 0;
            this.swapCart('oreCart');
        } else if (this.cartFill === 1) {
            this.swapCart('oreCartFull');
        }
    }

    swapCart(name) {
        const p = Art.PROPS[name]();
        this.cart.texture = tex(`prop:${name}`, p.g);
    }

    // react to a live feed event — brief, non-destructive interruption
    react(type) {
        if (!this.ready) return;
        const a = pick(this.actors);
        if (type === 'escalation' || type === 'critical') {
            this.effect('flash', a);
            if (a) a.interrupt([{ pose: 'stretch', ms: 420 }]);
        } else if (type === 'mail') {
            this.effect('mail', a);
        } else {
            this.effect('spark', a);
        }
    }

    // ------------------------------------------------------------ population

    setAgents(agents) {
        if (!this.ready) return;
        const wanted = agents.filter(a => a.running);
        const have = new Map(this.actors.map(a => [a.agent.id, a]));

        // remove actors whose agent stopped or vanished
        for (const [id, actor] of have) {
            if (!wanted.find(a => a.id === id)) {
                this.actorLayer.removeChild(actor.view);
                actor.view.destroy({ children: true });
                this.actors = this.actors.filter(x => x !== actor);
            }
        }
        // add newcomers
        for (const agent of wanted) {
            if (have.has(agent.id)) continue;
            const actor = new Actor(this, agent);
            this.actorLayer.addChild(actor.view);
            this.actors.push(actor);
        }

        // mineshaft goes dark when nobody is working it
        if (this.kind === 'mineshaft' && this.lamp) {
            const empty = this.actors.length === 0;
            const p = Art.PROPS[empty ? 'lampOff' : 'lamp']();
            this.lamp.texture = tex(`prop:${empty ? 'lampOff' : 'lamp'}`, p.g);
            this.world.alpha = empty ? 0.55 : 1;
        }
        return this.actors.length;
    }

    resize() {
        if (!this.ready) return;
        const box = this.canvas.parentElement.getBoundingClientRect();
        const boxW = Math.max(160, Math.floor(box.width));
        const boxH = Math.max(100, Math.floor(box.height));
        const scale = scaleFor(boxH);
        const worldW = Math.ceil(boxW / scale);
        const worldH = Math.ceil(boxH / scale);
        if (scale === this.scale && worldW === this.worldW && worldH === this.worldH) return;

        this.scale = scale;
        this.worldW = worldW;
        this.worldH = worldH;
        this.floorY = worldH - FLOOR_H;
        this.app.renderer.resize(worldW * scale, worldH * scale);
        this.world.scale.set(scale);
        this.bg.removeChildren().forEach(c => c.destroy());
        this.propLayer.removeChildren().forEach(c => c.destroy());
        this.buildInterior();
    }
}

window.MSRooms = { Room };
