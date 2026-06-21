# MMO Space Sandbox — Implementation Roadmap

> **Контекст**: 2D космос · Unity-клиент · Go · ECS-first · Clean Architecture  
> **MVP цель**: 1 звёздная система, 20 игроков, 100 NPC, mining, combat, persistence, economy  
> **Последнее обновление**: 2026-06-13

---

## Структура проекта

```
galaxy-mmo/
├── cmd/                            # Точки входа
│   ├── gateway/main.go             # Gateway сервер
│   ├── worldnode/main.go           # World Node сервер
│   └── tools/
│       └── botclient/main.go       # Бот для тестирования
│
├── internal/                       # Приватный код проекта
│   ├── domain/                     # Бизнес-логика (0 зависимостей от infra)
│   │   ├── entity.go               # Entity ID, базовые типы
│   │   ├── component.go            # Все ECS-компоненты
│   │   ├── events.go               # Доменные события
│   │   ├── errors.go               # Sentinel errors
│   │   └── ports.go                # Интерфейсы (репозитории, кэш и др.)
│   │
│   ├── ecs/                        # ECS движок
│   │   ├── world.go                # World — контейнер entities + components
│   │   ├── system.go               # System interface
│   │   ├── query.go                # Запросы компонентов по маске
│   │   └── archetype.go            # Archetype-based storage
│   │
│   ├── systems/                    # Игровые ECS-системы
│   │   ├── movement.go
│   │   ├── combat.go
│   │   ├── mining.go
│   │   ├── ai.go
│   │   ├── economy.go
│   │   ├── visibility.go
│   │   └── cleanup.go
│   │
│   ├── gameloop/                   # Tick engine
│   │   ├── loop.go                 # Fixed timestep loop
│   │   ├── scheduler.go            # Порядок выполнения систем
│   │   └── metrics.go              # Tick timing метрики
│   │
│   ├── spatial/                    # Пространственное разделение
│   │   ├── hashgrid.go             # Spatial hash grid
│   │   └── query.go                # Range/radius queries
│   │
│   ├── network/                    # Сетевой слой
│   │   ├── server.go               # UDP listener + goroutine pool
│   │   ├── session.go              # Сессия игрока
│   │   ├── reliability.go          # ACK/NACK, sequence numbers
│   │   ├── packet.go               # Packet framing, validation
│   │   └── snapshot.go             # Delta snapshots
│   │
│   ├── persistence/                # Работа с БД
│   │   ├── postgres/
│   │   │   ├── player_repo.go
│   │   │   ├── world_repo.go
│   │   │   └── migrations/
│   │   └── redis/
│   │       ├── session_cache.go
│   │       └── online_tracker.go
│   │
│   ├── auth/                       # Аутентификация
│   │   ├── service.go
│   │   └── token.go
│   │
│   └── config/                     # Конфигурация
│       ├── config.go
│       └── loader.go
│
├── pkg/                            # Публичные пакеты
│   ├── protocol/                   # Protobuf
│   │   ├── messages.proto
│   │   └── generated/
│   └── mathutil/                   # 2D векторная математика
│       ├── vec2.go
│       └── math.go
│
├── configs/
│   ├── server.yaml
│   └── gamedata/
│       ├── ships.json
│       ├── weapons.json
│       └── resources.json
│
├── scripts/
│   ├── proto-gen.sh
│   └── migrate.sh
│
├── deploy/
│   ├── Dockerfile
│   └── docker-compose.yaml
│
├── go.mod
├── go.sum
├── Makefile
└── .golangci.yml
```

> **Правило**: `internal/domain/` не импортирует ничего из `internal/persistence/`, `internal/network/` и т.д. Все зависимости идут через интерфейсы в `ports.go`.

---

## Phase 0 — Foundation

- [x] Инициализация `go.mod`
- [x] `Makefile` (build, run, test, bench, lint, proto, migrate)
- [x] `.golangci.yml` — конфигурация линтера
- [x] `configs/server.yaml` — типизированная конфигурация сервера
- [x] `internal/config/config.go` — загрузка конфига из YAML + env overrides
- [x] `deploy/docker-compose.yaml` — PostgreSQL 16 + Redis 7 для dev
- [x] ✅ Верификация: `make build` компилируется, `make lint` — 0 ошибок

---

## Phase 1 — Minimal Vertical Slice

### Этап 1.1 — Domain Layer

- [x] `internal/domain/entity.go` — EntityID, EntityType (Player, NPC, Asteroid, Projectile, Station)
- [x] `internal/domain/component.go` — компоненты:
  - [x] Transform (X, Y, Rotation)
  - [x] Velocity (X, Y)
  - [x] Health (Current, Max)
  - [x] Shield (Current, Max, RegenRate)
  - [x] Weapon (Type, Damage, Range, Cooldown, LastFire)
  - [x] Cargo (Items map, Capacity)
  - [x] MiningLaser (Power, Range, Active, TargetID)
  - [x] AIState (Behavior, TargetID, HomePos, StateTimer)
  - [x] FactionMember (FactionID, Reputation map)
  - [x] ShipConfig (ShipType, MaxSpeed, TurnRate)
  - [x] PlayerData (AccountID, Name, Credits, SessionID)
- [x] `internal/domain/events.go` — доменные события (EntityDestroyed, DamageDealt, ResourceMined, PlayerConnected, PlayerDisconnected)
- [x] `internal/domain/errors.go` — sentinel errors
- [x] `internal/domain/ports.go` — интерфейсы:
  - [x] PlayerRepository
  - [x] WorldRepository
  - [x] SessionCache
  - [x] EventBus
- [x] `pkg/mathutil/vec2.go` — 2D вектор (Add, Sub, Mul, Normalize, Length, Distance, Dot, Rotate)
- [x] ✅ Верификация: всё компилируется, unit-тесты для Vec2

---

### Этап 1.2 — ECS Engine

- [x] `internal/ecs/world.go` — World:
  - [x] CreateEntity(entityType) EntityID
  - [x] DestroyEntity(id)
  - [x] AddComponent(id, comp)
  - [x] GetComponent(id, compType) 
  - [x] RemoveComponent(id, compType)
  - [x] Query(mask) []EntityID
  - [x] EntityCount()
- [x] `internal/ecs/system.go` — System interface (Name, Update, Priority)
- [x] `internal/ecs/query.go` — запросы по битовым маскам компонентов
- [x] `internal/ecs/archetype.go` — archetype-based storage
- [x] ✅ Верификация:
  - [x] Бенчмарк: создание 10,000 entities < 10ms
  - [x] Бенчмарк: Query по 10,000 entities < 1ms
  - [x] Unit-тесты: CRUD операции, query correctness

---

### Этап 1.3 — Game Systems (без сети)

- [x] `internal/systems/movement.go` — применяет Velocity к Transform, clamp к границам мира
- [x] `internal/systems/combat.go` — cooldown → hitscan → Shield → Health → events
- [x] `internal/systems/mining.go` — range check → extract resource → add to Cargo
- [x] `internal/systems/ai.go` — FSM:
  - [x] Miner: Idle → MoveTo(asteroid) → Mine → MoveTo(station) → Dock → repeat
  - [x] Pirate: Patrol → Detect(player) → Attack → Loot → Retreat
  - [x] Patrol: Patrol(route) → Detect(pirate) → Attack → Resume
- [x] `internal/systems/cleanup.go` — удаление мёртвых entities, истекших projectiles
- [x] ✅ Верификация:
  - [x] Unit-тесты для каждой системы
  - [x] Интеграционный тест: все системы в World, 100 тиков
  - [x] Бенчмарк: 1000 entities × 5 систем × 20 TPS

---

### Этап 1.4 — Tick Engine

- [x] `internal/gameloop/loop.go` — fixed timestep loop (20 TPS):
  - [x] processCommands() — drain command channel
  - [x] update() — выполнить все системы
  - [x] generateSnapshots() — подготовить данные для отправки
  - [x] Graceful stop через context cancellation
- [x] `internal/gameloop/scheduler.go` — порядок выполнения систем по приоритету
- [x] `internal/gameloop/metrics.go` — tick duration (avg, p99), entity count
- [x] ✅ Верификация:
  - [x] Тест: 20 TPS стабильно на 1000 entities
  - [x] Метрики tick duration логируются

> **Конкурентность**: один goroutine владеет ECS World. Входящие команды через buffered channel. Никаких мьютексов на World.
>
> ```
> [UDP Goroutines] → [Command Channel] → [Tick Goroutine] → [Snapshot Queue] → [Send Goroutines]
> ```

---

### Этап 1.5 — Spatial Hash Grid

- [x] `internal/spatial/hashgrid.go`:
  - [x] NewHashGrid(cellSize)
  - [x] Insert / Remove / Update
  - [x] QueryRadius(x, y, radius) []EntityID
  - [x] QueryRect(x1, y1, x2, y2) []EntityID
- [x] `internal/systems/visibility.go` — определение видимых entities через HashGrid
- [x] ✅ Верификация:
  - [x] Бенчмарк: QueryRadius на 10,000 entities < 0.5ms
  - [x] Unit-тесты: edge cases (entity на границе ячеек)

---

### Этап 1.6 — Protobuf Protocol

- [x] `pkg/protocol/messages.proto`:
  - [x] **Client → Server**: AuthRequest, MoveInput, ShootInput, MineInput, DockInput, BuyInput, SellInput, Ping
  - [x] **Server → Client**: AuthResponse, WorldSnapshot, DeltaSnapshot, DamageEvent, DeathEvent, ChatMessage, MarketData, InventoryUpdate, Pong
  - [x] **Packet wrapper**: sequence, ack, ack_bitfield, type, payload
- [x] `scripts/proto-gen.ps1` — скрипт генерации Go-кода
- [x] ✅ Верификация:
  - [x] protoc генерирует Go-код
  - [x] Бенчмарк: сериализация snapshot для 100 entities < 0.1ms

---

### Этап 1.7 — UDP Networking

- [x] `internal/network/server.go`:
  - [x] net.ListenUDP + read goroutine pool
  - [x] Packet size limit (1400 bytes, MTU-safe)
  - [x] Flood protection (rate limiter per address)
  - [x] Heartbeat / disconnect timeout (15s)
- [x] `internal/network/session.go`:
  - [x] Привязка UDPAddr → EntityID
  - [x] Состояния: Connecting → Authenticating → InGame → Disconnecting
- [x] `internal/network/reliability.go`:
  - [x] Sequence numbers (uint16, wrapping)
  - [x] ACK + 32-bit ack bitfield
  - [x] Reliable send queue + resend timer
  - [x] Unreliable messages — fire and forget
- [x] `internal/network/packet.go` — framing, validation, decode
- [x] ✅ Верификация:
  - [x] Integration test: бот подключается, авторизуется, отправляет MoveInput
  - [x] Test: flood protection отсекает > 60 pps

---

### Этап 1.8 — Snapshot System

- [x] `internal/network/snapshot.go`:
  - [x] Full snapshot при подключении
  - [x] Delta snapshots каждый тик (только изменения)
  - [x] Interest management: только видимые entities (из VisibilitySystem)
  - [x] Не отправлять неизменившиеся компоненты
- [x] ✅ Верификация:
  - [x] Бенчмарк: delta snapshot для 20 игроков, 100 NPC < 2ms
  - [x] Test: клиент получает только entities в зоне видимости

---

### Этап 1.9 — Persistence

- [x] `internal/persistence/postgres/migrations/001_initial.sql`:
  - [x] Таблица accounts (id, login, password_hash, created_at)
  - [x] Таблица characters (id, account_id, name, ship_type, x, y, rotation, credits, cargo jsonb)
  - [x] Таблица stations
  - [x] Таблица factions
- [x] `internal/persistence/postgres/player_repo.go` — реализация PlayerRepository
- [x] `internal/persistence/postgres/world_repo.go` — реализация WorldRepository
- [x] `internal/persistence/redis/session_cache.go` — реализация SessionCache
- [x] `internal/persistence/redis/online_tracker.go` — отслеживание онлайна
- [x] Стратегия сохранения: каждые 60 сек + при disconnect + при shutdown
- [x] ✅ Верификация:
  - [x] Integration-тест: Save → Load roundtrip
  - [x] Test: graceful shutdown сохраняет все данные

---

### Этап 1.10 — Auth

- [x] `internal/auth/service.go`:
  - [x] Регистрация: login + password → bcrypt hash → DB
  - [x] Вход: verify hash → create session → Redis
- [x] `internal/auth/token.go`:
  - [x] Session token: crypto/rand, 32 bytes, hex-encoded
  - [x] Expiry: 24 часа
- [x] ✅ Верификация:
  - [x] Unit-тесты: регистрация, вход, невалидный пароль
  - [x] Test: session expiry

---

### Этап 1.11 — NPC AI + Economy

- [x] Обновить `internal/systems/ai.go`:
  - [x] 3 типа NPC: Miner, Pirate, Patrol
  - [x] FSM переходы с таймерами
  - [x] Spawner — поддержание N NPC в системе
- [x] `internal/systems/economy.go`:
  - [x] Станции с Market (buy/sell)
  - [x] Динамическое ценообразование: `price = basePrice * (1 + demand - supply)`
  - [x] 4 ресурса: Iron, Titanium, Crystal, RareGas
- [x] ✅ Верификация:
  - [x] Тест: NPC miner полный цикл
  - [x] Тест: цена растёт при высоком спросе

---

### Этап 1.12 — Integration & Polish

- [x] `cmd/worldnode/main.go` — сборка всех компонентов:
  - [x] Config → DB/Redis → Repos → ECS World → Systems → GameLoop → Network → Graceful Shutdown
- [x] `cmd/tools/botclient/main.go` — бот: connect → auth → move → mine
- [x] Event Bus — связь между системами (CombatSystem → events → EconomySystem)
- [x] `configs/gamedata/*.json` — балансные данные (ships, weapons, resources)
- [x] ✅ Финальная верификация MVP:
  - [x] 20 ботов играют одновременно
  - [x] Mining → Sell → Buy цикл работает
  - [x] NPC патрулируют, атакуют, добывают
  - [x] PvP combat работает
  - [x] Reconnect восстанавливает состояние
  - [x] Graceful shutdown сохраняет всех
  - [x] Сервер стабилен 24 часа
  - [x] Tick time < 50ms при полной нагрузке

---

## Phase 2 — Multi-System Galaxy

- [x] Galaxy Router — переходы между системами (Gateway)
- [x] System Nodes — один процесс на систему (World Nodes)
- [x] Cross-node Transfer — миграция игрока + сериализация состояния (JumpGateSystem)
- [x] NATS — inter-node communication (NATS & MockMessageBus)
- [x] NPC Trade Routes — добыча и перевозка ресурсов между системами
- [x] 10 звёздных систем + jump gates

---

## Phase 3 — Corporations & Production

- [x] Corporations — membership, roles, permissions, taxes
- [x] Player Stations — mining, refineries, factories, shipyards
- [ ] Territory Control — (Deferred) influence, ownership, military presence

---

## Phase 4 — Advanced Simulation (TODO)

- [ ] Production Chains — Ore → Metal → Components → Weapons → Ships
- [ ] Logistics — cargo fleets, supply chains, convoy attacks
- [ ] Research — tech trees (Weapons, Industry, Mining, AI, Energy)
- [ ] Strategic AI — NPC фракции захватывают территории, ведут войны

---

## Phase 5 — Massive Battles (TODO)

- [ ] Combat Optimization — LOD simulation, projectile batching, tick degradation
- [ ] Time Dilation — 20 TPS → 5 TPS при overload
- [ ] Reinforced Nodes — отдельный node для больших боёв
- [ ] Prometheus + Grafana + pprof
- [ ] Load Testing — bot clients, combat simulators
- [ ] Цели: 1000 игроков, 10000 NPC, стабильный packet throughput

---

## Critical Rules

- ❌ Никогда не доверять клиенту
- ❌ Никогда не использовать JSON для networking
- ❌ Никогда не отправлять full world state
- ❌ Никогда не хранить всю галактику в одном процессе
- ✅ Server-side validation каждого пакета
- ✅ Rate limiting per action type
- ✅ Anti-speedhack (проверка перемещений на сервере)
- ✅ Unit-тесты и бенчмарки с Phase 1

---

## Tech Stack

| Категория | Технология |
|-----------|-----------|
| Язык | Go |
| Протокол | UDP + reliability layer |
| Сериализация | Protobuf |
| БД | PostgreSQL 16 |
| Кэш | Redis 7 |
| Messaging | NATS (Phase 2+) |
| Логирование | go.uber.org/zap |
| Мониторинг | Prometheus + Grafana (Phase 5) |
| Контейнеры | Docker |
| Клиент | Unity (2D) |

---

# Дорожная карта в стиле Starsector — бои и глубина (с 2026-06-21)

> Это **продолжение** карты выше (этапы 0–5 MVP в основном выполнены). Новый блок задаёт
> следующее направление и при пересечении тем заменяет легаси-пункты «Phase 4/5 (TODO)»
> (Research, Production Chains и т.п.) более конкретными, поэтапно поставляемыми шагами.

**Цель:** развить сервер в сторону флотовых боёв и глубины в духе Starsector — игроки собирают
флот из корпусов/оружия/модулей, оснащают каждый корабль, перед боем задают **стратегию и роль
для каждого корабля**, после чего наблюдают **полностью автоматический** тактический бой до 5 флотов —
с опорой на производство, добычу, исследования/навыки, крафт модулей и развитие планет/баз.

**Фундамент:** в коде уже существует «спящий» слой оснастки (`BakeShip`, типы
`ShipHull`/`WeaponDefinition`/`Hullmod`/`ShipConfiguration`, таблицы, репозиторий). Карта
**активирует и унифицирует** его, а не пишет с нуля.

### Подтверждённые решения
- **Модель корабля:** единый источник истины — система оснастки; игроки собирают и оснащают флот
  прямо в игре (авторитет на сервере).
- **Глубина боя:** базовая симуляция — флакс (flux), броневая сетка + типы урона, несколько
  оружейных слотов, дуги щитов, сброс флакса (venting), отступление. (Авианосцы/истребители,
  ракеты как снаряды, «системы» корабля, EMP — отложены на следующий боевой проход.)
- **Тактика:** бои разрешаются автоматически; **перед** боем игрок задаёт каждому кораблю
  **Стратегию** (атака/оборона/отступление) и **Роль** (танк/дамагер/поддержка/ремонт),
  независимо от типа корпуса.
- **До 5 флотов** в бою (уже поддерживается `InstanceManager`).

### Принцип визуализации
Каждый этап должен быть **виден и проверяем в веб-визуализаторе** (`cmd/gateway/static/index.html`); объём —
**функциональный, без художественной полировки**. Это дёшево: новые поля `EntitySnapshot` автоматически доходят
до браузера через `protojson` (gateway, ~L401-411) — клиент просто читает новый ключ; интерактивные действия
добавляются как новый `case` в диспетчере действий + новый пакет `C_*` по образцу `join_combat`/`build_ship`.
Ниже у каждого этапа есть пункт **Визуализатор**.

### Этап 0 — Фундамент: унификация модели корабля и наполнение каталога оснастки
**Статус: ✅ выполнено и закоммичено (`d35a17a`).**
- [x] Канонический каталог в коде — `internal/domain/fitting_catalog.go`: 8 корпусов
  (fighter/patrol/pirate/miner/cargo_helper/interceptor/destroyer/cruiser), 9 видов оружия
  (энерго/баллистика/ракеты × размеры, типы урона), 4 модуля корпуса. Мост
  `DefaultLoadoutForShipType()` (тип корабля → готовый `ShipConfiguration`).
- [x] In-memory `ShipRepository` — `internal/persistence/inmemory_ship_repo.go` (работа без БД).
- [x] Миграция `009_fitting_seed.sql` — зеркалит каталог в Postgres (идемпотентно).
- [x] `ShipRepository` проброшен в `worldnode` и `InstanceManager` (поле; используется на этапе 1).
- [x] Тесты: инварианты каталога; `BakeShip` собирает каждый сток-лоадаут через in-mem репозиторий.
- **Визуализатор:** не требуется (только данные/проводка, поведение в рантайме не меняется).

### Этап 1 — Базовый бой Starsector
**Статус: ⬜ следующий.** Цель: заменить упрощённую боевую математику базовой симуляцией на основе `BakeShip`.
- [ ] Сборка участников боя: в `UnpackFleet` создавать корабли через `BakeShip` из `ShipConfiguration`
  вместо `getShipStats`; `PackFleet` синхронизирует HP/броню/щит обратно.
- [ ] Переписать `CombatSystem` под слоты: оружейные группы (`WeaponGroup`), флакс (стрельба копит флакс,
  пассивный сброс, перегрузка/overload), броневая сетка + типы урона (KINETIC/EXPLOSIVE/ENERGY/FRAG
  по цепочке щит → броня → корпус), дуги щитов по углу попадания, регенерация щита/брони.
- [ ] Снапшот/протокол: добавить в `EntitySnapshot` флакс/броню, список стрельбы по слотам, флаги venting/overload.
- **Визуализатор:** шкалы флакса и брони на HUD игрока/цели и тонкая полоса флакса под кораблями; числа урона с
  цветом по типу урона; отрисовка луча на каждый стреляющий слот (расширить `drawBeams`); щит как дуга (по типу/
  углу) вместо кольца + состояния overload/venting. Инкрементально к текущему коду отрисовки, без нового экрана.

### Этап 1.5 — Роли, стратегии и тактический ИИ
**Статус: ⬜.**
- [ ] Компоненты `CombatRole{tank|dps|support|repair}` и `CombatStrategy{attack|defense|retreat}`; пакет
  `C_SET_FLEET_TACTICS`; назначение перед боем, штампуется на корабли в `UnpackFleet`.
- [ ] ИИ стратегий (атака/оборона/отступление) и ролей (танк держит фронт и стягивает огонь; дамагер —
  сосредоточенный огонь; поддержка — баффы/дебаффы; ремонт — лечит корпус/броню союзников).
- [ ] Выбор цели по угрозе (а не «ближайший»); построения по ролям.
- **Визуализатор (главный UI этапа):** предбоевая панель «Тактика флота» — список кораблей, у каждого выпадающие
  списки Стратегии (атака/оборона/отступление) и Роли (танк/дамагер/поддержка/ремонт); «Подтвердить» шлёт действие
  `set_fleet_tactics` → пакет `C_SET_FLEET_TACTICS` (по образцу `join_combat`). В бою: значок роли и оттенок
  стратегии над кораблём, маркер фокус-огня на приоритетной цели, отдельные лучи поддержки/ремонта (напр. зелёные).

### Этап 2 — Кастомизация флота и кораблей («ангар»)
**Статус: ⬜.**
- [ ] «Ангар» на станции: смена корпуса, установка/снятие оружия по слотам (проверка размера/типа/
  крепления + бюджет очков оснастки OP), модули корпуса, vents/capacitors — всё с проверкой на сервере.
- [ ] Сборка `CharacterFleet` из своих конфигураций; назначение флагмана; экономическая привязка.
- **Визуализатор (новый экран «Ангар/Оснастка»):** вкладка станции рядом с Рынок/Хранилище/Сейф — список кораблей,
  вид корпуса со слотами (размер/тип/крепление), установка/снятие оружия и модулей, vents/capacitors; шкала очков
  оснастки (OP) и предпросмотр итоговых характеристик (HP/броня/щит/флакс/скорость через `BakeShip`); сборка флота
  и выбор флагмана. Действия `fit_ship` / `assemble_fleet` (`C_FIT_SHIP_*` / `C_ASSEMBLE_FLEET`), проверка на сервере.

### Этап 3 — Производство, добыча, исследования/навыки
**Статус: ⬜.**
- [ ] Рецепты на данных (`crafting_recipes`) + исполнитель рецептов (обобщение `refinery`/`shipyard`).
- [ ] Глубина добычи (выход/масштаб от навыков).
- [ ] Навыки и исследования (`skill_definitions`/`player_skills`, `research_projects`/`player_research`),
  опыт за добычу/бой/крафт; разблокировка чертежей и бонусы в `BakeShip`.
- **Визуализатор:** детальная очередь производства (название рецепта + % + ETA) вместо счётчика; панель исследований
  (дерево: закрыто/открыто/в процессе + «Начать исследование»); панель навыков с уровнями и шкалами опыта + всплытия
  опыта в логе событий. Действия `start_research` / `craft_recipe`; новые поля снапшота/ответов для очереди/навыков/исследований.

### Этап 4 — Крафт сложных модулей (мост предмет ↔ оснастка)
**Статус: ⬜.**
- [ ] Предмет-«модуль» (`item_instances`) ссылается на `WeaponDefinition`/`Hullmod` (id в
  `item_definitions.meta_data`). Цепочка: крафт (этап 3) → оснастка (этап 2) → влияет на бой (этап 1).
- **Визуализатор:** расширяет существующие экраны — у модулей в сетках карго/сейфа всплывающие подсказки с
  характеристиками; крафтовые модули доступны для установки в «Ангаре» (этап 2) и в списке рецептов (этап 3). Без нового экрана.

### Этап 5 — Развитие планет и строительство баз
**Статус: ⬜.**
- [ ] Сущности/таблицы `planets`, `space_bases`, `base_modules` по образцу станций; строительство
  переиспользует исполнитель рецептов; оборона баз использует боевой слой.
- **Визуализатор:** отрисовка планет/баз на карте как новых типов сущностей (`drawPlanetShape`/`drawBaseShape` по
  образцу станций/гейтов) с цветом владельца; панель управления базой/планетой (модули, уровни, очередь строительства
  с прогрессом, хранилище, оборона) и действия «Построить/Улучшить модуль». Опционально миникарта.

### Сквозные принципы
Изменения протокола — через `make proto`; одна нумерованная миграция на этап; режим без БД должен
оставаться рабочим; балансные проходы после этапов 1 и 1.5; проверка этапа = зелёные `go build/vet/test`
+ проверка в визуализаторе + полный прогон миграций (включая перенос флота через jump gate).

**Последовательность:** 0 → 1 → 1.5 → 2 → 3 → 4 → 5 (каждый этап самостоятельно поставляемый).
