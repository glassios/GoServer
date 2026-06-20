package spatial

import (
	"math"
	"sync"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type cellKey struct {
	x, y int
}

type HashGrid struct {
	cellSize float32
	grid     map[cellKey]map[domain.EntityID]struct{}
	entities map[domain.EntityID]cellKey
	mutex    sync.RWMutex
}

func NewHashGrid(cellSize float32) *HashGrid {
	return &HashGrid{
		cellSize: cellSize,
		grid:     make(map[cellKey]map[domain.EntityID]struct{}),
		entities: make(map[domain.EntityID]cellKey),
	}
}

func (g *HashGrid) getCellKey(x, y float32) cellKey {
	return cellKey{
		x: int(math.Floor(float64(x / g.cellSize))),
		y: int(math.Floor(float64(y / g.cellSize))),
	}
}

func (g *HashGrid) Insert(id domain.EntityID, x, y float32) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	key := g.getCellKey(x, y)
	g.entities[id] = key

	cell, exists := g.grid[key]
	if !exists {
		cell = make(map[domain.EntityID]struct{})
		g.grid[key] = cell
	}
	cell[id] = struct{}{}
}

func (g *HashGrid) Remove(id domain.EntityID) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	key, exists := g.entities[id]
	if !exists {
		return
	}

	delete(g.entities, id)

	if cell, found := g.grid[key]; found {
		delete(cell, id)
		if len(cell) == 0 {
			delete(g.grid, key)
		}
	}
}

func (g *HashGrid) Update(id domain.EntityID, x, y float32) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	newKey := g.getCellKey(x, y)
	oldKey, exists := g.entities[id]

	if exists && oldKey == newKey {
		return
	}

	// Remove from old cell
	if exists {
		if cell, found := g.grid[oldKey]; found {
			delete(cell, id)
			if len(cell) == 0 {
				delete(g.grid, oldKey)
			}
		}
	}

	// Insert into new cell
	g.entities[id] = newKey
	cell, exists := g.grid[newKey]
	if !exists {
		cell = make(map[domain.EntityID]struct{})
		g.grid[newKey] = cell
	}
	cell[id] = struct{}{}
}

func (g *HashGrid) QueryRadius(x, y, radius float32) []domain.EntityID {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	var result []domain.EntityID
	if radius < 0 {
		return result
	}

	minKey := g.getCellKey(x-radius, y-radius)
	maxKey := g.getCellKey(x+radius, y+radius)

	for cx := minKey.x; cx <= maxKey.x; cx++ {
		for cy := minKey.y; cy <= maxKey.y; cy++ {
			cell, exists := g.grid[cellKey{cx, cy}]
			if !exists {
				continue
			}

			// We don't have entity coordinates in the grid directly,
			// but we can query them from an external state or just return all in cell.
			// However, since we want to be exact, we can filter using the caller's check,
			// or just return all candidate entities from the overlapping cells.
			// Returning all candidates from cells is the standard "broadphase" query.
			// Let's return all entity IDs in the cells. The caller will do the exact distance filter.
			for id := range cell {
				result = append(result, id)
			}
		}
	}

	// Note: If exact distance filtering is needed, the caller should retrieve the positions from World
	// and filter. In systems, this is exactly what happens.
	return result
}

func (g *HashGrid) QueryRect(x1, y1, x2, y2 float32) []domain.EntityID {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	var result []domain.EntityID

	// Normalize rect
	minX := float32(math.Min(float64(x1), float64(x2)))
	maxX := float32(math.Max(float64(x1), float64(x2)))
	minY := float32(math.Min(float64(y1), float64(y2)))
	maxY := float32(math.Max(float64(y1), float64(y2)))

	minKey := g.getCellKey(minX, minY)
	maxKey := g.getCellKey(maxX, maxY)

	for cx := minKey.x; cx <= maxKey.x; cx++ {
		for cy := minKey.y; cy <= maxKey.y; cy++ {
			cell, exists := g.grid[cellKey{cx, cy}]
			if !exists {
				continue
			}
			for id := range cell {
				result = append(result, id)
			}
		}
	}

	return result
}

func (g *HashGrid) CellSize() float32 {
	return g.cellSize
}
