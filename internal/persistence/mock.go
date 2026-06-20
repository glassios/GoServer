package persistence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type InMemoryPlayerRepository struct {
	mutex      sync.RWMutex
	players    map[uint64]*domain.PlayerData
	components map[uint64]domain.PlayerComponents
}

func NewInMemoryPlayerRepository() *InMemoryPlayerRepository {
	return &InMemoryPlayerRepository{
		players:    make(map[uint64]*domain.PlayerData),
		components: make(map[uint64]domain.PlayerComponents),
	}
}

func (r *InMemoryPlayerRepository) Save(ctx context.Context, player *domain.PlayerData, comps domain.PlayerComponents) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.players[player.AccountID] = player
	r.components[player.AccountID] = comps
	return nil
}

func (r *InMemoryPlayerRepository) Load(ctx context.Context, accountID uint64) (*domain.PlayerData, domain.PlayerComponents, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	player, exists := r.players[accountID]
	if !exists {
		// Return defaults for first-time login
		pData := &domain.PlayerData{
			AccountID: accountID,
			Name:      "Explorer",
			Credits:   1000,
		}
		comps := domain.PlayerComponents{
			Transform: &domain.Transform{X: 0, Y: 0, Rotation: 0},
			Velocity:  &domain.Velocity{X: 0, Y: 0},
			Health:    &domain.Health{Current: 100, Max: 100},
			Shield:    &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0},
			Weapon:    &domain.Weapon{Type: domain.WeaponLaser, Damage: 10, Range: 200, Cooldown: 1.0},
			Cargo:     &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 100},
			ShipConfig: &domain.ShipConfig{ShipType: "interceptor", MaxSpeed: 80},
		}
		return pData, comps, nil
	}

	return player, r.components[accountID], nil
}

func (r *InMemoryPlayerRepository) ClearFleet(ctx context.Context, accountID uint64) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if comps, exists := r.components[accountID]; exists {
		comps.Fleet = nil
		if comps.Cargo != nil {
			comps.Cargo.Items = nil
		}
		if comps.Transform != nil {
			comps.Transform.X = 0
			comps.Transform.Y = 0
			comps.Transform.Rotation = 0
		}
		r.components[accountID] = comps
	}
	if pData, exists := r.players[accountID]; exists {
		pData.Credits = 1000
	}
	return nil
}

type InMemorySessionCache struct {
	mutex    sync.RWMutex
	sessions map[string]uint64
}

func NewInMemorySessionCache() *InMemorySessionCache {
	return &InMemorySessionCache{
		sessions: make(map[string]uint64),
	}
}

func (c *InMemorySessionCache) Set(ctx context.Context, sessionID string, accountID uint64, ttl time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.sessions[sessionID] = accountID
	return nil
}

func (c *InMemorySessionCache) Get(ctx context.Context, sessionID string) (uint64, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	acctID, exists := c.sessions[sessionID]
	if !exists {
		return 0, domain.ErrSessionExpired
	}
	return acctID, nil
}

func (c *InMemorySessionCache) Delete(ctx context.Context, sessionID string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.sessions, sessionID)
	return nil
}

type InMemoryCorporationRepository struct {
	mutex        sync.RWMutex
	corps        map[uint32]*domain.Corporation
	corpMembers  map[uint32]map[uint64]string // corpID -> accountID -> role
	memberToCorp map[uint64]uint32            // accountID -> corpID
	nextID       uint32
}

func NewInMemoryCorporationRepository() *InMemoryCorporationRepository {
	return &InMemoryCorporationRepository{
		corps:        make(map[uint32]*domain.Corporation),
		corpMembers:  make(map[uint32]map[uint64]string),
		memberToCorp: make(map[uint64]uint32),
		nextID:       1,
	}
}

func (r *InMemoryCorporationRepository) Create(ctx context.Context, name string, founderID uint64) (*domain.Corporation, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check unique name
	for _, c := range r.corps {
		if c.Name == name {
			return nil, fmt.Errorf("corporation name already exists: %s", name)
		}
	}

	id := r.nextID
	r.nextID++

	corp := &domain.Corporation{
		ID:        id,
		Name:      name,
		Wallet:    0,
		FounderID: founderID,
	}

	r.corps[id] = corp
	r.corpMembers[id] = make(map[uint64]string)
	r.corpMembers[id][founderID] = "Owner"
	r.memberToCorp[founderID] = id

	return corp, nil
}

func (r *InMemoryCorporationRepository) Get(ctx context.Context, corpID uint32) (*domain.Corporation, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	c, ok := r.corps[corpID]
	if !ok {
		return nil, nil
	}
	// return copy
	return &domain.Corporation{ID: c.ID, Name: c.Name, Wallet: c.Wallet, FounderID: c.FounderID}, nil
}

func (r *InMemoryCorporationRepository) GetByName(ctx context.Context, name string) (*domain.Corporation, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, c := range r.corps {
		if c.Name == name {
			return &domain.Corporation{ID: c.ID, Name: c.Name, Wallet: c.Wallet, FounderID: c.FounderID}, nil
		}
	}
	return nil, nil
}

func (r *InMemoryCorporationRepository) AddMember(ctx context.Context, corpID uint32, accountID uint64, role string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if already in another corp
	oldCorpID, inCorp := r.memberToCorp[accountID]
	if inCorp {
		delete(r.corpMembers[oldCorpID], accountID)
	}

	if _, ok := r.corpMembers[corpID]; !ok {
		r.corpMembers[corpID] = make(map[uint64]string)
	}
	r.corpMembers[corpID][accountID] = role
	r.memberToCorp[accountID] = corpID
	return nil
}

func (r *InMemoryCorporationRepository) RemoveMember(ctx context.Context, accountID uint64) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	corpID, inCorp := r.memberToCorp[accountID]
	if inCorp {
		delete(r.corpMembers[corpID], accountID)
		delete(r.memberToCorp, accountID)
	}
	return nil
}

func (r *InMemoryCorporationRepository) GetMemberRole(ctx context.Context, accountID uint64) (uint32, string, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	corpID, inCorp := r.memberToCorp[accountID]
	if !inCorp {
		return 0, "", nil
	}
	role := r.corpMembers[corpID][accountID]
	return corpID, role, nil
}

func (r *InMemoryCorporationRepository) GetMembers(ctx context.Context, corpID uint32) (map[uint64]string, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	members, ok := r.corpMembers[corpID]
	if !ok {
		return make(map[uint64]string), nil
	}

	res := make(map[uint64]string, len(members))
	for k, v := range members {
		res[k] = v
	}
	return res, nil
}

func (r *InMemoryCorporationRepository) UpdateWallet(ctx context.Context, corpID uint32, amount int64) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	corp, ok := r.corps[corpID]
	if !ok {
		return 0, fmt.Errorf("corporation not found")
	}

	corp.Wallet += amount
	return corp.Wallet, nil
}
