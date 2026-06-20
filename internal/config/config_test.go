package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	content := []byte(`
server:
  tickrate: 30
  max_players: 50
  address: ":8888"
grid:
  cell_size: 500
  world_width: 10000
  world_height: 10000
database:
  dsn: "postgres://test"
redis:
  address: "localhost:6379"
`)
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write config content: %v", err)
	}

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Tickrate != 30 {
		t.Errorf("expected tickrate 30, got %d", cfg.Server.Tickrate)
	}
	if cfg.Server.MaxPlayers != 50 {
		t.Errorf("expected max_players 50, got %d", cfg.Server.MaxPlayers)
	}
	if cfg.Server.Address != ":8888" {
		t.Errorf("expected address :8888, got %s", cfg.Server.Address)
	}
	if cfg.Grid.CellSize != 500 {
		t.Errorf("expected cell_size 500, got %f", cfg.Grid.CellSize)
	}
	if cfg.Database.DSN != "postgres://test" {
		t.Errorf("expected dsn postgres://test, got %s", cfg.Database.DSN)
	}

	// Test environment variable override
	t.Setenv("SERVER_ADDRESS", ":9999")
	t.Setenv("DATABASE_DSN", "postgres://override")
	t.Setenv("REDIS_ADDRESS", "redis:6379")

	cfg, err = Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Address != ":9999" {
		t.Errorf("expected overridden address :9999, got %s", cfg.Server.Address)
	}
	if cfg.Database.DSN != "postgres://override" {
		t.Errorf("expected overridden dsn postgres://override, got %s", cfg.Database.DSN)
	}
	if cfg.Redis.Address != "redis:6379" {
		t.Errorf("expected overridden redis address redis:6379, got %s", cfg.Redis.Address)
	}
}
