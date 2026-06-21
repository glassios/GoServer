package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/config"
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/gameloop"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/network"
	"github.com/Home/galaxy-mmo/internal/persistence"
	"github.com/Home/galaxy-mmo/internal/persistence/postgres"
	"github.com/Home/galaxy-mmo/internal/spatial"
	"github.com/Home/galaxy-mmo/internal/systems"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

func main() {
	// 1. Logger initialization
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// 2. Parse command line flags
	systemIDFlag := flag.Uint("system-id", 1, "System ID simulated by this node")
	configPath := flag.String("config", "configs/server.yaml", "Path to config file")
	flag.Parse()

	systemID := uint32(*systemIDFlag)
	logger.Info("Galaxy MMO World Node starting...", zap.Uint32("systemID", systemID))

	// 3. Config loading
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Warn("Failed to load config file, using default settings", zap.Error(err))
		cfg = &config.Config{
			Server: config.ServerConfig{Tickrate: 20, MaxPlayers: 100},
			Grid:   config.GridConfig{CellSize: 200, WorldWidth: 10000, WorldHeight: 10000},
			Redis:  config.RedisConfig{Address: "localhost:6379"},
			NATS:   config.NATSConfig{URL: "nats://localhost:4222"},
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Database & Redis Initialization (with fallback to In-Memory)
	var playerRepo domain.PlayerRepository
	var db *sql.DB
	var rClient *redis.Client

	if cfg.Database.DSN != "" {
		db, err = sql.Open("postgres", cfg.Database.DSN)
		if err == nil {
			err = db.PingContext(ctx)
		}
		if err != nil {
			logger.Warn("PostgreSQL is not reachable. Falling back to In-Memory storage.", zap.Error(err))
			db = nil
		}
	}

	if cfg.Redis.Address != "" {
		rClient = redis.NewClient(&redis.Options{
			Addr: cfg.Redis.Address,
		})
		err = rClient.Ping(ctx).Err()
		if err != nil {
			logger.Warn("Redis is not reachable.", zap.Error(err))
			rClient = nil
		}
	}

	// Setup adapters based on availability
	var worldRepo domain.WorldRepository
	if db != nil {
		logger.Info("Connected to PostgreSQL database. Running migrations...")
		err = postgres.RunMigrations(ctx, db, "internal/persistence/postgres/migrations")
		if err != nil {
			logger.Error("Database migration failed", zap.Error(err))
			return
		}
		playerRepo = postgres.NewPostgresPlayerRepository(db)
		worldRepo = postgres.NewPostgresWorldRepository(db)
	} else {
		logger.Info("Using In-Memory Player Repository")
		playerRepo = persistence.NewInMemoryPlayerRepository()
	}

	var corpRepo domain.CorporationRepository
	if db != nil {
		corpRepo = postgres.NewPostgresCorporationRepository(db)
	} else {
		corpRepo = persistence.NewInMemoryCorporationRepository()
	}

	// Ship fitting repository: serves the Starsector hull/weapon/hullmod catalog.
	// DB-backed when available, otherwise the in-memory catalog (kept in sync via
	// migration 009 and internal/domain/fitting_catalog.go).
	var shipRepo domain.ShipRepository
	if db != nil {
		shipRepo = postgres.NewPostgresShipRepository(db)
	} else {
		shipRepo = persistence.NewInMemoryShipRepository()
	}

	// 5. Connect to Messaging Bus (NATS or Mock)
	var bus messaging.MessageBus
	if cfg.NATS.URL != "" {
		bus, err = messaging.NewNATSMessageBus(cfg.NATS.URL)
		if err != nil {
			logger.Warn("NATS connection failed, falling back to In-Memory MockMessageBus", zap.Error(err))
			bus = messaging.NewMockMessageBus()
		} else {
			logger.Info("Connected to NATS Message Bus", zap.String("url", cfg.NATS.URL))
		}
	} else {
		logger.Info("NATS URL not specified, using In-Memory MockMessageBus")
		bus = messaging.NewMockMessageBus()
	}
	defer bus.Close()

	// 6. ECS & Grid Engine initialization
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(cfg.Grid.CellSize)

	// Create and register systems
	moveSys := systems.NewMovementSystem(cfg.Grid.WorldWidth, cfg.Grid.WorldHeight)
	visSys := systems.NewVisibilitySystem(grid)
	aiSys := systems.NewAISystem(500.0, 50, cfg.Grid.WorldWidth, cfg.Grid.WorldHeight) // Max 50 NPCs
	combatSys := systems.NewCombatSystem(nil)
	miningSys := systems.NewMiningSystem(nil)
	miningSys.SetProgressBus(bus) // push skill/XP updates to mining players (Phase 3)
	refinerySys := systems.NewRefinerySystem()
	shipyardSys := systems.NewShipyardSystem()
	productionSys := systems.NewProductionSystem(bus)
	researchSys := systems.NewResearchSystem(bus)
	econSys := systems.NewEconomySystem()
	lootSys := systems.NewLootSystem(grid)
	cleanupSys := systems.NewCleanupSystem(grid)
	jumpGateSys := systems.NewJumpGateSystem(bus, systemID, logger)

	instanceManager := systems.NewInstanceManager(bus, grid, systemID, playerRepo, shipRepo, logger)
	engagementSys := systems.NewFleetEngagementSystem(instanceManager, grid)

	ecsSystems := []ecs.System{
		moveSys,
		visSys,
		aiSys,
		combatSys,
		miningSys,
		refinerySys,
		shipyardSys,
		productionSys,
		researchSys,
		econSys,
		lootSys,
		cleanupSys,
		jumpGateSys,
		instanceManager,
		engagementSys,
	}

	// Register migration handler to receive players transferred from other system nodes
	_, err = systems.RegisterMigrationHandler(bus, world, grid, systemID, logger)
	if err != nil {
		logger.Fatal("Failed to register migration handler", zap.Error(err))
	}

	// Seed world static entities (Stations, Asteroids, and Jump Gates)
	var npcTemplates []domain.NPCSnapshot
	if worldRepo != nil {
		npcTemplates, err = loadWorldFromDB(context.Background(), world, grid, worldRepo, systemID, logger)
		if err != nil {
			logger.Error("Failed to load world from database, falling back to static seed", zap.Error(err))
			seedStaticWorld(world, grid, systemID)
		} else {
			aiSys.SetNPCTemplates(npcTemplates)
		}
	} else {
		seedStaticWorld(world, grid, systemID)
	}

	// 7. Loop Setup
	loop := gameloop.NewGameLoop(world, ecsSystems, cfg.Server.Tickrate, logger)

	// 7. Subscribe to Player Input commands from Gateway
	inputTopic := fmt.Sprintf("system.%d.input", systemID)
	_, err = bus.Subscribe(inputTopic, func(msg *messaging.Message) {
		var cmd protocol.ServerCommand
		if err := proto.Unmarshal(msg.Data, &cmd); err != nil {
			logger.Error("Failed to unmarshal ServerCommand on WorldNode", zap.Error(err))
			return
		}

		playerID := domain.EntityID(cmd.PlayerId)

		switch cmd.Type {
		case protocol.PacketType_C_AUTH_REQUEST:
			// Spawn player entity if it doesn't exist yet
			if _, exists := world.GetEntityType(playerID); !exists {
				world.RegisterEntityWithID(playerID, domain.EntityPlayer)

				loaded := false
				if playerRepo != nil {
					pData, comps, err := playerRepo.Load(context.Background(), uint64(playerID))
					if err == nil && pData != nil {
						pData.Name = string(cmd.Payload)
						pData.SystemID = systemID
						world.AddComponent(playerID, comps.Transform)
						world.AddComponent(playerID, comps.Velocity)
						world.AddComponent(playerID, comps.Health)
						world.AddComponent(playerID, comps.Shield)
						world.AddComponent(playerID, comps.ShipConfig)
						world.AddComponent(playerID, &domain.Visibility{Radius: 1500.0, VisibleEntities: make(map[domain.EntityID]struct{})})
						world.AddComponent(playerID, pData)
						world.AddComponent(playerID, &domain.FactionMember{FactionID: 2})
						world.AddComponent(playerID, comps.Cargo)
						world.AddComponent(playerID, comps.Weapon)
						world.AddComponent(playerID, &domain.MiningLaser{Power: 5, Range: 300})

						// Если игрок был уничтожен (здоровье <= 0 или пустой флот), даем ему базовый флот и восстанавливаем здоровье
						if comps.Health.Current <= 0 || comps.Fleet == nil || len(comps.Fleet.Ships) == 0 {
							comps.Health.Current = comps.Health.Max
							if comps.Health.Current <= 0 {
								comps.Health.Current = 100
								comps.Health.Max = 100
							}
							comps.Shield.Current = comps.Shield.Max
							if comps.Shield.Current < 0 {
								comps.Shield.Current = 50
								comps.Shield.Max = 50
							}
							comps.Fleet = &domain.Fleet{
								Ships: []domain.FleetShip{
									{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50, CargoCapacity: 100},
									{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 150},
								},
							}
						}

						if comps.Fleet != nil {
							world.AddComponent(playerID, comps.Fleet)
						}
						if comps.Progress != nil {
							world.AddComponent(playerID, comps.Progress)
						} else {
							world.AddComponent(playerID, domain.NewPlayerProgress())
						}
						if comps.Research != nil {
							world.AddComponent(playerID, comps.Research)
						} else {
							world.AddComponent(playerID, domain.NewPlayerResearch())
						}
						grid.Insert(playerID, comps.Transform.X, comps.Transform.Y)
						logger.Info("Loaded player profile from repository", zap.Uint64("playerID", uint64(playerID)), zap.String("name", pData.Name))
						loaded = true
					}
				}

				if !loaded {
					world.AddComponent(playerID, &domain.Transform{X: 0, Y: 0})
					world.AddComponent(playerID, &domain.Velocity{X: 0, Y: 0})
					world.AddComponent(playerID, &domain.Health{Current: 100, Max: 100})
					world.AddComponent(playerID, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})
					world.AddComponent(playerID, &domain.ShipConfig{ShipType: "fighter", MaxSpeed: 100, TurnRate: 1.5})
					world.AddComponent(playerID, &domain.Visibility{Radius: 1500.0, VisibleEntities: make(map[domain.EntityID]struct{})})
					world.AddComponent(playerID, &domain.PlayerData{AccountID: uint64(playerID), Name: string(cmd.Payload), Credits: 1000, SystemID: systemID})
					world.AddComponent(playerID, &domain.FactionMember{FactionID: 2})
					world.AddComponent(playerID, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 250})
					world.AddComponent(playerID, &domain.Weapon{Type: domain.WeaponLaser, Damage: 10, Range: 500, Cooldown: 0.5})
					world.AddComponent(playerID, &domain.MiningLaser{Power: 5, Range: 300})

					ships := []domain.FleetShip{
						{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50, CargoCapacity: 100},
						{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 150},
					}
					world.AddComponent(playerID, &domain.Fleet{Ships: ships})
					world.AddComponent(playerID, domain.NewPlayerProgress())
					world.AddComponent(playerID, domain.NewPlayerResearch())

					grid.Insert(playerID, 0, 0)
					logger.Info("Created fresh player entity", zap.Uint64("playerID", uint64(playerID)), zap.String("name", string(cmd.Payload)))
				}
			}
			// Push the fleet roster (with tactics) to the client on auth/reconnect.
			sendFleetStatus(world, bus, playerID)
			// Push the crafting catalog + any in-flight queue so the production panel can populate.
			sendProductionStatus(world, bus, playerID)
			// Push skill/XP progression so the skills panel can populate.
			systems.PublishPlayerProgress(bus, world, playerID)
			// Push research status so the research panel can populate.
			systems.PublishResearchStatus(bus, world, playerID)

		case protocol.PacketType_C_SET_FLEET_TACTICS:
			var req protocol.SetFleetTactics
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				if flVal, ok := world.GetComponent(playerID, domain.Fleet{}); ok {
					fleet := flVal.(*domain.Fleet)
					for _, ts := range req.Ships {
						for i := range fleet.Ships {
							if fleet.Ships[i].ShipID == ts.ShipId {
								if validCombatRole(ts.Role) {
									fleet.Ships[i].Role = ts.Role
								}
								if validCombatStrategy(ts.Strategy) {
									fleet.Ships[i].Strategy = ts.Strategy
								}
							}
						}
					}
					sendFleetStatus(world, bus, playerID)
					if playerRepo != nil {
						savePlayerNow(world, playerRepo, playerID, logger)
					}
				}
			}

		case protocol.PacketType_C_GET_HANGAR:
			sendHangarData(world, bus, playerID)
			sendFleetStatus(world, bus, playerID)

		case protocol.PacketType_C_FIT_SHIP:
			var req protocol.FitShipRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				if flVal, ok := world.GetComponent(playerID, domain.Fleet{}); ok {
					fleet := flVal.(*domain.Fleet)
					for i := range fleet.Ships {
						if fleet.Ships[i].ShipID != req.ShipId {
							continue
						}
						// Build the proposed loadout on top of the ship's hull and validate it
						// server-side before applying.
						hullID := fleet.Ships[i].HullID
						if hullID == 0 {
							if h := domain.HullByStringID(fleet.Ships[i].ShipType); h != nil {
								hullID = h.ID
							}
						}
						weapons := make(map[string]string, len(req.FittedWeapons))
						for slot, wid := range req.FittedWeapons {
							if wid != "" {
								weapons[slot] = wid
							}
						}
						cfg := &domain.ShipConfiguration{
							HullID:         hullID,
							FittedWeapons:  weapons,
							FittedHullmods: req.FittedHullmods,
							Vents:          req.Vents,
							Capacitors:     req.Capacitors,
						}
						if verr := systems.ValidateLoadout(cfg); verr != nil {
							sendSystemChat(bus, playerID, fmt.Sprintf("Оснастка отклонена: %s", verr.Error()))
							break
						}
						// Phase 4: reconcile crafted weapon modules against cargo (consume newly
						// fitted, return removed) before committing the loadout.
						currentCfg := fleet.Ships[i].EffectiveConfig()
						if cargoVal, hasCargo := world.GetComponent(playerID, domain.Cargo{}); hasCargo {
							if ierr := systems.ApplyFitInventory(cargoVal.(*domain.Cargo), currentCfg, cfg); ierr != nil {
								sendSystemChat(bus, playerID, fmt.Sprintf("Оснастка отклонена: %s", ierr.Error()))
								break
							}
						}
						fleet.Ships[i].Customized = true
						fleet.Ships[i].HullID = hullID
						fleet.Ships[i].FittedWeapons = weapons
						fleet.Ships[i].FittedHullmods = append([]string{}, req.FittedHullmods...)
						fleet.Ships[i].Vents = req.Vents
						fleet.Ships[i].Capacitors = req.Capacitors
						sendFleetStatus(world, bus, playerID)
						sendInventoryUpdate(world, bus, playerID)
						sendHangarData(world, bus, playerID) // refresh owned-module counts
						if playerRepo != nil {
							savePlayerNow(world, playerRepo, playerID, logger)
						}
						break
					}
				}
			}

		case protocol.PacketType_C_CRAFT_RECIPE:
			var req protocol.CraftRecipeRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				if err := systems.TryEnqueueCraft(world, playerID, req.RecipeId); err != nil {
					sendSystemChat(bus, playerID, fmt.Sprintf("Крафт отклонён: %s", err.Error()))
				} else {
					sendProductionStatus(world, bus, playerID)
					sendInventoryUpdate(world, bus, playerID)
				}
			}

		case protocol.PacketType_C_START_RESEARCH:
			var req protocol.StartResearchRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				if err := systems.TryStartResearch(world, playerID, req.ProjectId); err != nil {
					sendSystemChat(bus, playerID, fmt.Sprintf("Исследование отклонено: %s", err.Error()))
				} else {
					systems.PublishResearchStatus(bus, world, playerID)
					sendInventoryUpdate(world, bus, playerID) // credits changed
					if playerRepo != nil {
						savePlayerNow(world, playerRepo, playerID, logger)
					}
				}
			}

		case protocol.PacketType_C_BUILD_BASE:
			// Build a space base at the player's current position (Phase 5).
			tVal, okT := world.GetComponent(playerID, domain.Transform{})
			cargoVal, okC := world.GetComponent(playerID, domain.Cargo{})
			if okT && okC {
				cargo := cargoVal.(*domain.Cargo)
				const costIron, costTitan = 20, 10
				if cargo.GetResourceTypeQuantity("IronPlates") < costIron || cargo.GetResourceTypeQuantity("TitaniumPlates") < costTitan {
					sendSystemChat(bus, playerID, "Недостаточно ресурсов для базы (нужно 20 IronPlates + 10 TitaniumPlates)")
				} else {
					cargo.RemoveResourceTypeQuantity("IronPlates", costIron)
					cargo.RemoveResourceTypeQuantity("TitaniumPlates", costTitan)
					t := tVal.(*domain.Transform)
					var factionID uint32 = 2
					if fVal, ok := world.GetComponent(playerID, domain.FactionMember{}); ok {
						factionID = fVal.(*domain.FactionMember).FactionID
					}
					baseID := world.CreateEntity(domain.EntitySpaceBase)
					world.AddComponent(baseID, &domain.Transform{X: t.X + 60, Y: t.Y})
					world.AddComponent(baseID, &domain.Health{Current: 500, Max: 500})
					world.AddComponent(baseID, &domain.SpaceBase{OwnerID: uint64(playerID), Level: 1})
					world.AddComponent(baseID, &domain.FactionMember{FactionID: factionID})
					grid.Insert(baseID, t.X+60, t.Y)
					sendInventoryUpdate(world, bus, playerID)
					sendSystemChat(bus, playerID, "Звёздная база построена")
					logger.Info("Player built a space base", zap.Uint64("playerID", uint64(playerID)), zap.Uint64("baseID", uint64(baseID)))
				}
			}

		case protocol.PacketType_C_UPGRADE_BASE:
			var req protocol.UpgradeBaseRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				baseID := domain.EntityID(req.BaseId)
				baseVal, okB := world.GetComponent(baseID, domain.SpaceBase{})
				cargoVal, okC := world.GetComponent(playerID, domain.Cargo{})
				if okB && okC {
					base := baseVal.(*domain.SpaceBase)
					cargo := cargoVal.(*domain.Cargo)
					if base.OwnerID != uint64(playerID) {
						sendSystemChat(bus, playerID, "Это не ваша база")
					} else {
						const costIron, costTitan = 10, 5
						if cargo.GetResourceTypeQuantity("IronPlates") < costIron || cargo.GetResourceTypeQuantity("TitaniumPlates") < costTitan {
							sendSystemChat(bus, playerID, "Недостаточно ресурсов для улучшения (нужно 10 IronPlates + 5 TitaniumPlates)")
						} else {
							cargo.RemoveResourceTypeQuantity("IronPlates", costIron)
							cargo.RemoveResourceTypeQuantity("TitaniumPlates", costTitan)
							base.Level++
							if hVal, ok := world.GetComponent(baseID, domain.Health{}); ok {
								h := hVal.(*domain.Health)
								h.Max += 250
								h.Current = h.Max
							}
							sendInventoryUpdate(world, bus, playerID)
							sendSystemChat(bus, playerID, fmt.Sprintf("База улучшена до уровня %d", base.Level))
						}
					}
				}
			}

		case protocol.PacketType_C_JOIN_COMBAT_REQUEST:
			var joinReq protocol.JoinCombatRequest
			if err := proto.Unmarshal(cmd.Payload, &joinReq); err == nil {
				err := instanceManager.JoinCombatInstance(world, joinReq.CombatInstanceId, playerID, domain.EntityID(joinReq.AlignWithFleetId))
				if err != nil {
					logger.Warn("Player failed to join combat instance", zap.Uint64("playerID", uint64(playerID)), zap.Error(err))
					sendSystemChat(bus, playerID, fmt.Sprintf("Ошибка входа в бой: %s", err.Error()))
				} else {
					logger.Info("Player joined combat instance successfully", zap.Uint64("playerID", uint64(playerID)), zap.Uint32("instanceID", joinReq.CombatInstanceId))
				}
			}

		case protocol.PacketType_C_MOVE_INPUT:
			var moveInput protocol.MoveInput
			if err := proto.Unmarshal(cmd.Payload, &moveInput); err == nil {
				loop.EnqueueCommand(gameloop.Command{
					PlayerID: playerID,
					Type:     "move",
					Payload: domain.Velocity{
						X: moveInput.X,
						Y: moveInput.Y,
					},
				})
			}

		case protocol.PacketType_C_SHOOT_INPUT:
			var shootInput protocol.ShootInput
			if err := proto.Unmarshal(cmd.Payload, &shootInput); err == nil {
				loop.EnqueueCommand(gameloop.Command{
					PlayerID: playerID,
					Type:     "shoot",
					Payload: struct {
						Active   bool
						TargetID domain.EntityID
					}{
						Active:   shootInput.Active,
						TargetID: domain.EntityID(shootInput.TargetId),
					},
				})
			}

		case protocol.PacketType_C_MINE_INPUT:
			var mineInput protocol.MineInput
			if err := proto.Unmarshal(cmd.Payload, &mineInput); err == nil {
				loop.EnqueueCommand(gameloop.Command{
					PlayerID: playerID,
					Type:     "mine",
					Payload: struct {
						Active   bool
						TargetID domain.EntityID
					}{
						Active:   mineInput.Active,
						TargetID: domain.EntityID(mineInput.TargetId),
					},
				})
			}

		case protocol.PacketType_C_CREATE_CORP_REQUEST:
			var req protocol.CreateCorpRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				corp, err := corpRepo.Create(ctx, req.Name, cmd.PlayerId)
				var resp protocol.CreateCorpResponse
				if err != nil {
					resp.Success = false
					resp.ErrorMessage = err.Error()
				} else {
					resp.Success = true
					resp.CorpId = corp.ID
					if _, exists := world.GetEntityType(playerID); exists {
						world.AddComponent(playerID, &domain.CorporationMember{
							CorpID: corp.ID,
							Role:   "Owner",
						})
					}
				}
				respPayload, _ := proto.Marshal(&resp)
				packet := &protocol.Packet{
					Type:    protocol.PacketType_S_CREATE_CORP_RESPONSE,
					Payload: respPayload,
				}
				packetData, _ := proto.Marshal(packet)
				_ = bus.Publish(fmt.Sprintf("player.%d.response", cmd.PlayerId), packetData)
			}

		case protocol.PacketType_C_JOIN_CORP_REQUEST:
			var req protocol.JoinCorpRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				err := corpRepo.AddMember(ctx, req.CorpId, cmd.PlayerId, "Member")
				if err == nil {
					if _, exists := world.GetEntityType(playerID); exists {
						world.AddComponent(playerID, &domain.CorporationMember{
							CorpID: req.CorpId,
							Role:   "Member",
						})
					}
				}
			}

		case protocol.PacketType_C_BUY_INPUT:
			var req protocol.BuyInput
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				stationID := domain.EntityID(req.StationId)
				err := systems.ExecuteTrade(world, playerID, stationID, domain.ResourceType(req.Resource), req.Amount, true)
				if err != nil {
					logger.Warn("Failed to execute buy trade", zap.Error(err))
					sendSystemChat(bus, playerID, fmt.Sprintf("Ошибка покупки %d %s: %s", req.Amount, req.Resource, translateTradeError(err)))
				} else {
					logger.Info("Executed buy trade", zap.Uint64("playerID", uint64(playerID)), zap.Uint64("stationID", uint64(stationID)), zap.String("resource", req.Resource), zap.Int32("amount", req.Amount))
					sendInventoryUpdate(world, bus, playerID)
					sendSystemChat(bus, playerID, fmt.Sprintf("Куплено %d %s", req.Amount, req.Resource))
				}
			}

		case protocol.PacketType_C_SELL_INPUT:
			var req protocol.SellInput
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				stationID := domain.EntityID(req.StationId)
				err := systems.ExecuteTrade(world, playerID, stationID, domain.ResourceType(req.Resource), req.Amount, false)
				if err != nil {
					logger.Warn("Failed to execute sell trade", zap.Error(err))
					sendSystemChat(bus, playerID, fmt.Sprintf("Ошибка продажи %d %s: %s", req.Amount, req.Resource, translateTradeError(err)))
				} else {
					logger.Info("Executed sell trade", zap.Uint64("playerID", uint64(playerID)), zap.Uint64("stationID", uint64(stationID)), zap.String("resource", req.Resource), zap.Int32("amount", req.Amount))
					sendInventoryUpdate(world, bus, playerID)
					sendSystemChat(bus, playerID, fmt.Sprintf("Продано %d %s", req.Amount, req.Resource))
				}
			}

		case protocol.PacketType_C_START_REFINE_REQUEST:
			var req protocol.StartRefineRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				stationID := domain.EntityID(req.StationId)
				if refVal, ok := world.GetComponent(stationID, domain.Refinery{}); ok {
					ref := refVal.(*domain.Refinery)
					ref.IsActive = true
					logger.Info("Started refinery on station", zap.Uint64("stationID", uint64(stationID)))
				}
			}

		case protocol.PacketType_C_BUILD_SHIP_REQUEST:
			var req protocol.BuildShipRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				stationID := domain.EntityID(req.StationId)
				syVal, ok1 := world.GetComponent(stationID, domain.Shipyard{})
				cargoVal, ok2 := world.GetComponent(stationID, domain.Cargo{})
				if ok1 && ok2 {
					sy := syVal.(*domain.Shipyard)
					cargo := cargoVal.(*domain.Cargo)

					var costIronPlates int32 = 0
					var costTitaniumPlates int32 = 0
					var costMicrochips int32 = 0
					var costModule string = ""
					var totalTime float32 = 5.0

					if req.ShipType == "fighter" {
						costIronPlates = 10
						costTitaniumPlates = 5
						costMicrochips = 2
						costModule = "Laser Cannon"
					} else if req.ShipType == "miner" {
						costIronPlates = 15
						costTitaniumPlates = 10
						costMicrochips = 2
						costModule = "Mining Laser"
					}

					ironPlates := cargo.GetResourceTypeQuantity("IronPlates")
					titaniumPlates := cargo.GetResourceTypeQuantity("TitaniumPlates")
					microchips := cargo.GetResourceTypeQuantity(domain.ResourceMicrochips)
					moduleQty := cargo.GetResourceTypeQuantity(domain.ResourceType(costModule))

					if ironPlates >= costIronPlates && titaniumPlates >= costTitaniumPlates &&
						microchips >= costMicrochips && moduleQty >= 1 {

						cargo.RemoveResourceTypeQuantity("IronPlates", costIronPlates)
						cargo.RemoveResourceTypeQuantity("TitaniumPlates", costTitaniumPlates)
						cargo.RemoveResourceTypeQuantity(domain.ResourceMicrochips, costMicrochips)
						cargo.RemoveResourceTypeQuantity(domain.ResourceType(costModule), 1)

						sy.Queue = append(sy.Queue, domain.ShipBuildOrder{
							ShipType:  req.ShipType,
							Progress:  0.0,
							TotalTime: totalTime,
							OrderedBy: cmd.PlayerId,
						})
						logger.Info("Queued ship build order", zap.Uint64("stationID", uint64(stationID)), zap.String("shipType", req.ShipType), zap.Uint64("playerID", cmd.PlayerId))
					} else {
						logger.Warn("Insufficient materials for ship build",
							zap.Uint64("stationID", uint64(stationID)),
							zap.Int32("ironPlates", ironPlates),
							zap.Int32("titaniumPlates", titaniumPlates),
							zap.Int32("microchips", microchips),
							zap.Int32("moduleQty", moduleQty),
						)
					}
				}
			}

		case protocol.PacketType_C_VAULT_ACTION:
			var req protocol.VaultAction
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				stationID := domain.EntityID(req.StationId)

				eType, exists := world.GetEntityType(stationID)
				if !exists || eType != domain.EntityStation {
					logger.Warn("Vault action: invalid target station", zap.Uint64("stationID", uint64(stationID)))
					sendSystemChat(bus, playerID, "Ошибка: неверный ID станции")
					return
				}

				tPlayerVal, okP := world.GetComponent(playerID, domain.Transform{})
				tStationVal, okS := world.GetComponent(stationID, domain.Transform{})
				if !okP || !okS {
					sendSystemChat(bus, playerID, "Ошибка: координаты игрока или станции не найдены")
					return
				}
				pPos := tPlayerVal.(*domain.Transform)
				sPos := tStationVal.(*domain.Transform)
				dx := pPos.X - sPos.X
				dy := pPos.Y - sPos.Y

				if dx*dx+dy*dy > 250*250 {
					sendSystemChat(bus, playerID, "Ошибка: вы слишком далеко от станции")
					return
				}

				vPlayerVal, foundPV := world.GetComponent(stationID, domain.StationVaults{})
				vCorpVal, foundCV := world.GetComponent(stationID, domain.CorporationVault{})

				if !foundPV || !foundCV {
					sendSystemChat(bus, playerID, "Ошибка: складские компоненты станции не инициализированы")
					return
				}

				playerVaults := vPlayerVal.(*domain.StationVaults)
				corpVault := vCorpVal.(*domain.CorporationVault)

				var playerCorpID uint32 = 0
				if cMemberVal, ok := world.GetComponent(playerID, domain.CorporationMember{}); ok {
					playerCorpID = cMemberVal.(*domain.CorporationMember).CorpID
				}

				var stationCorpID uint32 = 0
				if ownerVal, ok := world.GetComponent(stationID, domain.StationOwnership{}); ok {
					stationCorpID = ownerVal.(*domain.StationOwnership).CorpID
				}

				if req.VaultType == "corporate" {
					if stationCorpID == 0 || playerCorpID != stationCorpID {
						sendSystemChat(bus, playerID, "Ошибка: доступ к складу корпорации заблокирован (вы не являетесь членом корпорации-владельца станции)")
						return
					}
				}

				cargoVal, foundC := world.GetComponent(playerID, domain.Cargo{})
				if !foundC {
					sendSystemChat(bus, playerID, "Ошибка: трюм игрока не найден")
					return
				}
				playerCargo := cargoVal.(*domain.Cargo)

				var defID uint32
				var resType domain.ResourceType
				if req.ActionType == "deposit" || req.ActionType == "withdraw" {
					resType = domain.ResourceType(req.Resource)
					defID = domain.ResourceToID[resType]
					if defID == 0 {
						sendSystemChat(bus, playerID, "Ошибка: неизвестный тип предмета")
						return
					}
				}
				amount := req.Amount

				if req.ActionType == "deposit" {
					if amount <= 0 {
						sendSystemChat(bus, playerID, "Ошибка: неверное количество")
						return
					}
					playerQty := playerCargo.GetResourceTypeQuantity(resType)
					if playerQty < amount {
						sendSystemChat(bus, playerID, fmt.Sprintf("Ошибка: недостаточно %s в трюме", resType))
						return
					}

					playerCargo.RemoveResourceTypeQuantity(resType, amount)

					if req.VaultType == "personal" {
						if playerVaults.PlayerVaults == nil {
							playerVaults.PlayerVaults = make(map[uint64][]domain.ItemInstance)
						}
						vaultItems := playerVaults.PlayerVaults[uint64(playerID)]
						vaultItems = addItemToSlice(vaultItems, defID, amount)
						playerVaults.PlayerVaults[uint64(playerID)] = vaultItems

						if worldRepo != nil {
							_ = worldRepo.SavePlayerVault(ctx, uint64(playerID), uint64(stationID), vaultItems)
						}
					} else {
						corpVault.Items = addItemToSlice(corpVault.Items, defID, amount)

						if worldRepo != nil {
							_ = worldRepo.SaveCorporationVault(ctx, corpVault.OwnerCorpID, uint64(stationID), corpVault.Items)
						}
					}

					sendSystemChat(bus, playerID, fmt.Sprintf("Внесено %d %s в %s", amount, resType, translateVaultType(req.VaultType)))

				} else if req.ActionType == "withdraw" {
					if amount <= 0 {
						sendSystemChat(bus, playerID, "Ошибка: неверное количество")
						return
					}
					var vaultQty int32 = 0
					if req.VaultType == "personal" {
						if playerVaults.PlayerVaults != nil {
							vaultQty = getQuantityInSlice(playerVaults.PlayerVaults[uint64(playerID)], defID)
						}
					} else {
						vaultQty = getQuantityInSlice(corpVault.Items, defID)
					}

					if vaultQty < amount {
						sendSystemChat(bus, playerID, fmt.Sprintf("Ошибка: недостаточно %s на складе", resType))
						return
					}

					var currentLoad int32 = 0
					for _, item := range playerCargo.Items {
						currentLoad += item.Quantity
					}
					if currentLoad+amount > playerCargo.Capacity {
						sendSystemChat(bus, playerID, "Ошибка: трюм переполнен")
						return
					}

					if req.VaultType == "personal" {
						vaultItems := playerVaults.PlayerVaults[uint64(playerID)]
						vaultItems, _ = removeItemFromSlice(vaultItems, defID, amount)
						playerVaults.PlayerVaults[uint64(playerID)] = vaultItems
						if worldRepo != nil {
							_ = worldRepo.SavePlayerVault(ctx, uint64(playerID), uint64(stationID), vaultItems)
						}
					} else {
						corpVault.Items, _ = removeItemFromSlice(corpVault.Items, defID, amount)
						if worldRepo != nil {
							_ = worldRepo.SaveCorporationVault(ctx, corpVault.OwnerCorpID, uint64(stationID), corpVault.Items)
						}
					}

					playerCargo.AddResourceTypeQuantity(resType, amount)

					sendSystemChat(bus, playerID, fmt.Sprintf("Выведено %d %s из %s", amount, resType, translateVaultType(req.VaultType)))
				}

				// Build personal and corporate items lists for protobuf
				var personalProtos []*protocol.ItemInstanceProto
				if playerVaults.PlayerVaults != nil {
					pItems := playerVaults.PlayerVaults[uint64(playerID)]
					for _, item := range pItems {
						personalProtos = append(personalProtos, &protocol.ItemInstanceProto{
							Id:           item.ID,
							DefinitionId: item.DefinitionID,
							Quantity:     item.Quantity,
							LocationType: "STATION_PERSONAL_VAULT",
							LocationId:   uint64(stationID),
							OwnerId:      uint64(playerID),
							State:        item.State,
							Name:         string(domain.IDToResource[item.DefinitionID]),
						})
					}
				}

				var corpProtos []*protocol.ItemInstanceProto
				for _, item := range corpVault.Items {
					corpProtos = append(corpProtos, &protocol.ItemInstanceProto{
						Id:           item.ID,
						DefinitionId: item.DefinitionID,
						Quantity:     item.Quantity,
						LocationType: "STATION_CORP_VAULT",
						LocationId:   uint64(stationID),
						OwnerId:      uint64(corpVault.OwnerCorpID),
						State:        item.State,
						Name:         string(domain.IDToResource[item.DefinitionID]),
					})
				}

				vaultStatus := &protocol.VaultStatus{
					StationId:      req.StationId,
					PersonalItems:  personalProtos,
					CorporateItems: corpProtos,
				}

				statusPayload, _ := proto.Marshal(vaultStatus)
				statusPacket := &protocol.Packet{
					Type:    protocol.PacketType_S_VAULT_STATUS,
					Payload: statusPayload,
				}
				packetData, _ := proto.Marshal(statusPacket)
				_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)

				if req.ActionType == "deposit" || req.ActionType == "withdraw" {
					sendInventoryUpdate(world, bus, playerID)
				}
			}
		}
	})
	if err != nil {
		logger.Fatal("Failed to subscribe to input topic", zap.String("topic", inputTopic), zap.Error(err))
	}

	// 8. Bind snapshot broadcast to GameLoop
	outputTopic := fmt.Sprintf("system.%d.output", systemID)
	loop.OnSnapshot = func(tick uint64) {
		// Build and serialize a WorldSnapshot containing all active entities in this system
		var entSnaps []*protocol.EntitySnapshot
		allEntities := world.Query(0)
		for _, id := range allEntities {
			snap := network.BuildEntitySnapshot(world, id)
			if snap != nil {
				entSnaps = append(entSnaps, snap)
			}
		}

		worldSnap := &protocol.WorldSnapshot{
			Tick:     tick,
			Entities: entSnaps,
		}

		data, err := proto.Marshal(worldSnap)
		if err != nil {
			logger.Error("Failed to marshal WorldSnapshot", zap.Error(err))
			return
		}

		if err := bus.Publish(outputTopic, data); err != nil {
			logger.Error("Failed to publish WorldSnapshot to NATS", zap.String("topic", outputTopic), zap.Error(err))
		}
	}

	// 9. Start Game Loop
	logger.Info("World node simulation running...")
	go loop.Run(ctx)

	// Wait for termination signal
	<-ctx.Done()
	logger.Info("Termination signal received. Shutting down gracefully...")

	// Graceful Shutdown Sequence
	loop.Stop()

	// Save all active players to DB before quitting
	saveActivePlayers(world, playerRepo, systemID, logger)

	if db != nil {
		db.Close()
	}
	if rClient != nil {
		rClient.Close()
	}

	logger.Info("Server shutdown complete.")
}

func seedStaticWorld(world *ecs.World, grid *spatial.HashGrid, systemID uint32) {
	// Seed Stations
	type stationSeed struct {
		id     domain.EntityID
		name   string
		x, y   float32
		corpID uint32
	}

	var stations []stationSeed
	if systemID == 1 {
		stations = []stationSeed{
			{5001, "Centauri Prime Station", -300, 200, 3},
			{5002, "Sol Mining Outpost", 400, -300, 2},
			{5005, "Pirate Haven", -800, -800, 1},
		}
	} else {
		stations = []stationSeed{
			{5003, "Centauri Prime Station", -300, 200, 3},
			{5004, "Sol Mining Outpost", 400, -300, 2},
		}
	}

	for _, s := range stations {
		e := world.CreateEntity(domain.EntityStation)
		world.AddComponent(e, &domain.Transform{X: s.x, Y: s.y})
		world.AddComponent(e, &domain.StationMarket{
			Items: map[domain.ResourceType]*domain.MarketItem{
				domain.ResourceIron:     {BasePrice: 10, Supply: 100, Demand: 50},
				domain.ResourceTitanium: {BasePrice: 25, Supply: 50, Demand: 50},
				domain.ResourceCrystal:  {BasePrice: 100, Supply: 10, Demand: 50},
				domain.ResourceRareGas:  {BasePrice: 150, Supply: 5, Demand: 50},
			},
		})
		world.AddComponent(e, &domain.Refinery{
			IsActive: false,
			Yield:    1.0,
		})
		world.AddComponent(e, &domain.Shipyard{
			Queue:    []domain.ShipBuildOrder{},
			Progress: 0,
		})
		world.AddComponent(e, &domain.Cargo{
			Items: []domain.ItemInstance{
				{DefinitionID: 1, Quantity: 100, LocationType: "STATION_MARKET", LocationID: uint64(e), State: "normal"},
				{DefinitionID: 2, Quantity: 50, LocationType: "STATION_MARKET", LocationID: uint64(e), State: "normal"},
				{DefinitionID: 3, Quantity: 10, LocationType: "STATION_MARKET", LocationID: uint64(e), State: "normal"},
				{DefinitionID: 4, Quantity: 5, LocationType: "STATION_MARKET", LocationID: uint64(e), State: "normal"},
			},
			Capacity: 1000,
		})
		world.AddComponent(e, &domain.StationOwnership{
			CorpID: s.corpID,
		})
		world.AddComponent(e, &domain.StationVaults{
			PlayerVaults: make(map[uint64][]domain.ItemInstance),
		})
		world.AddComponent(e, &domain.CorporationVault{
			OwnerCorpID: s.corpID,
			Items:       []domain.ItemInstance{},
		})
		grid.Insert(e, s.x, s.y)
	}

	// Seed Asteroids
	asteroids := []struct {
		id   domain.EntityID
		res  domain.ResourceType
		x, y float32
	}{
		{6001, domain.ResourceIron, -150, 100},
		{6002, domain.ResourceIron, -100, 120},
		{6003, domain.ResourceTitanium, 200, -200},
		{6004, domain.ResourceCrystal, 500, 500},
	}

	for _, a := range asteroids {
		e := world.CreateEntity(domain.EntityAsteroid)
		world.AddComponent(e, &domain.Transform{X: a.x, Y: a.y})
		world.AddComponent(e, &domain.AsteroidResource{Type: a.res, Amount: 1000})
		grid.Insert(e, a.x, a.y)
	}

	// Seed Jump Gate
	if systemID == 1 {
		gate := world.CreateEntity(domain.EntityJumpGate)
		world.AddComponent(gate, &domain.Transform{X: 2000, Y: 2000})
		world.AddComponent(gate, &domain.JumpGate{
			TargetSystemID: 2,
			TargetX:        -1800,
			TargetY:        -1800,
		})
		grid.Insert(gate, 2000, 2000)
	} else if systemID == 2 {
		gate := world.CreateEntity(domain.EntityJumpGate)
		world.AddComponent(gate, &domain.Transform{X: -2000, Y: -2000})
		world.AddComponent(gate, &domain.JumpGate{
			TargetSystemID: 1,
			TargetX:        1800,
			TargetY:        1800,
		})
		grid.Insert(gate, -2000, -2000)
	}
}

func loadWorldFromDB(ctx context.Context, world *ecs.World, grid *spatial.HashGrid, repo domain.WorldRepository, systemID uint32, logger *zap.Logger) ([]domain.NPCSnapshot, error) {
	logger.Info("Loading world entities from database...", zap.Uint32("systemID", systemID))
	snapshot, err := repo.LoadWorld(ctx, systemID)
	if err != nil {
		return nil, err
	}

	// 1. Stations
	for _, s := range snapshot.Stations {
		world.RegisterEntityWithID(s.EntityID, domain.EntityStation)
		world.AddComponent(s.EntityID, &domain.Transform{X: s.Transform.X, Y: s.Transform.Y})

		// Set default station properties
		world.AddComponent(s.EntityID, &domain.StationMarket{
			Items: map[domain.ResourceType]*domain.MarketItem{
				domain.ResourceIron:     {BasePrice: 10, Supply: 100, Demand: 50},
				domain.ResourceTitanium: {BasePrice: 25, Supply: 50, Demand: 50},
				domain.ResourceCrystal:  {BasePrice: 100, Supply: 10, Demand: 50},
				domain.ResourceRareGas:  {BasePrice: 150, Supply: 5, Demand: 50},
			},
		})
		world.AddComponent(s.EntityID, &domain.Refinery{IsActive: false, Yield: 1.0})
		world.AddComponent(s.EntityID, &domain.Shipyard{Queue: []domain.ShipBuildOrder{}, Progress: 0})
		// Load cargo items from DB if they exist
		cargoItems := s.Cargo
		if len(cargoItems) == 0 {
			cargoItems = []domain.ItemInstance{
				{DefinitionID: 1, Quantity: 100, LocationType: "STATION_MARKET", LocationID: uint64(s.EntityID), State: "normal"},
				{DefinitionID: 2, Quantity: 50, LocationType: "STATION_MARKET", LocationID: uint64(s.EntityID), State: "normal"},
				{DefinitionID: 3, Quantity: 10, LocationType: "STATION_MARKET", LocationID: uint64(s.EntityID), State: "normal"},
				{DefinitionID: 4, Quantity: 5, LocationType: "STATION_MARKET", LocationID: uint64(s.EntityID), State: "normal"},
			}
		}

		world.AddComponent(s.EntityID, &domain.Cargo{
			Items:    cargoItems,
			Capacity: 10000, // Stations have large market capacity
		})
		world.AddComponent(s.EntityID, &domain.StationOwnership{CorpID: s.FactionID})

		// Add vaults
		world.AddComponent(s.EntityID, &domain.StationVaults{
			PlayerVaults: s.PlayerVaults,
		})
		world.AddComponent(s.EntityID, &domain.CorporationVault{
			OwnerCorpID: s.FactionID,
			Items:       s.CorpVault,
		})

		grid.Insert(s.EntityID, s.Transform.X, s.Transform.Y)
		logger.Debug("Loaded Station from DB", zap.Uint64("id", uint64(s.EntityID)), zap.String("name", s.Name))
	}

	// 2. Asteroids
	for _, a := range snapshot.Asteroids {
		world.RegisterEntityWithID(a.EntityID, domain.EntityAsteroid)
		world.AddComponent(a.EntityID, &domain.Transform{X: a.Transform.X, Y: a.Transform.Y})
		world.AddComponent(a.EntityID, &domain.AsteroidResource{Type: a.Resource, Amount: a.Amount})
		grid.Insert(a.EntityID, a.Transform.X, a.Transform.Y)
		logger.Debug("Loaded Asteroid from DB", zap.Uint64("id", uint64(a.EntityID)), zap.String("resource", string(a.Resource)))
	}

	// 3. Jump Gates
	for _, g := range snapshot.JumpGates {
		world.RegisterEntityWithID(g.EntityID, domain.EntityJumpGate)
		world.AddComponent(g.EntityID, &domain.Transform{X: g.Transform.X, Y: g.Transform.Y})
		world.AddComponent(g.EntityID, &domain.JumpGate{
			TargetSystemID: g.TargetSystemID,
			TargetX:        g.TargetX,
			TargetY:        g.TargetY,
		})
		grid.Insert(g.EntityID, g.Transform.X, g.Transform.Y)
		logger.Debug("Loaded Jump Gate from DB", zap.Uint64("id", uint64(g.EntityID)), zap.Uint32("targetSystem", g.TargetSystemID))
	}

	return snapshot.NPCs, nil
}

func saveActivePlayers(world *ecs.World, repo domain.PlayerRepository, systemID uint32, logger *zap.Logger) {
	logger.Info("Saving active player profiles...")

	mask := ecs.BuildMask(domain.PlayerData{})
	players := world.Query(mask)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, id := range players {
		pVal, _ := world.GetComponent(id, domain.PlayerData{})
		player := pVal.(*domain.PlayerData)
		player.SystemID = systemID

		// Get other player components
		tVal, foundT := world.GetComponent(id, domain.Transform{})
		vVal, foundV := world.GetComponent(id, domain.Velocity{})
		hVal, foundH := world.GetComponent(id, domain.Health{})
		sVal, foundS := world.GetComponent(id, domain.Shield{})
		wVal, foundW := world.GetComponent(id, domain.Weapon{})
		cVal, foundC := world.GetComponent(id, domain.Cargo{})
		cfgVal, foundCfg := world.GetComponent(id, domain.ShipConfig{})
		fVal, foundF := world.GetComponent(id, domain.Fleet{})

		if !foundT || !foundV || !foundH || !foundS || !foundW || !foundC || !foundCfg {
			continue
		}

		comps := domain.PlayerComponents{
			Transform:  tVal.(*domain.Transform),
			Velocity:   vVal.(*domain.Velocity),
			Health:     hVal.(*domain.Health),
			Shield:     sVal.(*domain.Shield),
			Weapon:     wVal.(*domain.Weapon),
			Cargo:      cVal.(*domain.Cargo),
			ShipConfig: cfgVal.(*domain.ShipConfig),
		}
		if foundF {
			comps.Fleet = fVal.(*domain.Fleet)
		}
		if pgVal, foundPg := world.GetComponent(id, domain.PlayerProgress{}); foundPg {
			comps.Progress = pgVal.(*domain.PlayerProgress)
		}
		if rVal, foundR := world.GetComponent(id, domain.PlayerResearch{}); foundR {
			comps.Research = rVal.(*domain.PlayerResearch)
		}

		err := repo.Save(ctx, player, comps)
		if err != nil {
			logger.Error("Failed to save player state on shutdown", zap.Uint64("accountID", player.AccountID), zap.Error(err))
		} else {
			logger.Info("Player state saved successfully", zap.Uint64("accountID", player.AccountID))
		}
	}
}

func translateTradeError(err error) string {
	switch err {
	case domain.ErrInsufficientCredits:
		return "Недостаточно кредитов"
	case domain.ErrCargoFull:
		return "Трюм переполнен"
	case domain.ErrOutOfRange:
		return "Недостаточно товара на станции"
	case domain.ErrInvalidTarget:
		return "Неверная цель"
	default:
		return err.Error()
	}
}

func sendSystemChat(bus messaging.MessageBus, playerID domain.EntityID, message string) {
	chatMsg := &protocol.ChatMessage{
		Sender:  "System",
		Message: message,
	}
	payload, _ := proto.Marshal(chatMsg)
	packet := &protocol.Packet{
		Type:    protocol.PacketType_S_CHAT_MESSAGE,
		Payload: payload,
	}
	packetData, _ := proto.Marshal(packet)
	_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)
}

func validCombatRole(r string) bool {
	switch r {
	case domain.RoleTank, domain.RoleDPS, domain.RoleSupport, domain.RoleRepair:
		return true
	}
	return false
}

func validCombatStrategy(s string) bool {
	switch s {
	case domain.StanceAttack, domain.StanceDefense, domain.StanceRetreat:
		return true
	}
	return false
}

// sendFleetStatus pushes the player's fleet roster (with resolved tactics) to the client so
// the Fleet Tactics panel can be populated. Roles/strategies are run through ResolveTactics so
// the client always shows the concrete values a battle would use.
func sendFleetStatus(world *ecs.World, bus messaging.MessageBus, playerID domain.EntityID) {
	flVal, ok := world.GetComponent(playerID, domain.Fleet{})
	if !ok {
		return
	}
	fleet := flVal.(*domain.Fleet)
	ships := make([]*protocol.FleetStatusShip, 0, len(fleet.Ships))
	for i := range fleet.Ships {
		s := fleet.Ships[i]
		role, strat := domain.ResolveTactics(s.Role, s.Strategy, i)
		cfg := s.EffectiveConfig()
		stats := domain.ComputeStats(cfg)
		opUsed, opTotal := systems.ComputeOP(cfg)
		ships = append(ships, &protocol.FleetStatusShip{
			ShipId:                 s.ShipID,
			ShipType:               s.ShipType,
			Health:                 s.Health,
			MaxHealth:              s.MaxHealth,
			Shield:                 s.Shield,
			MaxShield:              s.MaxShield,
			Role:                   role,
			Strategy:               strat,
			HullId:                 cfg.HullID,
			FittedWeapons:          cfg.FittedWeapons,
			FittedHullmods:         cfg.FittedHullmods,
			Vents:                  cfg.Vents,
			Capacitors:             cfg.Capacitors,
			OpUsed:                 opUsed,
			OpTotal:                opTotal,
			PreviewHp:              stats.MaxHP,
			PreviewArmor:           stats.MaxArmor,
			PreviewShield:          stats.MaxShield,
			PreviewMaxSpeed:        stats.MaxSpeed,
			PreviewMaxFlux:         stats.MaxFlux,
			PreviewFluxDissipation: stats.FluxDissipation,
		})
	}
	status := &protocol.FleetStatus{Ships: ships}
	payload, _ := proto.Marshal(status)
	packet := &protocol.Packet{Type: protocol.PacketType_S_FLEET_STATUS, Payload: payload}
	packetData, _ := proto.Marshal(packet)
	_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)
}

// sendHangarData pushes the full fitting catalog (hulls/weapons/hullmods) to the client so the
// Hangar/Refit screen can offer compatible parts. The catalog is static (code-defined), so this
// is a straightforward projection of the domain Stock* tables.
func sendHangarData(world *ecs.World, bus messaging.MessageBus, playerID domain.EntityID) {
	hulls := make([]*protocol.HullDefProto, 0, len(domain.StockHulls))
	for i := range domain.StockHulls {
		h := &domain.StockHulls[i]
		slots := make([]*protocol.WeaponSlotProto, 0, len(h.WeaponSlots))
		for _, s := range h.WeaponSlots {
			slots = append(slots, &protocol.WeaponSlotProto{
				SlotId: s.SlotID, Size: s.Size, Type: s.Type, Mount: s.Mount,
			})
		}
		hulls = append(hulls, &protocol.HullDefProto{
			Id:             h.ID,
			HullId:         h.HullID,
			Name:           h.Name,
			BaseHp:         h.BaseHP,
			BaseArmor:      h.BaseArmor,
			BaseShieldMax:  h.BaseShieldMax,
			BaseMaxSpeed:   h.BaseMaxSpeed,
			OrdnancePoints: h.OrdnancePoints,
			SizeClass:      systems.HullSizeClass(h),
			Slots:          slots,
		})
	}

	weapons := make([]*protocol.WeaponDefProto, 0, len(domain.StockWeapons))
	for i := range domain.StockWeapons {
		w := &domain.StockWeapons[i]
		weapons = append(weapons, &protocol.WeaponDefProto{
			WeaponId:      w.WeaponID,
			Name:          w.Name,
			WeaponType:    w.WeaponType,
			WeaponSize:    w.WeaponSize,
			OpCost:        w.OPCost,
			DamagePerShot: w.DamagePerShot,
			DamageType:    w.DamageType,
			FluxCost:      w.FluxCost,
			Range:         w.Range,
			Cooldown:      w.Cooldown,
			ModuleItem:    w.ModuleItem,
		})
	}

	hullmods := make([]*protocol.HullmodDefProto, 0, len(domain.StockHullmods))
	for i := range domain.StockHullmods {
		m := &domain.StockHullmods[i]
		hullmods = append(hullmods, &protocol.HullmodDefProto{
			ModId:        m.ModID,
			Name:         m.Name,
			OpCostBySize: m.OPCostBySize,
		})
	}

	// Phase 4: report how many of each crafted module the player currently owns in cargo so the
	// hangar can show counts and gate selection.
	owned := map[string]int32{}
	if cargoVal, ok := world.GetComponent(playerID, domain.Cargo{}); ok {
		cargo := cargoVal.(*domain.Cargo)
		for i := range domain.StockWeapons {
			item := domain.StockWeapons[i].ModuleItem
			if item != "" {
				owned[item] = cargo.GetResourceTypeQuantity(domain.ResourceType(item))
			}
		}
	}

	data := &protocol.HangarData{Hulls: hulls, Weapons: weapons, Hullmods: hullmods, OwnedModules: owned}
	payload, _ := proto.Marshal(data)
	packet := &protocol.Packet{Type: protocol.PacketType_S_HANGAR_DATA, Payload: payload}
	packetData, _ := proto.Marshal(packet)
	_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)
}

// sendProductionStatus pushes the player's craft queue + the recipe catalog to the client so the
// production panel can render the picker and any in-flight jobs.
func sendProductionStatus(world *ecs.World, bus messaging.MessageBus, playerID domain.EntityID) {
	status := systems.BuildProductionStatus(world, playerID)
	payload, _ := proto.Marshal(status)
	packet := &protocol.Packet{Type: protocol.PacketType_S_PRODUCTION_STATUS, Payload: payload}
	packetData, _ := proto.Marshal(packet)
	_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)
}

// savePlayerNow persists a single player's current state immediately (used after a tactics change).
func savePlayerNow(world *ecs.World, repo domain.PlayerRepository, playerID domain.EntityID, logger *zap.Logger) {
	pVal, ok := world.GetComponent(playerID, domain.PlayerData{})
	if !ok {
		return
	}
	player := pVal.(*domain.PlayerData)

	tVal, foundT := world.GetComponent(playerID, domain.Transform{})
	vVal, foundV := world.GetComponent(playerID, domain.Velocity{})
	hVal, foundH := world.GetComponent(playerID, domain.Health{})
	sVal, foundS := world.GetComponent(playerID, domain.Shield{})
	wVal, foundW := world.GetComponent(playerID, domain.Weapon{})
	cVal, foundC := world.GetComponent(playerID, domain.Cargo{})
	cfgVal, foundCfg := world.GetComponent(playerID, domain.ShipConfig{})
	fVal, foundF := world.GetComponent(playerID, domain.Fleet{})
	if !foundT || !foundV || !foundH || !foundS || !foundW || !foundC || !foundCfg {
		return
	}

	comps := domain.PlayerComponents{
		Transform:  tVal.(*domain.Transform),
		Velocity:   vVal.(*domain.Velocity),
		Health:     hVal.(*domain.Health),
		Shield:     sVal.(*domain.Shield),
		Weapon:     wVal.(*domain.Weapon),
		Cargo:      cVal.(*domain.Cargo),
		ShipConfig: cfgVal.(*domain.ShipConfig),
	}
	if foundF {
		comps.Fleet = fVal.(*domain.Fleet)
	}
	if pgVal, foundPg := world.GetComponent(playerID, domain.PlayerProgress{}); foundPg {
		comps.Progress = pgVal.(*domain.PlayerProgress)
	}
	if rVal, foundR := world.GetComponent(playerID, domain.PlayerResearch{}); foundR {
		comps.Research = rVal.(*domain.PlayerResearch)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := repo.Save(ctx, player, comps); err != nil {
		logger.Warn("Failed to persist player tactics", zap.Uint64("playerID", uint64(playerID)), zap.Error(err))
	}
}

func sendInventoryUpdate(world *ecs.World, bus messaging.MessageBus, playerID domain.EntityID) {
	pDataVal, foundP := world.GetComponent(playerID, domain.PlayerData{})
	cargoVal, foundC := world.GetComponent(playerID, domain.Cargo{})
	if !foundP || !foundC {
		return
	}
	pData := pDataVal.(*domain.PlayerData)
	cargo := cargoVal.(*domain.Cargo)

	cargoProtos := make([]*protocol.ItemInstanceProto, 0, len(cargo.Items))
	for _, item := range cargo.Items {
		cargoProtos = append(cargoProtos, &protocol.ItemInstanceProto{
			Id:           item.ID,
			DefinitionId: item.DefinitionID,
			Quantity:     item.Quantity,
			LocationType: "SHIP_CARGO",
			LocationId:   uint64(playerID),
			OwnerId:      uint64(playerID),
			State:        item.State,
			Name:         string(domain.IDToResource[item.DefinitionID]),
		})
	}

	update := &protocol.InventoryUpdate{
		Credits: pData.Credits,
		Cargo:   cargoProtos,
	}
	payload, _ := proto.Marshal(update)
	packet := &protocol.Packet{
		Type:    protocol.PacketType_S_INVENTORY_UPDATE,
		Payload: payload,
	}
	packetData, _ := proto.Marshal(packet)
	_ = bus.Publish(fmt.Sprintf("player.%d.response", playerID), packetData)
}

func translateVaultType(vType string) string {
	if vType == "personal" {
		return "личный сейф"
	}
	return "склад корпорации"
}

func getQuantityInSlice(items []domain.ItemInstance, defID uint32) int32 {
	var total int32
	for _, item := range items {
		if item.DefinitionID == defID {
			total += item.Quantity
		}
	}
	return total
}

func addItemToSlice(items []domain.ItemInstance, defID uint32, qty int32) []domain.ItemInstance {
	isStackable := defID <= 6
	if isStackable {
		for i, item := range items {
			if item.DefinitionID == defID {
				items[i].Quantity += qty
				return items
			}
		}
	}
	return append(items, domain.ItemInstance{
		DefinitionID: defID,
		Quantity:     qty,
		State:        "normal",
	})
}

func removeItemFromSlice(items []domain.ItemInstance, defID uint32, qty int32) ([]domain.ItemInstance, bool) {
	for i, item := range items {
		if item.DefinitionID == defID {
			if item.Quantity >= qty {
				items[i].Quantity -= qty
				if items[i].Quantity <= 0 {
					items = append(items[:i], items[i+1:]...)
				}
				return items, true
			}
			return items, false
		}
	}
	return items, false
}
