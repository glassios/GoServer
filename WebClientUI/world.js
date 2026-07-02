/*
 * world.js — живой рендер космоса для WebClientUI (framework-agnostic).
 *
 * Точный порт render-loop из cmd/gateway/static/index.html: камера (follow + zoom),
 * интерполяция позиций (lerp по 50мс серверного тика), отрисовка всех типов
 * сущностей, лучи (бой/добыча/assist), частицы, числа урона, выделение, бары
 * HP/щит/флакс, бейджи ролей и focus-fire в бою. Плюс ввод: ЛКМ — выбор цели /
 * waypoint-автопилот, WASD — прямой вектор, колесо — zoom, drag — pan.
 *
 * Не зависит от dc-runtime: читает состояние из NetClient (net.entities и т.д.)
 * и общается с UI через колбэки (onSelect/onLog/onFps/onToggleShoot/...).
 * Цвета var(--accent-*) из референса заменены на конкретные hex (canvas не
 * резолвит CSS-переменные).
 */
(function () {
  'use strict';

  // Игровая палитра (game-semantic, независима от темы страницы).
  const COL = {
    bg: '#05070b',
    cyan: '#00f2fe',
    green: '#39ff14',
    orange: '#ff9f1c',
    magenta: '#ff007f',
    blue: '#007ffd',
    border: '#2b6fae',
  };

  const lerp = (a, b, t) => (1 - t) * a + t * b;

  class Particle {
    constructor(x, y, vx, vy, color, size, life) {
      this.x = x; this.y = y; this.vx = vx; this.vy = vy;
      this.color = color; this.size = size; this.life = life; this.maxLife = life;
    }
    update() { this.x += this.vx; this.y += this.vy; this.life -= 0.016; }
    draw(ctx) {
      ctx.save();
      ctx.globalAlpha = Math.max(0, this.life / this.maxLife);
      ctx.fillStyle = this.color;
      ctx.shadowColor = this.color;
      ctx.shadowBlur = 8;
      ctx.beginPath();
      ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  }

  // Projectile is a purely-cosmetic flying bolt (B3, thin channel). The server already applied
  // the damage instantly and only told us a shot was fired (origin→target, speed, tint); we
  // animate the bolt across world space and retire it when it reaches the target point.
  class Projectile {
    constructor(x, y, tx, ty, speed, color) {
      this.x = x; this.y = y;
      const dx = tx - x, dy = ty - y;
      const d = Math.hypot(dx, dy) || 1;
      this.rot = Math.atan2(dy, dx);
      const perFrame = Math.max(4, speed / 60); // world units per ~60fps frame
      this.vx = (dx / d) * perFrame;
      this.vy = (dy / d) * perFrame;
      this.remaining = d + 24; // travel to the target (+overshoot so it visibly reaches the hull)
      this.color = color || '#ffd36b';
      this.dead = false;
    }
    update() {
      this.x += this.vx; this.y += this.vy;
      this.remaining -= Math.hypot(this.vx, this.vy);
      if (this.remaining <= 0) this.dead = true;
    }
    draw(ctx) {
      ctx.save();
      ctx.translate(this.x, this.y);
      ctx.rotate(this.rot);
      ctx.shadowColor = this.color; ctx.shadowBlur = 8;
      ctx.strokeStyle = this.color; ctx.lineWidth = 2.5;
      ctx.beginPath(); ctx.moveTo(-8, 0); ctx.lineTo(6, 0); ctx.stroke();
      ctx.fillStyle = '#ffffff';
      ctx.beginPath(); ctx.arc(6, 0, 1.8, 0, Math.PI * 2); ctx.fill();
      ctx.restore();
    }
  }

  // Beam is a cosmetic energy-weapon flash (B3): an instant line from shooter to target that
  // fades over a few frames. Driven by beam-class fire events (thin channel), tinted by damage type.
  class Beam {
    constructor(x, y, tx, ty, color) {
      this.x = x; this.y = y; this.tx = tx; this.ty = ty;
      this.color = color || '#42f5b3';
      this.life = 0.14; this.maxLife = 0.14; this.dead = false;
    }
    update() { this.life -= 0.016; if (this.life <= 0) this.dead = true; }
    draw(ctx) {
      const a = Math.max(0, this.life / this.maxLife);
      ctx.save();
      ctx.shadowColor = this.color; ctx.shadowBlur = 11;
      ctx.globalAlpha = 0.5 * a; ctx.strokeStyle = this.color; ctx.lineWidth = 3.5;
      ctx.beginPath(); ctx.moveTo(this.x, this.y); ctx.lineTo(this.tx, this.ty); ctx.stroke();
      ctx.globalAlpha = a; ctx.strokeStyle = '#ffffff'; ctx.lineWidth = 1.3;
      ctx.beginPath(); ctx.moveTo(this.x, this.y); ctx.lineTo(this.tx, this.ty); ctx.stroke();
      ctx.restore();
    }
  }

  // Missile is a cosmetic guided munition (B3/B4-lite): it flies from the shooter and steers
  // toward the *live* target position each frame, so it visibly homes. Purely visual — the server
  // already applied the damage instantly; if the target vanishes it coasts to its last point.
  class Missile {
    constructor(x, y, tx, ty, targetId, speed, color, net) {
      this.x = x; this.y = y; this.tx = tx; this.ty = ty;
      this.net = net; this.targetId = targetId;
      this.rot = Math.atan2(ty - y, tx - x);
      this.spd = Math.max(3, (speed || 300) / 60); // per-frame
      const d = Math.hypot(tx - x, ty - y) || 1;
      this.life = d / this.spd + 120; // frames budget (+ homing slack)
      this.dead = false;
      this.color = color || '#ff8c3a';
    }
    update() {
      let gx = this.tx, gy = this.ty;
      const t = this.net && this.net.entities.get(this.targetId);
      if (t) { gx = t.currX; gy = t.currY; }
      let diff = Math.atan2(gy - this.y, gx - this.x) - this.rot;
      while (diff > Math.PI) diff -= Math.PI * 2;
      while (diff < -Math.PI) diff += Math.PI * 2;
      const turn = 0.13; // rad/frame steering limit
      this.rot += Math.max(-turn, Math.min(turn, diff));
      this.x += Math.cos(this.rot) * this.spd;
      this.y += Math.sin(this.rot) * this.spd;
      this.life--;
      if (Math.hypot(gx - this.x, gy - this.y) < 18 || this.life <= 0) this.dead = true;
    }
    draw(ctx) {
      ctx.save();
      ctx.translate(this.x, this.y);
      ctx.rotate(this.rot);
      ctx.shadowColor = this.color; ctx.shadowBlur = 8;
      ctx.fillStyle = this.color;
      ctx.beginPath(); ctx.moveTo(6, 0); ctx.lineTo(-4, 2.4); ctx.lineTo(-4, -2.4); ctx.closePath(); ctx.fill();
      ctx.restore();
    }
  }

  class DamageNumber {
    constructor(x, y, text, color) {
      this.x = x; this.y = y - 20; this.text = text; this.color = color;
      this.life = 1.0; this.vy = -0.5 - Math.random() * 0.5; this.vx = (Math.random() - 0.5) * 0.6;
    }
    update() { this.x += this.vx; this.y += this.vy; this.life -= 0.02; }
    draw(ctx) {
      ctx.save();
      ctx.font = "bold 13px 'JetBrains Mono', monospace";
      ctx.fillStyle = this.color;
      ctx.shadowColor = 'black'; ctx.shadowBlur = 4;
      ctx.globalAlpha = Math.max(0, this.life);
      ctx.fillText(this.text, this.x, this.y);
      ctx.restore();
    }
  }

  class WorldRenderer {
    constructor(net, opts) {
      this.net = net;
      opts = opts || {};
      // Колбэки в UI (все опциональны).
      this.onSelect = opts.onSelect || null;        // (ent|null) — выбрана/снята цель
      this.onLog = opts.onLog || null;              // (msg, category)
      this.onFps = opts.onFps || null;              // (fps:number)
      this.onToggleShoot = opts.onToggleShoot || null;
      this.onToggleMine = opts.onToggleMine || null;

      this.canvas = null;
      this.ctx = null;

      this.camera = { x: 0, y: 0, zoom: 0.8 };
      this.cameraFollowPlayer = true;
      this.isDragging = false;
      this.dragStart = { x: 0, y: 0 };
      this.cameraStart = { x: 0, y: 0 };
      this._clickStart = { x: 0, y: 0 };
      this._hasMoved = false;

      this.targetID = null;
      this.keysPressed = { w: false, a: false, s: false, d: false };
      this.mouseTargetPos = null;
      this.clickMarker = null;
      this.lastSentMove = { x: 0, y: 0 };

      this.particles = [];
      this.damageNumbers = [];
      this.projectiles = []; // cosmetic flying bolts from projectile fire events (B3)
      this.beams = [];       // cosmetic energy-beam flashes from beam fire events (B3)
      this.missiles = [];    // cosmetic homing missiles from missile fire events (B3/B4-lite)

      this._frameCount = 0;
      this._lastFPS = Date.now();
      this._fps = 60;

      this._raf = null;
      this._inputInterval = null;
      this._started = false;

      // Параллакс-звёзды.
      this.starsSeed = [];
      for (let i = 0; i < 200; i++) {
        this.starsSeed.push({
          x: Math.random() * 4000 - 2000,
          y: Math.random() * 4000 - 2000,
          size: 0.5 + Math.random() * 1.5,
          opacity: 0.15 + Math.random() * 0.7,
        });
      }

      // Эффекты, управляемые сетью (урон/смерть).
      net.onDamageFloat = (x, y, amount, dmgType, isShield) => {
        const col = isShield ? COL.cyan : this._damageTypeColor(dmgType);
        this.damageNumbers.push(new DamageNumber(x, y, `-${amount} ${isShield ? 'SH' : 'HP'}`, col));
        this._sparks(x, y, col);
      };
      net.onEntityRemove = (ent) => {
        if (!ent) return;
        this._explosion(ent.currX, ent.currY, ent.type === 2 ? '#666' : '#ff5533');
        if (ent.id === this.targetID) { this.targetID = null; this._emit(this.onSelect, null); }
      };
      // Выстрелы (B3): сервер шлёт события — ballistic рисуем летящим болтом, energy — лучом.
      net.onFire = (fires) => this._onFireEvents(fires);

      // Привязанные обработчики (для add/removeEventListener).
      this._h = {
        keydown: (e) => this._onKeyDown(e),
        keyup: (e) => this._onKeyUp(e),
        mousemove: (e) => this._onMouseMove(e),
        mouseup: () => { this.isDragging = false; },
        resize: () => this._resize(),
        wheel: (e) => this._onWheel(e),
        mousedown: (e) => this._onMouseDown(e),
        click: (e) => this._onClick(e),
        dblclick: () => this.focusCamera(),
      };
    }

    // Привязать (или переподключить) canvas — экран nav монтируется заново при
    // каждом возврате, поэтому ref может прийти с новым элементом.
    attach(canvas) {
      if (this.canvas === canvas) return;
      this._unbindCanvas();
      this.canvas = canvas;
      this.ctx = canvas.getContext('2d');
      this._resize();
      canvas.addEventListener('wheel', this._h.wheel, { passive: false });
      canvas.addEventListener('mousedown', this._h.mousedown);
      canvas.addEventListener('click', this._h.click);
      canvas.addEventListener('dblclick', this._h.dblclick);
      this.start();
      // Размер контейнера может быть ещё не готов при монтировании — пере-замерим.
      requestAnimationFrame(() => this._resize());
    }

    start() {
      if (this._started) return;
      this._started = true;
      window.addEventListener('keydown', this._h.keydown);
      window.addEventListener('keyup', this._h.keyup);
      window.addEventListener('mousemove', this._h.mousemove);
      window.addEventListener('mouseup', this._h.mouseup);
      window.addEventListener('resize', this._h.resize);
      this._setupInputSending();
      this._raf = requestAnimationFrame(() => this._draw());
    }

    dispose() {
      this._started = false;
      if (this._raf) cancelAnimationFrame(this._raf), (this._raf = null);
      if (this._inputInterval) clearInterval(this._inputInterval), (this._inputInterval = null);
      window.removeEventListener('keydown', this._h.keydown);
      window.removeEventListener('keyup', this._h.keyup);
      window.removeEventListener('mousemove', this._h.mousemove);
      window.removeEventListener('mouseup', this._h.mouseup);
      window.removeEventListener('resize', this._h.resize);
      this._unbindCanvas();
    }

    _unbindCanvas() {
      if (!this.canvas) return;
      this.canvas.removeEventListener('wheel', this._h.wheel);
      this.canvas.removeEventListener('mousedown', this._h.mousedown);
      this.canvas.removeEventListener('click', this._h.click);
      this.canvas.removeEventListener('dblclick', this._h.dblclick);
    }

    _resize() {
      if (!this.canvas) return;
      const rect = this.canvas.getBoundingClientRect();
      this.canvas.width = Math.max(1, Math.floor(rect.width));
      this.canvas.height = Math.max(1, Math.floor(rect.height));
    }

    getTarget() { return this.targetID ? this.net.entities.get(this.targetID) || null : null; }
    setTarget(id) {
      this.targetID = id || null;
      this._emit(this.onSelect, this.getTarget());
    }

    // ---------------------------------------------------------------------
    // Ввод
    // ---------------------------------------------------------------------
    _setupInputSending() {
      if (this._inputInterval) clearInterval(this._inputInterval);
      this.lastSentMove = { x: 0, y: 0 };
      const net = this.net;
      this._inputInterval = setInterval(() => {
        if (net.isSpectator || !net.localPlayerID) return;
        let vx = 0, vy = 0;
        const force = 120.0;
        const k = this.keysPressed;
        if (k.w || k.s || k.a || k.d) this.mouseTargetPos = null;

        if (this.mouseTargetPos) {
          const player = net.entities.get(net.localPlayerID);
          if (player) {
            const dx = this.mouseTargetPos.x - player.currX;
            const dy = this.mouseTargetPos.y - player.currY;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > 10) {
              const angle = Math.atan2(dy, dx);
              vx = Math.cos(angle) * force; vy = Math.sin(angle) * force;
            } else { this.mouseTargetPos = null; }
          }
        } else {
          if (k.w) vy -= force; if (k.s) vy += force;
          if (k.a) vx -= force; if (k.d) vx += force;
        }

        if (vx !== this.lastSentMove.x || vy !== this.lastSentMove.y) {
          net.send({ action: 'move', x: vx, y: vy });
          this.lastSentMove.x = vx; this.lastSentMove.y = vy;
        }
      }, 50);
    }

    focusCamera() {
      this.cameraFollowPlayer = true;
      const p = this._followedEntity();
      if (p) { this.camera.x = p.currX; this.camera.y = p.currY; }
    }

    _followedEntity() {
      const net = this.net;
      if (net.localPlayerID && net.entities.has(net.localPlayerID)) return net.entities.get(net.localPlayerID);
      if (net.localPlayerID && net.localPlayerFactionID !== null) {
        for (const ent of net.entities.values())
          if (ent.factionId === net.localPlayerFactionID && ent.hp > 0) return ent;
      }
      return null;
    }

    _onKeyDown(e) {
      const ae = document.activeElement;
      if (ae && ae.tagName === 'INPUT') return;
      if (e.code === 'KeyW' || e.code === 'ArrowUp') this.keysPressed.w = true;
      if (e.code === 'KeyS' || e.code === 'ArrowDown') this.keysPressed.s = true;
      if (e.code === 'KeyA' || e.code === 'ArrowLeft') this.keysPressed.a = true;
      if (e.code === 'KeyD' || e.code === 'ArrowRight') this.keysPressed.d = true;
      if (e.code === 'KeyC') this.focusCamera();
      if (e.code === 'Space' && this.targetID) { e.preventDefault(); this._emit(this.onToggleShoot); }
      if (e.code === 'KeyF' && this.targetID) this._emit(this.onToggleMine);
    }
    _onKeyUp(e) {
      if (e.code === 'KeyW' || e.code === 'ArrowUp') this.keysPressed.w = false;
      if (e.code === 'KeyS' || e.code === 'ArrowDown') this.keysPressed.s = false;
      if (e.code === 'KeyA' || e.code === 'ArrowLeft') this.keysPressed.a = false;
      if (e.code === 'KeyD' || e.code === 'ArrowRight') this.keysPressed.d = false;
    }
    _onWheel(e) {
      e.preventDefault();
      const f = 1.15;
      if (e.deltaY < 0) this.camera.zoom = Math.min(3.5, this.camera.zoom * f);
      else this.camera.zoom = Math.max(0.15, this.camera.zoom / f);
    }
    _onMouseDown(e) {
      this.isDragging = true;
      this._clickStart.x = e.clientX; this._clickStart.y = e.clientY;
      this.dragStart.x = e.clientX; this.dragStart.y = e.clientY;
      this.cameraStart.x = this.camera.x; this.cameraStart.y = this.camera.y;
      this._hasMoved = false;
    }
    _onMouseMove(e) {
      if (!this.isDragging) return;
      const dx = e.clientX - this.dragStart.x;
      const dy = e.clientY - this.dragStart.y;
      if (Math.abs(e.clientX - this._clickStart.x) > 3 || Math.abs(e.clientY - this._clickStart.y) > 3) this._hasMoved = true;
      this.camera.x = this.cameraStart.x - dx / this.camera.zoom;
      this.camera.y = this.cameraStart.y - dy / this.camera.zoom;
      this.cameraFollowPlayer = false;
    }
    _onClick(e) {
      if (this._hasMoved) return;
      const net = this.net;
      const rect = this.canvas.getBoundingClientRect();
      const mx = e.clientX - rect.left, my = e.clientY - rect.top;
      const worldX = (mx - this.canvas.width / 2) / this.camera.zoom + this.camera.x;
      const worldY = (my - this.canvas.height / 2) / this.camera.zoom + this.camera.y;

      let clicked = null;
      let minDist = 40.0 / this.camera.zoom;
      for (const ent of net.entities.values()) {
        if (ent.id === net.localPlayerID) continue;
        const dx = ent.currX - worldX, dy = ent.currY - worldY;
        const dist = Math.sqrt(dx * dx + dy * dy);
        if (dist < minDist) { minDist = dist; clicked = ent; }
      }

      if (clicked) {
        this.targetID = clicked.id;
        this._emit(this.onSelect, clicked);
        this._log(`Цель выбрана: ${clicked.name} (ID: ${clicked.id})`, 'system');
      } else {
        this.targetID = null;
        this._emit(this.onSelect, null);
        if (!net.isSpectator && net.localPlayerID) {
          this.mouseTargetPos = { x: worldX, y: worldY };
          this.clickMarker = { x: worldX, y: worldY, time: Date.now(), duration: 600 };
        }
      }
    }

    // ---------------------------------------------------------------------
    // Render loop
    // ---------------------------------------------------------------------
    _draw() {
      this._raf = requestAnimationFrame(() => this._draw());
      const ctx = this.ctx, canvas = this.canvas, net = this.net;
      if (!ctx || !canvas) return;

      this._frameCount++;
      const now = Date.now();
      if (now - this._lastFPS >= 1000) {
        this._fps = this._frameCount; this._frameCount = 0; this._lastFPS = now;
        this._emit(this.onFps, this._fps);
      }

      ctx.fillStyle = COL.bg;
      ctx.fillRect(0, 0, canvas.width, canvas.height);

      // 1. Интерполяция позиций + двигательные следы.
      for (const ent of net.entities.values()) {
        const t = Math.min(1.0, (now - ent.lastUpdate) / 50.0);
        ent.currX = lerp(ent.prevX, ent.targetX, t);
        ent.currY = lerp(ent.prevY, ent.targetY, t);
        let diff = ent.targetRot - ent.prevRot;
        while (diff < -Math.PI) diff += Math.PI * 2;
        while (diff > Math.PI) diff -= Math.PI * 2;
        ent.currRot = ent.prevRot + diff * t;

        if (ent.type === 0 || ent.type === 1) {
          const v = Math.sqrt(ent.vx * ent.vx + ent.vy * ent.vy);
          if (v > 5) {
            let trail = ent.type === 0 ? 'rgba(57,255,20,0.4)' : 'rgba(255,159,28,0.4)';
            if (ent.id === net.localPlayerID) trail = 'rgba(0,242,254,0.4)';
            const ox = ent.currX - Math.cos(ent.currRot) * 15;
            const oy = ent.currY - Math.sin(ent.currRot) * 15;
            this.particles.push(new Particle(
              ox + (Math.random() - 0.5) * 4, oy + (Math.random() - 0.5) * 4,
              -Math.cos(ent.currRot) * 1.5 + (Math.random() - 0.5) * 0.5,
              -Math.sin(ent.currRot) * 1.5 + (Math.random() - 0.5) * 0.5,
              trail, 2 + Math.random() * 2, 0.2 + Math.random() * 0.3));
          }
        }
      }

      // 2. Камера.
      if (this.cameraFollowPlayer) {
        const f = this._followedEntity();
        if (f) {
          const dx = f.currX - this.camera.x, dy = f.currY - this.camera.y;
          if (Math.sqrt(dx * dx + dy * dy) > 1000) { this.camera.x = f.currX; this.camera.y = f.currY; }
          else { this.camera.x = lerp(this.camera.x, f.currX, 0.1); this.camera.y = lerp(this.camera.y, f.currY, 0.1); }
        }
      }

      ctx.save();
      ctx.translate(canvas.width / 2, canvas.height / 2);
      ctx.scale(this.camera.zoom, this.camera.zoom);
      ctx.translate(-this.camera.x, -this.camera.y);

      this._drawStars();

      // Маркер клика (автопилот).
      if (this.clickMarker) {
        const elapsed = Date.now() - this.clickMarker.time;
        if (elapsed < this.clickMarker.duration) {
          const p = elapsed / this.clickMarker.duration;
          const radius = 4 + p * 24, alpha = 1.0 - p;
          ctx.save();
          ctx.strokeStyle = `rgba(0,242,254,${alpha})`;
          ctx.lineWidth = 2;
          ctx.beginPath(); ctx.arc(this.clickMarker.x, this.clickMarker.y, radius, 0, Math.PI * 2); ctx.stroke();
          ctx.fillStyle = `rgba(0,242,254,${alpha * 0.7})`;
          ctx.beginPath(); ctx.arc(this.clickMarker.x, this.clickMarker.y, 3, 0, Math.PI * 2); ctx.fill();
          ctx.restore();
        } else { this.clickMarker = null; }
      }

      this._drawBeams();
      for (const ent of net.entities.values()) this._drawEntity(ent);

      // Лучи, ракеты и летящие болты поверх кораблей (B3).
      this.beams = this.beams.filter(b => !b.dead);
      this.beams.forEach(b => { b.update(); b.draw(ctx); });
      this.missiles = this.missiles.filter(m => !m.dead);
      this.missiles.forEach(m => {
        m.update();
        if (Math.random() < 0.7) { // дымный след
          this.particles.push(new Particle(
            m.x - Math.cos(m.rot) * 5, m.y - Math.sin(m.rot) * 5,
            (Math.random() - 0.5) * 0.4, (Math.random() - 0.5) * 0.4,
            'rgba(180,180,190,0.5)', 1.2 + Math.random(), 0.25 + Math.random() * 0.25));
        }
        m.draw(ctx);
      });
      this.projectiles = this.projectiles.filter(p => !p.dead);
      this.projectiles.forEach(p => { p.update(); p.draw(ctx); });

      this.particles = this.particles.filter(p => p.life > 0);
      this.particles.forEach(p => { p.update(); p.draw(ctx); });
      this.damageNumbers = this.damageNumbers.filter(d => d.life > 0);
      this.damageNumbers.forEach(d => { d.update(); d.draw(ctx); });

      ctx.restore();
    }

    _drawStars() {
      const ctx = this.ctx, cam = this.camera;
      ctx.save();
      ctx.fillStyle = 'white';
      const size = 4000, half = size / 2;
      for (const star of this.starsSeed) {
        let sx = star.x - cam.x * 0.1, sy = star.y - cam.y * 0.1;
        sx = ((sx + half) % size); if (sx < 0) sx += size; sx -= half; sx += cam.x;
        sy = ((sy + half) % size); if (sy < 0) sy += size; sy -= half; sy += cam.y;
        ctx.globalAlpha = star.opacity;
        ctx.beginPath(); ctx.arc(sx, sy, star.size, 0, Math.PI * 2); ctx.fill();
      }
      ctx.restore();
    }

    _drawBeams() {
      const ctx = this.ctx, net = this.net;
      ctx.save();
      for (const ent of net.entities.values()) {
        // Assist-лучи (repair зелёный / support голубой).
        if (ent.assistTargetId && net.entities.has(ent.assistTargetId)) {
          const ally = net.entities.get(ent.assistTargetId);
          const isRepair = ent.assistType === 'repair';
          const c = isRepair ? '#39ff88' : '#00d0ff';
          const pulse = 1.5 + Math.sin(Date.now() / 60) * 0.8;
          ctx.save();
          ctx.shadowBlur = 8; ctx.shadowColor = c; ctx.strokeStyle = c;
          ctx.globalAlpha = 0.55; ctx.setLineDash([6, 4]); ctx.lineWidth = pulse + 1.5;
          ctx.beginPath(); ctx.moveTo(ent.currX, ent.currY); ctx.lineTo(ally.currX, ally.currY); ctx.stroke();
          ctx.setLineDash([]); ctx.restore();
        }

        if (ent.targetEntityId && net.entities.has(ent.targetEntityId)) {
          const target = net.entities.get(ent.targetEntityId);

          if (ent.isMining) {
            const pulse = 2 + Math.sin(Date.now() / 30) * 1.5;
            ctx.shadowBlur = 10; ctx.shadowColor = COL.cyan;
            ctx.strokeStyle = 'rgba(0,242,254,0.4)'; ctx.lineWidth = pulse + 4;
            ctx.beginPath(); ctx.moveTo(ent.currX, ent.currY); ctx.lineTo(target.currX, target.currY); ctx.stroke();
            ctx.strokeStyle = '#ffffff'; ctx.lineWidth = pulse; ctx.stroke();
            if (Math.random() < 0.35) {
              const tv = Math.random();
              this.particles.push(new Particle(
                lerp(target.currX, ent.currX, tv), lerp(target.currY, ent.currY, tv),
                (Math.random() - 0.5), (Math.random() - 0.5), COL.cyan, 1.0, 0.2));
            }
          }

          let shotElapsed = Date.now() - (ent.lastShotTime || 0);
          if (ent.isShooting) { ent.lastShotTime = Date.now(); ent.lastShotCount = ent.shotsFired || 1; shotElapsed = 0; }

          // Skip the legacy instant line when this ship's shot is already drawn from a fire event
          // (projectile bolt / beam flash) — otherwise ballistic/energy would render twice (B3).
          const drivenByEvent = ent._firedByEvent && (Date.now() - ent._firedByEvent) < 200;
          if (shotElapsed < 150 && !drivenByEvent) {
            let beamColor = COL.magenta, innerColor = '#ff3385';
            if (net.currentSystemID >= 10000) { beamColor = this._teamColor(ent.factionId); innerColor = '#ffffff'; }
            const opacity = 1.0 - shotElapsed / 150.0;
            const mounts = Math.max(1, ent.lastShotCount || 1);
            ctx.save();
            ctx.shadowBlur = 12; ctx.shadowColor = beamColor;
            ctx.strokeStyle = beamColor; ctx.globalAlpha = 0.4 * opacity; ctx.lineWidth = 4 + (mounts - 1) * 2;
            ctx.beginPath(); ctx.moveTo(ent.currX, ent.currY); ctx.lineTo(target.currX, target.currY); ctx.stroke();
            ctx.globalAlpha = 1.0 * opacity; ctx.strokeStyle = innerColor; ctx.lineWidth = 1.5 + (mounts - 1) * 0.6;
            ctx.beginPath(); ctx.moveTo(ent.currX, ent.currY); ctx.lineTo(target.currX, target.currY); ctx.stroke();
            ctx.restore();
            if (Math.random() < 0.3) {
              this.particles.push(new Particle(
                target.currX, target.currY, (Math.random() - 0.5) * 4, (Math.random() - 0.5) * 4,
                beamColor, 1.0 + Math.random() * 1.5, 0.2 + Math.random() * 0.3));
            }
          }
        }
      }
      ctx.restore();
    }

    _getNPCRanges(ent) {
      let attackRange = 150, pursuitRange = 500;
      if (this.net.currentSystemID >= 10000) {
        pursuitRange = 3000;
        switch (ent.shipType) {
          case 'fighter': attackRange = 500; break;
          case 'patrol': attackRange = 400; break;
          case 'pirate': attackRange = 350; break;
          case 'miner': attackRange = 300; break;
          case 'cargo': case 'cargo_helper': attackRange = 250; break;
          default: attackRange = 300;
        }
      } else {
        pursuitRange = 500;
        const n = ent.name || '';
        if (n.includes('Pirate') || n.includes('Outlaw')) attackRange = 50;
        else if (n.includes('Patrol') || n.includes('Enforcer')) attackRange = 60;
        else if (n.includes('Miner')) attackRange = 80;
        else attackRange = 50;
      }
      return { attackRange, pursuitRange };
    }

    _drawEntity(ent) {
      const ctx = this.ctx, net = this.net;
      ctx.save();
      ctx.translate(ent.currX, ent.currY);

      if (ent.type === 1 && ent.hp > 0) {
        const ranges = this._getNPCRanges(ent);
        ctx.save();
        ctx.strokeStyle = 'rgba(0,242,254,0.15)'; ctx.lineWidth = 1; ctx.setLineDash([4, 6]);
        ctx.beginPath(); ctx.arc(0, 0, ranges.pursuitRange, 0, Math.PI * 2); ctx.stroke(); ctx.restore();
        ctx.save();
        ctx.strokeStyle = (ent.name || '').includes('Miner') ? 'rgba(255,170,0,0.25)' : 'rgba(255,0,127,0.25)';
        ctx.lineWidth = 1; ctx.setLineDash([2, 4]);
        ctx.beginPath(); ctx.arc(0, 0, ranges.attackRange, 0, Math.PI * 2); ctx.stroke(); ctx.restore();
      }

      if (ent.id === this.targetID) {
        ctx.strokeStyle = COL.magenta; ctx.shadowBlur = 10; ctx.shadowColor = COL.magenta; ctx.lineWidth = 1.5;
        ctx.beginPath(); ctx.arc(0, 0, this._radius(ent) + 8, 0, Math.PI * 2); ctx.stroke(); ctx.shadowBlur = 0;
      }

      if (net.currentSystemID >= 10000 && (ent.type === 0 || ent.type === 1)) {
        this._drawShip(ent, this._teamColor(ent.factionId));
      } else {
        switch (ent.type) {
          case 0: this._drawShip(ent, ent.id === net.localPlayerID ? COL.cyan : COL.green); break;
          case 1: {
            let c = COL.orange;
            const n = ent.name || '';
            if (n.includes('Pirate') || n.includes('Outlaw')) c = COL.magenta;
            else if (n.includes('Patrol') || n.includes('Enforcer')) c = COL.blue;
            this._drawShip(ent, c); break;
          }
          case 2: this._drawAsteroid(ent); break;
          case 3: break;
          case 4: this._drawStation(ent); break;
          case 5: this._drawJumpGate(ent); break;
          case 6: this._drawLoot(ent); break;
          case 7: this._drawCombatMarker(ent); break;
          case 8: this._drawSpaceBase(ent); break;
          case 9: this._drawPlanet(ent); break;
        }
      }

      if (ent.type !== 3) {
        ctx.font = "bold 9px 'JetBrains Mono', monospace";
        ctx.fillStyle = '#ffffff'; ctx.textAlign = 'center'; ctx.shadowColor = 'black'; ctx.shadowBlur = 3;
        ctx.fillText(ent.name || '', 0, -this._radius(ent) - 15);

        if (ent.role && net.currentSystemID >= 10000) {
          const info = { tank: { ch: 'T', col: '#5ab0ff' }, dps: { ch: 'D', col: '#ff5d5d' },
            support: { ch: 'S', col: '#00d0ff' }, repair: { ch: 'R', col: '#39ff88' } }[ent.role];
          if (info) {
            ctx.font = "bold 8px 'JetBrains Mono', monospace"; ctx.fillStyle = info.col;
            const stance = ent.strategy === 'retreat' ? ' ⮌' : (ent.strategy === 'defense' ? ' ⛉' : '');
            ctx.fillText(info.ch + stance, 0, -this._radius(ent) - 26);
            ctx.font = "bold 9px 'JetBrains Mono', monospace"; ctx.fillStyle = '#ffffff';
          }
        }

        if ((net.focusCounts[ent.id] || 0) >= 2) {
          const rr = this._radius(ent) + 7, tt = Date.now() / 400;
          ctx.strokeStyle = 'rgba(255,80,80,0.9)'; ctx.lineWidth = 1.5;
          for (let k = 0; k < 4; k++) { const a = tt + k * Math.PI / 2; ctx.beginPath(); ctx.arc(0, 0, rr, a, a + 0.5); ctx.stroke(); }
        }

        if (ent.maxHp > 0 && ent.type !== 2 && ent.type !== 7) {
          const width = 30, height = 3, rx = -width / 2, ry = -this._radius(ent) - 10;
          ctx.fillStyle = 'rgba(0,0,0,0.6)'; ctx.fillRect(rx, ry, width, height);
          ctx.fillStyle = COL.green; ctx.fillRect(rx, ry, (ent.hp / ent.maxHp) * width, height);
          if (ent.maxShield > 0) { ctx.fillStyle = COL.cyan; ctx.fillRect(rx, ry + 3, (ent.shield / ent.maxShield) * width, height - 1); }
          if (ent.maxFlux > 0) { ctx.fillStyle = ent.overloaded ? '#ff3b3b' : '#b061ff'; ctx.fillRect(rx, ry + 5, (ent.flux / ent.maxFlux) * width, 1.5); }
        }

        if (ent.overloaded || ent.venting) {
          ctx.strokeStyle = ent.overloaded ? 'rgba(255,59,59,0.85)' : 'rgba(176,97,255,0.8)';
          ctx.lineWidth = 1.5; ctx.setLineDash(ent.venting && !ent.overloaded ? [3, 3] : []);
          ctx.beginPath(); ctx.arc(0, 0, this._radius(ent) + 4, 0, Math.PI * 2); ctx.stroke(); ctx.setLineDash([]);
        }

        // Directional shield arc (battle only): draw the shield emitter the ship is holding —
        // a partial arc for "front" hulls (aimed at shield_facing), a full ring for omni. Shown
        // whenever the shield is physically up (not overloaded/venting): dim at low charge,
        // bright when charged, so the arc geometry is visible even when the shield is drained.
        if (net.currentSystemID >= 10000 && ent.maxShield > 0 && ent.shieldArc > 0 && !ent.overloaded && !ent.venting) {
          const frac = ent.shield / ent.maxShield;
          const r = this._radius(ent) + 11;
          const half = ent.shieldArc >= 360 ? Math.PI : (ent.shieldArc * Math.PI / 360);
          const f = ent.shieldFacing || 0;
          ctx.save();
          ctx.strokeStyle = 'rgba(0,242,254,' + (0.22 + 0.6 * frac).toFixed(3) + ')';
          ctx.shadowColor = COL.cyan; ctx.shadowBlur = 7; ctx.lineWidth = 3;
          ctx.beginPath(); ctx.arc(0, 0, r, f - half, f + half); ctx.stroke();
          ctx.restore();
        }
      }

      ctx.restore();
    }

    _teamColor(id) {
      switch (id) {
        case 1: return '#00f2fe'; case 2: return '#ffaa00'; case 3: return '#ff007f';
        case 4: return '#10b981'; case 5: return '#eab308'; default: return '#ffffff';
      }
    }

    _radius(arg) {
      const type = (typeof arg === 'object' && arg) ? arg.type : arg;
      let r = 10;
      switch (type) {
        case 0: r = 12; break; case 1: r = 12; break; case 2: r = 24; break;
        case 4: r = 60; break; case 5: r = 45; break; case 6: r = 8; break;
        case 7: r = 18; break; case 8: r = 40; break; case 9: r = 70; break; default: r = 10;
      }
      if (type === 2 && typeof arg === 'object' && arg && arg.maxHp > 0) r = r * (0.5 + 0.5 * (arg.hp / arg.maxHp));
      return r;
    }

    _drawShip(ent, color) {
      const ctx = this.ctx;
      ctx.save();
      ctx.rotate(ent.currRot);
      const v = Math.sqrt(ent.vx * ent.vx + ent.vy * ent.vy);
      const isMiner = ent.shipType === 'miner' || (ent.name || '').includes('Miner');
      if (v > 5) {
        ctx.save();
        ctx.shadowBlur = 12; ctx.shadowColor = '#ff6600';
        const startX = isMiner ? -12 : -6;
        const flameLength = 12 + Math.random() * 8, flameWidth = 4 + Math.random() * 2;
        const grad = ctx.createLinearGradient(startX, 0, startX - flameLength, 0);
        grad.addColorStop(0, '#ffff00'); grad.addColorStop(0.3, '#ffaa00');
        grad.addColorStop(0.7, '#ff3300'); grad.addColorStop(1, 'rgba(255,0,0,0)');
        ctx.fillStyle = grad;
        ctx.beginPath();
        ctx.moveTo(startX, 0); ctx.lineTo(startX - 2, -flameWidth);
        ctx.lineTo(startX - flameLength, 0); ctx.lineTo(startX - 2, flameWidth);
        ctx.closePath(); ctx.fill();
        ctx.restore();
      }

      ctx.fillStyle = 'rgba(15,23,42,0.8)'; ctx.strokeStyle = color; ctx.lineWidth = 2;
      ctx.shadowColor = color; ctx.shadowBlur = 5;

      if (isMiner) {
        ctx.beginPath();
        ctx.moveTo(16, 0); ctx.lineTo(8, -6); ctx.lineTo(-8, -6); ctx.lineTo(-12, 0); ctx.lineTo(-8, 6); ctx.lineTo(8, 6);
        ctx.closePath(); ctx.fill(); ctx.stroke();
        ctx.fillStyle = 'rgba(30,41,59,0.95)';
        ctx.beginPath(); ctx.rect(-6, -12, 12, 5); ctx.fill(); ctx.stroke();
        ctx.beginPath(); ctx.rect(-6, 7, 12, 5); ctx.fill(); ctx.stroke();
        ctx.fillStyle = COL.cyan; ctx.shadowBlur = 0;
        ctx.beginPath(); ctx.moveTo(10, 0); ctx.lineTo(6, -3); ctx.lineTo(2, 0); ctx.lineTo(6, 3); ctx.closePath(); ctx.fill();
      } else {
        ctx.beginPath();
        ctx.moveTo(15, 0); ctx.lineTo(-10, -10); ctx.lineTo(-6, 0); ctx.lineTo(-10, 10);
        ctx.closePath(); ctx.fill(); ctx.stroke();
        ctx.fillStyle = 'rgba(255,255,255,0.8)';
        ctx.beginPath(); ctx.moveTo(7, 0); ctx.lineTo(-2, -4); ctx.lineTo(-2, 4); ctx.closePath(); ctx.fill();
      }
      ctx.restore();
    }

    _drawAsteroid(ent) {
      const ctx = this.ctx, r = this._radius(ent), n = ent.name || '';
      const isIron = n.includes('Iron'), isTitanium = n.includes('Titanium'),
        isCrystal = n.includes('Crystal'), isGas = n.includes('Gas') || n.includes('RareGas');
      if (isIron) { ctx.shadowColor = '#8a6d3b'; ctx.shadowBlur = 6; ctx.fillStyle = 'rgba(65,50,45,0.95)'; ctx.strokeStyle = '#a88959'; ctx.lineWidth = 2.5; }
      else if (isTitanium) { ctx.shadowColor = '#007ffd'; ctx.shadowBlur = 10; ctx.fillStyle = 'rgba(20,35,55,0.95)'; ctx.strokeStyle = '#4a8bd5'; ctx.lineWidth = 2.5; }
      else if (isCrystal) { ctx.shadowColor = '#ff007f'; ctx.shadowBlur = 12; ctx.fillStyle = 'rgba(45,10,40,0.9)'; ctx.strokeStyle = '#e02080'; ctx.lineWidth = 3; }
      else if (isGas) { ctx.shadowColor = '#39ff14'; ctx.shadowBlur = 15; ctx.fillStyle = 'rgba(15,60,25,0.6)'; ctx.strokeStyle = '#39ff14'; ctx.lineWidth = 1.5; }
      else { ctx.shadowColor = '#666'; ctx.shadowBlur = 4; ctx.fillStyle = 'rgba(30,27,27,0.9)'; ctx.strokeStyle = '#554d4d'; ctx.lineWidth = 2.5; }

      ctx.beginPath();
      let pts = 8; if (isCrystal) pts = 5; if (isTitanium) pts = 6; if (isGas) pts = 10;
      for (let i = 0; i < pts; i++) {
        const angle = (i / pts) * Math.PI * 2;
        const pr = r + Math.sin(ent.id + i) * (r * 0.25);
        const px = Math.cos(angle) * pr, py = Math.sin(angle) * pr;
        if (i === 0) ctx.moveTo(px, py); else ctx.lineTo(px, py);
      }
      ctx.closePath(); ctx.fill(); ctx.stroke();

      if (isIron) ctx.fillStyle = 'rgba(210,140,80,0.7)';
      else if (isTitanium) ctx.fillStyle = 'rgba(0,191,255,0.8)';
      else if (isCrystal) ctx.fillStyle = 'rgba(255,105,180,0.9)';
      else if (isGas) ctx.fillStyle = 'rgba(57,255,20,0.8)';
      else ctx.fillStyle = 'rgba(180,180,180,0.4)';

      if (isCrystal) {
        ctx.strokeStyle = 'rgba(255,192,203,0.6)'; ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(0, 0); ctx.lineTo(-r * 0.5, -r * 0.3);
        ctx.moveTo(0, 0); ctx.lineTo(r * 0.5, r * 0.4);
        ctx.moveTo(0, 0); ctx.lineTo(-r * 0.2, r * 0.5);
        ctx.stroke();
        ctx.beginPath(); ctx.arc(0, 0, r * 0.3, 0, Math.PI * 2); ctx.fill();
      } else {
        ctx.beginPath();
        ctx.arc(-r * 0.3, -r * 0.15, r * 0.15, 0, Math.PI * 2);
        ctx.arc(r * 0.25, r * 0.25, r * 0.2, 0, Math.PI * 2);
        ctx.arc(-r * 0.1, r * 0.3, r * 0.1, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    _drawPlanet(ent) {
      const ctx = this.ctx, r = this._radius(ent), dev = ent.baseLevel || 0;
      const grad = ctx.createRadialGradient(-r * 0.3, -r * 0.3, r * 0.2, 0, 0, r);
      grad.addColorStop(0, '#6fae8f'); grad.addColorStop(1, '#1f3b30');
      ctx.fillStyle = grad;
      ctx.beginPath(); ctx.arc(0, 0, r, 0, Math.PI * 2); ctx.fill();
      ctx.strokeStyle = 'rgba(255,255,255,0.25)'; ctx.lineWidth = 1.5; ctx.stroke();
      ctx.strokeStyle = COL.cyan; ctx.lineWidth = 1.5;
      const rings = Math.min(dev, 5);
      for (let i = 1; i <= rings; i++) {
        ctx.globalAlpha = 0.5;
        ctx.beginPath(); ctx.ellipse(0, 0, r + 6 + i * 5, (r + 6 + i * 5) * 0.32, Math.PI / 6, 0, Math.PI * 2); ctx.stroke();
      }
      ctx.globalAlpha = 1;
    }

    _drawSpaceBase(ent) {
      const ctx = this.ctx, r = this._radius(ent), col = this._teamColor(ent.factionId);
      ctx.shadowColor = col; ctx.shadowBlur = 10;
      ctx.strokeStyle = col; ctx.fillStyle = 'rgba(15,15,27,0.85)'; ctx.lineWidth = 3;
      ctx.beginPath();
      for (let i = 0; i < 6; i++) { const a = Math.PI / 3 * i + Date.now() / 4000; const x = Math.cos(a) * r, y = Math.sin(a) * r; if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y); }
      ctx.closePath(); ctx.fill(); ctx.stroke();
      ctx.shadowBlur = 0; ctx.fillStyle = col; ctx.globalAlpha = 0.5 + 0.3 * Math.sin(Date.now() / 400);
      ctx.beginPath(); ctx.arc(0, 0, r * 0.35, 0, Math.PI * 2); ctx.fill(); ctx.globalAlpha = 1;
      const lvl = ent.baseLevel || 1; ctx.fillStyle = '#ffffff';
      for (let i = 0; i < lvl; i++) { const a = -Math.PI / 2 + (Math.PI * 2 / Math.max(lvl, 1)) * i; ctx.beginPath(); ctx.arc(Math.cos(a) * r * 0.6, Math.sin(a) * r * 0.6, 2.5, 0, Math.PI * 2); ctx.fill(); }
    }

    _drawStation(ent) {
      const ctx = this.ctx, r = this._radius(ent);
      ctx.shadowColor = COL.cyan; ctx.shadowBlur = 8;
      ctx.strokeStyle = COL.border; ctx.lineWidth = 3;
      ctx.beginPath(); ctx.arc(0, 0, r, 0, Math.PI * 2); ctx.stroke();
      ctx.save();
      ctx.rotate(Date.now() / 1500);
      ctx.strokeStyle = COL.cyan; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.moveTo(-r - 10, 0); ctx.lineTo(r + 10, 0); ctx.moveTo(0, -r - 10); ctx.lineTo(0, r + 10); ctx.stroke();
      ctx.fillStyle = 'rgba(0,127,255,0.6)';
      ctx.fillRect(-r - 15, -6, 10, 12); ctx.fillRect(r + 5, -6, 10, 12); ctx.fillRect(-6, -r - 15, 12, 10); ctx.fillRect(-6, r + 5, 12, 10);
      ctx.restore();
      ctx.fillStyle = 'rgba(15,15,27,0.95)'; ctx.strokeStyle = '#ffffff'; ctx.lineWidth = 2.5;
      ctx.beginPath(); ctx.arc(0, 0, r * 0.4, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
      ctx.fillStyle = (Math.floor(Date.now() / 600) % 2 === 0) ? COL.green : '#113311';
      ctx.beginPath(); ctx.arc(0, 0, 4, 0, Math.PI * 2); ctx.fill();
    }

    _drawJumpGate(ent) {
      const ctx = this.ctx, r = this._radius(ent);
      ctx.shadowColor = COL.orange; ctx.shadowBlur = 15;
      ctx.strokeStyle = COL.orange; ctx.lineWidth = 4;
      ctx.beginPath(); ctx.arc(0, 0, r, 0, Math.PI * 2); ctx.stroke();
      ctx.save();
      ctx.rotate(-Date.now() / 400);
      const grad = ctx.createRadialGradient(0, 0, 5, 0, 0, r);
      grad.addColorStop(0, 'rgba(255,255,255,0.95)'); grad.addColorStop(0.3, 'rgba(255,120,0,0.7)');
      grad.addColorStop(0.7, 'rgba(120,0,255,0.4)'); grad.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = grad; ctx.beginPath(); ctx.arc(0, 0, r - 4, 0, Math.PI * 2); ctx.fill();
      ctx.restore();
      ctx.fillStyle = '#3f3f3f'; ctx.strokeStyle = '#777'; ctx.lineWidth = 2; ctx.shadowBlur = 0;
      for (let i = 0; i < 3; i++) { ctx.save(); ctx.rotate((i * 120 * Math.PI) / 180 + Date.now() / 3000); ctx.beginPath(); ctx.rect(r - 5, -8, 15, 16); ctx.fill(); ctx.stroke(); ctx.restore(); }
    }

    _drawLoot(ent) {
      const ctx = this.ctx, r = this._radius(ent);
      ctx.strokeStyle = COL.orange; ctx.fillStyle = 'rgba(255,159,28,0.45)'; ctx.lineWidth = 2;
      ctx.shadowColor = COL.orange; ctx.shadowBlur = 8;
      ctx.beginPath(); ctx.rect(-r, -r, r * 2, r * 2); ctx.fill(); ctx.stroke();
      ctx.shadowBlur = 0; ctx.beginPath(); ctx.moveTo(-r, -r); ctx.lineTo(r, r); ctx.moveTo(r, -r); ctx.lineTo(-r, r); ctx.stroke();
    }

    _drawCombatMarker(ent) {
      const ctx = this.ctx, r = this._radius(ent);
      ctx.save();
      const pulse = 4 + Math.sin(Date.now() / 150) * 4;
      ctx.shadowColor = COL.magenta; ctx.shadowBlur = 10 + pulse;
      ctx.strokeStyle = 'rgba(255,0,127,0.7)'; ctx.lineWidth = 2.5;
      ctx.beginPath(); ctx.arc(0, 0, r + pulse * 0.4, 0, Math.PI * 2); ctx.stroke();
      ctx.strokeStyle = COL.magenta; ctx.lineWidth = 3; ctx.shadowBlur = 0;
      ctx.beginPath(); ctx.moveTo(-10, 10); ctx.lineTo(10, -10); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(10, 10); ctx.lineTo(-10, -10); ctx.stroke();
      ctx.restore();
    }

    // ---------------------------------------------------------------------
    _damageTypeColor(t) {
      switch (t) {
        case 'KINETIC': return '#9fd0ff';
        case 'EXPLOSIVE': return '#ff8c3a';
        case 'ENERGY': return '#42f5b3';
        case 'FRAGMENTATION': return '#fff07a';
        default: return '#ff5555';
      }
    }
    _sparks(x, y, color) {
      for (let i = 0; i < 8; i++) {
        const a = Math.random() * Math.PI * 2, s = 1.0 + Math.random() * 2.5;
        this.particles.push(new Particle(x, y, Math.cos(a) * s, Math.sin(a) * s, color, 1.5 + Math.random() * 1.5, 0.3 + Math.random() * 0.4));
      }
    }
    _explosion(x, y, color) {
      for (let i = 0; i < 30; i++) {
        const a = Math.random() * Math.PI * 2, s = 0.5 + Math.random() * 4.0;
        this.particles.push(new Particle(x, y, Math.cos(a) * s, Math.sin(a) * s, color, 2.0 + Math.random() * 3.0, 0.5 + Math.random() * 0.8));
      }
    }

    // Turn server fire events into cosmetic shots (B3). Fields are snake_case (protojson
    // UseProtoNames). Purely visual — the server already applied the damage. A projectile event
    // spawns a flying bolt; a beam event spawns an instant fading beam line. We also stamp the
    // attacker so _drawBeams skips its legacy is_shooting line (avoids a double render).
    _onFireEvents(fires) {
      if (!Array.isArray(fires)) return;
      const now = Date.now();
      for (const f of fires) {
        const atk = this.net.entities.get(Number(f.attacker_id));
        if (atk) atk._firedByEvent = now;
        const color = this._damageTypeColor(f.damage_type);
        if (f.weapon_class === 'beam') {
          if (this.beams.length < 400) {
            this.beams.push(new Beam(
              Number(f.origin_x) || 0, Number(f.origin_y) || 0,
              Number(f.target_x) || 0, Number(f.target_y) || 0, color));
          }
        } else if (f.weapon_class === 'missile') {
          if (this.missiles.length < 300) {
            this.missiles.push(new Missile(
              Number(f.origin_x) || 0, Number(f.origin_y) || 0,
              Number(f.target_x) || 0, Number(f.target_y) || 0,
              Number(f.target_id) || 0, Number(f.speed) || 300, color, this.net));
          }
        } else if (this.projectiles.length < 600) {
          this.projectiles.push(new Projectile(
            Number(f.origin_x) || 0, Number(f.origin_y) || 0,
            Number(f.target_x) || 0, Number(f.target_y) || 0,
            Number(f.speed) || 600, color));
        }
      }
    }

    _emit(fn, ...args) { if (typeof fn === 'function') { try { fn(...args); } catch (e) {} } }
    _log(m, c) { this._emit(this.onLog, m, c); }
  }

  window.WorldRenderer = WorldRenderer;
})();
