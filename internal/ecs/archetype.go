package ecs

import (
	"github.com/Home/galaxy-mmo/internal/domain"
)

// Archetype groups entities that share the exact same component layout.
// This is reserved for future memory optimization (data-oriented contiguous layout).
type Archetype struct {
	Mask     ComponentMask
	Entities []domain.EntityID
}

func NewArchetype(mask ComponentMask) *Archetype {
	return &Archetype{
		Mask:     mask,
		Entities: make([]domain.EntityID, 0),
	}
}

func (a *Archetype) AddEntity(id domain.EntityID) {
	a.Entities = append(a.Entities, id)
}

func (a *Archetype) RemoveEntity(id domain.EntityID) {
	for i, entity := range a.Entities {
		if entity == id {
			a.Entities[i] = a.Entities[len(a.Entities)-1]
			a.Entities = a.Entities[:len(a.Entities)-1]
			break
		}
	}
}
