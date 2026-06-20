package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load loads the configuration from a file path.
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Override from env variables if present
	if envAddr := os.Getenv("SERVER_ADDRESS"); envAddr != "" {
		cfg.Server.Address = envAddr
	}
	if envDSN := os.Getenv("DATABASE_DSN"); envDSN != "" {
		cfg.Database.DSN = envDSN
	}
	if envRedis := os.Getenv("REDIS_ADDRESS"); envRedis != "" {
		cfg.Redis.Address = envRedis
	}
	if envNATS := os.Getenv("NATS_URL"); envNATS != "" {
		cfg.NATS.URL = envNATS
	}

	return &cfg, nil
}
