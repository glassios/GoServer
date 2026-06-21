package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
)

// cruiser hull (id 8) has WS1 LARGE BALLISTIC, WS2/WS3 MEDIUM ENERGY, WS4 SMALL MISSILE; OP=200.
func cruiserConfig() *domain.ShipConfiguration {
	return &domain.ShipConfiguration{
		HullID: 8,
		FittedWeapons: map[string]string{
			"WS1": domain.WeaponHellbore,     // LARGE BALLISTIC, 22 OP
			"WS2": domain.WeaponHeavyBlaster, // MEDIUM ENERGY, 12 OP
		},
		Vents:      5,
		Capacitors: 5,
	}
}

func TestValidateLoadout_Valid(t *testing.T) {
	cfg := cruiserConfig()
	if err := ValidateLoadout(cfg); err != nil {
		t.Fatalf("expected valid loadout, got %v", err)
	}
}

func TestValidateLoadout_WrongType(t *testing.T) {
	cfg := cruiserConfig()
	cfg.FittedWeapons["WS4"] = domain.WeaponHeavyBlaster // ENERGY weapon into a MISSILE slot
	if err := ValidateLoadout(cfg); err == nil {
		t.Fatal("expected type-mismatch rejection")
	}
}

func TestValidateLoadout_OversizeWeapon(t *testing.T) {
	cfg := cruiserConfig()
	cfg.FittedWeapons["WS2"] = domain.WeaponHellbore // LARGE weapon into a MEDIUM slot
	if err := ValidateLoadout(cfg); err == nil {
		t.Fatal("expected oversize rejection")
	}
}

func TestValidateLoadout_SmallerWeaponInLargerSlot(t *testing.T) {
	cfg := cruiserConfig()
	cfg.FittedWeapons["WS1"] = domain.WeaponLightAutocannon // SMALL ballistic in LARGE ballistic slot: ok
	if err := ValidateLoadout(cfg); err != nil {
		t.Fatalf("smaller weapon should fit larger slot, got %v", err)
	}
}

func TestValidateLoadout_OverBudget(t *testing.T) {
	cfg := cruiserConfig()
	cfg.Vents = 500 // 500 OP of vents blows the 200 OP budget
	if err := ValidateLoadout(cfg); err == nil {
		t.Fatal("expected over-budget rejection")
	}
}

func TestValidateLoadout_UnknownHullmod(t *testing.T) {
	cfg := cruiserConfig()
	cfg.FittedHullmods = []string{"nonexistent_mod"}
	if err := ValidateLoadout(cfg); err == nil {
		t.Fatal("expected unknown-hullmod rejection")
	}
}

func TestComputeOP(t *testing.T) {
	cfg := cruiserConfig()
	// Hellbore 22 + HeavyBlaster 12 + 5 vents + 5 caps = 44; total 200.
	used, total := ComputeOP(cfg)
	if used != 44 || total != 200 {
		t.Fatalf("expected 44/200 OP, got %d/%d", used, total)
	}
}

func TestEffectiveConfig_DefaultWhenUncustomized(t *testing.T) {
	fs := domain.FleetShip{ShipType: "fighter"}
	cfg := fs.EffectiveConfig()
	def := domain.DefaultLoadoutForShipType("fighter")
	if len(cfg.FittedWeapons) != len(def.FittedWeapons) {
		t.Fatalf("uncustomized ship should use stock loadout: got %d weapons want %d",
			len(cfg.FittedWeapons), len(def.FittedWeapons))
	}
}

func TestEffectiveConfig_CustomLoadout(t *testing.T) {
	fs := domain.FleetShip{
		ShipType:      "cruiser",
		Customized:    true,
		HullID:        8,
		FittedWeapons: map[string]string{"WS1": domain.WeaponHellbore, "WS2": ""},
		Vents:         3,
		Capacitors:    7,
	}
	cfg := fs.EffectiveConfig()
	if cfg.HullID != 8 || cfg.Vents != 3 || cfg.Capacitors != 7 {
		t.Fatalf("custom loadout not carried: %+v", cfg)
	}
	if _, ok := cfg.FittedWeapons["WS2"]; ok {
		t.Error("empty slot should be dropped from effective config")
	}
	if cfg.FittedWeapons["WS1"] != domain.WeaponHellbore {
		t.Error("fitted weapon not carried")
	}
}
