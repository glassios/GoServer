package ecs

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type TestTransform struct {
	X, Y float32
}

type TestVelocity struct {
	X, Y float32
}

func TestECS_CRUD(t *testing.T) {
	world := NewWorld()

	// Create Entity
	e1 := world.CreateEntity(domain.EntityPlayer)
	if e1 != 1 {
		t.Errorf("expected entity ID 1, got %d", e1)
	}

	if world.EntityCount() != 1 {
		t.Errorf("expected 1 entity in world, got %d", world.EntityCount())
	}

	// Add Components
	world.AddComponent(e1, &TestTransform{X: 10, Y: 20})
	world.AddComponent(e1, &TestVelocity{X: 1, Y: 2})

	// Get Components
	cTrans, found := world.GetComponent(e1, TestTransform{})
	if !found {
		t.Fatal("expected to find TestTransform component")
	}
	trans := cTrans.(*TestTransform)
	if trans.X != 10 || trans.Y != 20 {
		t.Errorf("expected TestTransform (10, 20), got (%f, %f)", trans.X, trans.Y)
	}

	cVel, found := world.GetComponent(e1, TestVelocity{})
	if !found {
		t.Fatal("expected to find TestVelocity component")
	}
	vel := cVel.(*TestVelocity)
	if vel.X != 1 || vel.Y != 2 {
		t.Errorf("expected TestVelocity (1, 2), got (%f, %f)", vel.X, vel.Y)
	}

	// Remove Component
	world.RemoveComponent(e1, TestVelocity{})
	_, found = world.GetComponent(e1, TestVelocity{})
	if found {
		t.Error("expected TestVelocity component to be removed")
	}

	// Destroy Entity
	world.DestroyEntity(e1)
	if world.EntityCount() != 0 {
		t.Errorf("expected 0 entities, got %d", world.EntityCount())
	}
	_, found = world.GetComponent(e1, TestTransform{})
	if found {
		t.Error("expected TestTransform to be cleaned up after entity destruction")
	}
}

func TestECS_Query(t *testing.T) {
	world := NewWorld()

	e1 := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(e1, &TestTransform{X: 10, Y: 20})
	world.AddComponent(e1, &TestVelocity{X: 1, Y: 2})

	e2 := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(e2, &TestTransform{X: 30, Y: 40})

	maskTrans := BuildMask(TestTransform{})
	maskTransVel := BuildMask(TestTransform{}, TestVelocity{})
	maskVel := BuildMask(TestVelocity{})

	// Query TestTransform
	resTrans := world.Query(maskTrans)
	if len(resTrans) != 2 {
		t.Errorf("expected 2 entities matching TestTransform, got %d", len(resTrans))
	}

	// Query TestTransform + TestVelocity
	resTransVel := world.Query(maskTransVel)
	if len(resTransVel) != 1 {
		t.Errorf("expected 1 entity matching TestTransform + TestVelocity, got %d", len(resTransVel))
	}
	if resTransVel[0] != e1 {
		t.Errorf("expected entity ID 1, got %d", resTransVel[0])
	}

	// Query only TestVelocity
	resVel := world.Query(maskVel)
	if len(resVel) != 1 {
		t.Errorf("expected 1 entity matching TestVelocity, got %d", len(resVel))
	}
}

func BenchmarkECS_CreateEntity(b *testing.B) {
	world := NewWorld()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.CreateEntity(domain.EntityPlayer)
	}
}

func BenchmarkECS_Query_10000(b *testing.B) {
	world := NewWorld()
	for i := 0; i < 10000; i++ {
		e := world.CreateEntity(domain.EntityPlayer)
		world.AddComponent(e, &TestTransform{X: 1, Y: 2})
		if i%2 == 0 {
			world.AddComponent(e, &TestVelocity{X: 3, Y: 4})
		}
	}

	mask := BuildMask(TestTransform{}, TestVelocity{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := world.Query(mask)
		_ = res
	}
}
