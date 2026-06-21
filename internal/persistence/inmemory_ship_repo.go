package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/Home/galaxy-mmo/internal/domain"
)

// InMemoryShipRepository implements domain.ShipRepository for DB-less mode.
// Hull/weapon/hullmod resolution is served from the canonical code catalog
// (domain.StockHulls/StockWeapons/StockHullmods), so BakeShip behaves identically
// whether or not a database is present. Saved configurations and fleets live in memory.
type InMemoryShipRepository struct {
	mutex   sync.RWMutex
	configs map[uint64]*domain.ShipConfiguration
	fleets  map[string]*domain.CharacterFleet // key: ownerType + ":" + ownerID
	nextID  uint64
}

func NewInMemoryShipRepository() *InMemoryShipRepository {
	return &InMemoryShipRepository{
		configs: make(map[uint64]*domain.ShipConfiguration),
		fleets:  make(map[string]*domain.CharacterFleet),
		nextID:  1,
	}
}

func fleetKey(ownerID uint64, ownerType string) string {
	return fmt.Sprintf("%s:%d", ownerType, ownerID)
}

func cloneConfig(c *domain.ShipConfiguration) *domain.ShipConfiguration {
	cp := *c
	cp.FittedWeapons = make(map[string]string, len(c.FittedWeapons))
	for k, v := range c.FittedWeapons {
		cp.FittedWeapons[k] = v
	}
	cp.FittedHullmods = append([]string(nil), c.FittedHullmods...)
	cp.Hull = nil // resolved on demand
	return &cp
}

func (r *InMemoryShipRepository) SaveConfiguration(ctx context.Context, config *domain.ShipConfiguration) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if config.ID == 0 {
		config.ID = r.nextID
		r.nextID++
	}
	r.configs[config.ID] = cloneConfig(config)
	return nil
}

func (r *InMemoryShipRepository) LoadConfiguration(ctx context.Context, configID uint64) (*domain.ShipConfiguration, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	c, ok := r.configs[configID]
	if !ok {
		return nil, fmt.Errorf("ship configuration not found: %d", configID)
	}
	out := cloneConfig(c)
	out.Hull = domain.HullByNumericID(out.HullID)
	return out, nil
}

func (r *InMemoryShipRepository) SaveFleet(ctx context.Context, fleet *domain.CharacterFleet) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if fleet.ID == 0 {
		fleet.ID = r.nextID
		r.nextID++
	}
	cp := *fleet
	cp.ShipIDs = append([]uint64(nil), fleet.ShipIDs...)
	r.fleets[fleetKey(fleet.OwnerID, fleet.OwnerType)] = &cp
	return nil
}

func (r *InMemoryShipRepository) LoadFleet(ctx context.Context, ownerID uint64, ownerType string) (*domain.CharacterFleet, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	f, ok := r.fleets[fleetKey(ownerID, ownerType)]
	if !ok {
		// Match the Postgres repo: return an empty fleet rather than an error.
		return &domain.CharacterFleet{
			OwnerID:   ownerID,
			OwnerType: ownerType,
			SystemID:  1,
			ShipIDs:   []uint64{},
		}, nil
	}
	cp := *f
	cp.ShipIDs = append([]uint64(nil), f.ShipIDs...)
	return &cp, nil
}

func (r *InMemoryShipRepository) ResolveHull(ctx context.Context, hullID uint32) (*domain.ShipHull, error) {
	if h := domain.HullByNumericID(hullID); h != nil {
		return h, nil
	}
	return nil, fmt.Errorf("ship hull not found: %d", hullID)
}

func (r *InMemoryShipRepository) ResolveWeapon(ctx context.Context, weaponID string) (*domain.WeaponDefinition, error) {
	if w := domain.WeaponByID(weaponID); w != nil {
		return w, nil
	}
	return nil, fmt.Errorf("weapon definition not found: %s", weaponID)
}

func (r *InMemoryShipRepository) ResolveHullmods(ctx context.Context, modIDs []string) (map[string]*domain.Hullmod, error) {
	res := make(map[string]*domain.Hullmod, len(modIDs))
	for _, id := range modIDs {
		if m := domain.HullmodByID(id); m != nil {
			res[m.ModID] = m
		}
	}
	return res, nil
}
