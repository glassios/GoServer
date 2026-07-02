/*
 * net.js — сетевой слой WebClientUI (framework-agnostic, без dc-runtime/React).
 *
 * Порт проверенной логики из cmd/gateway/static/index.html (connectWS /
 * handleServerMessage / processWorldSnapshot) и паттерна off-loop роутинга из
 * UnityClient GalaxyClient.cs. Живёт глобально (window.NetClient) и владеет:
 *   - WebSocket-соединением (auth + авто-реконнект с повторной авторизацией);
 *   - картой сущностей entities: Map<id, EntityState> с prev/target для
 *     интерполяции в рендере (имена полей 1:1 с index.html, чтобы world.js
 *     портировался без переименований);
 *   - focusCounts (focus-fire), currentSystemID, localPlayerID и пр.
 *
 * Снапшоты (20 Гц) обрабатываются ЗДЕСЬ и НЕ должны гнаться через React.setState.
 * Рендер (world.js) читает net.entities напрямую в rAF-цикле. Для панелей UI
 * подписывается на редкие/троттлингованные хуки.
 */
(function () {
  'use strict';

  // EntityType ints (см. domain.EntityType / messages.proto):
  // 0 player, 1 npc, 2 asteroid, 3 projectile, 4 station, 5 jump-gate,
  // 6 loot, 7 combat-marker, 8 space-base, 9 planet.

  class NetClient {
    constructor() {
      this.socket = null;
      this.url = null;
      this.login = '';

      // Игровое состояние
      this.currentSystemID = 1;
      this.entities = new Map();   // EntityID -> EntityState (поля как в index.html)
      this.focusCounts = {};       // entityID -> сколько кораблей по нему стреляют
      this.localPlayerID = null;
      this.localPlayerFactionID = null;
      this.isSpectator = true;
      this.connected = false;

      // Метрики
      this.serverTPS = 20;
      this._lastSnapTime = 0;

      // Реконнект
      this._reconnectTimer = null;
      this._wantClose = false;

      // ---- Хуки (назначаются приложением/рендером; все опциональны) ----
      this.onStatus = null;            // (connected:bool, text:string)
      this.onAuth = null;              // (entityId:number)
      this.onSystemTransition = null;  // (newSystemID:number)
      this.onTyped = null;             // (type:string, data:object)  — все типизированные пуши
      this.onChat = null;              // (sender:string, message:string, category:string)
      this.onSnapshot = null;          // () — после обработки снапшота (для троттлинга HUD)
      this.onEntitySpawn = null;       // (ent) — появился новый объект
      this.onEntityRemove = null;      // (ent) — объект исчез из снапшота
      this.onDamageFloat = null;       // (x, y, amount:number, damageType:string, isShield:bool)
      this.onLog = null;               // (message:string, category:string)
      this.onFire = null;              // (fireEvents:array) — залпы летящих снарядов этого тика (B3)
      this.onMissiles = null;          // (missiles:array) — авторитетные позиции летящих ракет этого тика (B4)
    }

    // ---------------------------------------------------------------------
    // Соединение
    // ---------------------------------------------------------------------
    connect(url, login) {
      if (login != null) this.login = String(login);
      // URL по умолчанию — тот же хост, что отдал страницу (роут /play/ на gateway).
      if (!url) {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        url = `${proto}//${window.location.host}/ws`;
      }
      this.url = url;
      this._wantClose = false;
      this._open();
    }

    _open() {
      try {
        this.socket = new WebSocket(this.url);
      } catch (e) {
        this._scheduleReconnect();
        return;
      }

      this.socket.onopen = () => {
        this.connected = true;
        this._emit(this.onStatus, true, 'CONNECTED');
        this._log('Соединение с шлюзом установлено', 'system');
        // Авторизуемся (и при реконнекте — повторно тем же логином).
        if (this.login) this.send({ action: 'auth', login: this.login });
      };

      this.socket.onclose = () => {
        this.connected = false;
        this._emit(this.onStatus, false, 'DISCONNECTED');
        this._log('Соединение разорвано. Повторное подключение...', 'system');
        // Сброс визуального/боевого состояния, чтобы не оставались «призраки».
        this.entities.clear();
        this.focusCounts = {};
        if (!this._wantClose) this._scheduleReconnect();
      };

      this.socket.onerror = (err) => { /* onclose последует и запланирует реконнект */ };

      this.socket.onmessage = (event) => {
        let msg;
        try { msg = JSON.parse(event.data); }
        catch (e) { return; }
        this._handle(msg);
      };
    }

    _scheduleReconnect() {
      if (this._reconnectTimer || this._wantClose) return;
      this._reconnectTimer = setTimeout(() => {
        this._reconnectTimer = null;
        this._open();
      }, 2000);
    }

    close() {
      this._wantClose = true;
      if (this._reconnectTimer) { clearTimeout(this._reconnectTimer); this._reconnectTimer = null; }
      try { this.socket && this.socket.close(); } catch (e) {}
      this.socket = null;
      this.connected = false;
    }

    // Отправить действие ({action:"move", x, y} и т.п.). Возвращает true, если ушло.
    send(action) {
      if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return false;
      try { this.socket.send(JSON.stringify(action)); return true; }
      catch (e) { return false; }
    }

    // Локальный игрок (или null).
    getLocalPlayer() {
      return (this.localPlayerID && this.entities.get(this.localPlayerID)) || null;
    }

    // ---------------------------------------------------------------------
    // Маршрутизация входящих
    // ---------------------------------------------------------------------
    _handle(msg) {
      // Снапшот мира: {system_id, snapshot:{tick, entities:[...]}}.
      if (msg.system_id && msg.snapshot) {
        if (Number(msg.system_id) !== this.currentSystemID) return; // чужая система — игнор
        this._processSnapshot(msg.snapshot);
        return;
      }

      const type = msg.type;
      if (!type) return;

      if (type === 'system_transition') {
        const newSysID = Number(msg.system_id);
        this._log(`Переход в систему ${newSysID}...`, 'system');
        this.currentSystemID = newSysID;
        this.entities.clear();
        this.focusCounts = {};
        // Остановить корабль, перекрыв любые «в полёте» команды движения.
        this.send({ action: 'move', x: 0, y: 0 });
        this._emit(this.onSystemTransition, newSysID);
        return;
      }

      if (type === 'auth_response') {
        const d = msg.data || {};
        if (d.success) {
          this.localPlayerID = Number(d.entity_id || d.entityId);
          this.isSpectator = false;
          this._log('Авторизация выполнена! ID корабля: ' + this.localPlayerID, 'join');
          this._emit(this.onAuth, this.localPlayerID);
        } else {
          this._log('Авторизация отклонена: ' + (d.error_message || '?'), 'combat');
          this._emit(this.onAuth, 0);
        }
        return;
      }

      if (type === 'chatmessage' || type === 's_chat_message') {
        const d = msg.data || {};
        const sender = d.sender || 'System';
        let category = 'system';
        if (sender === 'System') {
          const m = d.message || '';
          category = (m.includes('Ошибка') || m.includes('недостаточно') || m.includes('переполнен'))
            ? 'combat' : 'join';
        }
        this._emit(this.onChat, sender, d.message || '', category);
        this._log((sender === 'System' ? 'Система: ' : `[CHAT] ${sender}: `) + (d.message || ''), category);
        return;
      }

      // Остальные типизированные пуши (инвентарь/рынок/склад/флот/ангар/
      // производство/прогресс/исследования) — нормализуем алиасы и отдаём в UI.
      const norm = NetClient.TYPE_ALIASES[type] || type;
      this._emit(this.onTyped, norm, msg.data || {});
    }

    // ---------------------------------------------------------------------
    // Обработка снапшота мира (порт processWorldSnapshot из index.html)
    // ---------------------------------------------------------------------
    _processSnapshot(snap) {
      const now = Date.now();
      const incoming = snap.entities || [];

      if (this._lastSnapTime) {
        let tps = Math.round(1000 / Math.max(1, now - this._lastSnapTime));
        if (tps > 30) tps = 20;
        this.serverTPS = tps;
      }
      this._lastSnapTime = now;

      const activeIds = new Set();

      for (const ent of incoming) {
        const id = Number(ent.entity_id);
        activeIds.add(id);
        const old = this.entities.get(id);

        if (old) {
          // Дельты HP/щита -> плавающие числа урона + искры (через хук рендера).
          if (ent.hp < old.hp) {
            this._emit(this.onDamageFloat, ent.x, ent.y, old.hp - ent.hp, ent.last_damage_type || '', false);
            if (id === this.localPlayerID) this._log(`Ваш корабль получил урон: -${old.hp - ent.hp} HP`, 'combat');
          }
          if (ent.shield < old.shield) {
            this._emit(this.onDamageFloat, ent.x, ent.y, old.shield - ent.shield, '', true);
          }
        }

        const st = this._mapEntity(ent, old, now);
        this.entities.set(id, st);
        if (!old) this._emit(this.onEntitySpawn, st);
      }

      // Удаляем пропавшие из снапшота.
      for (const key of Array.from(this.entities.keys())) {
        if (!activeIds.has(key)) {
          const ent = this.entities.get(key);
          this._emit(this.onEntityRemove, ent);
          this.entities.delete(key);
        }
      }

      // Пересчёт focus-fire (сколько кораблей стреляют по каждой цели).
      const fc = {};
      for (const e of this.entities.values()) {
        if (e.isShooting && e.targetEntityId) fc[e.targetEntityId] = (fc[e.targetEntityId] || 0) + 1;
      }
      this.focusCounts = fc;

      // Запомнить фракцию игрока (нужно для командных цветов в бою).
      const p = this.getLocalPlayer();
      if (p && p.factionId) this.localPlayerFactionID = p.factionId;

      // Летящие снаряды этого тика (B3, тонкий канал): урон уже нанесён сервером,
      // это только косметика — рендер сам симулирует полёт болта.
      if (snap.fire_events && snap.fire_events.length) {
        this._emit(this.onFire, snap.fire_events);
      }

      // Live flying missiles this tick (B4, authoritative). Full list each tick (empty = none),
      // so the client can render true positions and drop missiles that were hit or shot down.
      if (this.onMissiles) this._emit(this.onMissiles, snap.missiles || []);

      this._emit(this.onSnapshot);
    }

    // Преобразование сырого EntitySnapshot -> EntityState (имена полей 1:1 с index.html).
    _mapEntity(ent, old, now) {
      return {
        id: Number(ent.entity_id),
        type: ent.entity_type,
        name: ent.name,
        targetX: ent.x,
        targetY: ent.y,
        targetRot: ent.rotation,
        prevX: old ? old.currX : ent.x,
        prevY: old ? old.currY : ent.y,
        prevRot: old ? old.currRot : ent.rotation,
        currX: old ? old.currX : ent.x,
        currY: old ? old.currY : ent.y,
        currRot: old ? old.currRot : ent.rotation,
        vx: ent.vx,
        vy: ent.vy,
        hp: ent.hp,
        maxHp: ent.max_hp,
        shield: ent.shield,
        maxShield: ent.max_shield,
        shieldArc: ent.shield_arc || 0,      // degrees (>=360 = omni); for directional-shield arc render
        shieldFacing: ent.shield_facing || 0, // world radians the shield is held toward
        armor: ent.armor || 0,
        maxArmor: ent.max_armor || 0,
        flux: ent.flux || 0,
        maxFlux: ent.max_flux || 0,
        overloaded: ent.overloaded || false,
        venting: ent.venting || false,
        overloadTimer: ent.overload_timer || 0,   // seconds left on overload lockout (B2/D)
        engineHit: ent.engine_hit || false,        // engines knocked out by a missile (B4)
        weaponHit: ent.weapon_hit || false,        // weapons suppressed by a missile (B4)
        shotsFired: ent.shots_fired || 0,
        lastDamageType: ent.last_damage_type || '',
        role: ent.role || '',
        strategy: ent.strategy || '',
        assistTargetId: ent.assist_target_id ? Number(ent.assist_target_id) : 0,
        assistType: ent.assist_type || '',
        factionId: ent.faction_id,
        corpId: ent.corp_id,
        targetEntityId: ent.target_id ? Number(ent.target_id) : 0,
        isShooting: ent.is_shooting,
        isMining: ent.is_mining || ent.isMining || false,
        refineryActive: ent.refinery_active,
        shipyardQueueLen: ent.shipyard_queue_len,
        shipType: ent.ship_type,
        baseLevel: ent.base_level || 0,
        priceIron: ent.price_iron,
        priceTitanium: ent.price_titanium,
        priceCrystal: ent.price_crystal,
        qtyIron: ent.qty_iron,
        qtyTitanium: ent.qty_titanium,
        qtyCrystal: ent.qty_crystal,
        qtyIronPlates: ent.qty_iron_plates,
        qtyTitaniumPlates: ent.qty_titanium_plates,
        cargoCapacity: ent.cargo_capacity,
        cargoLoad: ent.cargo_load,
        credits: Number(ent.credits || 0),
        lastShotTime: old ? (old.lastShotTime || 0) : 0,
        lastUpdate: now,
      };
    }

    // ---------------------------------------------------------------------
    _emit(fn, ...args) { if (typeof fn === 'function') { try { fn(...args); } catch (e) { /* хук не должен ронять сеть */ } } }
    _log(message, category) { this._emit(this.onLog, message, category); }
  }

  // Нормализация алиасов типов (сервер шлёт s_*; index.html принимал оба).
  NetClient.TYPE_ALIASES = {
    inventoryupdate: 's_inventory_update',
    vaultstatus: 's_vault_status',
    fleetstatus: 's_fleet_status',
    hangardata: 's_hangar_data',
    productionstatus: 's_production_status',
    playerprogressmsg: 's_player_progress',
    researchstatus: 's_research_status',
  };

  window.NetClient = NetClient;
})();
