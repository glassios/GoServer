MMO Space Sandbox — Implementation Roadmap
Цель проекта
Создать масштабируемый MMO backend для космической sandbox-игры с:
•	единой галактикой
•	добычей ресурсов
•	строительством
•	экономикой
•	PvP/PvE
•	NPC фракциями
•	территориальными войнами
•	persistent world
•	поддержкой 10 000+ игроков
________________________________________
Phase 1 — Minimal Vertical Slice
Цель
Создать:
•	1 звездную систему
•	20 игроков
•	100 NPC
•	mining
•	combat
•	persistence
________________________________________
Этап 1.1 — Базовая структура проекта
Создать monorepo
/server
    /gateway
    /world
    /auth
    /shared
/client
/tools
/deploy
________________________________________
Инициализация Go workspace
go mod init galaxy-mmo
________________________________________
Установить зависимости
Networking
go get google.golang.org/protobuf
Logging
go get go.uber.org/zap
ECS
На MVP лучше написать свою минимальную ECS.
________________________________________
Этап 1.2 — Gateway Server
Цель
Создать UDP gateway.
________________________________________
Реализовать
UDP listener
net.ListenUDP()
________________________________________
Packet pipeline
Receive
 → Validate
 → Decode
 → Route
________________________________________
Packet types
Auth
Ping
Move
Shoot
Mine
Interact
________________________________________
Добавить:
•	packet size limit
•	flood protection
•	heartbeat
•	disconnect timeout
________________________________________
Этап 1.3 — Binary Protocol
Использовать Protobuf
Создать:
/shared/protocol
________________________________________
packet.proto
message MovePacket {
  uint64 entity_id = 1;
  float x = 2;
  float y = 3;
  float rotation = 4;
}
________________________________________
Generate
protoc --go_out=. packet.proto
________________________________________
Этап 1.4 — Tick Engine
Реализовать fixed tick simulation
Tickrate
20 TPS
________________________________________
Game loop
for {
    processInput()
    simulate()
    sendSnapshots()
}
________________________________________
Tick systems
Systems:
•	movement
•	combat
•	mining
•	AI
•	cleanup
________________________________________
Этап 1.5 — ECS
Реализовать minimal ECS
________________________________________
Components
type Transform struct {
    X float32
    Y float32
}

type Velocity struct {
    X float32
    Y float32
}

type Health struct {
    Value int
}
________________________________________
Systems
MovementSystem
CombatSystem
MiningSystem
AISystem
________________________________________
Этап 1.6 — Spatial Partitioning
Использовать Spatial Hash Grid
________________________________________
Grid responsibilities
•	nearby entities
•	combat queries
•	mining queries
•	visibility
________________________________________
Grid cell size
1000x1000
________________________________________
Этап 1.7 — Snapshot System
Реализовать delta snapshots
________________________________________
Клиент получает:
•	nearby ships
•	projectiles
•	asteroids
•	stations
________________________________________
НЕ отправлять:
•	всю галактику
•	hidden entities
•	distant NPC
________________________________________
Этап 1.8 — Prediction & Interpolation
Клиент
Prediction:
•	movement
•	rotation
Interpolation:
•	remote ships
________________________________________
Этап 1.9 — Combat Prototype
Реализовать:
•	laser weapons
•	shields
•	hull
•	destruction
________________________________________
Combat model
Hitscan MVP
Позже:
•	projectiles
•	missiles
•	turrets
________________________________________
Этап 1.10 — Mining Prototype
Реализовать:
•	asteroids
•	mining lasers
•	cargo
•	refining
________________________________________
Resource types
Iron
Titanium
Crystal
RareGas
________________________________________
Этап 1.11 — Persistence
PostgreSQL
________________________________________
Tables
accounts
id
login
password_hash
characters
id
account_id
ship_type
x
y
credits
inventory
stations
factions
________________________________________
Этап 1.12 — Redis
Использовать для:
•	sessions
•	cache
•	online players
•	temporary state
________________________________________
Этап 1.13 — NPC AI
Реализовать:
•	miners
•	pirates
•	patrols
________________________________________
AI states
Idle
Patrol
Attack
Mine
Retreat
Dock
________________________________________
Этап 1.14 — Factions
NPC factions
Каждая фракция имеет:
•	territory
•	reputation
•	economy
•	fleets
________________________________________
Этап 1.15 — Basic Economy
Реализовать:
•	resource prices
•	buy/sell
•	station market
________________________________________
Dynamic pricing
Цена зависит от:
•	добычи
•	потребления
•	войны
•	логистики
________________________________________
Phase 2 — Multi-System Galaxy
Цель
Создать:
•	10 звездных систем
•	jump gates
•	межсистемные перелеты
________________________________________
Этап 2.1 — Galaxy Router
Реализовать сервис:
•	переходов между системами
•	transfer игрока между world nodes
________________________________________
Этап 2.2 — System Nodes
Каждый процесс:
•	отдельная звездная система
________________________________________
Responsibilities
Simulation
Combat
AI
Economy
Mining
Visibility
________________________________________
Этап 2.3 — Cross-node Transfer
Реализовать:
•	player migration
•	session transfer
•	state serialization
________________________________________
Этап 2.4 — NPC Trade Routes
NPC перевозят:
•	ресурсы
•	компоненты
•	оружие
________________________________________
Войны влияют:
•	на supply
•	цены
•	доступность
________________________________________
Phase 3 — Corporations & Territory
Цель
Добавить:
•	player corporations
•	ownership
•	wars
•	stations
________________________________________
Этап 3.1 — Corporations
Реализовать:
•	membership
•	roles
•	permissions
•	taxes
________________________________________
Этап 3.2 — Stations
Игроки строят:
•	mining stations
•	refineries
•	factories
•	shipyards
________________________________________
Structures:
•	persistent
•	destructible
________________________________________
Этап 3.3 — Territory Control
Territory mechanics
Контроль через:
•	influence
•	station ownership
•	military presence
________________________________________
Phase 4 — Advanced Simulation
Цель
Добавить:
•	полноценную экономику
•	производство
•	исследования
•	logistics warfare
________________________________________
Этап 4.1 — Production Chains
Ore
 → Metal
 → Components
 → Weapons
 → Ships
________________________________________
Этап 4.2 — Logistics
Реализовать:
•	cargo fleets
•	supply chains
•	convoy attacks
________________________________________
Этап 4.3 — Research
Tech trees
Weapons
Industry
Mining
AI
Energy
________________________________________
Research ownership
Personal
Corporation
Faction
________________________________________
Этап 4.4 — Strategic AI
NPC factions:
•	захватывают территории
•	ведут войны
•	строят станции
•	добывают ресурсы
________________________________________
Phase 5 — Massive Battles
Цель
Поддержка:
•	1000+ ships
•	массовых PvP сражений
________________________________________
Этап 5.1 — Combat Optimization
Добавить:
•	LOD simulation
•	projectile batching
•	tick degradation
________________________________________
Этап 5.2 — Time Dilation
При overload
20 TPS → 5 TPS
________________________________________
Этап 5.3 — Reinforced Nodes
При больших боях:
•	выделять отдельный node
•	переносить simulation
________________________________________
Этап 5.4 — Metrics & Profiling
Добавить:
•	Prometheus
•	Grafana
•	pprof
________________________________________
Этап 5.5 — Load Testing
Создать:
•	bot clients
•	combat simulators
________________________________________
Цели
Проверить:
•	1000 игроков
•	10000 NPC
•	packet throughput
•	CPU load
•	memory pressure
________________________________________
Recommended Tech Stack
Backend
Go
UDP
Protobuf
PostgreSQL
Redis
NATS
Docker
________________________________________
Infrastructure
Prometheus
Grafana
Loki
OpenTelemetry
________________________________________
Networking
MVP
UDP + reliability layer
________________________________________
Позже
QUIC
________________________________________
ECS Architecture
Components
Transform
Velocity
Health
Shield
Weapon
Cargo
MiningLaser
Faction
Research
Ownership
________________________________________
Core Systems
MovementSystem
CombatSystem
AISystem
MiningSystem
EconomySystem
VisibilitySystem
________________________________________
Production Priorities
Делать сначала
✅ Networking
✅ Tick simulation
✅ ECS
✅ Persistence
✅ AI
✅ Economy
________________________________________
НЕ делать сначала
❌ Seamless galaxy
❌ Full physics
❌ Procedural everything
❌ Microservices everywhere
❌ Realistic orbital mechanics
________________________________________
Critical Rules
Никогда:
•	не доверять клиенту
•	не использовать JSON networking
•	не отправлять full world state
•	не хранить всю галактику в одном процессе
________________________________________
MVP Definition
MVP готов когда:
•	20 игроков могут играть одновременно
•	есть mining
•	есть combat
•	есть NPC
•	есть persistence
•	есть economy
•	сервер работает стабильно 24h+
________________________________________
Long-term Goal
Создать:
•	живую persistent galaxy
•	sandbox economy
•	emergent warfare
•	player-driven politics
•	масштабируемую MMO инфраструктуру
