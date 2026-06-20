package mathutil

import (
	"math"
	"testing"
)

func TestVec2_Operations(t *testing.T) {
	v1 := NewVec2(3, 4)
	v2 := NewVec2(1, 2)

	// Add
	add := v1.Add(v2)
	if add.X != 4 || add.Y != 6 {
		t.Errorf("Add failed: expected (4, 6), got (%f, %f)", add.X, add.Y)
	}

	// Sub
	sub := v1.Sub(v2)
	if sub.X != 2 || sub.Y != 2 {
		t.Errorf("Sub failed: expected (2, 2), got (%f, %f)", sub.X, sub.Y)
	}

	// Mul
	mul := v1.Mul(2)
	if mul.X != 6 || mul.Y != 8 {
		t.Errorf("Mul failed: expected (6, 8), got (%f, %f)", mul.X, mul.Y)
	}

	// Length
	length := v1.Length()
	if length != 5.0 {
		t.Errorf("Length failed: expected 5.0, got %f", length)
	}

	// Normalize
	norm := v1.Normalize()
	if math.Abs(float64(norm.Length()-1.0)) > 1e-6 {
		t.Errorf("Normalize failed: expected length 1.0, got %f", norm.Length())
	}
	if math.Abs(float64(norm.X-0.6)) > 1e-6 || math.Abs(float64(norm.Y-0.8)) > 1e-6 {
		t.Errorf("Normalize values failed: got (%f, %f)", norm.X, norm.Y)
	}

	// Normalize zero vector
	vZero := NewVec2(0, 0)
	normZero := vZero.Normalize()
	if normZero.X != 0 || normZero.Y != 0 {
		t.Errorf("Normalize zero vector failed: got (%f, %f)", normZero.X, normZero.Y)
	}

	// Distance
	dist := v1.Distance(v2)
	expectedDist := float32(math.Sqrt(4 + 4)) // sqrt(8)
	if math.Abs(float64(dist-expectedDist)) > 1e-6 {
		t.Errorf("Distance failed: expected %f, got %f", expectedDist, dist)
	}

	// Dot
	dot := v1.Dot(v2)
	if dot != 11 {
		t.Errorf("Dot failed: expected 11, got %f", dot)
	}

	// Rotate 90 degrees (pi/2)
	v3 := NewVec2(1, 0)
	rot := v3.Rotate(math.Pi / 2)
	if math.Abs(float64(rot.X)) > 1e-6 || math.Abs(float64(rot.Y-1.0)) > 1e-6 {
		t.Errorf("Rotate failed: expected (0, 1), got (%f, %f)", rot.X, rot.Y)
	}
}
