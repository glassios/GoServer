package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type PostgresPlayerRepository struct {
	db *sql.DB
}

func NewPostgresPlayerRepository(db *sql.DB) *PostgresPlayerRepository {
	return &PostgresPlayerRepository{
		db: db,
	}
}

func (r *PostgresPlayerRepository) Save(ctx context.Context, player *domain.PlayerData, comps domain.PlayerComponents) error {
	// Use transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO characters (account_id, name, x, y, rotation, credits, system_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (account_id) DO UPDATE SET
			x = EXCLUDED.x,
			y = EXCLUDED.y,
			rotation = EXCLUDED.rotation,
			credits = EXCLUDED.credits,
			system_id = EXCLUDED.system_id;
	`

	_, err = tx.ExecContext(ctx, query,
		player.AccountID,
		player.Name,
		comps.Transform.X,
		comps.Transform.Y,
		comps.Transform.Rotation,
		player.Credits,
		player.SystemID,
	)
	if err != nil {
		return fmt.Errorf("failed to save character in tx: %w", err)
	}

	// Save player cargo items to item_instances table
	_, err = tx.ExecContext(ctx, "DELETE FROM item_instances WHERE location_type = 'SHIP_CARGO' AND location_id = $1", player.AccountID)
	if err != nil {
		return fmt.Errorf("failed to delete old ship cargo: %w", err)
	}

	if comps.Cargo != nil && len(comps.Cargo.Items) > 0 {
		stmtCargo, err := tx.PrepareContext(ctx, `
			INSERT INTO item_instances (definition_id, quantity, location_type, location_id, owner_id, state)
			VALUES ($1, $2, 'SHIP_CARGO', $3, $4, $5)`)
		if err != nil {
			return fmt.Errorf("failed to prepare cargo insert: %w", err)
		}
		defer stmtCargo.Close()

		for _, item := range comps.Cargo.Items {
			ownerVal := player.AccountID
			if item.OwnerID > 0 {
				ownerVal = item.OwnerID
			}
			stateVal := item.State
			if stateVal == "" {
				stateVal = "normal"
			}
			_, err = stmtCargo.ExecContext(ctx, item.DefinitionID, item.Quantity, player.AccountID, ownerVal, stateVal)
			if err != nil {
				return fmt.Errorf("failed to insert cargo item: %w", err)
			}
		}
	}

	// Save fleet ships if component exists
	if comps.Fleet != nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM fleet_ships WHERE owner_id = $1 AND owner_type = 'player'", player.AccountID)
		if err != nil {
			return fmt.Errorf("failed to delete old fleet ships: %w", err)
		}

		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO fleet_ships (owner_id, owner_type, ship_type, health, max_health, shield, max_shield, cargo_capacity, role, strategy)
			VALUES ($1, 'player', $2, $3, $4, $5, $6, $7, $8, $9)`)
		if err != nil {
			return fmt.Errorf("failed to prepare fleet insert: %w", err)
		}
		defer stmt.Close()

		for _, s := range comps.Fleet.Ships {
			_, err = stmt.ExecContext(ctx, player.AccountID, s.ShipType, s.Health, s.MaxHealth, s.Shield, s.MaxShield, s.CargoCapacity, s.Role, s.Strategy)
			if err != nil {
				return fmt.Errorf("failed to insert fleet ship: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit save transaction: %w", err)
	}

	return nil
}

func (r *PostgresPlayerRepository) Load(ctx context.Context, accountID uint64) (*domain.PlayerData, domain.PlayerComponents, error) {
	query := `
		SELECT name, x, y, rotation, credits, system_id
		FROM characters
		WHERE account_id = $1;
	`

	row := r.db.QueryRowContext(ctx, query, accountID)

	var name string
	var x, y, rotation float32
	var credits int64
	var systemID uint32

	err := row.Scan(&name, &x, &y, &rotation, &credits, &systemID)
	if err == sql.ErrNoRows {
		return nil, domain.PlayerComponents{}, domain.ErrPlayerNotFound
	} else if err != nil {
		return nil, domain.PlayerComponents{}, fmt.Errorf("failed to query character: %w", err)
	}

	// Load items from item_instances for this ship cargo
	var cargoItems []domain.ItemInstance
	itemsQuery := `
		SELECT id, definition_id, quantity, location_type, location_id, owner_id, state
		FROM item_instances
		WHERE location_type = 'SHIP_CARGO' AND location_id = $1;
	`
	rowsItems, err := r.db.QueryContext(ctx, itemsQuery, accountID)
	if err == nil {
		defer rowsItems.Close()
		for rowsItems.Next() {
			var item domain.ItemInstance
			var ownerID sql.NullInt64
			if err := rowsItems.Scan(&item.ID, &item.DefinitionID, &item.Quantity, &item.LocationType, &item.LocationID, &ownerID, &item.State); err == nil {
				if ownerID.Valid {
					item.OwnerID = uint64(ownerID.Int64)
				}
				cargoItems = append(cargoItems, item)
			}
		}
	}

	playerData := &domain.PlayerData{
		AccountID: accountID,
		Name:      name,
		Credits:   credits,
		SystemID:  systemID,
	}

	// Query fleet ships
	shipsQuery := `
		SELECT ship_type, health, max_health, shield, max_shield, cargo_capacity, role, strategy
		FROM fleet_ships
		WHERE owner_id = $1 AND owner_type = 'player'
		ORDER BY id ASC;
	`
	rows, err := r.db.QueryContext(ctx, shipsQuery, accountID)
	var ships []domain.FleetShip
	if err == nil {
		defer rows.Close()
		var sID uint32 = 1
		for rows.Next() {
			var sType, role, strategy string
			var hp, maxHp, sh, maxSh, capVal int32
			if err := rows.Scan(&sType, &hp, &maxHp, &sh, &maxSh, &capVal, &role, &strategy); err == nil {
				ships = append(ships, domain.FleetShip{
					ShipID:        sID,
					ShipType:      sType,
					Health:        hp,
					MaxHealth:     maxHp,
					Shield:        sh,
					MaxShield:     maxSh,
					CargoCapacity: capVal,
					Role:          role,
					Strategy:      strategy,
				})
				sID++
			}
		}
	}

	// If no ships found in DB, seed a default fleet (fighter + miner)
	if len(ships) == 0 {
		ships = []domain.FleetShip{
			{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50, CargoCapacity: 100},
			{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 150},
		}
		// Write them to the DB so they are persistent
		stmt, err := r.db.PrepareContext(ctx, `
			INSERT INTO fleet_ships (owner_id, owner_type, ship_type, health, max_health, shield, max_shield, cargo_capacity)
			VALUES ($1, 'player', $2, $3, $4, $5, $6, $7)`)
		if err == nil {
			defer stmt.Close()
			for _, s := range ships {
				_, _ = stmt.ExecContext(ctx, accountID, s.ShipType, s.Health, s.MaxHealth, s.Shield, s.MaxShield, s.CargoCapacity)
			}
		}
	}

	// Calculate total fleet cargo capacity
	var totalCargoCapacity int32 = 0
	for _, s := range ships {
		totalCargoCapacity += s.CargoCapacity
	}

		flagshipType := "fighter"
		if len(ships) > 0 {
			flagshipType = ships[0].ShipType
		}

		comps := domain.PlayerComponents{
			Transform: &domain.Transform{
				X:        x,
				Y:        y,
				Rotation: rotation,
			},
			Velocity: &domain.Velocity{
				X: 0,
				Y: 0,
			},
			Health: &domain.Health{
				Current: ships[0].Health,
				Max:     ships[0].MaxHealth,
			},
			Shield: &domain.Shield{
				Current:   ships[0].Shield,
				Max:       ships[0].MaxShield,
				RegenRate: 1.0,
			},
			Weapon: &domain.Weapon{
				Type:     domain.WeaponLaser,
				Damage:   10,
				Range:    200,
				Cooldown: 1.0,
			},
			Cargo: &domain.Cargo{
				Items:    cargoItems,
				Capacity: totalCargoCapacity,
			},
			ShipConfig: &domain.ShipConfig{
				ShipType: flagshipType,
				MaxSpeed: 80,
				TurnRate: 2.0,
			},
		Fleet: &domain.Fleet{
			Ships: ships,
		},
	}

	return playerData, comps, nil
}

func (r *PostgresPlayerRepository) ClearFleet(ctx context.Context, accountID uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM fleet_ships WHERE owner_id = $1 AND owner_type = 'player'", accountID)
	if err != nil {
		return fmt.Errorf("failed to delete fleet ships: %w", err)
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM item_instances WHERE location_type = 'SHIP_CARGO' AND location_id = $1", accountID)
	if err != nil {
		return fmt.Errorf("failed to delete ship cargo: %w", err)
	}

	_, err = tx.ExecContext(ctx, "UPDATE characters SET x = 0, y = 0, rotation = 0, credits = 1000, system_id = 1 WHERE account_id = $1", accountID)
	if err != nil {
		return fmt.Errorf("failed to update character: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
