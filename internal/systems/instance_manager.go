package systems

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/spatial"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

type CombatInstance struct {
	InstanceID       uint32
	World            *ecs.World
	Grid             *spatial.HashGrid
	Systems          []ecs.System
	CombatSys        *CombatSystem // typed ref into Systems, for draining this tick's fire events
	OriginalSystemID uint32
	Participants     []domain.EntityID
	Teams            map[domain.EntityID]uint32 // FleetID -> TeamID
	NextTeamID       uint32
	ShipEntities     map[domain.EntityID][]domain.EntityID // FleetID -> list of ship entity IDs in instance
	Subscriptions    []messaging.Subscription
	TickCount        uint64
	LastSnapshotTime float64
	AccumulatedTime  float64
	MaxDuration      float64 // Предел длительности боя (сек) — защита от зависших инстансов
}

type InstanceManager struct {
	mu         sync.Mutex
	bus        messaging.MessageBus
	mainGrid   *spatial.HashGrid
	logger     *zap.Logger
	instances  map[uint32]*CombatInstance
	nextInstID uint32
	randSource *rand.Rand
	systemID   uint32
	playerRepo domain.PlayerRepository
	shipRepo   domain.ShipRepository // fitting catalog source (Phase 0: wired; Phase 1: used by combat baking)
}

func NewInstanceManager(bus messaging.MessageBus, mainGrid *spatial.HashGrid, systemID uint32, playerRepo domain.PlayerRepository, shipRepo domain.ShipRepository, logger *zap.Logger) *InstanceManager {
	return &InstanceManager{
		bus:        bus,
		mainGrid:   mainGrid,
		logger:     logger,
		instances:  make(map[uint32]*CombatInstance),
		nextInstID: 10000 + systemID*1000,
		randSource: rand.New(rand.NewSource(time.Now().UnixNano())),
		systemID:   systemID,
		playerRepo: playerRepo,
		shipRepo:   shipRepo,
	}
}

func (m *InstanceManager) Name() string {
	return "InstanceManager"
}

func (m *InstanceManager) Priority() int {
	return 95 // Выполняется после основных игровых систем, но до очистки
}

func (m *InstanceManager) GetInstance(id uint32) *CombatInstance {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instances[id]
}

func (m *InstanceManager) CreateCombatInstance(mainWorld *ecs.World, attackerID, defenderID domain.EntityID) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Prevent fleets already in combat from initiating/entering a new combat instance
	if _, ok := mainWorld.GetComponent(attackerID, domain.CombatState{}); ok {
		m.logger.Warn("Attacker is already in combat, cannot create new instance", zap.Uint64("attackerID", uint64(attackerID)))
		return 0
	}
	if _, ok := mainWorld.GetComponent(defenderID, domain.CombatState{}); ok {
		m.logger.Warn("Defender is already in combat, cannot create new instance", zap.Uint64("defenderID", uint64(defenderID)))
		return 0
	}

	instID := m.nextInstID
	m.nextInstID++

	m.logger.Info("Creating dynamic combat instance",
		zap.Uint32("instanceID", instID),
		zap.Uint64("attackerID", uint64(attackerID)),
		zap.Uint64("defenderID", uint64(defenderID)),
	)

	// Гарантируем наличие флота у участников
	m.ensureFleet(mainWorld, attackerID)
	m.ensureFleet(mainWorld, defenderID)

	// Создаем ECS мир для боя
	instWorld := ecs.NewWorld()
	instGrid := spatial.NewHashGrid(100)

	// Инициализируем локальные системы боевого мира с увеличенной в 5 раз ареной
	moveSys := NewMovementSystem(6000, 6000) // Ограниченный размер арены
	moveSys.SetGrid(instGrid)                // keep the arena's spatial index live
	visSys := NewVisibilitySystem(instGrid)
	aiSys := NewAISystem(3000.0, 0, 6000, 6000) // maxNPCs=0, только обновление поведения существующих
	aiSys.SetGrid(instGrid)
	combatSys := NewCombatSystem(nil)
	missileSys := NewMissileSystem(combatSys, int64(instID)) // seeded per-instance for deterministic PD
	lootSys := NewLootSystem(instGrid)
	cleanupSys := NewCleanupSystem(instGrid)

	instSystems := []ecs.System{
		moveSys,
		visSys,
		aiSys,
		combatSys,
		missileSys,
		lootSys,
		cleanupSys,
	}

	inst := &CombatInstance{
		InstanceID:       instID,
		World:            instWorld,
		Grid:             instGrid,
		Systems:          instSystems,
		CombatSys:        combatSys,
		OriginalSystemID: m.systemID,
		Participants:     []domain.EntityID{attackerID, defenderID},
		Teams:            make(map[domain.EntityID]uint32),
		NextTeamID:       3,
		ShipEntities:     make(map[domain.EntityID][]domain.EntityID),
		Subscriptions:    nil,
		MaxDuration:      120.0,
	}

	// Распределяем участников по стартовым сторонам
	inst.Teams[attackerID] = 1 // Team 1: Атакующие
	inst.Teams[defenderID] = 2 // Team 2: Обороняющиеся

	// Распаковываем флоты на расстоянии в 5 раз дальше (дистанция 3000 вместо 600)
	attackerShips := UnpackFleet(mainWorld, instWorld, attackerID, 1, -1500, 0, 0)
	defenderShips := UnpackFleet(mainWorld, instWorld, defenderID, 2, 1500, 0, math.Pi)

	inst.ShipEntities[attackerID] = attackerShips
	inst.ShipEntities[defenderID] = defenderShips

	// Добавляем корабли в пространственную сетку инстанса
	for _, sID := range attackerShips {
		if tVal, ok := instWorld.GetComponent(sID, domain.Transform{}); ok {
			t := tVal.(*domain.Transform)
			instGrid.Insert(sID, t.X, t.Y)
		}
	}
	for _, sID := range defenderShips {
		if tVal, ok := instWorld.GetComponent(sID, domain.Transform{}); ok {
			t := tVal.(*domain.Transform)
			instGrid.Insert(sID, t.X, t.Y)
		}
	}

	// Помечаем флоты как находящиеся в бою в основном мире
	m.setFleetCombatState(mainWorld, attackerID, true, instID, defenderID)
	m.setFleetCombatState(mainWorld, defenderID, true, instID, attackerID)

	// Бой полностью автоматический (AI vs AI), поэтому подписка на команды ввода игрока
	// не создаётся — это исключает утечку горутины колбэка и мёртвый канал команд.

	m.instances[instID] = inst

	// Отправляем Gateway команду смены маршрута для игроков
	m.sendRoutingUpdates(mainWorld, inst)

	return instID
}

func (m *InstanceManager) JoinCombatInstance(mainWorld *ecs.World, instanceID uint32, joiningFleetID domain.EntityID, alignWithFleetID domain.EntityID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("combat instance %d not found", instanceID)
	}

	// Prevent fleets already in combat from joining another combat
	if _, inCombat := mainWorld.GetComponent(joiningFleetID, domain.CombatState{}); inCombat {
		return fmt.Errorf("fleet %d is already in combat", joiningFleetID)
	}

	if len(inst.Participants) >= 5 {
		return fmt.Errorf("combat instance %d is full (max 5 participants)", instanceID)
	}

	m.logger.Info("Fleet joining combat instance",
		zap.Uint32("instanceID", instanceID),
		zap.Uint64("joiningFleetID", uint64(joiningFleetID)),
		zap.Uint64("alignWithFleetID", uint64(alignWithFleetID)),
	)

	// Гарантируем наличие флота у присоединяющегося
	m.ensureFleet(mainWorld, joiningFleetID)

	// Вычисляем TeamID и координаты
	var teamID uint32
	var baseX, baseY float32
	var angle float64

	if alignWithFleetID == 0 {
		// Каждый сам за себя (FFA)
		teamID = inst.NextTeamID
		inst.NextTeamID++

		// Размещаем на альтернативных позициях арены, увеличенных в 5 раз
		switch teamID {
		case 3:
			baseX, baseY, angle = 0, -1500, math.Pi/2
		case 4:
			baseX, baseY, angle = 0, 1500, -math.Pi/2
		default:
			baseX, baseY, angle = -1000, -1000, math.Pi/4
		}
	} else {
		// Присоединяется на сторону существующего участника
		existingTeam, ok := inst.Teams[alignWithFleetID]
		if !ok {
			return fmt.Errorf("align target fleet %d not in combat", alignWithFleetID)
		}
		teamID = existingTeam

		// Находим координаты союзного флагмана для спавна рядом
		allyX, allyY := float32(0), float32(0)
		if tVal, ok := inst.World.GetComponent(alignWithFleetID, domain.Transform{}); ok {
			t := tVal.(*domain.Transform)
			allyX = t.X
			allyY = t.Y
		}
		baseX = allyX + float32(m.randSource.Float64()*100-50)
		baseY = allyY + float32(m.randSource.Float64()*100-50)
		angle = m.randSource.Float64() * math.Pi * 2
	}

	// Распаковываем флот
	ships := UnpackFleet(mainWorld, inst.World, joiningFleetID, teamID, baseX, baseY, angle)
	inst.ShipEntities[joiningFleetID] = ships

	for _, sID := range ships {
		if tVal, ok := inst.World.GetComponent(sID, domain.Transform{}); ok {
			t := tVal.(*domain.Transform)
			inst.Grid.Insert(sID, t.X, t.Y)
		}
	}

	inst.Participants = append(inst.Participants, joiningFleetID)
	inst.Teams[joiningFleetID] = teamID

	// Помечаем в основном мире
	m.setFleetCombatState(mainWorld, joiningFleetID, true, instanceID, alignWithFleetID)

	// Отправляем Gateway смену маршрута для вошедшего игрока
	if _, isPlayer := mainWorld.GetComponent(joiningFleetID, domain.PlayerData{}); isPlayer {
		updateMsg := fmt.Sprintf("%d,%d", joiningFleetID, instanceID)
		_ = m.bus.Publish("system.routing.update", []byte(updateMsg))
	}

	return nil
}

func (m *InstanceManager) Update(mainWorld *ecs.World, dt float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var finishedInstances []uint32

	for id, inst := range m.instances {
		inst.AccumulatedTime += dt

		// 1. Тикаем локальные ECS системы боевого инстанса
		for _, sys := range inst.Systems {
			sys.Update(inst.World, dt)
		}

		inst.TickCount++

		// 2. Отправляем периодические снимки состояния боя (snapshot) клиентам
		if inst.AccumulatedTime-inst.LastSnapshotTime >= 0.05 { // 20 TPS snapshots
			inst.LastSnapshotTime = inst.AccumulatedTime
			m.broadcastInstanceSnapshot(inst)
		}

		// 3. Проверяем условие завершения боя или превышение лимита времени
		if m.checkCombatEnded(inst) {
			finishedInstances = append(finishedInstances, id)
		} else if inst.MaxDuration > 0 && inst.AccumulatedTime >= inst.MaxDuration {
			m.logger.Warn("Combat instance exceeded max duration, force-resolving",
				zap.Uint32("instanceID", inst.InstanceID),
				zap.Float64("duration", inst.AccumulatedTime),
			)
			finishedInstances = append(finishedInstances, id)
		}
	}

	// 5. Завершаем отработанные инстансы
	for _, id := range finishedInstances {
		inst := m.instances[id]
		m.resolveCombat(mainWorld, inst)
		delete(m.instances, id)
	}
}

func (m *InstanceManager) broadcastInstanceSnapshot(inst *CombatInstance) {
	allEntities := inst.World.Query(0)
	entSnaps := make([]*protocol.EntitySnapshot, 0, len(allEntities))
	var missiles []*protocol.MissileState
	for _, id := range allEntities {
		// Missiles ride their own thin channel (below), not the ship-rendering entity list.
		if et, ok := inst.World.GetEntityType(id); ok && et == domain.EntityMissile {
			if tVal, ok := inst.World.GetComponent(id, domain.Transform{}); ok {
				t := tVal.(*domain.Transform)
				var dtype string
				if mVal, ok := inst.World.GetComponent(id, domain.Missile{}); ok {
					dtype = mVal.(*domain.Missile).DamageType
				}
				missiles = append(missiles, &protocol.MissileState{
					Id: uint64(id), X: t.X, Y: t.Y, Rotation: t.Rotation, DamageType: dtype,
				})
			}
			continue
		}
		snap := BuildCombatEntitySnapshot(inst.World, id)
		if snap != nil {
			entSnaps = append(entSnaps, snap)
		}
	}

	worldSnap := &protocol.WorldSnapshot{
		Tick:     inst.TickCount,
		Entities: entSnaps,
		Missiles: missiles,
	}

	// Attach this tick's traveling-shot fire events (thin channel B3) so the client can draw
	// bolts flying. Damage was already applied on the server; these are cosmetic hints only.
	if inst.CombatSys != nil && len(inst.CombatSys.Fires) > 0 {
		fires := make([]*protocol.FireEvent, 0, len(inst.CombatSys.Fires))
		for _, f := range inst.CombatSys.Fires {
			fires = append(fires, &protocol.FireEvent{
				AttackerId:  uint64(f.AttackerID),
				TargetId:    uint64(f.TargetID),
				OriginX:     f.OriginX,
				OriginY:     f.OriginY,
				TargetX:     f.TargetX,
				TargetY:     f.TargetY,
				Speed:       f.Speed,
				DamageType:  f.DamageType,
				WeaponClass: f.WeaponClass,
			})
		}
		worldSnap.FireEvents = fires
	}

	data, err := proto.Marshal(worldSnap)
	if err != nil {
		m.randSource.Float64() // Nop
		return
	}

	outputTopic := fmt.Sprintf("system.%d.output", inst.InstanceID)
	_ = m.bus.Publish(outputTopic, data)
}

func (m *InstanceManager) checkCombatEnded(inst *CombatInstance) bool {
	// Подсчитываем количество выживших команд
	activeTeams := make(map[uint32]struct{})

	entities := inst.World.Query(ecs.BuildMask(domain.Health{}, domain.CombatTeam{}))
	for _, id := range entities {
		hVal, _ := inst.World.GetComponent(id, domain.Health{})
		ctVal, _ := inst.World.GetComponent(id, domain.CombatTeam{})

		if hVal.(*domain.Health).Current > 0 {
			activeTeams[ctVal.(*domain.CombatTeam).TeamID] = struct{}{}
		}
	}

	// Бой окончен, если осталась максимум 1 активная сторона (или вообще никто не выжил)
	return len(activeTeams) <= 1
}

// lootDropChance is the per-item probability that a destroyed fleet's cargo item or fitted module
// drops into the post-battle loot container.
const lootDropChance = 0.10

// rollLootDrop returns true ~lootDropChance of the time.
func (m *InstanceManager) rollLootDrop() bool {
	return m.randSource.Float64() < lootDropChance
}

// tallyEnemyKills computes, per participating fleet, how many enemy (other-team) ships it helped
// destroy, plus the set of teams that still have survivors. Pure so it can be unit-tested.
func tallyEnemyKills(participants []domain.EntityID, teams map[domain.EntityID]uint32, initialCounts, aliveCounts map[domain.EntityID]int32) (killed map[domain.EntityID]int32, teamsWithSurvivors map[uint32]bool) {
	killed = make(map[domain.EntityID]int32, len(participants))
	teamsWithSurvivors = make(map[uint32]bool)
	for _, f := range participants {
		if aliveCounts[f] > 0 {
			teamsWithSurvivors[teams[f]] = true
		}
	}
	for _, f := range participants {
		myTeam := teams[f]
		var k int32
		for _, other := range participants {
			if teams[other] == myTeam {
				continue
			}
			k += initialCounts[other] - aliveCounts[other]
		}
		killed[f] = k
	}
	return killed, teamsWithSurvivors
}

func (m *InstanceManager) resolveCombat(mainWorld *ecs.World, inst *CombatInstance) {
	m.logger.Info("Resolving combat instance", zap.Uint32("instanceID", inst.InstanceID))

	// Combat XP (Phase 3): tally enemy ships each fleet destroyed BEFORE PackFleet removes survivors.
	initialCounts := make(map[domain.EntityID]int32, len(inst.Participants))
	aliveCounts := make(map[domain.EntityID]int32, len(inst.Participants))
	for _, fleetID := range inst.Participants {
		initialCounts[fleetID] = int32(len(inst.ShipEntities[fleetID]))
		var alive int32
		for _, shipID := range inst.ShipEntities[fleetID] {
			if _, ok := inst.World.GetEntityType(shipID); ok {
				alive++
			}
		}
		aliveCounts[fleetID] = alive
	}
	enemyKilled, teamsWithSurvivors := tallyEnemyKills(inst.Participants, inst.Teams, initialCounts, aliveCounts)

	// Удаляем маркер боя из основного мира и получаем его координаты
	var markerX, markerY float32
	var foundMarker bool

	markers := mainWorld.Query(ecs.BuildMask(domain.CombatMarker{}))
	for _, markerID := range markers {
		if mVal, ok := mainWorld.GetComponent(markerID, domain.CombatMarker{}); ok {
			if mVal.(*domain.CombatMarker).CombatInstanceID == inst.InstanceID {
				if tVal, ok := mainWorld.GetComponent(markerID, domain.Transform{}); ok {
					t := tVal.(*domain.Transform)
					markerX = t.X
					markerY = t.Y
					foundMarker = true
				}
				mainWorld.DestroyEntity(markerID)
				m.mainGrid.Remove(markerID)
			}
		}
	}

	// 1. Отписываемся от NATS событий инстанса
	for _, sub := range inst.Subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			m.logger.Error("Failed to unsubscribe combat instance subscription",
				zap.Uint32("instanceID", inst.InstanceID), zap.Error(err))
		}
	}

	// 2. Собираем результаты выживших кораблей обратно во флоты в основном мире
	for _, fleetID := range inst.Participants {
		// Проверяем, является ли сущность игроком, до возможного ее уничтожения
		_, isPlayer := mainWorld.GetComponent(fleetID, domain.PlayerData{})

		// Снимаем установленные крафтовые модули ДО PackFleet (он обнуляет состав флота),
		// чтобы при полном уничтожении флота они могли выпасть в лут (см. ниже).
		var fittedModules []string
		if flVal, ok := mainWorld.GetComponent(fleetID, domain.Fleet{}); ok {
			fleet := flVal.(*domain.Fleet)
			for i := range fleet.Ships {
				cfg := fleet.Ships[i].EffectiveConfig()
				for _, wid := range cfg.FittedWeapons {
					if item := domain.ModuleItemForWeapon(wid); item != "" {
						fittedModules = append(fittedModules, item)
					}
				}
			}
		}

		shipEntities := inst.ShipEntities[fleetID]
		alive := PackFleet(mainWorld, inst.World, fleetID, shipEntities)

		if alive {
			// Переносим флот в основном мире ровно на место маркера боя
			if foundMarker {
				if tVal, ok := mainWorld.GetComponent(fleetID, domain.Transform{}); ok {
					t := tVal.(*domain.Transform)
					t.X = markerX
					t.Y = markerY
				}
			}
			// Возвращаем в строй в основном мире
			m.setFleetCombatState(mainWorld, fleetID, false, 0, 0)
			m.logger.Info("Fleet survived combat and returned to main world", zap.Uint64("fleetID", uint64(fleetID)))

			// Combat XP (Phase 3): reward surviving players for enemy ships destroyed + a win bonus.
			if isPlayer {
				if pgVal, ok := mainWorld.GetComponent(fleetID, domain.PlayerProgress{}); ok {
					pg := pgVal.(*domain.PlayerProgress)
					xp := enemyKilled[fleetID] * 25
					if len(teamsWithSurvivors) == 1 && teamsWithSurvivors[inst.Teams[fleetID]] {
						xp += 50 // last team standing
					}
					if xp > 0 {
						pg.AddXP(domain.SkillCombat, xp)
						PublishPlayerProgress(m.bus, mainWorld, fleetID)
					}
				}
			}
		} else {
			// Флот полностью уничтожен! Удаляем сущность флота из основного мира
			m.logger.Info("Fleet completely destroyed in combat", zap.Uint64("fleetID", uint64(fleetID)))

			// Сгенерируем контейнер с добычей в основном мире в месте, где завершился бой (положение маркера боя)
			var posX, posY float32
			if foundMarker {
				posX = markerX
				posY = markerY
			} else if tVal, ok := mainWorld.GetComponent(fleetID, domain.Transform{}); ok {
				t := tVal.(*domain.Transform)
				posX = t.X
				posY = t.Y
			}

			// Вытягиваем кредиты и формируем добычу. Содержимое карго и установленные модули
			// уничтоженных кораблей попадают в лут лишь с шансом lootDropChance (10%) каждый.
			var credits int64
			var items []domain.ItemInstance
			if pVal, ok := mainWorld.GetComponent(fleetID, domain.PlayerData{}); ok {
				credits = pVal.(*domain.PlayerData).Credits
			}
			if cargoVal, ok := mainWorld.GetComponent(fleetID, domain.Cargo{}); ok {
				for _, it := range cargoVal.(*domain.Cargo).Items {
					if it.Quantity > 0 && m.rollLootDrop() {
						items = append(items, it)
					}
				}
			}
			// Установленные крафтовые модули уничтоженных кораблей: 10% за каждый.
			for _, mod := range fittedModules {
				if !m.rollLootDrop() {
					continue
				}
				if defID, ok := domain.ResourceToID[domain.ResourceType(mod)]; ok {
					items = append(items, domain.ItemInstance{DefinitionID: defID, Quantity: 1, State: "normal"})
				}
			}

			if len(items) > 0 || credits > 0 {
				lootEntity := mainWorld.CreateEntity(domain.EntityLootContainer)
				mainWorld.AddComponent(lootEntity, &domain.Transform{X: posX, Y: posY})
				mainWorld.AddComponent(lootEntity, &domain.Loot{Credits: credits})
				// Cargo is always attached (even for credit-only drops) — LootSystem's
				// query requires it, otherwise the container is never auto-picked-up.
				mainWorld.AddComponent(lootEntity, &domain.Cargo{Items: items, Capacity: 99999})
				m.mainGrid.Insert(lootEntity, posX, posY)
			}

			mainWorld.DestroyEntity(fleetID)
			m.mainGrid.Remove(fleetID)

			if isPlayer && m.playerRepo != nil {
				// Очищаем флот игрока в БД, чтобы при респавне выдать новый
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := m.playerRepo.ClearFleet(ctx, uint64(fleetID))
				cancel()
				if err != nil {
					m.logger.Error("Failed to clear player fleet on destruction", zap.Uint64("playerID", uint64(fleetID)), zap.Error(err))
				} else {
					m.logger.Info("Successfully cleared player fleet in repository", zap.Uint64("playerID", uint64(fleetID)))
				}
			}
		}

		// Если участник был игроком, обновляем Gateway маршрутизацию обратно на оригинальный сектор
		if isPlayer {
			updateMsg := fmt.Sprintf("%d,%d", fleetID, inst.OriginalSystemID)
			_ = m.bus.Publish("system.routing.update", []byte(updateMsg))
		}
	}
}

func (m *InstanceManager) setFleetCombatState(world *ecs.World, fleetID domain.EntityID, inCombat bool, instID uint32, opponentID domain.EntityID) {
	if inCombat {
		world.AddComponent(fleetID, &domain.CombatState{
			InCombat:         true,
			CombatInstanceID: instID,
			OpponentID:       opponentID,
		})
		// Прячем флот с радаров и убираем скорость
		if vVal, ok := world.GetComponent(fleetID, domain.Velocity{}); ok {
			v := vVal.(*domain.Velocity)
			v.X = 0
			v.Y = 0
		}
		m.mainGrid.Remove(fleetID)
	} else {
		world.RemoveComponent(fleetID, domain.CombatState{})
		if tVal, ok := world.GetComponent(fleetID, domain.Transform{}); ok {
			t := tVal.(*domain.Transform)
			m.mainGrid.Insert(fleetID, t.X, t.Y)
		}
	}
}

func (m *InstanceManager) sendRoutingUpdates(mainWorld *ecs.World, inst *CombatInstance) {
	for _, participantID := range inst.Participants {
		// Отсылаем маршрутизацию только для реальных игроков
		if _, isPlayer := mainWorld.GetComponent(participantID, domain.PlayerData{}); isPlayer {
			updateMsg := fmt.Sprintf("%d,%d", participantID, inst.InstanceID)
			_ = m.bus.Publish("system.routing.update", []byte(updateMsg))
		}
	}
}

func (m *InstanceManager) ensureFleet(world *ecs.World, fleetID domain.EntityID) {
	fleetVal, found := world.GetComponent(fleetID, domain.Fleet{})

	// Определяем тип кораблей для спавна в зависимости от фракции/типа
	shipType := "fighter"
	if pVal, ok := world.GetComponent(fleetID, domain.PlayerData{}); ok {
		if shipCfgVal, ok2 := world.GetComponent(fleetID, domain.ShipConfig{}); ok2 {
			shipType = shipCfgVal.(*domain.ShipConfig).ShipType
		}
		m.logger.Debug("Ensuring player fleet", zap.Uint64("id", uint64(fleetID)), zap.String("name", pVal.(*domain.PlayerData).Name))
	}

	if !found {
		// Генерируем новый флот от 1 до 3 кораблей
		numShips := m.randSource.Intn(3) + 1 // 1, 2 или 3 корабля
		var ships []domain.FleetShip
		for i := 1; i <= numShips; i++ {
			ships = append(ships, domain.FleetShip{
				ShipID:        uint32(i),
				ShipType:      shipType,
				Health:        100,
				MaxHealth:     100,
				Shield:        50,
				MaxShield:     50,
				CargoCapacity: 100,
			})
		}
		world.AddComponent(fleetID, &domain.Fleet{Ships: ships})
	} else {
		fleet := fleetVal.(*domain.Fleet)
		if len(fleet.Ships) == 0 {
			// Заполняем пустой флот от 1 до 3 кораблей
			numShips := m.randSource.Intn(3) + 1
			var ships []domain.FleetShip
			for i := 1; i <= numShips; i++ {
				ships = append(ships, domain.FleetShip{
					ShipID:        uint32(i),
					ShipType:      shipType,
					Health:        100,
					MaxHealth:     100,
					Shield:        50,
					MaxShield:     50,
					CargoCapacity: 100,
				})
			}
			fleet.Ships = ships
		}
	}
}

func BuildCombatEntitySnapshot(world *ecs.World, id domain.EntityID) *protocol.EntitySnapshot {
	tVal, foundT := world.GetComponent(id, domain.Transform{})
	if !foundT {
		return nil
	}
	trans := tVal.(*domain.Transform)

	eType, exists := world.GetEntityType(id)
	if !exists {
		eType = domain.EntityNPC
	}

	snap := &protocol.EntitySnapshot{
		EntityId:   uint64(id),
		EntityType: uint32(eType),
		X:          trans.X,
		Y:          trans.Y,
		Rotation:   trans.Rotation,
	}

	if vVal, ok := world.GetComponent(id, domain.Velocity{}); ok {
		v := vVal.(*domain.Velocity)
		snap.Vx = v.X
		snap.Vy = v.Y
	}
	if hVal, ok := world.GetComponent(id, domain.Health{}); ok {
		h := hVal.(*domain.Health)
		snap.Hp = h.Current
		snap.MaxHp = h.Max
	}
	if sVal, ok := world.GetComponent(id, domain.Shield{}); ok {
		s := sVal.(*domain.Shield)
		snap.Shield = s.Current
		snap.MaxShield = s.Max
		// Directional-shield rendering: full bubble for omni/unset, otherwise the hull's arc,
		// plus the heading the shield is actually held toward (matches the damage resolver).
		if s.Type == "omni" || s.Type == "" || s.Arc <= 0 || s.Arc >= 360 {
			snap.ShieldArc = 360
		} else {
			snap.ShieldArc = s.Arc
		}
		snap.ShieldFacing = shieldFacing(world, id, trans)
	}
	if aVal, ok := world.GetComponent(id, domain.ArmorGrid{}); ok {
		a := aVal.(*domain.ArmorGrid)
		snap.Armor = int32(a.Current)
		snap.MaxArmor = int32(a.Max)
	}
	if fVal, ok := world.GetComponent(id, domain.FluxState{}); ok {
		f := fVal.(*domain.FluxState)
		snap.Flux = int32(f.Current)
		snap.MaxFlux = int32(f.Capacity)
		snap.Overloaded = f.Overloaded
		snap.Venting = f.Venting
		if f.OverloadTimer > 0 {
			snap.OverloadTimer = f.OverloadTimer
		}
	}
	if fxVal, ok := world.GetComponent(id, domain.CombatFx{}); ok {
		fx := fxVal.(*domain.CombatFx)
		snap.ShotsFired = fx.ShotsFired
		snap.LastDamageType = fx.LastDamageType
	}
	if ssVal, ok := world.GetComponent(id, domain.SubsystemState{}); ok {
		ss := ssVal.(*domain.SubsystemState)
		snap.EngineHit = ss.EngineHitTimer > 0
		snap.WeaponHit = ss.WeaponHitTimer > 0
	}
	if rVal, ok := world.GetComponent(id, domain.CombatRole{}); ok {
		r := rVal.(*domain.CombatRole)
		snap.Role = r.Role
		if r.AssistTargetID != 0 {
			snap.AssistTargetId = uint64(r.AssistTargetID)
			if r.Role == domain.RoleRepair {
				snap.AssistType = "repair"
			} else if r.Role == domain.RoleSupport {
				snap.AssistType = "support"
			}
		}
	}
	if stVal, ok := world.GetComponent(id, domain.CombatStrategy{}); ok {
		snap.Strategy = stVal.(*domain.CombatStrategy).Stance
	}
	if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
		cfg := cfgVal.(*domain.ShipConfig)
		snap.ShipType = cfg.ShipType
	}
	if wVal, ok := world.GetComponent(id, domain.Weapon{}); ok {
		w := wVal.(*domain.Weapon)
		snap.TargetId = uint64(w.TargetID)
		snap.IsShooting = w.IsFiring
	}
	if pVal, ok := world.GetComponent(id, domain.PlayerData{}); ok {
		p := pVal.(*domain.PlayerData)
		snap.Name = p.Name
		snap.Credits = p.Credits
	}
	if corpVal, ok := world.GetComponent(id, domain.CorporationMember{}); ok {
		snap.CorpId = corpVal.(*domain.CorporationMember).CorpID
	}
	if teamVal, ok := world.GetComponent(id, domain.CombatTeam{}); ok {
		snap.FactionId = teamVal.(*domain.CombatTeam).TeamID
	}

	return snap
}
