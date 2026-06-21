package systems

import (
	"math/rand"
	"testing"
)

// rollLootDrop should fire ~10% of the time.
func TestRollLootDrop_Ratio(t *testing.T) {
	m := &InstanceManager{randSource: rand.New(rand.NewSource(42))}
	const n = 100000
	hits := 0
	for i := 0; i < n; i++ {
		if m.rollLootDrop() {
			hits++
		}
	}
	ratio := float64(hits) / float64(n)
	if ratio < 0.085 || ratio > 0.115 {
		t.Fatalf("expected loot drop ratio ~0.10, got %.3f", ratio)
	}
}
