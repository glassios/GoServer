package network

import (
	"net"
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

func TestSnapshot_BuildAndDelta(t *testing.T) {
	world := ecs.NewWorld()

	// 1. Create observer player
	player := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(player, &domain.Transform{X: 100, Y: 100, Rotation: 0})
	world.AddComponent(player, &domain.Visibility{Radius: 200, VisibleEntities: make(map[domain.EntityID]struct{})})
	world.AddComponent(player, &domain.PlayerData{Name: "Observer"})

	// 2. Create entity in sight
	eInSight := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(eInSight, &domain.Transform{X: 120, Y: 100})
	world.AddComponent(eInSight, &domain.Health{Current: 80, Max: 100})

	// 3. Create entity out of sight
	eOutSight := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(eOutSight, &domain.Transform{X: 400, Y: 400})

	// 4. Test EntitySnapshot serialization
	snap := BuildEntitySnapshot(world, eInSight)
	if snap == nil {
		t.Fatal("expected non-nil entity snapshot")
	}
	if snap.Hp != 80 || snap.MaxHp != 100 {
		t.Errorf("expected HP 80/100, got %d/%d", snap.Hp, snap.MaxHp)
	}

	// 5. Test DeltaSnapshot
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	session := NewSession(addr)
	session.SetEntityID(player)
	session.SetState(StateInGame)

	// Simulate visibility system filling the visible entities
	visVal, _ := world.GetComponent(player, domain.Visibility{})
	vis := visVal.(*domain.Visibility)
	vis.VisibleEntities[eInSight] = struct{}{}

	// First tick delta snapshot (broad phase initialization)
	delta, ok := BuildDeltaSnapshot(world, session, 1)
	if !ok {
		t.Fatal("expected DeltaSnapshot generation to succeed")
	}
	if len(delta.UpdatedEntities) != 2 { // Player itself + eInSight
		t.Errorf("expected 2 updated entities (player + in-sight NPC), got %d", len(delta.UpdatedEntities))
	}

	// Second tick delta snapshot without changes (only player is always added by design)
	_, ok = BuildDeltaSnapshot(world, session, 2)
	if !ok {
		t.Fatal("expected DeltaSnapshot to succeed on second tick")
	}
	// In our implementation, we send visible entities every time for broadphase, but let's check
	// if previouslyVisible is stored correctly.
	prevVis := session.GetPreviouslyVisible()
	if _, exists := prevVis[eInSight]; !exists {
		t.Error("expected eInSight to be stored in previouslyVisible map")
	}

	// Third tick: eInSight goes out of sight
	delete(vis.VisibleEntities, eInSight)
	delta3, ok := BuildDeltaSnapshot(world, session, 3)
	if !ok {
		t.Fatal("expected DeltaSnapshot to succeed on third tick")
	}

	// It should contain eInSight in destroyed/out-of-sight list
	foundDestroyed := false
	for _, id := range delta3.DestroyedEntities {
		if id == uint64(eInSight) {
			foundDestroyed = true
			break
		}
	}
	if !foundDestroyed {
		t.Error("expected eInSight to be reported in DestroyedEntities list")
	}
}

func BenchmarkSnapshot_BuildDelta_20_Players_100_NPCs(b *testing.B) {
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(100)

	// Create 20 players
	sessions := make([]*Session, 20)
	for i := 0; i < 20; i++ {
		p := world.CreateEntity(domain.EntityPlayer)
		world.AddComponent(p, &domain.Transform{X: float32(i * 10), Y: float32(i * 10)})
		world.AddComponent(p, &domain.Visibility{Radius: 300, VisibleEntities: make(map[domain.EntityID]struct{})})
		world.AddComponent(p, &domain.PlayerData{Name: "Player"})

		addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		sess := NewSession(addr)
		sess.SetEntityID(p)
		sess.SetState(StateInGame)
		sessions[i] = sess
	}

	// Create 100 NPCs
	for i := 0; i < 100; i++ {
		npc := world.CreateEntity(domain.EntityNPC)
		world.AddComponent(npc, &domain.Transform{X: float32(i * 15), Y: float32(i * 15)})
		world.AddComponent(npc, &domain.Health{Current: 100, Max: 100})
		world.AddComponent(npc, &domain.Velocity{X: 1, Y: 1})
	}

	// Simulate one visibility system run to populate interest lists
	tMask := ecs.BuildMask(domain.Transform{})
	tEntities := world.Query(tMask)
	for _, id := range tEntities {
		tVal, _ := world.GetComponent(id, domain.Transform{})
		trans := tVal.(*domain.Transform)
		grid.Update(id, trans.X, trans.Y)
	}

	vMask := ecs.BuildMask(domain.Transform{}, domain.Visibility{})
	vEntities := world.Query(vMask)
	for _, observerID := range vEntities {
		tVal, _ := world.GetComponent(observerID, domain.Transform{})
		visVal, _ := world.GetComponent(observerID, domain.Visibility{})
		trans := tVal.(*domain.Transform)
		vis := visVal.(*domain.Visibility)

		candidates := grid.QueryRadius(trans.X, trans.Y, vis.Radius)
		for _, cand := range candidates {
			if cand != observerID {
				vis.VisibleEntities[cand] = struct{}{}
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, sess := range sessions {
			_, _ = BuildDeltaSnapshot(world, sess, uint64(i))
		}
	}
}
