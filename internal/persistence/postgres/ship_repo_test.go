package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/systems"
)

func TestShipFitting_Integration(t *testing.T) {
	ctx := context.Background()

	// 1. Try PostgreSQL connection
	pgDSN := "postgres://postgres:postgres@localhost:5432/galaxy?sslmode=disable"
	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Skip("Skipping test: failed to open postgres connection")
	}
	defer db.Close()

	db.SetConnMaxLifetime(time.Second)
	err = db.PingContext(ctx)
	if err != nil {
		t.Skip("Skipping test: PostgreSQL is not running")
	}

	// Clean up database tables for fresh migration
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS character_fleets, ship_configurations, hullmods, weapon_definitions, ship_hulls, item_instances, item_definitions, corporation_station_vaults, player_station_vaults, fleet_ships, npcs, npc_behaviors, jump_gates, asteroids, corporation_members, corporations, stations, factions, characters, accounts CASCADE;")

	// 2. Migrate database schema (which now includes 007)
	migrationPath := filepath.Join("migrations")
	err = RunMigrations(ctx, db, migrationPath)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	shipRepo := NewPostgresShipRepository(db)

	// 3. Seed Static Ship/Weapon Info
	slots := []domain.WeaponSlot{
		{SlotID: "WS001", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: 10, Y: 15, Angle: 0},
		{SlotID: "WS002", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: -10, Y: 15, Angle: 0},
	}
	slotsJSON, _ := json.Marshal(slots)

	// Use an id outside the range seeded by migration 009_fitting_seed.sql to avoid PK collision.
	_, err = db.ExecContext(ctx, `
		INSERT INTO ship_hulls (id, hull_id, name, base_hp, base_armor, base_shield_max, shield_type, shield_arc, shield_efficiency, base_max_speed, base_turn_rate, ordnance_points, weapon_slots)
		VALUES (9001, 'lasher', 'Lasher Frigate', 100.0, 50.0, 50.0, 'omni', 90.0, 1.0, 100.0, 2.0, 50, $1)
	`, slotsJSON)
	if err != nil {
		t.Fatalf("failed to seed ship hulls: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO weapon_definitions (weapon_id, name, weapon_type, weapon_size, op_cost, damage_per_shot, damage_type, flux_cost, range, cooldown)
		VALUES ('vulcan', 'Vulcan Cannon', 'BALLISTIC', 'SMALL', 4, 5.0, 'FRAGMENTATION', 2.0, 300.0, 0.1)
	`)
	if err != nil {
		t.Fatalf("failed to seed weapon definitions: %v", err)
	}

	opCost := map[string]int32{"FRIGATE": 5, "DESTROYER": 10}
	opCostJSON, _ := json.Marshal(opCost)
	modifiers := map[string]float32{"max_speed_mult": 1.2, "shield_max_mult": 0.8}
	modifiersJSON, _ := json.Marshal(modifiers)

	_, err = db.ExecContext(ctx, `
		INSERT INTO hullmods (mod_id, name, op_cost_by_size, modifiers)
		VALUES ('auxiliary_thrusters', 'Auxiliary Thrusters', $1, $2)
	`, opCostJSON, modifiersJSON)
	if err != nil {
		t.Fatalf("failed to seed hullmods: %v", err)
	}

	// 4. Test Configuration Save
	config := &domain.ShipConfiguration{
		OwnerID:        1,
		OwnerType:      "player",
		HullID:         9001,
		CustomName:     "HMS Bounty",
		FittedWeapons:  map[string]string{"WS001": "vulcan"},
		FittedHullmods: []string{"auxiliary_thrusters"},
		Vents:          5,
		Capacitors:     10,
	}
	err = shipRepo.SaveConfiguration(ctx, config)
	if err != nil {
		t.Fatalf("failed to save ship configuration: %v", err)
	}

	if config.ID == 0 {
		t.Errorf("expected config ID to be returned, got 0")
	}

	// 5. Test Configuration Load
	loadedConfig, err := shipRepo.LoadConfiguration(ctx, config.ID)
	if err != nil {
		t.Fatalf("failed to load ship configuration: %v", err)
	}

	if loadedConfig.CustomName != "HMS Bounty" || loadedConfig.Vents != 5 || loadedConfig.Capacitors != 10 {
		t.Errorf("loaded config mismatch: %+v", loadedConfig)
	}
	if loadedConfig.FittedWeapons["WS001"] != "vulcan" {
		t.Errorf("loaded fitted weapons mismatch: %+v", loadedConfig.FittedWeapons)
	}

	// 6. Test Fleet Save and Load
	fleet := &domain.CharacterFleet{
		OwnerID:   1,
		OwnerType: "player",
		SystemID:  1,
		X:         500.0,
		Y:         -300.0,
		ShipIDs:   []uint64{config.ID},
	}
	err = shipRepo.SaveFleet(ctx, fleet)
	if err != nil {
		t.Fatalf("failed to save fleet: %v", err)
	}

	loadedFleet, err := shipRepo.LoadFleet(ctx, 1, "player")
	if err != nil {
		t.Fatalf("failed to load fleet: %v", err)
	}

	if loadedFleet.X != 500.0 || loadedFleet.Y != -300.0 || len(loadedFleet.ShipIDs) != 1 || loadedFleet.ShipIDs[0] != config.ID {
		t.Errorf("loaded fleet mismatch: %+v", loadedFleet)
	}

	// 7. Test ECS Baking
	world := ecs.NewWorld()
	entity := domain.EntityID(101)

	err = systems.BakeShip(world, entity, loadedConfig, shipRepo, ctx)
	if err != nil {
		t.Fatalf("failed to bake ship config into ECS: %v", err)
	}

	// Assert ECS components exist and contain calculated values
	healthVal, foundHP := world.GetComponent(entity, domain.Health{})
	if !foundHP || healthVal.(*domain.Health).Max != 100 {
		t.Errorf("expected max health 100, got %+v", healthVal)
	}

	fluxVal, foundFlux := world.GetComponent(entity, domain.FluxState{})
	// base max flux = HP * 10 = 1000
	// capacitors bonus = 10 * 200 = 2000
	// total = 3000
	if !foundFlux || fluxVal.(*domain.FluxState).Capacity != 3000.0 {
		t.Errorf("expected flux capacity 3000.0, got %+v", fluxVal)
	}

	speedVal, foundSpeed := world.GetComponent(entity, domain.ShipConfig{})
	// base max speed = 100.0
	// hullmod thrusters speed_mult = 1.2
	// total = 120.0
	if !foundSpeed || speedVal.(*domain.ShipConfig).MaxSpeed < 119.99 || speedVal.(*domain.ShipConfig).MaxSpeed > 120.01 {
		t.Errorf("expected max speed ~120.0, got %+v", speedVal)
	}

	weaponGroupVal, foundWeapons := world.GetComponent(entity, domain.WeaponGroup{})
	if !foundWeapons || len(weaponGroupVal.(*domain.WeaponGroup).Weapons) != 1 || weaponGroupVal.(*domain.WeaponGroup).Weapons[0].SlotID != "WS001" {
		t.Errorf("expected weapon in WS001 slot, got %+v", weaponGroupVal)
	}
}
