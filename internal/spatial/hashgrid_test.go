package spatial

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
)

func TestHashGrid_CRUDAndQueries(t *testing.T) {
	grid := NewHashGrid(100.0) // Cell size 100

	e1 := domain.EntityID(1)
	e2 := domain.EntityID(2)
	e3 := domain.EntityID(3)

	grid.Insert(e1, 50, 50)     // Cell (0, 0)
	grid.Insert(e2, 120, 50)    // Cell (1, 0)
	grid.Insert(e3, -50, -50)   // Cell (-1, -1)

	// Test Query Radius centered at (0, 0)
	res := grid.QueryRadius(0, 0, 80) // Should overlap cell (0, 0), cell (-1, -1), cell (-1, 0), etc.
	// Check that we got e1 and e3, but not e2
	found1, found2, found3 := false, false, false
	for _, id := range res {
		if id == e1 {
			found1 = true
		}
		if id == e2 {
			found2 = true
		}
		if id == e3 {
			found3 = true
		}
	}
	if !found1 {
		t.Error("expected to find e1 (50, 50)")
	}
	if found2 {
		t.Error("did not expect to find e2 (120, 50)")
	}
	if !found3 {
		t.Error("expected to find e3 (-50, -50)")
	}

	// Test Update
	grid.Update(e2, 80, 50) // Move e2 into Cell (0, 0)
	res2 := grid.QueryRadius(0, 0, 80)
	found2 = false
	for _, id := range res2 {
		if id == e2 {
			found2 = true
		}
	}
	if !found2 {
		t.Error("expected to find e2 after update to (80, 50)")
	}

	// Test Remove
	grid.Remove(e1)
	res3 := grid.QueryRadius(0, 0, 80)
	found1 = false
	for _, id := range res3 {
		if id == e1 {
			found1 = true
		}
	}
	if found1 {
		t.Error("did not expect to find e1 after removal")
	}
}

func TestHashGrid_EdgeCases(t *testing.T) {
	grid := NewHashGrid(100.0)

	e1 := domain.EntityID(1)
	// Place entity EXACTLY on the cell boundary (100.0, 0.0)
	grid.Insert(e1, 100.0, 0.0) // Floor(100/100) = 1, Floor(0) = 0 -> Cell (1, 0)

	res := grid.QueryRadius(99.0, 0.0, 5.0) // Cell overlaps (0, 0) and (1, 0)
	found := false
	for _, id := range res {
		if id == e1 {
			found = true
		}
	}
	if !found {
		t.Error("expected to find entity on cell boundary")
	}
}

func BenchmarkHashGrid_QueryRadius_10000(b *testing.B) {
	grid := NewHashGrid(100.0)
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Insert 10,000 entities randomly in a 5000x5000 world
	for i := 1; i <= 10000; i++ {
		x := (random.Float32() - 0.5) * 5000
		y := (random.Float32() - 0.5) * 5000
		grid.Insert(domain.EntityID(i), x, y)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Query radius 300 at origin
		res := grid.QueryRadius(0, 0, 300)
		_ = res
	}
}
