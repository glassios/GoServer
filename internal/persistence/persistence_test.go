package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/persistence/postgres"
	redisPersist "github.com/Home/galaxy-mmo/internal/persistence/redis"
)

func TestPersistence_IntegrationRoundtrip(t *testing.T) {
	ctx := context.Background()

	// 1. Try connecting to PostgreSQL
	pgDSN := "postgres://postgres:postgres@localhost:5432/galaxy?sslmode=disable"
	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Skip("Skipping test: failed to open postgres connection definition")
	}
	defer db.Close()

	// Ping with timeout to verify service is actually running
	db.SetConnMaxLifetime(time.Second)
	err = db.PingContext(ctx)
	if err != nil {
		t.Skip("Skipping test: PostgreSQL is not running on localhost:5432")
	}

	// 2. Try connecting to Redis
	rClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rClient.Close()

	err = rClient.Ping(ctx).Err()
	if err != nil {
		t.Skip("Skipping test: Redis is not running on localhost:6379")
	}

	// Clean up previous test database tables
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS character_fleets, ship_configurations, hullmods, weapon_definitions, ship_hulls, item_instances, item_definitions, corporation_station_vaults, player_station_vaults, fleet_ships, npcs, npc_behaviors, jump_gates, asteroids, corporation_members, corporations, stations, factions, characters, accounts CASCADE;")

	// 3. Run Migrations
	migrationPath := filepath.Join("postgres", "migrations")
	err = postgres.RunMigrations(ctx, db, migrationPath)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create repositories
	playerRepo := postgres.NewPostgresPlayerRepository(db)
	sessionCache := redisPersist.NewRedisSessionCache(rClient)
	tracker := redisPersist.NewOnlineTracker(rClient)

	// Clean Redis key
	_ = rClient.Del(ctx, "session:test_sess_token").Err()
	_ = rClient.Del(ctx, "online_players").Err()

	// 4. Test Auth Account creation for foreign keys
	// A character must reference an account
	_, err = db.ExecContext(ctx, "INSERT INTO accounts (id, login, password_hash) VALUES (1, 'Player1', 'hash123')")
	if err != nil {
		t.Fatalf("failed to insert mock account: %v", err)
	}

	// 5. Test Player Save
	playerData := &domain.PlayerData{
		AccountID: 1,
		Name:      "Player1",
		Credits:   1500,
		SystemID:  2,
	}

	comps := domain.PlayerComponents{
		Transform: &domain.Transform{X: 120.5, Y: -50.2, Rotation: 1.57},
		Cargo: &domain.Cargo{
			Items: []domain.ItemInstance{
				{DefinitionID: 1, Quantity: 10, State: "normal"},
				{DefinitionID: 2, Quantity: 5, State: "normal"},
			},
			Capacity: 100,
		},
		ShipConfig: &domain.ShipConfig{ShipType: "interceptor"},
		Fleet: &domain.Fleet{
			Ships: []domain.FleetShip{
				{ShipID: 1, ShipType: "interceptor", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50, CargoCapacity: 100},
			},
		},
	}

	err = playerRepo.Save(ctx, playerData, comps)
	if err != nil {
		t.Fatalf("failed to save player: %v", err)
	}

	// 6. Test Player Load
	loadedData, loadedComps, err := playerRepo.Load(ctx, 1)
	if err != nil {
		t.Fatalf("failed to load player: %v", err)
	}

	if loadedData.Name != "Player1" || loadedData.Credits != 1500 || loadedData.SystemID != 2 {
		t.Errorf("loaded data mismatch: %+v", loadedData)
	}

	if loadedComps.Transform.X != 120.5 || loadedComps.Transform.Y != -50.2 || loadedComps.Transform.Rotation != 1.57 {
		t.Errorf("loaded transform mismatch: %+v", loadedComps.Transform)
	}

	if loadedComps.Cargo.GetResourceTypeQuantity(domain.ResourceIron) != 10 || loadedComps.Cargo.GetResourceTypeQuantity(domain.ResourceTitanium) != 5 {
		t.Errorf("loaded cargo mismatch: %+v", loadedComps.Cargo.Items)
	}

	if loadedComps.ShipConfig.ShipType != "interceptor" {
		t.Errorf("loaded ship type mismatch: %s", loadedComps.ShipConfig.ShipType)
	}

	// 7. Test Session Cache in Redis
	err = sessionCache.Set(ctx, "test_sess_token", 1, 10*time.Minute)
	if err != nil {
		t.Fatalf("redis set failed: %v", err)
	}

	acctID, err := sessionCache.Get(ctx, "test_sess_token")
	if err != nil {
		t.Fatalf("redis get failed: %v", err)
	}
	if acctID != 1 {
		t.Errorf("expected account ID 1 from session cache, got %d", acctID)
	}

	err = sessionCache.Delete(ctx, "test_sess_token")
	if err != nil {
		t.Fatalf("redis delete failed: %v", err)
	}

	_, err = sessionCache.Get(ctx, "test_sess_token")
	if err != domain.ErrSessionExpired {
		t.Errorf("expected session expired error, got %v", err)
	}

	// 8. Test Online Tracker in Redis
	err = tracker.TrackOnline(ctx, 1)
	if err != nil {
		t.Fatalf("online tracker add failed: %v", err)
	}

	count, err := tracker.GetOnlineCount(ctx)
	if err != nil {
		t.Fatalf("get online count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected online count 1, got %d", count)
	}

	err = tracker.TrackOffline(ctx, 1)
	if err != nil {
		t.Fatalf("online tracker remove failed: %v", err)
	}

	count, err = tracker.GetOnlineCount(ctx)
	if err != nil {
		t.Fatalf("get online count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected online count 0, got %d", count)
	}
}

func TestInMemoryCorporationRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryCorporationRepository()

	// 1. Create corp
	corp, err := repo.Create(ctx, "TestCorp", 1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if corp.Name != "TestCorp" || corp.FounderID != 1 {
		t.Errorf("Unexpected corp data: %+v", corp)
	}

	// 2. Get corp
	c, err := repo.Get(ctx, corp.ID)
	if err != nil || c == nil {
		t.Fatalf("Get failed: %v, %v", err, c)
	}
	if c.Name != "TestCorp" {
		t.Errorf("Expected TestCorp, got %s", c.Name)
	}

	// 3. Add member
	err = repo.AddMember(ctx, corp.ID, 2, "Officer")
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// 4. Get role
	cID, role, err := repo.GetMemberRole(ctx, 2)
	if err != nil || cID != corp.ID || role != "Officer" {
		t.Errorf("GetMemberRole failed: corpID=%d, role=%s, err=%v", cID, role, err)
	}

	// 5. Get members
	members, err := repo.GetMembers(ctx, corp.ID)
	if err != nil || len(members) != 2 {
		t.Errorf("GetMembers failed: %v, len=%d", err, len(members))
	}
	if members[1] != "Owner" || members[2] != "Officer" {
		t.Errorf("Unexpected members role map: %v", members)
	}

	// 6. Update Wallet
	balance, err := repo.UpdateWallet(ctx, corp.ID, 500)
	if err != nil || balance != 500 {
		t.Errorf("UpdateWallet failed: balance=%d, err=%v", balance, err)
	}

	// 7. Remove Member
	err = repo.RemoveMember(ctx, 2)
	if err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	cID, role, err = repo.GetMemberRole(ctx, 2)
	if err != nil || cID != 0 || role != "" {
		t.Errorf("Expected removed member to have no corp/role, got cID=%d, role=%s", cID, role)
	}
}

func TestPostgresCorporationRepository(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/galaxy?sslmode=disable")
	if err == nil {
		err = db.PingContext(ctx)
	}
	if err != nil {
		t.Skip("Skipping test: PostgreSQL is not running on localhost:5432")
	}
	defer db.Close()

	// Clean up previous test database tables
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS corporation_members CASCADE; DROP TABLE IF EXISTS corporations CASCADE;")

	// Run migrations to ensure tables are fresh
	migrationPath := filepath.Join("postgres", "migrations")
	err = postgres.RunMigrations(ctx, db, migrationPath)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	repo := postgres.NewPostgresCorporationRepository(db)

	// Test identical roundtrip as in-memory
	corp, err := repo.Create(ctx, "TestCorpPostgres", 1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	c, err := repo.Get(ctx, corp.ID)
	if err != nil || c == nil {
		t.Fatalf("Get failed")
	}

	err = repo.AddMember(ctx, corp.ID, 2, "Officer")
	if err != nil {
		t.Fatalf("AddMember failed")
	}

	cID, role, err := repo.GetMemberRole(ctx, 2)
	if err != nil || cID != corp.ID || role != "Officer" {
		t.Errorf("GetMemberRole failed")
	}

	members, err := repo.GetMembers(ctx, corp.ID)
	if err != nil || len(members) != 2 {
		t.Errorf("GetMembers failed")
	}

	balance, err := repo.UpdateWallet(ctx, corp.ID, 1000)
	if err != nil || balance != 1000 {
		t.Errorf("UpdateWallet failed")
	}

	err = repo.RemoveMember(ctx, 2)
	if err != nil {
		t.Fatalf("RemoveMember failed")
	}

	cID, role, err = repo.GetMemberRole(ctx, 2)
	if err != nil || cID != 0 || role != "" {
		t.Errorf("Expected removed member to have no corp/role")
	}
}
