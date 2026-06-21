package systems

import (
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

func TestCombatInstance_UnpackAndPack(t *testing.T) {
	sourceWorld := ecs.NewWorld()
	targetWorld := ecs.NewWorld()

	// 1. Создаем сущность флота игрока с 3 кораблями в основном мире
	playerID := domain.EntityID(101)
	sourceWorld.RegisterEntityWithID(playerID, domain.EntityPlayer)
	ships := []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50},
		{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30},
		{ShipID: 3, ShipType: "cargo_helper", Health: 90, MaxHealth: 90, Shield: 40, MaxShield: 40},
	}
	sourceWorld.AddComponent(playerID, &domain.Fleet{Ships: ships})
	sourceWorld.AddComponent(playerID, &domain.PlayerData{Name: "Fleet Commander", Credits: 1000})

	// 2. Распаковываем флот в боевой мир
	teamID := uint32(1)
	shipEntities := UnpackFleet(sourceWorld, targetWorld, playerID, teamID, 100.0, 200.0, 0.0)

	if len(shipEntities) != 3 {
		t.Fatalf("Expected 3 unpacked ship entities, got %d", len(shipEntities))
	}

	// 3. Проверяем компоненты флагмана (ID флагмана должен равняться playerID)
	flagshipID := shipEntities[0]
	if flagshipID != playerID {
		t.Errorf("Expected flagship ID to be playerID %d, got %d", playerID, flagshipID)
	}

	eType, exists := targetWorld.GetEntityType(flagshipID)
	if !exists || eType != domain.EntityPlayer {
		t.Errorf("Expected flagship entity type to be Player, got %s", eType)
	}

	hVal, ok := targetWorld.GetComponent(flagshipID, domain.Health{})
	if !ok || hVal.(*domain.Health).Current != 100 {
		t.Errorf("Expected flagship health 100, got %+v", hVal)
	}

	// 4. Проверяем компоненты эскорта (другие ID, тип NPC, наличие ИИ)
	escortID1 := shipEntities[1]
	eType1, exists1 := targetWorld.GetEntityType(escortID1)
	if !exists1 || eType1 != domain.EntityNPC {
		t.Errorf("Expected escort to be NPC, got %s", eType1)
	}

	aiVal, ok := targetWorld.GetComponent(escortID1, domain.AIState{})
	if !ok || aiVal.(*domain.AIState).Behavior != domain.BehaviorAttack {
		t.Errorf("Expected escort with BehaviorAttack, got %+v", aiVal)
	}

	// 5. Симулируем повреждения и уничтожение в бою
	// Флагман получает урон (HP 100 -> 60)
	hValFlag, _ := targetWorld.GetComponent(flagshipID, domain.Health{})
	hValFlag.(*domain.Health).Current = 60

	// Второй корабль уничтожен (удаляем его из мира инстанса)
	targetWorld.DestroyEntity(shipEntities[1])

	// Третий корабль получает легкий урон (HP 90 -> 85)
	hValEsc2, _ := targetWorld.GetComponent(shipEntities[2], domain.Health{})
	hValEsc2.(*domain.Health).Current = 85

	// 6. Упаковываем обратно
	alive := PackFleet(sourceWorld, targetWorld, playerID, shipEntities)
	if !alive {
		t.Error("Expected flagship to survive, but PackFleet returned false")
	}

	fleetVal, _ := sourceWorld.GetComponent(playerID, domain.Fleet{})
	fleet := fleetVal.(*domain.Fleet)

	// Должно остаться 2 корабля (первый и третий)
	if len(fleet.Ships) != 2 {
		t.Fatalf("Expected 2 surviving ships in fleet, got %d", len(fleet.Ships))
	}

	if fleet.Ships[0].ShipID != 1 || fleet.Ships[0].Health != 60 {
		t.Errorf("Flagship data mismatch: %+v", fleet.Ships[0])
	}

	if fleet.Ships[1].ShipID != 3 || fleet.Ships[1].Health != 85 {
		t.Errorf("Escort data mismatch: %+v", fleet.Ships[1])
	}
}

func TestCombatInstance_PackFleetFlagshipDestroyedEscortSurvives(t *testing.T) {
	sourceWorld := ecs.NewWorld()
	targetWorld := ecs.NewWorld()

	// 1. Создаем сущность флота игрока с 3 кораблями в основном мире
	playerID := domain.EntityID(101)
	sourceWorld.RegisterEntityWithID(playerID, domain.EntityPlayer)
	ships := []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50},
		{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30},
		{ShipID: 3, ShipType: "cargo_helper", Health: 90, MaxHealth: 90, Shield: 40, MaxShield: 40},
	}
	sourceWorld.AddComponent(playerID, &domain.Fleet{Ships: ships})
	sourceWorld.AddComponent(playerID, &domain.PlayerData{Name: "Fleet Commander", Credits: 1000})

	// 2. Распаковываем флот в боевой мир
	teamID := uint32(1)
	shipEntities := UnpackFleet(sourceWorld, targetWorld, playerID, teamID, 100.0, 200.0, 0.0)

	if len(shipEntities) != 3 {
		t.Fatalf("Expected 3 unpacked ship entities, got %d", len(shipEntities))
	}

	// 3. Симулируем уничтожение флагмана и второго корабля
	targetWorld.DestroyEntity(shipEntities[0]) // Флагман уничтожен!
	targetWorld.DestroyEntity(shipEntities[1]) // Второй корабль уничтожен!

	// Третий корабль выживает с уроном
	hValEsc2, _ := targetWorld.GetComponent(shipEntities[2], domain.Health{})
	hValEsc2.(*domain.Health).Current = 45

	// 4. Упаковываем обратно
	alive := PackFleet(sourceWorld, targetWorld, playerID, shipEntities)
	if !alive {
		t.Error("Expected fleet to survive via surviving escort, but PackFleet returned false")
	}

	fleetVal, _ := sourceWorld.GetComponent(playerID, domain.Fleet{})
	fleet := fleetVal.(*domain.Fleet)

	// Должен остаться только 1 корабль (третий)
	if len(fleet.Ships) != 1 {
		t.Fatalf("Expected 1 surviving ship in fleet, got %d", len(fleet.Ships))
	}

	if fleet.Ships[0].ShipID != 3 || fleet.Ships[0].Health != 45 {
		t.Errorf("Surviving ship data mismatch: %+v", fleet.Ships[0])
	}
}

func TestCombatInstance_LimitAndAlliances(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	mainWorld := ecs.NewWorld()
	mainGrid := spatial.NewHashGrid(100)
	systemID := uint32(1)

	im := NewInstanceManager(bus, mainGrid, systemID, nil, logger)

	// Создаем участников
	var fleets []domain.EntityID
	for i := 1; i <= 6; i++ {
		fleetID := domain.EntityID(100 + i)
		mainWorld.RegisterEntityWithID(fleetID, domain.EntityPlayer)
		mainWorld.AddComponent(fleetID, &domain.Fleet{
			Ships: []domain.FleetShip{
				{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50},
			},
		})
		mainWorld.AddComponent(fleetID, &domain.Transform{X: float32(i * 10), Y: 0})
		mainWorld.AddComponent(fleetID, &domain.PlayerData{Name: fmt.Sprintf("Fleet %d", i), Credits: 100})
		fleets = append(fleets, fleetID)
	}

	// 1. Создаем бой 1 на 1 (инициация)
	instID := im.CreateCombatInstance(mainWorld, fleets[0], fleets[1])
	inst := im.GetInstance(instID)

	if inst == nil {
		t.Fatal("Expected combat instance to be created")
	}
	if len(inst.Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(inst.Participants))
	}

	// 2. Третий присоединяется за первого (Союзники)
	err := im.JoinCombatInstance(mainWorld, instID, fleets[2], fleets[0])
	if err != nil {
		t.Fatalf("Failed to join combat: %v", err)
	}
	if inst.Teams[fleets[2]] != inst.Teams[fleets[0]] {
		t.Error("Expected allied fleets to share TeamID")
	}

	// 3. Четвертый присоединяется за второго (Союзники)
	_ = im.JoinCombatInstance(mainWorld, instID, fleets[3], fleets[1])
	if inst.Teams[fleets[3]] != inst.Teams[fleets[1]] {
		t.Error("Expected allied fleets to share TeamID")
	}

	// 4. Пятый присоединяется сам за себя (FFA)
	_ = im.JoinCombatInstance(mainWorld, instID, fleets[4], 0)
	team5 := inst.Teams[fleets[4]]
	if team5 == inst.Teams[fleets[0]] || team5 == inst.Teams[fleets[1]] {
		t.Error("Expected FFA fleet to have a unique TeamID")
	}

	if len(inst.Participants) != 5 {
		t.Errorf("Expected 5 participants, got %d", len(inst.Participants))
	}

	// 5. Шестой пытается присоединиться (должен быть отказ из-за лимита в 5 участников)
	err = im.JoinCombatInstance(mainWorld, instID, fleets[5], 0)
	if err == nil {
		t.Error("Expected error when 6th fleet tries to join combat room, but got nil")
	}
}

func TestCombatInstance_EngagementAndResolution(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	mainWorld := ecs.NewWorld()
	mainGrid := spatial.NewHashGrid(100)
	systemID := uint32(1)

	im := NewInstanceManager(bus, mainGrid, systemID, nil, logger)
	engageSys := NewFleetEngagementSystem(im, mainGrid)

	// Создаем пирата и шахтера на расстоянии 18 единиц (< 30 авто-бой)
	pirate := domain.EntityID(201)
	mainWorld.RegisterEntityWithID(pirate, domain.EntityNPC)
	mainWorld.AddComponent(pirate, &domain.Transform{X: 100, Y: 100})
	mainWorld.AddComponent(pirate, &domain.FactionMember{FactionID: 1}) // Пираты
	mainWorld.AddComponent(pirate, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "pirate", Health: 80, MaxHealth: 80, Shield: 20, MaxShield: 20},
	}})
	mainGrid.Insert(pirate, 100, 100)

	miner := domain.EntityID(202)
	mainWorld.RegisterEntityWithID(miner, domain.EntityNPC)
	mainWorld.AddComponent(miner, &domain.Transform{X: 115, Y: 110})
	mainWorld.AddComponent(miner, &domain.FactionMember{FactionID: 2}) // Шахтеры
	mainWorld.AddComponent(miner, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "miner", Health: 100, MaxHealth: 100, Shield: 30, MaxShield: 30},
	}})
	mainGrid.Insert(miner, 115, 110)

	// Запускаем систему детекции боя
	engageSys.Update(mainWorld, 0.05)

	// Проверяем, что создался маркер боя в основном мире
	markers := mainWorld.Query(ecs.BuildMask(domain.CombatMarker{}))
	if len(markers) != 1 {
		t.Fatalf("Expected 1 CombatMarker in main world, got %d", len(markers))
	}

	markerID := markers[0]
	mVal, _ := mainWorld.GetComponent(markerID, domain.CombatMarker{})
	instID := mVal.(*domain.CombatMarker).CombatInstanceID

	inst := im.GetInstance(instID)
	if inst == nil {
		t.Fatal("Expected combat instance to exist")
	}

	// Оба флота должны находиться в состоянии боя
	cs1Val, ok1 := mainWorld.GetComponent(pirate, domain.CombatState{})
	cs2Val, ok2 := mainWorld.GetComponent(miner, domain.CombatState{})
	if !ok1 || !ok2 || cs1Val.(*domain.CombatState).CombatInstanceID != instID || cs2Val.(*domain.CombatState).CombatInstanceID != instID {
		t.Error("Expected entities to be in CombatState matching instance ID")
	}

	// Симулируем уничтожение пирата в инстансе
	// В инстансе пират имеет тот же ID, что и на карте (флагман)
	hVal, ok := inst.World.GetComponent(pirate, domain.Health{})
	if !ok {
		t.Fatal("Pirate flagship not found in combat instance world")
	}
	hVal.(*domain.Health).Current = 0 // Убит!

	// Удаляем сущность пирата из мира инстанса (как это делает CleanupSystem)
	inst.World.DestroyEntity(pirate)

	// Запускаем обновление менеджера инстансов, чтобы он заметил победу шахтера и завершил бой
	im.Update(mainWorld, 0.05)

	// Бой должен быть очищен
	if im.GetInstance(instID) != nil {
		t.Error("Expected combat instance to be cleaned up after resolution")
	}

	// Маркер боя должен исчезнуть
	markersAfter := mainWorld.Query(ecs.BuildMask(domain.CombatMarker{}))
	if len(markersAfter) != 0 {
		t.Errorf("Expected CombatMarker to be destroyed, but found %d", len(markersAfter))
	}

	// Пират должен быть удален из основного мира (так как его флагман уничтожен)
	_, pirateExists := mainWorld.GetEntityType(pirate)
	if pirateExists {
		t.Error("Expected pirate fleet to be destroyed from main world")
	}

	// Шахтер должен выжить, выйти из режима боя и вернуться на карту
	_, minerExists := mainWorld.GetEntityType(miner)
	if !minerExists {
		t.Fatal("Expected miner fleet to survive in main world")
	}

	if _, inCombat := mainWorld.GetComponent(miner, domain.CombatState{}); inCombat {
		t.Error("Expected miner to leave CombatState after resolution")
	}
}

func TestCombatInstance_PreventDoubleCombat(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	mainWorld := ecs.NewWorld()
	mainGrid := spatial.NewHashGrid(100)
	systemID := uint32(1)

	im := NewInstanceManager(bus, mainGrid, systemID, nil, logger)

	fleet1 := domain.EntityID(301)
	mainWorld.RegisterEntityWithID(fleet1, domain.EntityPlayer)
	mainWorld.AddComponent(fleet1, &domain.Transform{X: 100, Y: 100})
	mainWorld.AddComponent(fleet1, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50},
	}})

	fleet2 := domain.EntityID(302)
	mainWorld.RegisterEntityWithID(fleet2, domain.EntityNPC)
	mainWorld.AddComponent(fleet2, &domain.Transform{X: 110, Y: 110})
	mainWorld.AddComponent(fleet2, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "patrol", Health: 80, MaxHealth: 80, Shield: 20, MaxShield: 20},
	}})

	fleet3 := domain.EntityID(303)
	mainWorld.RegisterEntityWithID(fleet3, domain.EntityNPC)
	mainWorld.AddComponent(fleet3, &domain.Transform{X: 120, Y: 120})
	mainWorld.AddComponent(fleet3, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "pirate", Health: 80, MaxHealth: 80, Shield: 20, MaxShield: 20},
	}})

	// 1. Create first combat instance between fleet1 and fleet2
	instID := im.CreateCombatInstance(mainWorld, fleet1, fleet2)
	if instID == 0 {
		t.Fatal("Expected first combat instance to be created successfully")
	}

	// 2. Attempt to create second combat instance with fleet1 (already in combat) and fleet3
	instID2 := im.CreateCombatInstance(mainWorld, fleet1, fleet3)
	if instID2 != 0 {
		t.Error("Expected CreateCombatInstance to fail (return 0) when attacker is already in combat")
	}

	// 3. Attempt to create second combat instance with fleet3 and fleet2 (already in combat)
	instID3 := im.CreateCombatInstance(mainWorld, fleet3, fleet2)
	if instID3 != 0 {
		t.Error("Expected CreateCombatInstance to fail (return 0) when defender is already in combat")
	}

	// 4. Attempt to join an existing combat with fleet2 (already in combat)
	err := im.JoinCombatInstance(mainWorld, instID, fleet2, fleet1)
	if err == nil {
		t.Error("Expected JoinCombatInstance to fail when joining fleet is already in combat")
	}
}

// Регрессионный тест: после боя HP/SH сущности флота в основном мире должны отражать
// урон ведущего корабля, а не доболевые значения (баг "после боя HP и SH отображаются неверно").
func TestCombatInstance_PackFleetSyncsEntityHealth(t *testing.T) {
	sourceWorld := ecs.NewWorld()
	targetWorld := ecs.NewWorld()

	playerID := domain.EntityID(101)
	sourceWorld.RegisterEntityWithID(playerID, domain.EntityPlayer)
	sourceWorld.AddComponent(playerID, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50},
	}})
	// Сущность в основном мире имеет полные HP/SH до боя
	sourceWorld.AddComponent(playerID, &domain.Health{Current: 100, Max: 100})
	sourceWorld.AddComponent(playerID, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})

	shipEntities := UnpackFleet(sourceWorld, targetWorld, playerID, 1, 0, 0, 0)

	// Флагман получает урон в бою: HP 100->40, SH 50->10
	hVal, _ := targetWorld.GetComponent(shipEntities[0], domain.Health{})
	hVal.(*domain.Health).Current = 40
	sVal, _ := targetWorld.GetComponent(shipEntities[0], domain.Shield{})
	sVal.(*domain.Shield).Current = 10

	if alive := PackFleet(sourceWorld, targetWorld, playerID, shipEntities); !alive {
		t.Fatal("Expected fleet to survive")
	}

	hMain, _ := sourceWorld.GetComponent(playerID, domain.Health{})
	if hMain.(*domain.Health).Current != 40 {
		t.Errorf("Expected entity Health synced to 40 after combat, got %d", hMain.(*domain.Health).Current)
	}
	sMain, _ := sourceWorld.GetComponent(playerID, domain.Shield{})
	if sMain.(*domain.Shield).Current != 10 {
		t.Errorf("Expected entity Shield synced to 10 after combat, got %d", sMain.(*domain.Shield).Current)
	}
}

// Регрессионный тест: миграция через прыжковый гейт должна сохранять полный состав флота с HP/SH
// (баг "при входе в гейт HP и SH флота сбрасываются").
func TestJumpGate_MigrationPreservesFleet(t *testing.T) {
	srcWorld := ecs.NewWorld()

	playerID := domain.EntityID(777)
	srcWorld.RegisterEntityWithID(playerID, domain.EntityPlayer)
	srcWorld.AddComponent(playerID, &domain.Transform{X: 10, Y: 20})
	srcWorld.AddComponent(playerID, &domain.Health{Current: 55, Max: 100})
	srcWorld.AddComponent(playerID, &domain.Shield{Current: 5, Max: 50, RegenRate: 1.0})
	srcWorld.AddComponent(playerID, &domain.PlayerData{Name: "Jumper", Credits: 500})
	// Флот с повреждённым эскортом
	srcWorld.AddComponent(playerID, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 55, MaxHealth: 100, Shield: 5, MaxShield: 50, CargoCapacity: 100},
		{ShipID: 2, ShipType: "miner", Health: 12, MaxHealth: 80, Shield: 3, MaxShield: 30, CargoCapacity: 150},
	}})

	payload := SerializePlayer(srcWorld, playerID)

	// Восстанавливаем во "втором" мире (как на целевом узле)
	dstWorld := ecs.NewWorld()
	newID := DeserializePlayer(dstWorld, payload)

	flVal, ok := dstWorld.GetComponent(newID, domain.Fleet{})
	if !ok {
		t.Fatal("Expected Fleet component after migration")
	}
	fleet := flVal.(*domain.Fleet)
	if len(fleet.Ships) != 2 {
		t.Fatalf("Expected 2 ships preserved across gate, got %d", len(fleet.Ships))
	}
	if fleet.Ships[0].Health != 55 || fleet.Ships[0].Shield != 5 {
		t.Errorf("Flagship HP/SH not preserved: %+v", fleet.Ships[0])
	}
	if fleet.Ships[1].ShipType != "miner" || fleet.Ships[1].Health != 12 || fleet.Ships[1].Shield != 3 {
		t.Errorf("Escort HP/SH/roster not preserved (was reset?): %+v", fleet.Ships[1])
	}
}
