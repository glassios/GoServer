package systems

import (
	"math"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type MovementSystem struct {
	worldWidth  float32
	worldHeight float32
}

func NewMovementSystem(width, height float32) *MovementSystem {
	return &MovementSystem{
		worldWidth:  width,
		worldHeight: height,
	}
}

func (s *MovementSystem) Name() string {
	return "MovementSystem"
}

func (s *MovementSystem) Priority() int {
	return 100 // High priority, runs early
}

func (s *MovementSystem) Update(world *ecs.World, dt float64) {
	mask := ecs.BuildMask(domain.Transform{}, domain.Velocity{})
	entities := world.Query(mask)

	halfW := s.worldWidth / 2
	halfH := s.worldHeight / 2

	for _, id := range entities {
		tVal, foundT := world.GetComponent(id, domain.Transform{})
		vVal, foundV := world.GetComponent(id, domain.Velocity{})
		if !foundT || !foundV {
			continue
		}

		// Пропускаем движение на карте мира, если флот находится в бою
		if _, inCombat := world.GetComponent(id, domain.CombatState{}); inCombat {
			continue
		}

		trans := tVal.(*domain.Transform)
		vel := vVal.(*domain.Velocity)

		// В боевом инстансе (есть CombatTeam) движением управляет AISystem,
		// который держит дистанцию ведения огня. Здесь мы НЕ перехватываем
		// движение к цели, иначе корабль подлетает вплотную (dist<=25) и
		// игнорирует боевую дистанцию.
		_, inCombatInstance := world.GetComponent(id, domain.CombatTeam{})

		// Если у сущности есть активное оружие с целью на карте мира,
		// направляем ее движение к этой цели (только вне боевого инстанса).
		if wVal, hasWeapon := world.GetComponent(id, domain.Weapon{}); hasWeapon && !inCombatInstance {
			weapon := wVal.(*domain.Weapon)
			if weapon.Active && weapon.TargetID != 0 {
				targetTVal, foundTargetT := world.GetComponent(weapon.TargetID, domain.Transform{})
				if foundTargetT {
					targetTrans := targetTVal.(*domain.Transform)
					dx := targetTrans.X - trans.X
					dy := targetTrans.Y - trans.Y
					dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

					if dist > 25.0 { // Чуть меньше engageRadius (30.0), чтобы надежно войти в бой
						maxSpeed := float32(50.0) // Дефолтная скорость
						if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
							maxSpeed = cfgVal.(*domain.ShipConfig).MaxSpeed
						}
						vel.X = (dx / dist) * maxSpeed
						vel.Y = (dy / dist) * maxSpeed
					} else {
						vel.X = 0
						vel.Y = 0
					}
				} else {
					// Цель больше не существует
					weapon.Active = false
					weapon.TargetID = 0
				}
			}
		}

		trans.X += vel.X * float32(dt)
		trans.Y += vel.Y * float32(dt)

		if vel.X != 0 || vel.Y != 0 {
			trans.Rotation = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}

		// Boundary checks and clamps
		if trans.X < -halfW {
			trans.X = -halfW
			vel.X = 0
		} else if trans.X > halfW {
			trans.X = halfW
			vel.X = 0
		}

		if trans.Y < -halfH {
			trans.Y = -halfH
			vel.Y = 0
		} else if trans.Y > halfH {
			trans.Y = halfH
			vel.Y = 0
		}
	}
}
