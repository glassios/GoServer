package ecs

type System interface {
	Name() string
	Update(world *World, dt float64)
	Priority() int // Higher priority runs first
}
