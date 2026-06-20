package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type PostgresShipRepository struct {
	db *sql.DB
}

func NewPostgresShipRepository(db *sql.DB) *PostgresShipRepository {
	return &PostgresShipRepository{db: db}
}

func (r *PostgresShipRepository) SaveConfiguration(ctx context.Context, config *domain.ShipConfiguration) error {
	weaponsJSON, err := json.Marshal(config.FittedWeapons)
	if err != nil {
		return fmt.Errorf("failed to marshal fitted weapons: %w", err)
	}

	hullmodsJSON, err := json.Marshal(config.FittedHullmods)
	if err != nil {
		return fmt.Errorf("failed to marshal fitted hullmods: %w", err)
	}

	if config.ID == 0 {
		query := `
			INSERT INTO ship_configurations (owner_id, owner_type, hull_id, custom_name, fitted_weapons, fitted_hullmods, vents, capacitors)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id;
		`
		err = r.db.QueryRowContext(ctx, query,
			config.OwnerID,
			config.OwnerType,
			config.HullID,
			config.CustomName,
			weaponsJSON,
			hullmodsJSON,
			config.Vents,
			config.Capacitors,
		).Scan(&config.ID)
		if err != nil {
			return fmt.Errorf("failed to insert ship configuration: %w", err)
		}
	} else {
		query := `
			UPDATE ship_configurations
			SET owner_id = $1, owner_type = $2, hull_id = $3, custom_name = $4, fitted_weapons = $5, fitted_hullmods = $6, vents = $7, capacitors = $8
			WHERE id = $9;
		`
		_, err = r.db.ExecContext(ctx, query,
			config.OwnerID,
			config.OwnerType,
			config.HullID,
			config.CustomName,
			weaponsJSON,
			hullmodsJSON,
			config.Vents,
			config.Capacitors,
			config.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update ship configuration: %w", err)
		}
	}
	return nil
}

func (r *PostgresShipRepository) LoadConfiguration(ctx context.Context, configID uint64) (*domain.ShipConfiguration, error) {
	query := `
		SELECT id, owner_id, owner_type, hull_id, custom_name, fitted_weapons, fitted_hullmods, vents, capacitors
		FROM ship_configurations
		WHERE id = $1;
	`
	var config domain.ShipConfiguration
	var weaponsBytes, hullmodsBytes []byte

	err := r.db.QueryRowContext(ctx, query, configID).Scan(
		&config.ID,
		&config.OwnerID,
		&config.OwnerType,
		&config.HullID,
		&config.CustomName,
		&weaponsBytes,
		&hullmodsBytes,
		&config.Vents,
		&config.Capacitors,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ship configuration not found: %d", configID)
	} else if err != nil {
		return nil, fmt.Errorf("failed to query ship configuration: %w", err)
	}

	if err := json.Unmarshal(weaponsBytes, &config.FittedWeapons); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fitted weapons: %w", err)
	}
	if err := json.Unmarshal(hullmodsBytes, &config.FittedHullmods); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fitted hullmods: %w", err)
	}

	// Resolve the hull details too
	hull, err := r.ResolveHull(ctx, config.HullID)
	if err == nil {
		config.Hull = hull
	}

	return &config, nil
}

func (r *PostgresShipRepository) SaveFleet(ctx context.Context, fleet *domain.CharacterFleet) error {
	shipIDsJSON, err := json.Marshal(fleet.ShipIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal fleet ship IDs: %w", err)
	}

	query := `
		INSERT INTO character_fleets (owner_id, owner_type, system_id, x, y, ship_ids)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (owner_id, owner_type) DO UPDATE SET
			system_id = EXCLUDED.system_id,
			x = EXCLUDED.x,
			y = EXCLUDED.y,
			ship_ids = EXCLUDED.ship_ids
		RETURNING id;
	`
	err = r.db.QueryRowContext(ctx, query,
		fleet.OwnerID,
		fleet.OwnerType,
		fleet.SystemID,
		fleet.X,
		fleet.Y,
		shipIDsJSON,
	).Scan(&fleet.ID)
	if err != nil {
		return fmt.Errorf("failed to save character fleet: %w", err)
	}
	return nil
}

func (r *PostgresShipRepository) LoadFleet(ctx context.Context, ownerID uint64, ownerType string) (*domain.CharacterFleet, error) {
	query := `
		SELECT id, owner_id, owner_type, system_id, x, y, ship_ids
		FROM character_fleets
		WHERE owner_id = $1 AND owner_type = $2;
	`
	var fleet domain.CharacterFleet
	var shipIDsBytes []byte

	err := r.db.QueryRowContext(ctx, query, ownerID, ownerType).Scan(
		&fleet.ID,
		&fleet.OwnerID,
		&fleet.OwnerType,
		&fleet.SystemID,
		&fleet.X,
		&fleet.Y,
		&shipIDsBytes,
	)
	if err == sql.ErrNoRows {
		// Return empty fleet instead of error
		return &domain.CharacterFleet{
			OwnerID:   ownerID,
			OwnerType: ownerType,
			SystemID:  1,
			ShipIDs:   []uint64{},
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to query character fleet: %w", err)
	}

	if err := json.Unmarshal(shipIDsBytes, &fleet.ShipIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fleet ship IDs: %w", err)
	}

	return &fleet, nil
}

func (r *PostgresShipRepository) ResolveHull(ctx context.Context, hullID uint32) (*domain.ShipHull, error) {
	query := `
		SELECT id, hull_id, name, base_hp, base_armor, base_shield_max, shield_type, shield_arc, shield_efficiency, base_max_speed, base_turn_rate, ordnance_points, weapon_slots
		FROM ship_hulls
		WHERE id = $1;
	`
	var hull domain.ShipHull
	var slotsBytes []byte

	err := r.db.QueryRowContext(ctx, query, hullID).Scan(
		&hull.ID,
		&hull.HullID,
		&hull.Name,
		&hull.BaseHP,
		&hull.BaseArmor,
		&hull.BaseShieldMax,
		&hull.ShieldType,
		&hull.ShieldArc,
		&hull.ShieldEfficiency,
		&hull.BaseMaxSpeed,
		&hull.BaseTurnRate,
		&hull.OrdnancePoints,
		&slotsBytes,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ship hull not found: %d", hullID)
	} else if err != nil {
		return nil, fmt.Errorf("failed to query ship hull: %w", err)
	}

	if err := json.Unmarshal(slotsBytes, &hull.WeaponSlots); err != nil {
		return nil, fmt.Errorf("failed to unmarshal weapon slots: %w", err)
	}

	return &hull, nil
}

func (r *PostgresShipRepository) ResolveWeapon(ctx context.Context, weaponID string) (*domain.WeaponDefinition, error) {
	query := `
		SELECT id, weapon_id, name, weapon_type, weapon_size, op_cost, damage_per_shot, damage_type, flux_cost, range, cooldown
		FROM weapon_definitions
		WHERE weapon_id = $1;
	`
	var weapon domain.WeaponDefinition

	err := r.db.QueryRowContext(ctx, query, weaponID).Scan(
		&weapon.ID,
		&weapon.WeaponID,
		&weapon.Name,
		&weapon.WeaponType,
		&weapon.WeaponSize,
		&weapon.OPCost,
		&weapon.DamagePerShot,
		&weapon.DamageType,
		&weapon.FluxCost,
		&weapon.Range,
		&weapon.Cooldown,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("weapon definition not found: %s", weaponID)
	} else if err != nil {
		return nil, fmt.Errorf("failed to query weapon definition: %w", err)
	}

	return &weapon, nil
}

func (r *PostgresShipRepository) ResolveHullmods(ctx context.Context, modIDs []string) (map[string]*domain.Hullmod, error) {
	if len(modIDs) == 0 {
		return make(map[string]*domain.Hullmod), nil
	}

	query := `
		SELECT id, mod_id, name, op_cost_by_size, modifiers
		FROM hullmods
		WHERE mod_id = ANY($1);
	`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(modIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query hullmods: %w", err)
	}
	defer rows.Close()

	res := make(map[string]*domain.Hullmod)
	for rows.Next() {
		var mod domain.Hullmod
		var opBytes, modBytes []byte
		err := rows.Scan(
			&mod.ID,
			&mod.ModID,
			&mod.Name,
			&opBytes,
			&modBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan hullmod: %w", err)
		}

		if err := json.Unmarshal(opBytes, &mod.OPCostBySize); err != nil {
			return nil, fmt.Errorf("failed to unmarshal op cost by size: %w", err)
		}
		if err := json.Unmarshal(modBytes, &mod.Modifiers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal modifiers: %w", err)
		}

		res[mod.ModID] = &mod
	}

	return res, nil
}
