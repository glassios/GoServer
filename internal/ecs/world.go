package ecs

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type ComponentMask uint64

var (
	registryMutex     sync.RWMutex
	componentRegistry = make(map[reflect.Type]uint8)
	registryCounter   uint32
)

// RegisterComponent registers a component type and returns its bit index (0-63).
func RegisterComponent(val interface{}) uint8 {
	t := reflect.TypeOf(val)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	registryMutex.RLock()
	id, exists := componentRegistry[t]
	registryMutex.RUnlock()
	if exists {
		return id
	}

	registryMutex.Lock()
	defer registryMutex.Unlock()
	// Double-check under write lock
	if id, exists = componentRegistry[t]; exists {
		return id
	}

	currentID := atomic.AddUint32(&registryCounter, 1) - 1
	if currentID >= 64 {
		panic("ECS supports at most 64 unique component types")
	}
	componentRegistry[t] = uint8(currentID)
	return uint8(currentID)
}

// GetComponentBit returns the bitmask for a component.
func GetComponentBit(val interface{}) uint64 {
	return 1 << RegisterComponent(val)
}

// GetComponentBitByType returns the bitmask for a component by reflect.Type.
func GetComponentBitByType(t reflect.Type) uint64 {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	registryMutex.RLock()
	id, exists := componentRegistry[t]
	registryMutex.RUnlock()
	if exists {
		return 1 << id
	}

	registryMutex.Lock()
	defer registryMutex.Unlock()
	if id, exists = componentRegistry[t]; exists {
		return 1 << id
	}

	currentID := atomic.AddUint32(&registryCounter, 1) - 1
	if currentID >= 64 {
		panic("ECS supports at most 64 unique component types")
	}
	componentRegistry[t] = uint8(currentID)
	return 1 << currentID
}

type World struct {
	nextEntityID   uint64
	entities       map[domain.EntityID]domain.EntityType
	entityMasks    map[domain.EntityID]ComponentMask
	componentPools map[reflect.Type]map[domain.EntityID]interface{}
	mutex          sync.RWMutex
}

func NewWorld() *World {
	return &World{
		nextEntityID:   0,
		entities:       make(map[domain.EntityID]domain.EntityType),
		entityMasks:    make(map[domain.EntityID]ComponentMask),
		componentPools: make(map[reflect.Type]map[domain.EntityID]interface{}),
	}
}

func (w *World) CreateEntity(entityType domain.EntityType) domain.EntityID {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id := domain.EntityID(atomic.AddUint64(&w.nextEntityID, 1))
	w.entities[id] = entityType
	w.entityMasks[id] = 0
	return id
}

func (w *World) RegisterEntityWithID(id domain.EntityID, entityType domain.EntityType) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.entities[id] = entityType
	w.entityMasks[id] = 0
	if uint64(id) > atomic.LoadUint64(&w.nextEntityID) {
		atomic.StoreUint64(&w.nextEntityID, uint64(id))
	}
}

func (w *World) DestroyEntity(id domain.EntityID) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	delete(w.entities, id)
	delete(w.entityMasks, id)

	for _, pool := range w.componentPools {
		delete(pool, id)
	}
}

func (w *World) AddComponent(id domain.EntityID, comp interface{}) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if _, exists := w.entities[id]; !exists {
		return
	}

	t := reflect.TypeOf(comp)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pool, exists := w.componentPools[t]
	if !exists {
		pool = make(map[domain.EntityID]interface{})
		w.componentPools[t] = pool
	}

	pool[id] = comp
	bit := GetComponentBitByType(t)
	w.entityMasks[id] |= ComponentMask(bit)
}

func (w *World) GetComponent(id domain.EntityID, compVal interface{}) (interface{}, bool) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	t := reflect.TypeOf(compVal)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pool, exists := w.componentPools[t]
	if !exists {
		return nil, false
	}

	comp, found := pool[id]
	return comp, found
}

func (w *World) RemoveComponent(id domain.EntityID, compVal interface{}) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	t := reflect.TypeOf(compVal)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pool, exists := w.componentPools[t]
	if !exists {
		return
	}

	delete(pool, id)
	bit := GetComponentBitByType(t)
	w.entityMasks[id] &= ^ComponentMask(bit)
}

func (w *World) Query(mask ComponentMask) []domain.EntityID {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	var result []domain.EntityID
	for id, entityMask := range w.entityMasks {
		if (entityMask & mask) == mask {
			result = append(result, id)
		}
	}
	return result
}

func (w *World) EntityCount() int {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return len(w.entities)
}

// GetEntityType returns the type of the given entity.
func (w *World) GetEntityType(id domain.EntityID) (domain.EntityType, bool) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	t, exists := w.entities[id]
	return t, exists
}
