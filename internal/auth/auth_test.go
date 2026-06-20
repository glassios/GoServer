package auth

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

func TestToken_Generation(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if len(token) != 64 { // 32 bytes hex encoded is 64 characters
		t.Errorf("expected token length 64, got %d", len(token))
	}

	token2, _ := GenerateSessionToken()
	if token == token2 {
		t.Error("expected tokens to be unique")
	}
}

func TestAuth_Integration(t *testing.T) {
	ctx := context.Background()

	// 1. Try PostgreSQL connection
	pgDSN := "postgres://postgres:postgres@localhost:5432/galaxy?sslmode=disable"
	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Skip("Skipping test: failed to open postgres definition")
	}
	defer db.Close()

	db.SetConnMaxLifetime(time.Second)
	err = db.PingContext(ctx)
	if err != nil {
		t.Skip("Skipping test: PostgreSQL is not running")
	}

	// 2. Try Redis connection
	rClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rClient.Close()

	err = rClient.Ping(ctx).Err()
	if err != nil {
		t.Skip("Skipping test: Redis is not running")
	}

	// Clean up database tables for fresh migration
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS character_fleets, ship_configurations, hullmods, weapon_definitions, ship_hulls, item_instances, item_definitions, corporation_station_vaults, player_station_vaults, fleet_ships, npcs, npc_behaviors, jump_gates, asteroids, corporation_members, corporations, stations, factions, characters, accounts CASCADE;")

	// 3. Migrate database schema
	migrationPath := filepath.Join("..", "persistence", "postgres", "migrations")
	err = postgres.RunMigrations(ctx, db, migrationPath)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	sessionCache := redisPersist.NewRedisSessionCache(rClient)
	authService := NewAuthService(db, sessionCache)

	// 4. Test Register
	acctID, err := authService.Register(ctx, "User1", "mypassword123")
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	if acctID != 1 {
		t.Errorf("expected registered account ID 1, got %d", acctID)
	}

	// 5. Test Login (Success)
	token, loginID, err := authService.Login(ctx, "User1", "mypassword123")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginID != 1 {
		t.Errorf("expected logged in ID 1, got %d", loginID)
	}
	if len(token) != 64 {
		t.Errorf("expected session token length 64, got %d", len(token))
	}

	// Verify session was stored in Redis
	cachedID, err := sessionCache.Get(ctx, token)
	if err != nil {
		t.Fatalf("failed to retrieve session from cache: %v", err)
	}
	if cachedID != 1 {
		t.Errorf("expected cached account ID 1, got %d", cachedID)
	}

	// 6. Test Login (Fail - Invalid Password)
	_, _, err = authService.Login(ctx, "User1", "wrongpassword")
	if err != domain.ErrInvalidCredentials {
		t.Errorf("expected invalid credentials error, got %v", err)
	}

	// 7. Test Login (Fail - Unknown Login)
	_, _, err = authService.Login(ctx, "Stranger", "mypassword123")
	if err != domain.ErrInvalidCredentials {
		t.Errorf("expected invalid credentials error, got %v", err)
	}
}
