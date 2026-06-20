package ecs

import "reflect"

// BuildMask constructs a ComponentMask from a slice of component instances or reflect.Types.
func BuildMask(components ...interface{}) ComponentMask {
	var mask uint64
	for _, comp := range components {
		if t, ok := comp.(reflect.Type); ok {
			mask |= GetComponentBitByType(t)
		} else {
			mask |= GetComponentBit(comp)
		}
	}
	return ComponentMask(mask)
}
