package systems

import (
	"context"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// BakeShip takes a ShipConfiguration, resolves its mods/weapons, calculates final stats,
// and attaches the calculated flat components to the ECS entity in the world.
func BakeShip(world *ecs.World, entityID domain.EntityID, config *domain.ShipConfiguration, repo domain.ShipRepository, ctx context.Context) error {
	// 1. Resolve Hull if not already populated
	if config.Hull == nil {
		hull, err := repo.ResolveHull(ctx, config.HullID)
		if err != nil {
			return err
		}
		config.Hull = hull
	}

	// 2. Resolve fitted mods
	mods, err := repo.ResolveHullmods(ctx, config.FittedHullmods)
	if err != nil {
		return err
	}

	// 3. Resolve fitted weapons definitions
	weaponDefs := make(map[string]*domain.WeaponDefinition)
	for slotID, weaponID := range config.FittedWeapons {
		def, err := repo.ResolveWeapon(ctx, weaponID)
		if err != nil {
			return err
		}
		weaponDefs[slotID] = def
	}

	// 4. Calculate stats multipliers
	var speedMult float32 = 1.0
	var turnRateMult float32 = 1.0
	var armorMult float32 = 1.0
	var shieldCapacityMult float32 = 1.0
	var shieldEfficiencyMult float32 = 1.0
	var maxFluxMult float32 = 1.0
	var fluxDissipationMult float32 = 1.0

	for _, mod := range mods {
		if val, ok := mod.Modifiers["max_speed_mult"]; ok {
			speedMult *= val
		}
		if val, ok := mod.Modifiers["turn_rate_mult"]; ok {
			turnRateMult *= val
		}
		if val, ok := mod.Modifiers["armor_mult"]; ok {
			armorMult *= val
		}
		if val, ok := mod.Modifiers["shield_max_mult"]; ok {
			shieldCapacityMult *= val
		}
		if val, ok := mod.Modifiers["shield_efficiency_mult"]; ok {
			shieldEfficiencyMult *= val
		}
		if val, ok := mod.Modifiers["max_flux_mult"]; ok {
			maxFluxMult *= val
		}
		if val, ok := mod.Modifiers["flux_dissipation_mult"]; ok {
			fluxDissipationMult *= val
		}
	}

	// 5. Calculate final values with Vents and Capacitors
	// Capacitors: each increases Max Flux by 200 units
	// Vents: each increases Flux Dissipation by 10 units
	baseMaxFlux := float32(config.Hull.BaseHP * 10) // Base flux capacity formula
	maxFlux := (baseMaxFlux + float32(config.Capacitors)*200.0) * maxFluxMult
	fluxDissipation := (float32(config.Hull.OrdnancePoints)/2.0 + float32(config.Vents)*10.0) * fluxDissipationMult

	maxSpeed := config.Hull.BaseMaxSpeed * speedMult
	turnRate := config.Hull.BaseTurnRate * turnRateMult
	maxHP := config.Hull.BaseHP
	maxArmor := config.Hull.BaseArmor * armorMult
	maxShield := config.Hull.BaseShieldMax * shieldCapacityMult

	// 6. Register entity and attach flat components
	if config.OwnerType == "player" {
		world.RegisterEntityWithID(entityID, domain.EntityPlayer)
	} else {
		world.RegisterEntityWithID(entityID, domain.EntityNPC)
	}

	// Attach basic movement components
	world.AddComponent(entityID, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(entityID, &domain.Velocity{X: 0, Y: 0})

	// Attach calculated battle stats
	world.AddComponent(entityID, &domain.Health{
		Current: int32(maxHP),
		Max:     int32(maxHP),
	})

	if config.Hull.ShieldType != "none" {
		eff := config.Hull.ShieldEfficiency * shieldEfficiencyMult
		if eff <= 0 {
			eff = 1.0
		}
		world.AddComponent(entityID, &domain.Shield{
			Current:    int32(maxShield),
			Max:        int32(maxShield),
			RegenRate:  maxShield * 0.05, // 5% / sec
			Type:       config.Hull.ShieldType,
			Arc:        config.Hull.ShieldArc,
			Efficiency: eff,
		})
	}

	// Armor grid (Phase 1): sits between shields and hull, does not regen in combat.
	world.AddComponent(entityID, &domain.ArmorGrid{
		Current: maxArmor,
		Max:     maxArmor,
	})

	world.AddComponent(entityID, &domain.ShipConfig{
		ShipType: config.Hull.HullID,
		MaxSpeed: maxSpeed,
		TurnRate: turnRate,
	})

	// Attach Starsector-specific FluxState component
	world.AddComponent(entityID, &domain.FluxState{
		Current:         0.0,
		Capacity:        maxFlux,
		DissipationRate: fluxDissipation,
	})

	// Attach weapon group component
	var weaponsList []domain.FittedWeaponState
	for _, slot := range config.Hull.WeaponSlots {
		if _, fitted := config.FittedWeapons[slot.SlotID]; fitted {
			if def, resolved := weaponDefs[slot.SlotID]; resolved {
				weaponsList = append(weaponsList, domain.FittedWeaponState{
					SlotID:     slot.SlotID,
					Definition: *def,
					Cooldown:   0.0,
					Ammo:       9999, // default infinite or config ammo
				})
			}
		}
	}
	world.AddComponent(entityID, &domain.WeaponGroup{
		Weapons: weaponsList,
	})

	return nil
}
