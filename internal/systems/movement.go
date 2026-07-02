package systems

import (
	"math"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type MovementSystem struct {
	worldWidth  float32
	worldHeight float32
	grid        *spatial.HashGrid
}

func NewMovementSystem(width, height float32) *MovementSystem {
	return &MovementSystem{
		worldWidth:  width,
		worldHeight: height,
	}
}

// SetGrid wires the spatial grid so moved entities keep their grid cell up to date.
// Without it the grid only reflects spawn positions and neighbour queries go stale.
func (s *MovementSystem) SetGrid(grid *spatial.HashGrid) { s.grid = grid }

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

		// Follow/escort order: steer toward the target and hold a standoff distance.
		// Takes precedence over raw velocity; a manual move clears the order (see gameloop).
		if foVal, hasFollow := world.GetComponent(id, domain.FollowOrder{}); hasFollow && !inCombatInstance {
			fo := foVal.(*domain.FollowOrder)
			targetTVal, foundTargetT := world.GetComponent(fo.TargetID, domain.Transform{})
			if fo.TargetID == 0 || !foundTargetT {
				world.RemoveComponent(id, domain.FollowOrder{})
				vel.X = 0
				vel.Y = 0
			} else {
				targetTrans := targetTVal.(*domain.Transform)
				dx := targetTrans.X - trans.X
				dy := targetTrans.Y - trans.Y
				dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

				maxSpeed := float32(50.0)
				if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
					maxSpeed = cfgVal.(*domain.ShipConfig).MaxSpeed
				}
				switch {
				case dist > fo.StandoffMax: // far: full speed
					vel.X = (dx / dist) * maxSpeed
					vel.Y = (dy / dist) * maxSpeed
				case dist > fo.StandoffMin: // medium: ease in at half speed
					vel.X = (dx / dist) * maxSpeed * 0.5
					vel.Y = (dy / dist) * maxSpeed * 0.5
				default: // close enough: hold position
					vel.X = 0
					vel.Y = 0
				}
			}
		}

		// Engine subsystem hit (B4): a missile strike cuts thrust for a few seconds, so the ship
		// drifts at reduced speed. AISystem re-sets velocity each tick, so scaling here holds steady.
		if ssVal, ok := world.GetComponent(id, domain.SubsystemState{}); ok {
			if ssVal.(*domain.SubsystemState).EngineHitTimer > 0 {
				vel.X *= 0.45
				vel.Y *= 0.45
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

		// Keep the spatial index in sync with the new position (no-op if cell unchanged).
		if s.grid != nil {
			s.grid.Update(id, trans.X, trans.Y)
		}
	}
}
