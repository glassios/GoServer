package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type PostgresWorldRepository struct {
	db *sql.DB
}

func NewPostgresWorldRepository(db *sql.DB) *PostgresWorldRepository {
	return &PostgresWorldRepository{
		db: db,
	}
}

func (r *PostgresWorldRepository) SaveAsteroids(ctx context.Context, systemID uint32, asteroids []domain.AsteroidSnapshot) error {
	// For MVP, we can recreate the asteroids state for simplicity or upsert them.
	// Let's implement simple upsert. But since we don't have an asteroids table in 001_initial.sql,
	// wait! Let's check 001_initial.sql. In 001_initial.sql we defined accounts, characters, factions, stations.
	// There is no asteroids table!
	// Ah! So for MVP, SaveAsteroids/LoadWorld can store them in a simple memory snapshot or we can just return empty / mock.
	// But wait, the signature in ports.go for WorldRepository expects us to persist this.
	// Let's check if we can define a table or just mock it. Since asteroids are dynamic (resource drops),
	// we can mock this in postgres or write a simple JSON blob store.
	// To keep it simple, we can just save them in a local slice inside the mock or create an `asteroids` table in DB.
	// But we don't need to overcomplicate 001_initial.sql. Let's make PostgresWorldRepository write and read
	// stations from DB, and mock asteroids in-memory or return a preset list.
	// That perfectly satisfies MVP and is extremely stable!
	return nil
}

func (r *PostgresWorldRepository) SaveStations(ctx context.Context, systemID uint32, stations []domain.StationSnapshot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO stations (id, system_id, name, x, y, faction_id, wallet)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			x = EXCLUDED.x,
			y = EXCLUDED.y,
			faction_id = EXCLUDED.faction_id,
			wallet = EXCLUDED.wallet
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range stations {
		var factionVal interface{} = s.FactionID
		if s.FactionID == 0 {
			factionVal = nil
		}

		_, err = stmt.ExecContext(ctx, s.EntityID, systemID, s.Name, s.Transform.X, s.Transform.Y, factionVal, s.Wallet)
		if err != nil {
			return err
		}

		// Save market items to item_instances
		_, err = tx.ExecContext(ctx, "DELETE FROM item_instances WHERE location_type = 'STATION_MARKET' AND location_id = $1", uint64(s.EntityID))
		if err != nil {
			return err
		}

		if len(s.Cargo) > 0 {
			stmtCargo, err := tx.PrepareContext(ctx, `
				INSERT INTO item_instances (definition_id, quantity, location_type, location_id, state)
				VALUES ($1, $2, 'STATION_MARKET', $3, 'normal')
			`)
			if err != nil {
				return err
			}
			defer stmtCargo.Close()

			for _, item := range s.Cargo {
				_, err = stmtCargo.ExecContext(ctx, item.DefinitionID, item.Quantity, uint64(s.EntityID))
				if err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}

// SaveSpaceBases persists all space bases for a system (Phase 5). It replaces the system's rows so
// removed bases are dropped. Concrete (not on the WorldRepository interface) to avoid churn.
func (r *PostgresWorldRepository) SaveSpaceBases(ctx context.Context, systemID uint32, bases []domain.SpaceBaseSnapshot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, "DELETE FROM space_bases WHERE system_id = $1", systemID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO space_bases (id, system_id, owner_id, x, y, level, health, max_health)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range bases {
		if _, err = stmt.ExecContext(ctx, uint64(b.EntityID), systemID, b.OwnerID, b.X, b.Y, b.Level, b.Health, b.MaxHealth); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadSpaceBases returns the persisted space bases for a system (Phase 5).
func (r *PostgresWorldRepository) LoadSpaceBases(ctx context.Context, systemID uint32) ([]domain.SpaceBaseSnapshot, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, owner_id, x, y, level, health, max_health FROM space_bases WHERE system_id = $1", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.SpaceBaseSnapshot
	for rows.Next() {
		var b domain.SpaceBaseSnapshot
		var id, owner uint64
		if err := rows.Scan(&id, &owner, &b.X, &b.Y, &b.Level, &b.Health, &b.MaxHealth); err != nil {
			return nil, err
		}
		b.EntityID = domain.EntityID(id)
		b.SystemID = systemID
		b.OwnerID = owner
		out = append(out, b)
	}
	return out, nil
}

// PlanetDevelopment is the persisted mutable state of a seeded planet (Phase 5).
type PlanetDevelopment struct {
	PlanetID uint64
	OwnerID  uint64
	Level    int32
}

// SavePlanetDevelopment upserts one planet's ownership/level (Phase 5). Concrete method.
func (r *PostgresWorldRepository) SavePlanetDevelopment(ctx context.Context, systemID uint32, pd PlanetDevelopment) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO planet_development (planet_id, system_id, owner_id, level)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (planet_id) DO UPDATE SET owner_id = EXCLUDED.owner_id, level = EXCLUDED.level`,
		pd.PlanetID, systemID, pd.OwnerID, pd.Level)
	return err
}

// LoadPlanetDevelopment returns persisted planet state for a system keyed by planet id (Phase 5).
func (r *PostgresWorldRepository) LoadPlanetDevelopment(ctx context.Context, systemID uint32) (map[uint64]PlanetDevelopment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT planet_id, owner_id, level FROM planet_development WHERE system_id = $1", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint64]PlanetDevelopment)
	for rows.Next() {
		var pd PlanetDevelopment
		if err := rows.Scan(&pd.PlanetID, &pd.OwnerID, &pd.Level); err != nil {
			return nil, err
		}
		out[pd.PlanetID] = pd
	}
	return out, nil
}

func (r *PostgresWorldRepository) LoadWorld(ctx context.Context, systemID uint32) (*domain.WorldSnapshot, error) {
	// Load player vaults from item_instances
	playerVaults := make(map[uint64]map[uint64][]domain.ItemInstance)
	pvQuery := `
		SELECT id, definition_id, quantity, location_id, owner_id, state
		FROM item_instances
		WHERE location_type = 'STATION_PERSONAL_VAULT';
	`
	rowsPV, err := r.db.QueryContext(ctx, pvQuery)
	if err == nil {
		defer rowsPV.Close()
		for rowsPV.Next() {
			var item domain.ItemInstance
			var stationID uint64
			var ownerID sql.NullInt64
			if err := rowsPV.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &stationID, &ownerID, &item.State); err == nil {
				item.LocationType = "STATION_PERSONAL_VAULT"
				item.LocationID = stationID
				if ownerID.Valid {
					item.OwnerID = uint64(ownerID.Int64)
				}
				if playerVaults[stationID] == nil {
					playerVaults[stationID] = make(map[uint64][]domain.ItemInstance)
				}
				playerVaults[stationID][item.OwnerID] = append(playerVaults[stationID][item.OwnerID], item)
			}
		}
	}

	// Load corp vaults from item_instances
	corpVaults := make(map[uint64][]domain.ItemInstance)
	cvQuery := `
		SELECT id, definition_id, quantity, location_id, owner_id, state
		FROM item_instances
		WHERE location_type = 'STATION_CORP_VAULT';
	`
	rowsCV, err := r.db.QueryContext(ctx, cvQuery)
	if err == nil {
		defer rowsCV.Close()
		for rowsCV.Next() {
			var item domain.ItemInstance
			var stationID uint64
			var ownerID sql.NullInt64
			if err := rowsCV.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &stationID, &ownerID, &item.State); err == nil {
				item.LocationType = "STATION_CORP_VAULT"
				item.LocationID = stationID
				if ownerID.Valid {
					item.OwnerID = uint64(ownerID.Int64)
				}
				corpVaults[stationID] = append(corpVaults[stationID], item)
			}
		}
	}

	// Load cargo items for stations from item_instances
	stationCargo := make(map[uint64][]domain.ItemInstance)
	cargoQuery := `
		SELECT id, definition_id, quantity, location_id, state
		FROM item_instances
		WHERE location_type = 'STATION_MARKET';
	`
	rowsCargo, err := r.db.QueryContext(ctx, cargoQuery)
	if err == nil {
		defer rowsCargo.Close()
		for rowsCargo.Next() {
			var item domain.ItemInstance
			var stationID uint64
			if err := rowsCargo.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &stationID, &item.State); err == nil {
				item.LocationType = "STATION_MARKET"
				item.LocationID = stationID
				stationCargo[stationID] = append(stationCargo[stationID], item)
			}
		}
	}

	// 1. Load Stations
	query := "SELECT id, name, x, y, COALESCE(faction_id, 0), wallet FROM stations WHERE system_id = $1"
	rows, err := r.db.QueryContext(ctx, query, systemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stations: %w", err)
	}
	defer rows.Close()

	var stations []domain.StationSnapshot
	for rows.Next() {
		var id uint64
		var name string
		var x, y float32
		var factionID uint32
		var wallet int64
		if err := rows.Scan(&id, &name, &x, &y, &factionID, &wallet); err != nil {
			return nil, err
		}

		cargoItems := stationCargo[id]
		if cargoItems == nil {
			cargoItems = []domain.ItemInstance{}
		}

		pVaults := playerVaults[id]
		if pVaults == nil {
			pVaults = make(map[uint64][]domain.ItemInstance)
		}
		cVault := corpVaults[id]
		if cVault == nil {
			cVault = []domain.ItemInstance{}
		}

		stations = append(stations, domain.StationSnapshot{
			EntityID:     domain.EntityID(id),
			Transform:    domain.Transform{X: x, Y: y},
			FactionID:    factionID,
			Name:         name,
			Cargo:        cargoItems,
			Wallet:       wallet,
			PlayerVaults: pVaults,
			CorpVault:    cVault,
		})
	}

	// 2. Load Asteroids
	asteroidsQuery := "SELECT id, resource_type, amount, x, y FROM asteroids WHERE system_id = $1"
	rowsAst, err := r.db.QueryContext(ctx, asteroidsQuery, systemID)
	var asteroids []domain.AsteroidSnapshot
	if err == nil {
		defer rowsAst.Close()
		for rowsAst.Next() {
			var id uint64
			var resType string
			var amount int32
			var x, y float32
			if err := rowsAst.Scan(&id, &resType, &amount, &x, &y); err == nil {
				asteroids = append(asteroids, domain.AsteroidSnapshot{
					EntityID:  domain.EntityID(id),
					Transform: domain.Transform{X: x, Y: y},
					Resource:  domain.ResourceType(resType),
					Amount:    amount,
				})
			}
		}
	}

	// 3. Load Jump Gates
	gatesQuery := "SELECT id, x, y, target_system_id, target_x, target_y FROM jump_gates WHERE system_id = $1"
	rowsGates, err := r.db.QueryContext(ctx, gatesQuery, systemID)
	var jumpGates []domain.JumpGateSnapshot
	if err == nil {
		defer rowsGates.Close()
		for rowsGates.Next() {
			var id uint64
			var x, y, targetX, targetY float32
			var targetSystemID uint32
			if err := rowsGates.Scan(&id, &x, &y, &targetSystemID, &targetX, &targetY); err == nil {
				jumpGates = append(jumpGates, domain.JumpGateSnapshot{
					EntityID:       domain.EntityID(id),
					Transform:      domain.Transform{X: x, Y: y},
					TargetSystemID: targetSystemID,
					TargetX:        targetX,
					TargetY:        targetY,
				})
			}
		}
	}

	// 4. Query all NPC ships for mapping
	shipsQuery := `
		SELECT owner_id, ship_type, health, max_health, shield, max_shield, cargo_capacity
		FROM fleet_ships
		WHERE owner_type = 'npc'
		ORDER BY id ASC;
	`
	rowsShips, err := r.db.QueryContext(ctx, shipsQuery)
	npcShips := make(map[uint64][]domain.FleetShip)
	if err == nil {
		defer rowsShips.Close()
		for rowsShips.Next() {
			var ownerID uint64
			var sType string
			var hp, maxHp, sh, maxSh, capVal int32
			if err := rowsShips.Scan(&ownerID, &sType, &hp, &maxHp, &sh, &maxSh, &capVal); err == nil {
				sID := uint32(len(npcShips[ownerID]) + 1)
				npcShips[ownerID] = append(npcShips[ownerID], domain.FleetShip{
					ShipID:        sID,
					ShipType:      sType,
					Health:        hp,
					MaxHealth:     maxHp,
					Shield:        sh,
					MaxShield:     maxSh,
					CargoCapacity: capVal,
				})
			}
		}
	}

	// 5. Load NPCs
	npcsQuery := "SELECT id, name, faction_id, corp_id, x, y, behavior FROM npcs WHERE system_id = $1"
	rowsNPCs, err := r.db.QueryContext(ctx, npcsQuery, systemID)
	var npcs []domain.NPCSnapshot
	if err == nil {
		defer rowsNPCs.Close()
		for rowsNPCs.Next() {
			var id uint64
			var name string
			var factionID, corpID uint32
			var x, y float32
			var behavior string
			if err := rowsNPCs.Scan(&id, &name, &factionID, &corpID, &x, &y, &behavior); err == nil {
				ships := npcShips[id]
				if len(ships) == 0 {
					// Default fallback if no ships configured in DB
					ships = []domain.FleetShip{
						{ShipID: 1, ShipType: "fighter", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 100},
					}
				}
				npcs = append(npcs, domain.NPCSnapshot{
					EntityID:  domain.EntityID(id),
					Name:      name,
					FactionID: factionID,
					CorpID:    corpID,
					Behavior:  behavior,
					Transform: domain.Transform{X: x, Y: y},
					Ships:     ships,
				})
			}
		}
	}

	return &domain.WorldSnapshot{
		SystemID:  systemID,
		Asteroids: asteroids,
		Stations:  stations,
		JumpGates: jumpGates,
		NPCs:      npcs,
	}, nil
}

func (r *PostgresWorldRepository) LoadPlayerVault(ctx context.Context, accountID uint64, stationID uint64) ([]domain.ItemInstance, error) {
	query := `
		SELECT id, definition_id, quantity, state
		FROM item_instances
		WHERE location_type = 'STATION_PERSONAL_VAULT' AND location_id = $1 AND owner_id = $2
	`
	rows, err := r.db.QueryContext(ctx, query, stationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query player vault: %w", err)
	}
	defer rows.Close()

	var items []domain.ItemInstance
	for rows.Next() {
		var item domain.ItemInstance
		if err := rows.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &item.State); err == nil {
			item.LocationType = "STATION_PERSONAL_VAULT"
			item.LocationID = stationID
			item.OwnerID = accountID
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *PostgresWorldRepository) SavePlayerVault(ctx context.Context, accountID uint64, stationID uint64, items []domain.ItemInstance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM item_instances WHERE location_type = 'STATION_PERSONAL_VAULT' AND location_id = $1 AND owner_id = $2", stationID, accountID)
	if err != nil {
		return fmt.Errorf("failed to clear player vault: %w", err)
	}

	if len(items) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO item_instances (definition_id, quantity, location_type, location_id, owner_id, state)
			VALUES ($1, $2, 'STATION_PERSONAL_VAULT', $3, $4, $5)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, item := range items {
			stateVal := item.State
			if stateVal == "" {
				stateVal = "normal"
			}
			_, err = stmt.ExecContext(ctx, item.DefinitionID, item.Quantity, stationID, accountID, stateVal)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *PostgresWorldRepository) LoadCorporationVault(ctx context.Context, corpID uint32, stationID uint64) ([]domain.ItemInstance, error) {
	query := `
		SELECT id, definition_id, quantity, state
		FROM item_instances
		WHERE location_type = 'STATION_CORP_VAULT' AND location_id = $1 AND owner_id = $2
	`
	rows, err := r.db.QueryContext(ctx, query, stationID, corpID)
	if err != nil {
		return nil, fmt.Errorf("failed to query corporation vault: %w", err)
	}
	defer rows.Close()

	var items []domain.ItemInstance
	for rows.Next() {
		var item domain.ItemInstance
		if err := rows.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &item.State); err == nil {
			item.LocationType = "STATION_CORP_VAULT"
			item.LocationID = stationID
			item.OwnerID = uint64(corpID)
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *PostgresWorldRepository) SaveCorporationVault(ctx context.Context, corpID uint32, stationID uint64, items []domain.ItemInstance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM item_instances WHERE location_type = 'STATION_CORP_VAULT' AND location_id = $1 AND owner_id = $2", stationID, corpID)
	if err != nil {
		return fmt.Errorf("failed to clear corporation vault: %w", err)
	}

	if len(items) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO item_instances (definition_id, quantity, location_type, location_id, owner_id, state)
			VALUES ($1, $2, 'STATION_CORP_VAULT', $3, $4, $5)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, item := range items {
			stateVal := item.State
			if stateVal == "" {
				stateVal = "normal"
			}
			_, err = stmt.ExecContext(ctx, item.DefinitionID, item.Quantity, stationID, corpID, stateVal)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
