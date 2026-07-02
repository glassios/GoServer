/* Snapshot -> B adapter for the "Order New Dawn" battle client.
   Reuses the /play/ NetClient transport (WS + snapshots + fire_events) and builds the
   simulation object `B` that battle-gl.js renders each frame. The server is authoritative for
   ship state (x/y/angle/hull/flux/shield/overload); projectiles, beams and particles are a
   client-local COSMETIC layer reconstructed from thin-channel fire events (IMPLEMENTATION_PLAN
   §2.2). Coordinates are mapped from the server's world (arena ±3000) into the prototype's
   normalized world (0..3) so all of battle-gl.js's tuned constants stay valid. */
(function () {
  'use strict';

  // Server world units -> normalized world. Arena is ~6000 wide centred on 0; /WDIV + WORLD/2
  // puts server 0 at the arena centre (1.5) and ±3000 at the 0..3 edges.
  var WDIV = 2000, WORLD = 3, CENTER = WORLD / 2;
  var toW = function (v) { return v / WDIV + CENTER; };
  var toWLen = function (v) { return v / WDIV; };

  // ship_type -> hull sprite + normalized radius + aspect (H/W, from hulls.json defaults).
  var HULL = {
    fighter:     { tex: 'frig', radius: 15, aspect: 0.847 },
    interceptor: { tex: 'frig', radius: 15, aspect: 0.847 },
    frigate:     { tex: 'frig', radius: 16, aspect: 0.847 },
    pirate:      { tex: 'ship', radius: 16, aspect: 0.391 },
    miner:       { tex: 'ship', radius: 18, aspect: 0.391 },
    patrol:      { tex: 'ship', radius: 18, aspect: 0.391 },
    cargo:       { tex: 'ship', radius: 22, aspect: 0.391 },
    cargo_helper:{ tex: 'ship', radius: 22, aspect: 0.391 },
    destroyer:   { tex: 'dest', radius: 26, aspect: 0.493 },
    cruiser:     { tex: 'dest', radius: 30, aspect: 0.493 },
    _default:    { tex: 'ship', radius: 18, aspect: 0.391 },
  };
  var hullFor = function (shipType) { return HULL[shipType] || HULL._default; };

  // Per-hull geometry (engine-nozzle anchors as [forward, lateral] fractions of the sprite quad,
  // from hulls.json) — used to spawn engine-exhaust flames at the right sockets.
  var GEO = {
    ship: { aspect: 0.3915,
      engines: [[-0.400, -0.260], [-0.400, -0.160], [-0.345, 0.000], [-0.400, 0.160], [-0.400, 0.260]],
      turrets: [[0.420, 0.000], [0.150, -0.130], [0.150, 0.130], [-0.050, -0.160], [-0.050, 0.160]] },
    dest: { aspect: 0.4928,
      engines: [[-0.488, -0.290], [-0.5, -0.135], [-0.5, 0.135], [-0.488, 0.290]],
      turrets: [[0.440, -0.040], [0.440, 0.040], [0.180, -0.160], [0.180, 0.160], [-0.050, -0.270], [-0.050, 0.270]] },
    frig: { aspect: 0.8469,
      engines: [[-0.48, 0.0], [-0.5, -0.130], [-0.5, 0.130], [-0.47, -0.422], [-0.47, 0.422]],
      turrets: [[0.460, 0.000], [0.120, -0.130], [0.120, 0.130], [-0.050, -0.160], [-0.050, 0.160]] },
  };
  // Turret sprite keys (battle-gl loads these from textures/sprites/); cycled across a hull's mounts.
  var TURRET_TEX = ['t2m', 'tlas', 't2c', 'trk', 't2l', 't2m'];

  var TAU = Math.PI * 2;
  var lerp = function (a, b, t) { return a + (b - a) * t; };
  function angLerp(a, b, t) {
    var d = b - a; while (d > Math.PI) d -= TAU; while (d < -Math.PI) d += TAU;
    return a + d * t;
  }

  function BattleAdapter(net) {
    this.net = net;
    this._s = new Map();   // entityId -> persistent adapter ship state (smoothing, beam, hitT, deploy)
    this._hp = new Map();  // entityId -> last hp (hit-flash detection)
    this.B = {
      ships: [], projs: [], parts: [], warps: [], asteroids: [],
      camX: CENTER, camY: CENTER, zoom: (1 / WORLD * 1.02) * 1.8, t: 0, world: WORLD,
      tCamX: CENTER, tCamY: CENTER, tZoom: (1 / WORLD * 1.02) * 1.8,
    };
    this._framed = false; // one-time auto-frame when the first battle snapshot arrives
    this.selectedId = 0;  // entity id of the ship the player clicked (0 = auto/flagship)
    this._missiles = new Map(); // server missile id -> render obj in B.projs (authoritative, B4)
    var self = this;
    net.onFire = function (fires) { self.onFire(fires); };
    net.onMissiles = function (list) { self.onMissiles(list); };
    // A ship dropping out of a live battle snapshot means it was destroyed → detonate it.
    net.onEntityRemove = function (e) { self._onRemove(e); };
  }

  // Is the client currently looking at a combat instance (system id >= 10000)?
  BattleAdapter.prototype.inBattle = function () {
    return this.net && this.net.currentSystemID >= 10000;
  };

  // Rebuild B.ships / B.asteroids from the live entity map + advance the cosmetic layer.
  // dtF = elapsed frames since last call (1 at 60fps).
  BattleAdapter.prototype.update = function (dtF) {
    var B = this.B, net = this.net;
    B.t += 0.016 * dtF;

    var ships = [], asteroids = [], seen = {};
    var ents = net.entities;
    ents.forEach(function (e) {
      if (e.type === 2) { // asteroid
        asteroids.push({ x: toW(e.targetX), y: toW(e.targetY), rot: e.targetRot || 0, scale: 0.5, par: 0.82 });
        return;
      }
      if (e.type !== 0 && e.type !== 1) return; // only ships (player / npc)
      seen[e.id] = 1;
      ships.push(this._ship(e, dtF));
    }, this);

    // drop persistent state for ships that left the snapshot
    this._s.forEach(function (_, id) { if (!seen[id]) { this._s.delete(id); this._hp.delete(id); } }, this);

    B.ships = ships;
    B.asteroids = asteroids;

    this._emitFlames(dtF);
    this._stepProjectiles(dtF);
    this._stepParticles(dtF);
    this._stepWarps(dtF);
    this._stepShieldHits(dtF);
    this._stepHeat(dtF);

    if (this.inBattle() && !this._framed && ships.length) { this.frameAll(); this._framed = true; }
    if (!this.inBattle()) this._framed = false;

    // camera easing (target -> live), matches the prototype feel
    B.zoom = lerp(B.zoom, B.tZoom, 0.12 * dtF);
    B.camX = lerp(B.camX, B.tCamX, 0.14 * dtF);
    B.camY = lerp(B.camY, B.tCamY, 0.14 * dtF);
  };

  // Build/refresh one ship object from a live entity.
  BattleAdapter.prototype._ship = function (e, dtF) {
    var st = this._s.get(e.id);
    var tx = toW(e.targetX), ty = toW(e.targetY);
    if (!st) {
      st = { x: tx, y: ty, angle: e.targetRot || 0, beam: 0, beamTgt: 0, beamTX: 0, beamTY: 0, hitT: 0, deploy: 0 };
      this._s.set(e.id, st);
    }
    // exponential smoothing toward the authoritative position (snapshots arrive at 20 TPS)
    var k = Math.min(1, 0.32 * dtF);
    st.x = lerp(st.x, tx, k); st.y = lerp(st.y, ty, k);
    st.angle = angLerp(st.angle, e.targetRot || 0, k);

    // hit flash on hull loss
    var last = this._hp.get(e.id);
    if (last != null && e.hp < last) st.hitT = 1;
    this._hp.set(e.id, e.hp);
    if (st.hitT > 0) st.hitT = Math.max(0, st.hitT - 0.08 * dtF);

    // shield deploy ramp (0..1); ~4s to full, collapses when down/overloaded/venting
    var up = e.shield > 0 && !e.overloaded && !e.venting && e.maxShield > 0;
    var rate = (1 / (4 * 60)) * 6; // a touch faster than the prototype so it reads on entry
    st.deploy = up ? Math.min(1, st.deploy + rate * dtF) : Math.max(0, st.deploy - rate * 2 * dtF);
    if (st.beam > 0) st.beam = Math.max(0, st.beam - 0.12 * dtF);

    var hull = hullFor(e.shipType);
    var localF = this.net.localPlayerFactionID;
    var side = (localF && e.factionId === localF) ? 'player' : 'enemy';
    var arc = e.shieldArc >= 360 ? 1.0 : (e.shieldArc > 0 ? e.shieldArc / 360 : 0.62);

    // reuse the persistent object as the render ship so beam/hit state carries frame to frame
    st.side = side;
    st.name = e.name || (e.type === 0 ? 'Ship' : 'Contact');
    st.cls = (e.shipType || 'ship');
    st.radius = hull.radius;
    st.hullTex = hull.tex;
    st.hullAspect = hull.aspect;
    st.dead = e.hp <= 0;
    st.hull = e.hp; st.hullMax = e.maxHp;
    st.flux = e.flux; st.fluxMax = e.maxFlux || 100;
    st.overload = e.overloaded ? 1 : 0;
    st.engineHit = !!e.engineHit; // missile knocked out engines (B4) → no exhaust flames
    st.weaponHit = !!e.weaponHit; // missile suppressed weapons (B4)
    st.shieldOn = up;
    st.shieldDeploy = st.deploy;
    st.shieldArc = arc;
    st.shieldA = e.shieldFacing || st.angle;
    st.heat = st.heat || [];
    st.sHits = st.sHits || [];
    // turret sprites (built once per hull) that aim at the ship's current target
    if (!st._turrets || st._turretHull !== hull.tex) {
      st._turretHull = hull.tex;
      var tg = (GEO[hull.tex] || GEO.ship).turrets || [];
      st._turrets = tg.map(function (t, idx) {
        return { x: t[0], y: t[1], gl: { texKey: TURRET_TEX[idx % TURRET_TEX.length], tint: [1, 1, 1], metal: 0.6, rough: 0.42, scale: 0.14 } };
      });
    }
    st.target = this._s.get(e.targetEntityId) || null;
    st._id = e.id;
    return st;
  };

  // ---- cosmetic layer (fire events -> projectiles / beams) ------------------------------

  BattleAdapter.prototype.onFire = function (fires) {
    if (!Array.isArray(fires)) return;
    for (var i = 0; i < fires.length; i++) {
      var f = fires[i];
      var ox = toW(Number(f.origin_x) || 0), oy = toW(Number(f.origin_y) || 0);
      var txv = toW(Number(f.target_x) || 0), tyv = toW(Number(f.target_y) || 0);
      var cls = f.weapon_class, side = this._sideOf(Number(f.attacker_id));
      var he = (f.damage_type === 'EXPLOSIVE');

      if (cls === 'beam') {
        var sh = this._s.get(Number(f.attacker_id));
        if (sh) {
          sh.beam = 1;
          sh.beamTgt = this._s.get(Number(f.target_id)) || null;
          sh.beamMuzWX = ox; sh.beamMuzWY = oy;
          sh.beamTX = txv; sh.beamTY = tyv;
        }
        continue;
      }
      // Missiles are authoritative flying entities now (B4) — rendered from onMissiles, not here.
      if (cls === 'missile') continue;
      // projectile: a client-simulated tracer (damage already applied server-side)
      var dx = txv - ox, dy = tyv - oy, d = Math.hypot(dx, dy) || 1e-4;
      var spdW = (Number(f.speed) || 600) / WDIV / 60; // world units per frame
      var vx = dx / d * spdW, vy = dy / d * spdW;
      if (this.B.projs.length > 1200) continue;
      this.B.projs.push({
        x: ox, y: oy, vx: vx, vy: vy, side: side,
        type: he ? 'he' : 'gun', r: 2.6, life: d / spdW + 40,
        tgtId: Number(f.target_id),
      });
    }
  };

  // Authoritative flying missiles (B4). The server streams the full live-missile list each tick;
  // we keep a render obj per missile id in B.projs and smooth it toward the reported position. A
  // missile that drops out of the list was hit or shot down by point-defense → spawn an impact.
  BattleAdapter.prototype.onMissiles = function (list) {
    if (!Array.isArray(list)) list = [];
    var B = this.B, seen = {};
    for (var i = 0; i < list.length; i++) {
      var m = list[i], id = Number(m.id);
      seen[id] = true;
      var wx = toW(Number(m.x) || 0), wy = toW(Number(m.y) || 0);
      var he = (m.damage_type === 'EXPLOSIVE');
      var obj = this._missiles.get(id);
      if (!obj) {
        obj = { authoritative: true, missile: true, x: wx, y: wy, tx: wx, ty: wy, side: 'enemy', type: he ? 'he' : 'gun', r: 3.0, life: 1e9 };
        this._missiles.set(id, obj);
        if (B.projs.length <= 1200) B.projs.push(obj);
      }
      obj.tx = wx; obj.ty = wy; obj.he = he;
    }
    var self = this;
    this._missiles.forEach(function (obj, id) {
      if (!seen[id]) {
        self._spawnImpact(obj.x, obj.y, obj.he ? 1.4 : 0.9);
        var idx = B.projs.indexOf(obj); if (idx >= 0) B.projs.splice(idx, 1);
        self._missiles.delete(id);
      }
    });
  };

  BattleAdapter.prototype._sideOf = function (id) {
    var e = this.net.entities.get(id);
    var localF = this.net.localPlayerFactionID;
    if (!e) return 'enemy';
    return (localF && e.factionId === localF) ? 'player' : 'enemy';
  };

  BattleAdapter.prototype._stepProjectiles = function (dtF) {
    var projs = this.B.projs, S = this._s;
    for (var i = projs.length - 1; i >= 0; i--) {
      var p = projs[i];
      // Authoritative missiles (B4): server owns their lifecycle — just ease toward the reported
      // position and face travel; onMissiles adds/removes them.
      if (p.authoritative) {
        var ex = p.tx - p.x, ey = p.ty - p.y;
        p.x += ex * Math.min(1, 0.35 * dtF); p.y += ey * Math.min(1, 0.35 * dtF);
        if (ex || ey) p.ang = Math.atan2(ey, ex);
        continue;
      }
      p.x += p.vx * dtF; p.y += p.vy * dtF; p.life -= dtF;
      if (p.life <= 0) { projs.splice(i, 1); continue; }
      // retire on arrival at the target hull → spawn the impact effect (cosmetic)
      var tg = S.get(p.tgtId);
      if (tg && Math.hypot(tg.x - p.x, tg.y - p.y) < toWLen(45)) { this._impactOn(tg, p); projs.splice(i, 1); }
    }
  };

  BattleAdapter.prototype._stepParticles = function (dtF) {
    var parts = this.B.parts;
    for (var i = parts.length - 1; i >= 0; i--) {
      var p = parts[i];
      p.x += (p.vx || 0) * dtF; p.y += (p.vy || 0) * dtF; p.life -= dtF;
      if (p.life <= 0) parts.splice(i, 1);
    }
  };

  BattleAdapter.prototype._stepWarps = function (dtF) {
    var w = this.B.warps;
    for (var i = w.length - 1; i >= 0; i--) { w[i].t -= 0.008 * dtF; if (w[i].t <= 0) w.splice(i, 1); }
  };

  BattleAdapter.prototype._stepShieldHits = function (dtF) {
    this._s.forEach(function (st) {
      if (!st.sHits || !st.sHits.length) return;
      for (var i = st.sHits.length - 1; i >= 0; i--) { st.sHits[i].t -= 0.05 * dtF; if (st.sHits[i].t <= 0) st.sHits.splice(i, 1); }
    });
  };

  // ---- cosmetic effect spawners --------------------------------------------------------

  // Blue engine-exhaust torches at each nozzle of every living ship. Nozzle world positions use
  // the SAME quad math battle-gl.js uses for turrets (radius*3.4/Hcss world quad), so flames sit
  // exactly on the sprite's engine sockets under any pan/zoom.
  BattleAdapter.prototype._emitFlames = function (dtF) {
    var B = this.B; if (B.parts.length > 3200) return;
    var Hcss = B.gl_Hcss || 700;
    for (var i = 0; i < B.ships.length; i++) {
      var s = B.ships[i]; if (s.dead || s.engineHit) continue; // engine-hit ships coast without exhaust (B4)
      var geo = GEO[s.hullTex] || GEO.ship;
      var wW = s.radius * 3.4 / Hcss, wH = wW * (geo.aspect || 0.44);
      var fwx = Math.cos(s.angle), fwy = Math.sin(s.angle);
      var rgx = -Math.sin(s.angle), rgy = Math.cos(s.angle);
      for (var j = 0; j < geo.engines.length; j++) {
        var fx = geo.engines[j][0], ly = geo.engines[j][1];
        var nx = s.x + fwx * (fx * wW) + rgx * (ly * wH);
        var ny = s.y + fwy * (fx * wW) + rgy * (ly * wH);
        var jit = (Math.random() - 0.5);
        var spd = wW * 0.07 * (0.7 + Math.random() * 0.5);   // world/frame, streaming aft (short torch)
        var bvx = -fwx * spd + rgx * jit * wH * 0.10;
        var bvy = -fwy * spd + rgy * jit * wH * 0.10;
        B.parts.push({ flame: true, core: true, x: nx, y: ny, vx: bvx, vy: bvy, life: 5 + Math.random() * 3, life0: 8, r: 1.5 + Math.random() * 0.7 });
        B.parts.push({ flame: true, core: false, x: nx, y: ny, vx: bvx * 0.85, vy: bvy * 0.85, life: 7 + Math.random() * 5, life0: 12, r: 2.2 + Math.random() * 1.2 });
      }
    }
  };

  // Impact bloom + sparks at (x,y). scale ~ punch (HE > kinetic > shield-graze).
  BattleAdapter.prototype._spawnImpact = function (x, y, scale) {
    var B = this.B; scale = scale || 1;
    B.parts.push({ flash: true, x: x, y: y, vx: 0, vy: 0, life: 10, life0: 10, r0: 22 * scale, col: [1, 0.9, 0.6], li: 1.4 });
    var n = Math.round(5 * scale);
    for (var i = 0; i < n; i++) {
      var a = Math.random() * Math.PI * 2, sp = 0.004 + Math.random() * 0.014;
      B.parts.push({ spark: true, x: x, y: y, vx: Math.cos(a) * sp, vy: Math.sin(a) * sp, life: 14 + Math.random() * 18, r: 3 + Math.random() * 3 });
    }
  };

  // Detonation: fireball + shockwave ring + a bright flash + shrapnel + a space-warp refraction.
  BattleAdapter.prototype._spawnExplosion = function (x, y, radius) {
    var B = this.B, sc = Math.max(0.7, (radius || 18) / 18);
    B.parts.push({ blast: true, x: x, y: y, vx: 0, vy: 0, life: 26, life0: 26, r0: 8 * sc, grow: 46 * sc });
    B.parts.push({ ring: true, x: x, y: y, vx: 0, vy: 0, life: 24, life0: 24, r0: 5 * sc, grow: 52 * sc });
    B.parts.push({ flash: true, x: x, y: y, vx: 0, vy: 0, life: 14, life0: 14, r0: 30 * sc, col: [1, 0.8, 0.45], li: 2.0 });
    for (var i = 0; i < Math.round(14 * sc); i++) {
      var a = Math.random() * Math.PI * 2, sp = 0.006 + Math.random() * 0.03;
      B.parts.push({ spark: true, x: x, y: y, vx: Math.cos(a) * sp, vy: Math.sin(a) * sp, life: 18 + Math.random() * 24, r: 3 + Math.random() * 4 });
    }
    if (B.warps.length < 10) B.warps.push({ x: x, y: y, t: 1 });
  };

  BattleAdapter.prototype._onRemove = function (e) {
    if (!e || !this.inBattle()) return;
    if (e.type !== 0 && e.type !== 1) return;
    var st = this._s.get(e.id);
    if (st) this._spawnExplosion(st.x, st.y, st.radius);
  };

  // Resolve a projectile hit: ripple on a raised, facing shield; else a hull-hit spark burst.
  BattleAdapter.prototype._impactOn = function (tg, p) {
    var ang = Math.atan2(p.y - tg.y, p.x - tg.x);
    var shieldUp = tg.shieldOn && tg.shieldDeploy > 0.2;
    var within = false;
    if (shieldUp) {
      var da = ang - tg.shieldA; while (da > Math.PI) da -= Math.PI * 2; while (da < -Math.PI) da += Math.PI * 2;
      within = Math.abs(da) < (tg.shieldArc || 0.66) * Math.PI * tg.shieldDeploy;
    }
    if (shieldUp && within) {
      var er = tg.radius * 2.1 / (this.B.gl_Hcss || 700);   // shield-bubble radius in world units
      var wx = tg.x + Math.cos(ang) * er, wy = tg.y + Math.sin(ang) * er;
      if (!tg.sHits) tg.sHits = [];
      if (tg.sHits.length < 8) tg.sHits.push({ wx: wx, wy: wy, ang: ang, t: 1 });
      this._spawnImpact(wx, wy, 0.55);
    } else {
      this._spawnImpact(p.x, p.y, p.type === 'he' ? 1.2 : 0.8);
      // smoldering heat mark on the hull at the hit's sprite-UV (shader clips to the hull silhouette)
      var uv = this._hullUV(tg, p.x, p.y);
      if (uv[0] > 0.04 && uv[0] < 0.96 && uv[1] > 0.04 && uv[1] < 0.96) {
        if (!tg.heat) tg.heat = [];
        tg.heat.push({ u: uv[0], v: uv[1], r: 0.10, t: 0.9 });
        if (tg.heat.length > 16) tg.heat.shift();
      }
    }
  };

  // Invert battle-gl's hull sprite transform (scr = center + rotate(angle+π, (aPos-0.5)*size))
  // to get the sprite-UV (0..1) of a world hit point. worldScale = zoom*Hcss; the hull screen quad
  // is radius*3.4*zoom wide × ×aspect tall.
  BattleAdapter.prototype._hullUV = function (s, hx, hy) {
    var ws = this.B.zoom * (this.B.gl_Hcss || 700);
    var dw = s.radius * 3.4 * this.B.zoom, dh = dw * (s.hullAspect || 0.44);
    var soX = (hx - s.x) * ws, soY = (hy - s.y) * ws;
    var ur = s.angle + Math.PI, cs = Math.cos(ur), sn = Math.sin(ur);
    var lx = soX * cs + soY * sn, ly = -soX * sn + soY * cs;   // rotate by -ur
    return [0.5 + lx / dw, 0.5 + ly / dh];
  };

  BattleAdapter.prototype._stepHeat = function (dtF) {
    this._s.forEach(function (st) {
      if (!st.heat || !st.heat.length) return;
      for (var i = st.heat.length - 1; i >= 0; i--) { st.heat[i].t -= 0.006 * dtF; if (st.heat[i].t <= 0) st.heat.splice(i, 1); }
    });
  };

  // ---- camera --------------------------------------------------------------------------

  BattleAdapter.prototype.minZoom = function () { return 1 / this.B.world * 1.02; };

  BattleAdapter.prototype.frameAll = function () {
    var B = this.B, ss = B.ships;
    if (!ss.length) return;
    var minX = 1e9, minY = 1e9, maxX = -1e9, maxY = -1e9;
    for (var i = 0; i < ss.length; i++) {
      var s = ss[i];
      if (s.x < minX) minX = s.x; if (s.x > maxX) maxX = s.x;
      if (s.y < minY) minY = s.y; if (s.y > maxY) maxY = s.y;
    }
    B.tCamX = (minX + maxX) / 2; B.tCamY = (minY + maxY) / 2;
    var spanX = Math.max(0.5, maxX - minX + 0.6), spanY = Math.max(0.5, maxY - minY + 0.6);
    // zoom so the larger span fits ~90% of the view (worldScale = zoom*Hcss maps 1 world unit;
    // 1/span keeps span on screen). Clamp to the arena-fit min and the 9x max.
    var z = Math.min(1 / spanX, 1 / spanY);
    B.tZoom = Math.max(this.minZoom(), Math.min(9, z));
  };

  BattleAdapter.prototype.focusPlayer = function () {
    var sel = this.selectedId && this._s.get(this.selectedId);
    if (sel && !sel.dead) { this.B.tCamX = sel.x; this.B.tCamY = sel.y; return; }
    var ss = this.B.ships, px = 0, py = 0, n = 0;
    for (var i = 0; i < ss.length; i++) if (ss[i].side === 'player' && !ss[i].dead) { px += ss[i].x; py += ss[i].y; n++; }
    if (n) { this.B.tCamX = px / n; this.B.tCamY = py / n; }
    else this.frameAll();
  };

  BattleAdapter.prototype.panScreen = function (dxPx, dyPx, Hcss) {
    var H = Hcss || 700;
    // Y+ is up on screen under the FBO flip (matches the prototype's pan: camX0 - dx, camY0 + dy),
    // so the vertical term is added while the horizontal is subtracted.
    this.B.tCamX -= (dxPx / H) / this.B.tZoom;
    this.B.tCamY += (dyPx / H) / this.B.tZoom;
  };

  BattleAdapter.prototype.zoomBy = function (dir, cnx, cny) {
    var B = this.B;
    var wx = (cnx - 0.5) / B.tZoom + B.tCamX, wy = (cny - 0.5) / B.tZoom + B.tCamY;
    var factor = dir < 0 ? 1.12 : 1 / 1.12;
    B.tZoom = Math.max(this.minZoom(), Math.min(9, B.tZoom * factor));
    B.tCamX = wx - (cnx - 0.5) / B.tZoom;
    B.tCamY = wy - (cny - 0.5) / B.tZoom;
  };

  // ---- HUD helpers ---------------------------------------------------------------------

  // Fleet strength (fraction of summed hull) per side, + ship counts.
  BattleAdapter.prototype.fleetStats = function () {
    var p = { hull: 0, max: 0, n: 0 }, e = { hull: 0, max: 0, n: 0 };
    var ss = this.B.ships;
    for (var i = 0; i < ss.length; i++) {
      var s = ss[i]; var side = s.side === 'player' ? p : e;
      side.hull += Math.max(0, s.hull || 0); side.max += (s.hullMax || 0);
      if (!s.dead) side.n++;
    }
    return { player: p, enemy: e };
  };

  // The local player's flagship if present, else the strongest player ship.
  BattleAdapter.prototype.playerShip = function () {
    var ss = this.B.ships, best = null, localId = this.net.localPlayerID;
    if (this.selectedId) {
      for (var j = 0; j < ss.length; j++) if (ss[j]._id === this.selectedId && !ss[j].dead) return ss[j];
    }
    for (var i = 0; i < ss.length; i++) {
      var s = ss[i];
      if (s._id === localId) return s;
      if (s.side === 'player' && !s.dead && (!best || (s.hull > best.hull))) best = s;
    }
    return best;
  };

  // Nearest living enemy to the given ship (for the right-hand panel).
  BattleAdapter.prototype.nearestEnemy = function (ref) {
    if (!ref) return null;
    var ss = this.B.ships, best = null, bd = 1e9;
    for (var i = 0; i < ss.length; i++) {
      var s = ss[i]; if (s.side !== 'enemy' || s.dead) continue;
      var d = (s.x - ref.x) * (s.x - ref.x) + (s.y - ref.y) * (s.y - ref.y);
      if (d < bd) { bd = d; best = s; }
    }
    return best;
  };

  window.BattleAdapter = BattleAdapter;
})();
