package config

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Grid     GridConfig     `yaml:"grid"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	NATS     NATSConfig     `yaml:"nats"`
}

type ServerConfig struct {
	Tickrate   int    `yaml:"tickrate"`
	MaxPlayers int    `yaml:"max_players"`
	Address    string `yaml:"address"`
}

type GridConfig struct {
	CellSize    float32 `yaml:"cell_size"`
	WorldWidth  float32 `yaml:"world_width"`
	WorldHeight float32 `yaml:"world_height"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Address string `yaml:"address"`
}

type NATSConfig struct {
	URL string `yaml:"url"`
}
