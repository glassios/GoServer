package systems

import (
	"math"
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestMovementSystem_Rotation(t *testing.T) {
	world := ecs.NewWorld()
	sys := NewMovementSystem(1000, 1000)

	entity := world.CreateEntity(domain.EntityPlayer)
	transform := &domain.Transform{X: 0, Y: 0, Rotation: 0}
	velocity := &domain.Velocity{X: 10, Y: 10}

	world.AddComponent(entity, transform)
	world.AddComponent(entity, velocity)

	// Update movement system
	sys.Update(world, 0.1)

	// Check rotation. Atan2(10, 10) should be Pi/4 (approx 0.785398)
	expectedRotation := float32(math.Atan2(10, 10))
	if transform.Rotation != expectedRotation {
		t.Errorf("Expected rotation to be %f, got %f", expectedRotation, transform.Rotation)
	}

	// Change velocity to negative X, positive Y. Atan2(10, -10) = 3*Pi/4
	velocity.X = -10
	velocity.Y = 10
	sys.Update(world, 0.1)

	expectedRotation = float32(math.Atan2(10, -10))
	if transform.Rotation != expectedRotation {
		t.Errorf("Expected rotation to be %f, got %f", expectedRotation, transform.Rotation)
	}

	// Set velocity to zero. Rotation should not change from the last computed value.
	velocity.X = 0
	velocity.Y = 0
	sys.Update(world, 0.1)

	if transform.Rotation != expectedRotation {
		t.Errorf("Expected rotation to remain %f, got %f", expectedRotation, transform.Rotation)
	}
}
