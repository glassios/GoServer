/* Order New Dawn — client shell.
   Wires the reused /play/ NetClient transport to the battle-gl.js renderer via BattleAdapter,
   drives the render loop, binds the OLDSECTOR battle HUD, camera controls, and a minimal
   "engage nearest hostile" path to get the player into a server combat instance. */
(function () {
  'use strict';

  var $ = function (id) { return document.getElementById(id); };
  var canvas = $('c');

  var net = new window.NetClient();
  var renderer = window.BattleGL && window.BattleGL.create(canvas);
  var adapter = new window.BattleAdapter(net);
  var B = adapter.B;

  // 2D overlay canvas for floating damage numbers (drawn on top of the WebGL scene).
  var c2 = $('c2'), ctx2 = c2.getContext('2d');
  function resizeAll() {
    if (renderer) renderer.resize();
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    c2.width = canvas.clientWidth * dpr; c2.height = canvas.clientHeight * dpr;
    ctx2.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  if (!renderer) { $('loginStat').textContent = 'WebGL unavailable — this client needs WebGL'; }
  else { resizeAll(); window.addEventListener('resize', resizeAll); }

  // Floating damage numbers: net emits (serverX, serverY, amount, type, isShield) on hull/shield loss.
  var dmgNums = [];
  net.onDamageFloat = function (x, y, amount, type, isShield) {
    if (!adapter.inBattle()) return;
    dmgNums.push({ x: x / 2000 + 1.5, y: y / 2000 + 1.5, amount: Math.round(amount || 0), shield: !!isShield, life: 1.0, dtype: type || '' });
  };
  function dmgColor(t) {
    switch (t) { case 'KINETIC': return '#9fd0ff'; case 'EXPLOSIVE': return '#ff8c3a';
      case 'ENERGY': return '#7df5c0'; case 'FRAGMENTATION': return '#fff07a'; default: return '#ff6a5f'; }
  }
  // Angular corner brackets (OLDSECTOR style) around a ship at screen (x,y).
  function drawBracket(x, y, rr, color) {
    var L = Math.max(6, rr * 0.5), corners = [[-1, -1], [1, -1], [-1, 1], [1, 1]];
    ctx2.save();
    ctx2.strokeStyle = color; ctx2.lineWidth = 1.6; ctx2.globalAlpha = 0.9;
    ctx2.shadowColor = color; ctx2.shadowBlur = 6;
    for (var i = 0; i < 4; i++) {
      var c = corners[i], cx = x + c[0] * rr, cy = y + c[1] * rr;
      ctx2.beginPath(); ctx2.moveTo(cx - c[0] * L, cy); ctx2.lineTo(cx, cy); ctx2.lineTo(cx, cy - c[1] * L); ctx2.stroke();
    }
    ctx2.restore();
  }

  function drawDmg(dtF) {
    if (!ctx2) return;
    ctx2.clearRect(0, 0, c2.width, c2.height);
    if (!adapter.inBattle() || !renderer) { dmgNums.length = 0; return; }
    // battle-gl renders into a Y-up FBO and flips on blit, so a hull at sy() shows at css-y = H - sy().
    var H = B.gl_Hcss || canvas.clientHeight || 700;

    // selection bracket around the player's ship (amber) + the tracked hostile (red)
    var me = adapter.playerShip();
    if (me) { var pm = renderer.toScreen(B, me.x, me.y); drawBracket(pm[0], H - pm[1], Math.max(14, me.radius * 3.4 * B.zoom * 0.75), '#f7b26a'); }
    var en = adapter.nearestEnemy(me || B.ships[0]);
    if (en) { var pe = renderer.toScreen(B, en.x, en.y); drawBracket(pe[0], H - pe[1], Math.max(12, en.radius * 3.4 * B.zoom * 0.75), '#e0584f'); }

    // floating damage numbers
    ctx2.font = "700 13px 'JetBrains Mono', monospace"; ctx2.textAlign = 'center';
    for (var i = dmgNums.length - 1; i >= 0; i--) {
      var d = dmgNums[i]; d.life -= 0.02 * dtF;
      if (d.life <= 0) { dmgNums.splice(i, 1); continue; }
      var p = renderer.toScreen(B, d.x, d.y);
      var yy = (H - p[1]) - (1 - d.life) * 34 - 12;
      ctx2.globalAlpha = Math.max(0, Math.min(1, d.life * 1.4));
      ctx2.fillStyle = d.shield ? '#6fd3ff' : dmgColor(d.dtype);
      ctx2.shadowColor = 'rgba(0,0,0,0.8)'; ctx2.shadowBlur = 3;
      ctx2.fillText('-' + d.amount + (d.shield ? ' SH' : ''), p[0], yy);
    }
    ctx2.globalAlpha = 1; ctx2.shadowBlur = 0;
  }

  // ---- connection / auth ----------------------------------------------------------------
  var authed = false;
  net.onStatus = function (ok, text) { $('status').textContent = 'ORDER NEW DAWN · ' + text; };
  net.onAuth = function (entityId) {
    if (entityId) {
      authed = true;
      $('login').classList.add('hidden');
      $('loginStat').textContent = '';
    } else {
      $('loginStat').textContent = 'authentication rejected — try another callsign';
    }
  };

  function connect() {
    var email = ($('email').value || '').trim();
    if (email.length < 3) { $('loginStat').textContent = 'callsign too short'; return; }
    $('loginStat').textContent = 'connecting…';
    net.connect(null, email);
  }
  $('connectBtn').addEventListener('click', connect);
  $('email').addEventListener('keydown', function (e) { if (e.key === 'Enter') connect(); });

  // ---- in-game shell: screen router + server-data capture ------------------------------
  var srv = {};   // latest typed server payloads keyed by type (s_fleet_status, s_hangar_data, ...)
  var screen = 'nav';
  var SCREEN_TITLES = { nav: 'COMMAND', fleet: 'TASK FORCE', refit: 'DRYDOCK', cargo: 'CARGO HOLD', map: 'SECTOR MAP', research: 'RESEARCH DIVISION', colony: 'COLONIAL HOLDINGS', skills: 'COMMANDER' };
  net.onTyped = function (type, data) {
    srv[type] = data;
    if (screen !== 'nav' && !$('screen').classList.contains('hidden')) renderScreen();
  };

  function el(tag, css, txt, cls) {
    var e = document.createElement(tag);
    if (css) e.setAttribute('style', css);
    if (txt != null) e.textContent = txt;
    if (cls) e.className = cls;
    return e;
  }
  function bar(frac, color, h) {
    var w = el('div', 'height:' + (h || 6) + 'px;background:rgba(0,0,0,.5);margin-top:3px;');
    w.appendChild(el('div', 'height:100%;width:' + Math.round(Math.max(0, Math.min(1, frac || 0)) * 100) + '%;background:' + color + ';'));
    return w;
  }

  function setScreen(name) {
    screen = name;
    var btns = document.querySelectorAll('#nav .navbtn');
    for (var i = 0; i < btns.length; i++) btns[i].classList.toggle('active', btns[i].getAttribute('data-screen') === name);
    if (name === 'fleet' || name === 'refit') net.send({ action: 'get_hangar' });
    if (name === 'colony') net.send({ action: 'get_colonies' });
    updateChrome();
    renderScreen();
  }
  (function () {
    var btns = document.querySelectorAll('#nav .navbtn');
    for (var i = 0; i < btns.length; i++) (function (b) { b.addEventListener('click', function () { setScreen(b.getAttribute('data-screen')); }); })(btns[i]);
  })();
  $('logout').addEventListener('click', function () { try { net.close && net.close(); } catch (e) {} location.reload(); });

  function updateChrome() {
    var inB = authed && adapter.inBattle();
    var shell = authed && !inB;
    $('hud').classList.toggle('hidden', !inB);
    $('topbar').classList.toggle('hidden', !shell);
    $('nav').classList.toggle('hidden', !shell);
    $('idle').classList.toggle('hidden', !(shell && screen === 'nav'));
    $('screen').classList.toggle('hidden', !(shell && screen !== 'nav'));
  }

  // Top status bar: live stats + location + FPS.
  var fpsSmooth = 0;
  function updateTopbar(fps) {
    if ($('topbar').classList.contains('hidden')) return;
    $('tbScreen').textContent = SCREEN_TITLES[screen] || screen.toUpperCase();
    var inv = srv['s_inventory_update'], fs = srv['s_fleet_status'], pp = srv['s_player_progress'];
    var stats = [
      ['CREDITS', inv ? String(inv.credits || 0) : '—', '#e8b04b'],
      ['HULLS', fs && fs.ships ? String(fs.ships.length) : '—', '#57c98a'],
      ['LEVEL', pp && pp.command_level != null ? String(pp.command_level) : (pp && pp.skills && pp.skills.length ? String(pp.skills.length) : '—'), 'var(--ac2)'],
      ['STATUS', 'DOCKED', '#8aa0b2']
    ];
    var host = $('tbStats');
    if (host.childElementCount !== stats.length) {
      host.innerHTML = '';
      stats.forEach(function () { var s = el('div', null, null, 'stat'); s.appendChild(el('div', null, '', 'k')); s.appendChild(el('div', null, '', 'v')); host.appendChild(s); });
    }
    for (var i = 0; i < stats.length; i++) {
      var cell = host.children[i];
      cell.children[0].textContent = stats[i][0];
      cell.children[1].textContent = stats[i][1];
      cell.children[1].style.color = stats[i][2];
    }
    $('tbLoc').textContent = 'SYSTEM ' + (net.currentSystemID || 1);
    if (fps) { fpsSmooth = fpsSmooth ? fpsSmooth * 0.9 + fps * 0.1 : fps; $('tbFps').textContent = Math.round(fpsSmooth); }
  }

  // screen state + hangar lookups
  var selFleetId = 0;                                   // fleet-screen dossier selection
  var selResearchId = '';                               // research-screen dossier selection
  var refitShipId = 0, refitSlot = null, fitDraft = null;
  var SIZE = { SMALL: 1, MEDIUM: 2, LARGE: 3 };
  function fleetShipById(id) { var fs = srv['s_fleet_status']; return fs && (fs.ships || []).filter(function (s) { return Number(s.ship_id) === Number(id); })[0]; }
  function hullById(id) { var h = srv['s_hangar_data']; return h && (h.hulls || []).filter(function (x) { return Number(x.id) === Number(id); })[0]; }
  function weaponById(id) { var h = srv['s_hangar_data']; return h && (h.weapons || []).filter(function (w) { return w.weapon_id === id; })[0]; }
  function slotAccepts(slot, w) {
    if (!slot) return false;
    var st = (slot.type || '').toUpperCase(), wt = (w.weapon_type || '').toUpperCase();
    var typeOk = st === 'UNIVERSAL' || st === '' || st === wt;
    var sizeOk = (SIZE[(w.weapon_size || '').toUpperCase()] || 1) <= (SIZE[(slot.size || '').toUpperCase()] || 3);
    return typeOk && sizeOk;
  }
  // Ordnance = sum of fitted weapons' op_cost (server ships carry no op_used field).
  function computeOP(fittedWeapons, hull) {
    var used = 0, fw = fittedWeapons || {};
    for (var k in fw) { if (!fw[k]) continue; var w = weaponById(fw[k]); if (w) used += (w.op_cost || 0); }
    return { used: used, max: hull ? (hull.ordnance_points || 0) : 0 };
  }
  function shipIcon(sizeClass) {
    var s = (sizeClass || '').toUpperCase();
    if (s.indexOf('CAP') >= 0) return '⬢';
    if (s.indexOf('CRUIS') >= 0) return '◆';
    if (s.indexOf('DEST') >= 0) return '◈';
    if (s.indexOf('FRIG') >= 0 || s.indexOf('FIGHT') >= 0) return '▲';
    return '◆';
  }

  // full-screen grid container above the .grid-bg
  function screenGrid(cols) { return el('div', 'position:absolute; inset:0; z-index:1; display:grid; grid-template-columns:' + cols + '; gap:18px; padding:22px 24px;'); }
  var ROLE_OPTS = [['tank', 'TANK'], ['dps', 'DPS'], ['support', 'SUPPORT'], ['repair', 'REPAIR']];
  var STANCE_OPTS = [['attack', 'ATTACK'], ['defense', 'DEFENSE'], ['retreat', 'RETREAT']];
  function pickRow(opts, cur, cb) {
    var r = el('div', 'display:flex;flex-wrap:wrap;gap:5px;margin-top:6px;');
    opts.forEach(function (o) { var b = el('div', 'flex:1 1 calc(50% - 5px);', o[1], 'btn2' + (o[0] === cur ? ' on' : '')); b.addEventListener('click', function () { cb(o[0]); }); r.appendChild(b); });
    return r;
  }

  // ---- REFIT hull-schematic drawing (sprite + slot anchors) ----------------------------
  var GEO2 = {
    ship: { turrets: [[0.42, 0], [0.15, -0.13], [0.15, 0.13], [-0.05, -0.16], [-0.05, 0.16]] },
    dest: { turrets: [[0.44, -0.04], [0.44, 0.04], [0.18, -0.16], [0.18, 0.16], [-0.05, -0.27], [-0.05, 0.27]] },
    frig: { turrets: [[0.46, 0], [0.12, -0.13], [0.12, 0.13], [-0.05, -0.16], [-0.05, 0.16]] }
  };
  var SPRITE = { ship: 'ship', dest: 'destroyer', frig: 'frigate' };
  function hullTexKey(hull) {
    var s = (hull && hull.size_class || '').toUpperCase();
    if (s.indexOf('CAP') >= 0 || s.indexOf('CRUIS') >= 0 || s.indexOf('DEST') >= 0) return 'dest';
    if (s.indexOf('FRIG') >= 0 || s.indexOf('FIGHT') >= 0) return 'frig';
    return 'ship';
  }
  var imgCache = {};
  function getImg(name, onload) {
    if (!imgCache[name]) { var im = new Image(); im.onload = onload; im.src = 'textures/sprites/' + name + '.png'; imgCache[name] = im; }
    else if (onload && imgCache[name].complete) onload();
    return imgCache[name];
  }
  function drawSchematic(canvas, hull, slots, draft, selSlotId) {
    if (!canvas) return;
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var W = canvas.clientWidth, H = canvas.clientHeight;
    if (!W || !H) return;
    canvas.width = W * dpr; canvas.height = H * dpr;
    var g = canvas.getContext('2d'); g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, W, H);
    var tex = hullTexKey(hull), img = getImg(SPRITE[tex], function () { drawSchematic(canvas, hull, slots, draft, selSlotId); });
    var cx = W / 2, cy = H / 2, halfW, halfH;
    if (img && img.complete && img.naturalWidth) {
      var fitW = Math.min(W * 0.6, H * 0.72 * (img.naturalWidth / img.naturalHeight));
      var dw = fitW, dh = fitW * (img.naturalHeight / img.naturalWidth);
      halfW = dw / 2; halfH = dh / 2;
      g.save(); g.globalAlpha = 0.96; g.drawImage(img, cx - halfW, cy - halfH, dw, dh); g.restore();
    } else { halfW = W * 0.22; halfH = halfW * 0.5; }
    // slot anchors: forward is toward the nose (art faces left → -x); lateral → +y. fractions of half-sprite.
    var geo = GEO2[tex] || GEO2.ship, ts = geo.turrets, fw = (draft && draft.weapons) || {};
    (slots || []).forEach(function (sl, i) {
      var a = ts[i % ts.length]; if (!a) return;
      var x = cx - a[0] * halfW, y = cy + a[1] * halfH;
      var filled = !!fw[sl.slot_id], sel = sl.slot_id === selSlotId;
      g.beginPath(); g.arc(x, y, sel ? 8 : 6, 0, Math.PI * 2);
      g.fillStyle = filled ? 'rgba(255,155,92,.9)' : 'rgba(120,150,170,.5)';
      g.fill();
      if (sel) { g.lineWidth = 2; g.strokeStyle = '#57c98a'; g.beginPath(); g.arc(x, y, 11, 0, Math.PI * 2); g.stroke(); }
    });
  }

  // ---- FLEET formation grid (client-only cosmetic ordering) ----------------------------
  var formSlots = null, formSig = '', formDrag = -1;
  var RANKS = ['VANGUARD', 'BATTLE LINE', 'RESERVE'];
  // Build the 18-cell grid from the server's per-ship formation slot when it's a valid unique
  // arrangement (honors a saved battle line); otherwise fall back to a sequential fill.
  function ensureFormation(ships) {
    var sig = ships.map(function (s) { return s.ship_id + ':' + (s.formation_rank || 0) + ':' + (s.formation_col || 0); }).join(',');
    if (formSlots && formSig === sig) return;
    formSig = sig; formSlots = []; for (var i = 0; i < 18; i++) formSlots[i] = null;
    var byIdx = {}, unique = true;
    ships.forEach(function (s) {
      var idx = (s.formation_rank || 0) * 6 + (s.formation_col || 0);
      if (idx < 0 || idx > 17 || byIdx[idx] != null) unique = false; else byIdx[idx] = Number(s.ship_id);
    });
    if (unique) { for (var k in byIdx) formSlots[k] = byIdx[k]; }
    else { ships.forEach(function (s, i) { if (i < 18) formSlots[i] = Number(s.ship_id); }); }
  }
  // Push the current grid to the server as the pre-battle battle line.
  function saveFormation() {
    var slots = [];
    for (var i = 0; i < 18; i++) { if (formSlots[i]) slots.push({ ship_id: Number(formSlots[i]), rank: Math.floor(i / 6), col: i % 6 }); }
    net.send({ action: 'set_fleet_formation', formation: slots });
  }

  function renderScreen() {
    if (screen === 'nav') return;
    var host = $('screen'); host.innerHTML = '';
    host.appendChild(el('div', null, null, 'grid-bg'));
    if (screen === 'fleet') renderFleet(host);
    else if (screen === 'refit') renderRefit(host);
    else if (screen === 'cargo') renderCargo(host);
    else if (screen === 'map') renderMap(host);
    else if (screen === 'research') renderResearch(host);
    else if (screen === 'colony') renderColony(host);
    else if (screen === 'skills') renderSkills(host);
  }

  function renderFleet(host) {
    var fs = srv['s_fleet_status'], hd = srv['s_hangar_data'];
    var grid = screenGrid('300px 1fr 340px'); host.appendChild(grid);
    if (!fs || !fs.ships || !fs.ships.length) { grid.appendChild(el('div', null, 'Loading fleet…', 'msg')); return; }
    var ships = fs.ships;
    if (!selFleetId || !fleetShipById(selFleetId)) selFleetId = Number(ships[0].ship_id);
    ensureFormation(ships);

    // ---- left: roster ----
    var left = el('div', null, null, 'col'); left.style.gap = '14px';
    var head = el('div', null);
    head.appendChild(el('div', null, 'TASK FORCE VANGUARD', 'sh'));
    head.appendChild(el('div', 'font-size:22px;font-weight:700;letter-spacing:1px;margin-top:3px;', 'Fleet Composition'));
    var boxes = el('div', 'display:flex;gap:8px;margin-top:10px;');
    [['DEPLOY PTS', String(ships.length), 'var(--ac2)'], ['HULLS', String(ships.length), '#57c98a']].forEach(function (b) {
      var bx = el('div', 'padding:8px 16px;background:rgba(12,17,24,.6);border:1px solid rgba(120,150,170,.14);');
      bx.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;letter-spacing:2px;color:#56697a;', b[0]));
      bx.appendChild(el('div', 'font-size:17px;font-weight:700;color:' + b[2] + ';', b[1]));
      boxes.appendChild(bx);
    });
    head.appendChild(boxes);
    left.appendChild(head);
    left.appendChild(el('div', null, '▲ DEPLOYED · ' + ships.length + ' HULLS', 'sh'));
    var list = el('div', 'display:flex;flex-direction:column;gap:9px;', null, 'scy');
    ships.forEach(function (s) {
      var hull = hullById(s.hull_id);
      var card = el('div', null, null, 'fcard' + (Number(s.ship_id) === selFleetId ? ' sel' : ''));
      var fh = el('div', null, null, 'fh');
      fh.appendChild(el('div', 'font-size:16px;color:var(--ac2);', shipIcon(hull && hull.size_class)));
      fh.appendChild(el('div', null, s.ship_type || 'Ship', 'nm2'));
      fh.appendChild(el('div', null, (hull && hull.size_class) || '', 'cls2'));
      card.appendChild(fh);
      card.appendChild(bar((s.health || 0) / (s.max_health || 1), '#57c98a', 3));
      card.appendChild(bar((s.shield || 0) / (s.max_shield || 1), '#6fa8c0', 3));
      var acts = el('div', 'display:flex;gap:5px;margin-top:8px;');
      var rf = el('div', 'flex:1;', 'REFIT', 'btn2');
      rf.addEventListener('click', function (ev) { ev.stopPropagation(); refitShipId = Number(s.ship_id); refitSlot = null; fitDraft = null; setScreen('refit'); });
      acts.appendChild(rf);
      card.appendChild(acts);
      card.addEventListener('click', function () { selFleetId = Number(s.ship_id); renderScreen(); });
      list.appendChild(card);
    });
    left.appendChild(list);
    grid.appendChild(left);

    // ---- center: battle formation grid ----
    var cen = el('div', null, null, 'col box bevel');
    var ch = el('div', 'display:flex;justify-content:space-between;align-items:center;padding:15px 18px;border-bottom:1px solid rgba(var(--acRGB),.16);');
    var cht = el('div', null);
    cht.appendChild(el('div', null, 'BATTLE FORMATION', 'sh'));
    cht.appendChild(el('div', 'font-size:18px;font-weight:700;margin-top:2px;', 'Arrange the Line'));
    ch.appendChild(cht);
    var clr = el('div', null, 'RESET LINE', 'btn2'); clr.style.padding = '7px 14px';
    clr.addEventListener('click', function () {
      for (var i = 0; i < 18; i++) formSlots[i] = null;
      ships.forEach(function (s, i) { if (i < 18) formSlots[i] = Number(s.ship_id); });
      saveFormation(); renderScreen();
    });
    ch.appendChild(clr);
    cen.appendChild(ch);
    var fbody = el('div', 'flex:1;display:flex;flex-direction:column;gap:11px;padding:18px;', null, 'scy');
    for (var r = 0; r < 3; r++) {
      var row = el('div', 'display:flex;align-items:stretch;gap:11px;');
      var lab = el('div', 'flex-shrink:0;width:74px;display:flex;flex-direction:column;justify-content:center;border-left:2px solid rgba(var(--acRGB),.35);padding-left:9px;');
      lab.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;font-weight:700;letter-spacing:1px;color:var(--ac2);', RANKS[r]));
      lab.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:7.5px;letter-spacing:1px;color:#56697a;margin-top:2px;', 'RANK ' + (r + 1)));
      row.appendChild(lab);
      var cells = el('div', 'flex:1;display:grid;grid-template-columns:repeat(6,1fr);gap:8px;');
      for (var c = 0; c < 6; c++) (function (idx) {
        var sid = formSlots[idx], s = sid ? fleetShipById(sid) : null;
        var cell = el('div', null, null, 'fcell' + (s ? ' filled' : ''));
        if (s) {
          var hull = hullById(s.hull_id);
          cell.appendChild(el('div', 'color:' + (Number(sid) === selFleetId ? 'var(--ac2)' : '#8ea1b2') + ';', shipIcon(hull && hull.size_class), 'ic'));
          cell.appendChild(el('div', null, s.ship_type || '', 'nm'));
          cell.setAttribute('draggable', 'true');
          cell.addEventListener('click', function () { selFleetId = Number(sid); renderScreen(); });
        }
        cell.addEventListener('dragstart', function () { formDrag = idx; });
        cell.addEventListener('dragover', function (e) { e.preventDefault(); cell.classList.add('drop'); });
        cell.addEventListener('dragleave', function () { cell.classList.remove('drop'); });
        cell.addEventListener('drop', function (e) {
          e.preventDefault();
          if (formDrag >= 0 && formDrag !== idx) { var t = formSlots[idx]; formSlots[idx] = formSlots[formDrag]; formSlots[formDrag] = t; saveFormation(); }
          formDrag = -1; renderScreen();
        });
        cells.appendChild(cell);
      })(r * 6 + c);
      row.appendChild(cells);
      fbody.appendChild(row);
    }
    fbody.appendChild(el('div', 'margin-top:4px;', '◆ DRAG A HULL INTO ANOTHER CELL TO REORDER · formation is cosmetic (server has no formation slots yet)', 'sh'));
    cen.appendChild(fbody);
    grid.appendChild(cen);

    // ---- right: hull dossier ----
    var ship = fleetShipById(selFleetId) || ships[0];
    var hull2 = hullById(ship.hull_id);
    var right = el('div', 'padding:20px 22px;', null, 'col box bevel scy');
    right.appendChild(el('div', null, 'HULL DOSSIER', 'sh'));
    right.appendChild(el('div', 'font-size:42px;margin-top:8px;color:var(--ac2);', shipIcon(hull2 && hull2.size_class)));
    right.appendChild(el('div', 'font-size:20px;font-weight:700;', ship.ship_type || 'Ship'));
    right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;letter-spacing:2px;color:#6f8496;margin-top:2px;', ((hull2 && hull2.size_class) || '') + ' · ' + String(ship.role || 'dps').toUpperCase()));
    right.appendChild(el('div', 'height:1px;background:rgba(120,150,170,.16);margin:14px 0;'));
    right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;', 'HULL INTEGRITY'));
    right.appendChild(bar((ship.health || 0) / (ship.max_health || 1), '#57c98a', 6));
    right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;margin-top:11px;', 'SHIELD CAPACITY'));
    right.appendChild(bar((ship.shield || 0) / (ship.max_shield || 1), '#6fa8c0', 6));
    var statG = el('div', 'display:grid;grid-template-columns:1fr 1fr;gap:1px;margin-top:14px;background:rgba(120,150,170,.1);');
    var op = computeOP(ship.fitted_weapons, hull2);
    [['MAX HULL', String(ship.max_health || 0)], ['MAX SHIELD', String(ship.max_shield || 0)],
     ['ARMOR', hull2 ? String(Math.round(hull2.base_armor || 0)) : '—'], ['ORDNANCE', op.used + ' / ' + op.max]].forEach(function (st) {
      var cell = el('div', 'padding:11px 13px;background:rgba(12,17,24,.85);');
      cell.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;letter-spacing:1px;color:#6f8496;', st[0]));
      cell.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:14px;font-weight:700;color:#cdd9e3;margin-top:3px;', st[1]));
      statG.appendChild(cell);
    });
    right.appendChild(statG);
    right.appendChild(el('div', 'margin-top:14px;', 'COMBAT ROLE', 'sh'));
    right.appendChild(pickRow(ROLE_OPTS, ship.role, function (rl) { net.send({ action: 'set_fleet_tactics', ship_id: Number(ship.ship_id), role: rl, strategy: ship.strategy || 'attack' }); }));
    right.appendChild(el('div', 'margin-top:12px;', 'FLEET TACTIC', 'sh'));
    right.appendChild(pickRow(STANCE_OPTS, ship.strategy, function (sc) { net.send({ action: 'set_fleet_tactics', ship_id: Number(ship.ship_id), role: ship.role || 'dps', strategy: sc }); }));
    grid.appendChild(right);
  }

  function renderRefit(host) {
    var hd = srv['s_hangar_data'], fs = srv['s_fleet_status'];
    var grid = screenGrid('260px 1fr 340px'); host.appendChild(grid);
    if (!hd || !fs || !fs.ships || !fs.ships.length) { grid.appendChild(el('div', null, 'Loading hangar…', 'msg')); return; }
    if (!refitShipId || !fleetShipById(refitShipId)) refitShipId = Number(fs.ships[0].ship_id);
    var ship = fleetShipById(refitShipId) || fs.ships[0];
    var hull = hullById(ship.hull_id);
    if (!fitDraft || fitDraft._for !== refitShipId) {
      fitDraft = { _for: refitShipId, weapons: Object.assign({}, ship.fitted_weapons || {}), mods: (ship.fitted_hullmods || []).slice(), vents: ship.vents || 0, caps: ship.capacitors || 0 };
    }
    var slots = (hull && hull.slots) || [];
    var op = computeOP(fitDraft.weapons, hull), over = op.used > op.max;
    var opColor = over ? '#e0584f' : 'var(--ac1)';

    // ---- left: hull selector + selected box + hull modules ----
    var left = el('div', null, null, 'col'); left.style.gap = '14px';
    left.appendChild(el('div', null, 'SELECT HULL', 'sh'));
    var hlist = el('div', 'display:flex;flex-direction:column;gap:6px;max-height:180px;', null, 'scy');
    fs.ships.forEach(function (s) {
      var on = Number(s.ship_id) === refitShipId, sh = hullById(s.hull_id);
      var row = el('div', 'display:flex;align-items:center;gap:9px;padding:8px 11px;cursor:pointer;border-left:3px solid ' + (on ? 'var(--ac1)' : 'rgba(120,150,170,.3)') + ';background:' + (on ? 'rgba(var(--acRGB),.1)' : 'rgba(14,19,27,.6)') + ';');
      row.appendChild(el('div', 'font-size:16px;color:' + (on ? 'var(--ac2)' : '#8ea1b2') + ';', shipIcon(sh && sh.size_class)));
      var t = el('div', null);
      t.appendChild(el('div', 'font-size:13px;font-weight:600;color:#dbe5ed;', s.ship_type));
      t.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;', (sh && sh.size_class) || ''));
      row.appendChild(t);
      row.addEventListener('click', function () { refitShipId = Number(s.ship_id); refitSlot = null; fitDraft = null; renderScreen(); });
      hlist.appendChild(row);
    });
    left.appendChild(hlist);
    var selBox = el('div', 'background:rgba(12,17,24,.6);border:1px solid rgba(120,150,170,.14);padding:16px;');
    selBox.appendChild(el('div', 'font-size:17px;font-weight:700;', ship.ship_type || 'Ship'));
    selBox.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;color:var(--ac1);letter-spacing:1px;margin-top:2px;', (hull && hull.size_class) || 'HULL'));
    left.appendChild(selBox);
    left.appendChild(el('div', null, 'HULL MODULES · FROM CARGO', 'sh'));
    var mods = el('div', 'display:flex;flex-direction:column;gap:5px;flex:1;', null, 'scy');
    var owned = hd.owned_modules || {}, mk = Object.keys(owned);
    if (!mk.length) mods.appendChild(el('div', null, '(no modules in cargo)', 'msg'));
    mk.forEach(function (m) {
      var row = el('div', 'display:flex;align-items:center;gap:9px;padding:7px 10px;background:rgba(14,19,27,.6);border:1px solid rgba(120,150,170,.12);');
      row.appendChild(el('div', 'font-size:14px;color:var(--ac2);width:17px;text-align:center;', '⬡'));
      row.appendChild(el('div', 'flex:1;font-size:12px;color:#dbe5ed;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;', m));
      row.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;color:#6f8496;', '×' + owned[m]));
      mods.appendChild(row);
    });
    left.appendChild(mods);
    grid.appendChild(left);

    // ---- center: live hull schematic ----
    var cen = el('div', 'position:relative;min-height:0;background:radial-gradient(120% 100% at 50% 45%, rgba(var(--acRGB),.05), transparent 62%);border:1px solid rgba(120,150,170,.14);overflow:hidden;');
    cen.appendChild(el('div', null, null, 'grid-bg'));
    var cv = document.createElement('canvas');
    cv.setAttribute('style', 'position:absolute;inset:0;width:100%;height:100%;display:block;z-index:2;');
    cen.appendChild(cv);
    var dlbl = el('div', 'position:absolute;left:12px;top:11px;z-index:4;display:flex;align-items:center;gap:8px;font-family:JetBrains Mono,monospace;font-size:10px;letter-spacing:2px;color:#6f8496;');
    dlbl.appendChild(el('span', 'width:7px;height:7px;background:#57c98a;border-radius:50%;box-shadow:0 0 7px #57c98a;'));
    dlbl.appendChild(el('span', null, 'DRYDOCK · LIVE HULL'));
    cen.appendChild(dlbl);
    var sbox = el('div', 'position:absolute;right:12px;top:11px;z-index:4;display:flex;gap:1px;background:rgba(120,150,170,.12);border:1px solid rgba(120,150,170,.16);');
    [['ARMOR', hull ? Math.round(hull.base_armor || 0) : '—'], ['HULL', hull ? Math.round(hull.base_hp || 0) : '—'],
     ['SHIELD', hull ? Math.round(hull.base_shield_max || 0) : '—'], ['MNT', slots.length]].forEach(function (s) {
      var c = el('div', 'padding:4px 8px;background:rgba(9,13,19,.85);text-align:center;');
      c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;letter-spacing:1px;color:#6f8496;', s[0]));
      c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:12px;font-weight:700;color:#cdd9e3;', String(s[1])));
      sbox.appendChild(c);
    });
    cen.appendChild(sbox);
    var opWrap = el('div', 'position:absolute;left:0;right:0;bottom:0;z-index:4;padding:0 14px 12px;');
    var opTop = el('div', 'display:flex;justify-content:space-between;font-family:JetBrains Mono,monospace;font-size:10px;margin-bottom:5px;');
    opTop.appendChild(el('span', 'color:#6f8496;letter-spacing:2px;', 'ORDNANCE POINTS'));
    opTop.appendChild(el('span', 'color:' + opColor + ';', op.used + ' / ' + op.max));
    opWrap.appendChild(opTop);
    var opBar = el('div', 'height:8px;background:rgba(0,0,0,.5);border:1px solid rgba(120,150,170,.15);');
    opBar.appendChild(el('div', 'height:100%;width:' + Math.min(100, op.max ? op.used / op.max * 100 : 0) + '%;background:' + opColor + ';box-shadow:0 0 10px ' + opColor + ';'));
    opWrap.appendChild(opBar);
    cen.appendChild(opWrap);
    grid.appendChild(cen);
    requestAnimationFrame(function () { drawSchematic(cv, hull, slots, fitDraft, refitSlot); });

    // ---- right: slots + armory + save ----
    var right = el('div', null, null, 'col'); right.style.gap = '12px';
    right.appendChild(el('div', null, 'WEAPON SLOTS', 'sh'));
    var slist = el('div', 'display:flex;flex-direction:column;gap:5px;max-height:170px;', null, 'scy');
    if (!slots.length) slist.appendChild(el('div', null, '(no slot data for this hull)', 'msg'));
    slots.forEach(function (sl) {
      var w = fitDraft.weapons[sl.slot_id] ? weaponById(fitDraft.weapons[sl.slot_id]) : null;
      var on = refitSlot === sl.slot_id;
      var row = el('div', 'display:flex;align-items:center;gap:10px;padding:9px 11px;cursor:pointer;background:rgba(14,19,27,.6);border:1px solid ' + (on ? 'var(--ac1)' : 'rgba(120,150,170,.18)') + ';border-left:3px solid ' + (on ? 'var(--ac1)' : 'rgba(120,150,170,.3)') + ';');
      row.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:' + (on ? 'var(--ac2)' : '#6f8496') + ';width:62px;flex-shrink:0;', sl.size + ' ' + sl.type));
      row.appendChild(el('div', 'flex:1;font-size:12.5px;color:' + (w ? '#dbe5ed' : '#6f8496') + ';overflow:hidden;text-overflow:ellipsis;white-space:nowrap;', w ? w.name : '— empty —'));
      row.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;flex-shrink:0;', w ? (w.op_cost || 0) + ' OP' : ''));
      row.addEventListener('click', function () { refitSlot = sl.slot_id; renderScreen(); });
      slist.appendChild(row);
    });
    right.appendChild(slist);

    var armory = el('div', 'flex:1;background:rgba(12,17,24,.55);border:1px solid rgba(var(--acRGB),.14);padding:14px;', null, 'col');
    if (!refitSlot) { armory.appendChild(el('div', null, 'Select a weapon slot ◄', 'msg')); }
    else {
      var slot = slots.filter(function (x) { return x.slot_id === refitSlot; })[0];
      var ah = el('div', 'display:flex;justify-content:space-between;align-items:center;');
      ah.appendChild(el('div', 'color:var(--ac2);', '▸ ' + (slot ? slot.size + ' ' + slot.type : '') + ' ARMORY', 'sh'));
      var clr = el('div', 'cursor:pointer;color:#e0584f;font-family:JetBrains Mono,monospace;font-size:9px;', 'CLEAR ✕');
      clr.addEventListener('click', function () { delete fitDraft.weapons[refitSlot]; renderScreen(); });
      ah.appendChild(clr);
      armory.appendChild(ah);
      var wl = el('div', 'flex:1;display:flex;flex-direction:column;gap:6px;margin-top:10px;', null, 'scy');
      (hd.weapons || []).filter(function (w) { return slotAccepts(slot, w); }).forEach(function (w) {
        var fitted = fitDraft.weapons[refitSlot] === w.weapon_id;
        var row = el('div', 'display:flex;align-items:center;gap:11px;padding:10px 12px;cursor:pointer;background:rgba(14,19,27,.6);border:1px solid ' + (fitted ? 'var(--ac1)' : 'rgba(120,150,170,.18)') + ';');
        var dia = el('div', 'width:28px;height:28px;flex-shrink:0;display:flex;align-items:center;justify-content:center;font-size:13px;color:var(--ac2);border:1px solid ' + (fitted ? 'var(--ac1)' : 'rgba(120,150,170,.18)') + ';transform:rotate(45deg);');
        dia.appendChild(el('span', 'transform:rotate(-45deg);', '◣'));
        row.appendChild(dia);
        var mid = el('div', 'flex:1;min-width:0;');
        mid.appendChild(el('div', 'font-size:13px;font-weight:600;color:' + (fitted ? 'var(--ac2)' : '#dbe5ed') + ';', w.name));
        mid.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;', (w.weapon_type || '') + ' · ' + (w.damage_type || '')));
        row.appendChild(mid);
        var rr = el('div', 'text-align:right;flex-shrink:0;');
        rr.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:11px;font-weight:700;color:var(--ac2);', (w.op_cost || 0) + ' OP'));
        rr.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;color:#6f8496;', Math.round(w.damage_per_shot || 0) + ' dmg'));
        row.appendChild(rr);
        row.addEventListener('click', function () { fitDraft.weapons[refitSlot] = w.weapon_id; renderScreen(); });
        wl.appendChild(row);
      });
      armory.appendChild(wl);
    }
    right.appendChild(armory);
    var save = el('div', 'text-align:center;padding:11px;cursor:pointer;color:#0a0d12;font-weight:700;letter-spacing:2px;font-family:JetBrains Mono,monospace;background:linear-gradient(180deg,var(--ac2),var(--ac1));', 'SAVE LOADOUT → DRYDOCK');
    save.addEventListener('click', function () {
      net.send({ action: 'fit_ship', ship_id: refitShipId, fitted_weapons: fitDraft.weapons, fitted_hullmods: fitDraft.mods, vents: fitDraft.vents, capacitors: fitDraft.caps });
    });
    right.appendChild(save);
    grid.appendChild(right);
  }

  function renderCargo(host) {
    var inv = srv['s_inventory_update'];
    var grid = screenGrid('1fr 330px'); host.appendChild(grid);
    var man = el('div', 'padding:18px 20px;', null, 'col box bevel scy');
    man.appendChild(el('div', null, 'CARGO MANIFEST', 'sh'));
    if (!inv) { man.appendChild(el('div', 'margin-top:10px;', 'No cargo data yet.', 'msg')); }
    else {
      var cargo = inv.cargo || [];
      if (!cargo.length) man.appendChild(el('div', 'margin-top:10px;', 'Hold is empty.', 'msg'));
      cargo.forEach(function (it) {
        var row = el('div', 'display:flex;justify-content:space-between;padding:9px 2px;border-bottom:1px solid rgba(120,150,170,.1);font-family:JetBrains Mono,monospace;font-size:12px;color:#c7d3dd;');
        row.appendChild(el('span', null, it.name || ('#' + it.definition_id)));
        row.appendChild(el('span', 'color:var(--ac2);', '×' + it.quantity));
        man.appendChild(row);
      });
    }
    grid.appendChild(man);
    var side = el('div', 'padding:18px 20px;', null, 'col box bevel');
    side.appendChild(el('div', null, 'HOLD SUMMARY', 'sh'));
    side.appendChild(el('div', 'font-size:34px;font-weight:700;color:#e8b04b;margin-top:12px;', inv ? String(inv.credits || 0) : '—'));
    side.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;letter-spacing:2px;color:#6f8496;', 'CREDITS'));
    side.appendChild(el('div', 'height:1px;background:rgba(120,150,170,.16);margin:14px 0;'));
    side.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:12px;color:#c7d3dd;', 'ITEM TYPES: ' + (inv && inv.cargo ? inv.cargo.length : 0)));
    grid.appendChild(side);
  }

  function renderSkills(host) {
    var pp = srv['s_player_progress'];
    var grid = screenGrid('312px 1fr'); host.appendChild(grid);
    var prof = el('div', 'padding:20px 22px;', null, 'col box bevel');
    prof.appendChild(el('div', null, 'COMMAND PROFILE', 'sh'));
    prof.appendChild(el('div', 'width:74px;height:74px;margin-top:14px;display:flex;align-items:center;justify-content:center;font-size:40px;color:var(--ac2);border:1px solid rgba(var(--acRGB),.3);background:rgba(12,17,24,.6);', '☰'));
    prof.appendChild(el('div', 'font-size:19px;font-weight:700;margin-top:12px;', ($('email') && $('email').value) || 'COMMANDER'));
    prof.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;letter-spacing:2px;color:#6f8496;margin-top:2px;', 'FLEET COMMANDER'));
    prof.appendChild(el('div', 'height:1px;background:rgba(120,150,170,.16);margin:14px 0;'));
    var lvl = (pp && pp.command_level != null) ? pp.command_level : (pp && pp.skills ? pp.skills.length : '—');
    prof.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:12px;color:#c7d3dd;', 'COMMAND LEVEL: ' + lvl));
    grid.appendChild(prof);
    var skillsCol = el('div', 'padding:2px;', null, 'col scy');
    skillsCol.appendChild(el('div', 'padding:0 0 12px;', 'SKILL MATRIX', 'sh'));
    if (!pp || !pp.skills || !pp.skills.length) { skillsCol.appendChild(el('div', null, 'No progression data yet.', 'msg')); }
    else {
      var sg = el('div', 'display:grid;grid-template-columns:1fr 1fr;gap:10px;');
      pp.skills.forEach(function (sk) {
        var c = el('div', 'padding:14px 16px;', null, 'box');
        c.appendChild(el('div', 'font-size:14px;font-weight:700;', (sk.key || '').toUpperCase()));
        c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;color:var(--ac2);margin-top:2px;', 'LEVEL ' + sk.level));
        c.appendChild(bar((sk.xp || 0) / (sk.xp_next || 1), 'var(--ac1)', 5));
        c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;margin-top:4px;', 'XP ' + sk.xp + ' / ' + sk.xp_next));
        sg.appendChild(c);
      });
      skillsCol.appendChild(sg);
    }
    grid.appendChild(skillsCol);
  }

  var RSTATUS = {
    completed: ['#6fa8c0', 'COMPLETED'], active: ['#57c98a', 'IN PROGRESS'],
    available: ['var(--ac2)', 'AVAILABLE'], locked: ['#6f8496', 'LOCKED']
  };
  function renderResearch(host) {
    var rs = srv['s_research_status'];
    var grid = screenGrid('1fr 330px'); host.appendChild(grid);
    var projects = (rs && rs.projects) || [];
    if (!projects.length) { grid.appendChild(el('div', null, 'No research data yet.', 'msg')); return; }
    if (!selResearchId || !projects.filter(function (p) { return p.id === selResearchId; }).length) selResearchId = projects[0].id;

    // left: project list
    var left = el('div', 'padding:18px 20px;', null, 'col box bevel scy');
    left.appendChild(el('div', null, 'RESEARCH PROJECTS', 'sh'));
    var listWrap = el('div', 'display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:12px;');
    projects.forEach(function (p) {
      var st = RSTATUS[p.status] || RSTATUS.locked, on = p.id === selResearchId;
      var c = el('div', 'padding:12px 14px;cursor:pointer;background:rgba(14,19,27,.7);border:1px solid ' + (on ? 'var(--ac1)' : 'rgba(120,150,170,.16)') + ';border-left:3px solid ' + st[0] + ';');
      c.appendChild(el('div', 'font-size:13px;font-weight:700;color:#dbe5ed;', p.name || p.id));
      c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;letter-spacing:1px;color:' + st[0] + ';margin-top:3px;', st[1]));
      if (p.status === 'active') c.appendChild(bar((p.progress || 0) / (p.total_time || 1), '#57c98a', 4));
      else c.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;margin-top:4px;', (p.cost || 0) + ' cr · ' + Math.round(p.time_seconds || 0) + 's'));
      c.addEventListener('click', function () { selResearchId = p.id; renderScreen(); });
      listWrap.appendChild(c);
    });
    left.appendChild(listWrap);
    grid.appendChild(left);

    // right: project dossier
    var p = projects.filter(function (x) { return x.id === selResearchId; })[0];
    var st = RSTATUS[p.status] || RSTATUS.locked;
    var right = el('div', 'padding:20px 22px;', null, 'col box bevel scy');
    right.appendChild(el('div', null, 'PROJECT DOSSIER', 'sh'));
    right.appendChild(el('div', 'font-size:42px;margin-top:8px;color:' + st[0] + ';', '⚛'));
    right.appendChild(el('div', 'font-size:19px;font-weight:700;', p.name || p.id));
    right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:10px;letter-spacing:2px;color:' + st[0] + ';margin-top:2px;', st[1]));
    right.appendChild(el('div', 'height:1px;background:rgba(120,150,170,.16);margin:14px 0;'));
    var statG = el('div', 'display:grid;grid-template-columns:1fr 1fr;gap:1px;background:rgba(120,150,170,.1);');
    [['COST', (p.cost || 0) + ' cr'], ['TIME', Math.round(p.time_seconds || 0) + 's']].forEach(function (s) {
      var cell = el('div', 'padding:11px 13px;background:rgba(12,17,24,.85);');
      cell.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;letter-spacing:1px;color:#6f8496;', s[0]));
      cell.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:14px;font-weight:700;color:#cdd9e3;margin-top:3px;', s[1]));
      statG.appendChild(cell);
    });
    right.appendChild(statG);
    if (p.status === 'active') {
      right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;margin-top:14px;', 'PROGRESS'));
      right.appendChild(bar((p.progress || 0) / (p.total_time || 1), '#57c98a', 6));
    }
    if (p.prereqs && p.prereqs.length) {
      right.appendChild(el('div', 'margin-top:14px;', 'PREREQUISITES', 'sh'));
      right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:11px;color:#c7d3dd;margin-top:5px;', p.prereqs.join(', ')));
    }
    if (p.unlocks && p.unlocks.length) {
      right.appendChild(el('div', 'margin-top:12px;', 'UNLOCKS', 'sh'));
      right.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:11px;color:var(--ac2);margin-top:5px;', p.unlocks.join(', ')));
    }
    if (p.status === 'available') {
      var start = el('div', 'margin-top:16px;text-align:center;padding:11px;cursor:pointer;color:#0a0d12;font-weight:700;letter-spacing:2px;font-family:JetBrains Mono,monospace;background:linear-gradient(180deg,var(--ac2),var(--ac1));', 'BEGIN RESEARCH');
      start.addEventListener('click', function () { net.send({ action: 'start_research', project_id: p.id }); });
      right.appendChild(start);
    }
    grid.appendChild(right);
  }

  function renderColony(host) {
    var cs = srv['s_colonies_status'];
    var wrap = el('div', 'position:absolute;inset:0;z-index:1;display:flex;flex-direction:column;padding:22px 24px;gap:16px;');
    var colonies = (cs && cs.colonies) || [];
    // summary banner
    var bases = colonies.filter(function (c) { return c.kind === 'base'; }).length;
    var planets = colonies.filter(function (c) { return c.kind === 'planet'; }).length;
    var banner = el('div', 'display:flex;align-items:center;gap:24px;padding:18px 22px;', null, 'box bevel');
    banner.appendChild(el('div', 'font-size:40px;color:var(--ac2);', '◉'));
    var bt = el('div', 'flex:1;');
    bt.appendChild(el('div', null, 'COLONIAL HOLDINGS', 'sh'));
    bt.appendChild(el('div', 'font-size:22px;font-weight:700;', bases + ' BASES · ' + planets + ' PLANETS'));
    banner.appendChild(bt);
    var build = el('div', 'padding:11px 20px;cursor:pointer;color:#0a0d12;font-weight:700;letter-spacing:2px;font-family:JetBrains Mono,monospace;background:linear-gradient(180deg,var(--ac2),var(--ac1));', '＋ BUILD BASE');
    build.addEventListener('click', function () { net.send({ action: 'build_base' }); setTimeout(function () { net.send({ action: 'get_colonies' }); }, 300); });
    banner.appendChild(build);
    wrap.appendChild(banner);

    if (!colonies.length) {
      wrap.appendChild(el('div', 'margin-top:6px;', 'No colonies in this system. BUILD BASE deploys a station at your ship (costs cargo); claim & DEVELOP a planet from the open world.', 'msg'));
      host.appendChild(wrap); return;
    }

    var gridC = el('div', 'display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:14px;overflow-y:auto;', null, 'scy');
    colonies.forEach(function (c) {
      var isBase = c.kind === 'base';
      var card = el('div', 'padding:16px 18px;', null, 'box');
      card.style.borderLeft = '3px solid ' + (isBase ? 'var(--ac1)' : '#57c98a');
      var hd = el('div', 'display:flex;align-items:center;gap:11px;');
      hd.appendChild(el('div', 'font-size:26px;color:' + (isBase ? 'var(--ac2)' : '#57c98a') + ';', isBase ? '⬢' : '◉'));
      var ht = el('div', 'flex:1;');
      ht.appendChild(el('div', 'font-size:15px;font-weight:700;', (isBase ? 'Space Base' : 'Planet') + ' #' + c.entity_id));
      ht.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:9px;color:#6f8496;letter-spacing:1px;', (isBase ? 'LEVEL ' : 'DEV LEVEL ') + (c.level || 0) + ' · SYS ' + (c.system_id || 1)));
      hd.appendChild(ht);
      card.appendChild(hd);
      if (isBase && c.max_health) {
        card.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:8px;color:#6f8496;margin-top:10px;', 'INTEGRITY'));
        card.appendChild(bar((c.health || 0) / (c.max_health || 1), '#57c98a', 5));
      }
      card.appendChild(el('div', 'font-family:JetBrains Mono,monospace;font-size:11px;color:var(--ac2);margin-top:10px;', '+' + (c.income || 0) + ' ' + (c.income_unit || '') + ' / cycle'));
      var act = el('div', 'margin-top:12px;text-align:center;padding:9px;cursor:pointer;font-family:JetBrains Mono,monospace;font-size:10px;letter-spacing:1px;color:#0a0d12;font-weight:700;background:linear-gradient(180deg,var(--ac2),var(--ac1));',
        isBase ? 'UPGRADE BASE' : 'DEVELOP PLANET');
      act.addEventListener('click', function () {
        net.send({ action: isBase ? 'upgrade_base' : 'develop_planet', target_id: c.entity_id });
        setTimeout(function () { net.send({ action: 'get_colonies' }); }, 300);
      });
      card.appendChild(act);
      gridC.appendChild(card);
    });
    wrap.appendChild(gridC);
    host.appendChild(wrap);
  }

  function renderMap(host) {
    var wrap = el('div', 'position:absolute;inset:0;z-index:1;display:flex;align-items:center;justify-content:center;');
    var box = el('div', 'padding:34px 44px;text-align:center;', null, 'box bevel');
    box.appendChild(el('div', 'font-size:42px;color:var(--ac2);', '✧'));
    box.appendChild(el('div', 'font-size:20px;font-weight:700;letter-spacing:2px;margin-top:8px;', 'SECTOR MAP'));
    box.appendChild(el('div', 'margin-top:8px;', 'Star chart not yet wired to the live server.', 'msg'));
    box.appendChild(el('div', 'margin-top:4px;', 'CURRENT SYSTEM: ' + (net.currentSystemID || 1), 'msg'));
    wrap.appendChild(box);
    host.appendChild(wrap);
  }

  // ---- enter battle: attack the nearest hostile NPC in the open world -------------------
  $('engageBtn').addEventListener('click', function () {
    var me = net.getLocalPlayer();
    var best = null, bd = 1e18;
    net.entities.forEach(function (e) {
      if (e.type !== 1) return;                              // NPC ships only
      if (net.localPlayerFactionID && e.factionId === net.localPlayerFactionID) return; // not allies
      if (me) {
        var d = (e.targetX - me.targetX) * (e.targetX - me.targetX) + (e.targetY - me.targetY) * (e.targetY - me.targetY);
        if (d < bd) { bd = d; best = e; }
      } else if (!best) { best = e; }
    });
    if (best) { net.send({ action: 'attack', target_id: best.id }); $('idleMsg').textContent = 'ENGAGING ' + (best.name || 'hostile') + ' — closing to combat…'; }
    else { $('idleMsg').textContent = 'no hostiles detected in this system'; }
  });

  $('disengage').addEventListener('click', function () {
    // Battles resolve automatically on the server; this is informational for now.
    $('idleMsg').textContent = 'engagement resolves automatically';
  });
  $('focusBtn').addEventListener('click', function () { adapter.focusPlayer(); });

  // ---- camera controls ------------------------------------------------------------------
  var dragging = false, lastX = 0, lastY = 0, downX = 0, downY = 0, moved = false;
  canvas.addEventListener('wheel', function (e) {
    e.preventDefault();
    var r = canvas.getBoundingClientRect();
    adapter.zoomBy(e.deltaY, (e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height);
  }, { passive: false });
  canvas.addEventListener('pointerdown', function (e) { dragging = true; lastX = e.clientX; lastY = e.clientY; downX = e.clientX; downY = e.clientY; moved = false; });
  window.addEventListener('pointerup', function () { dragging = false; });
  window.addEventListener('pointermove', function (e) {
    if (!dragging) return;
    if (Math.abs(e.clientX - downX) + Math.abs(e.clientY - downY) > 6) moved = true;
    var r = canvas.getBoundingClientRect();
    adapter.panScreen(e.clientX - lastX, e.clientY - lastY, r.height);
    lastX = e.clientX; lastY = e.clientY;
  });

  // Click a ship to select it (left panel + FOCUS follow it). Uses the same Y mirror as the
  // damage overlay: the visible hull is at css-y = H - sy().
  canvas.addEventListener('click', function (e) {
    if (moved || !renderer || !adapter.inBattle()) return;
    var r = canvas.getBoundingClientRect();
    var cx = e.clientX - r.left, cy = e.clientY - r.top;
    var H = B.gl_Hcss || canvas.clientHeight || 700;
    var best = null, bd = 1e9;
    for (var i = 0; i < B.ships.length; i++) {
      var s = B.ships[i]; if (s.dead) continue;
      var p = renderer.toScreen(B, s.x, s.y);
      var dx = p[0] - cx, dy = (H - p[1]) - cy, d = dx * dx + dy * dy;
      var rad = Math.max(22, s.radius * 3.4 * B.zoom);
      if (d < rad * rad && d < bd) { bd = d; best = s; }
    }
    if (best) adapter.selectedId = best._id;
  });

  // ---- HUD binding ----------------------------------------------------------------------
  function shipStatus(s) {
    if (!s) return '—';
    if (s.dead) return 'DESTROYED';
    if (s.overload) return 'OVERLOAD';
    if (s.shieldOn && s.shieldDeploy > 0.2) return 'SHIELDS UP';
    return 'SHIELDS DOWN';
  }
  function pct(v, m) { return Math.max(0, Math.min(100, m > 0 ? (v / m) * 100 : 0)); }

  function updateHUD() {
    var fs = adapter.fleetStats();
    $('pCount').textContent = fs.player.n + (fs.player.n === 1 ? ' SHIP' : ' SHIPS');
    $('eCount').textContent = fs.enemy.n + (fs.enemy.n === 1 ? ' SHIP' : ' SHIPS');
    $('pBar').style.width = pct(fs.player.hull, fs.player.max) + '%';
    $('eBar').style.width = pct(fs.enemy.hull, fs.enemy.max) + '%';

    var me = adapter.playerShip();
    $('selName').textContent = me ? me.name : '—';
    $('selCls').textContent = me ? String(me.cls).toUpperCase() : '—';
    $('selStatus').textContent = shipStatus(me);
    $('selHull').style.width = me ? pct(me.hull, me.hullMax) + '%' : '0%';
    $('selFlux').style.width = me ? pct(me.flux, me.fluxMax) + '%' : '0%';

    var en = adapter.nearestEnemy(me || adapter.B.ships[0]);
    var enPanel = $('enPanel');
    if (en) {
      enPanel.classList.remove('hidden');
      $('enName').textContent = en.name;
      $('enCls').textContent = String(en.cls).toUpperCase() + ' · HOSTILE';
      $('enStatus').textContent = shipStatus(en);
      $('enHull').style.width = pct(en.hull, en.hullMax) + '%';
      $('enFlux').style.width = pct(en.flux, en.fluxMax) + '%';
    } else { enPanel.classList.add('hidden'); }
  }

  // ---- render loop ----------------------------------------------------------------------
  var last = 0;
  function loop(ts) {
    requestAnimationFrame(loop);
    var dt = last ? (ts - last) : 16.667;
    var dtF = last ? Math.max(0.1, Math.min(4, dt / 16.667)) : 1;
    last = ts;

    adapter.update(dtF);
    if (renderer) renderer.render(B);
    drawDmg(dtF);

    var inBattle = authed && adapter.inBattle();
    updateChrome();
    if (inBattle) updateHUD();
    else if (authed) updateTopbar(1000 / Math.max(1, dt));
  }
  requestAnimationFrame(loop);
})();
