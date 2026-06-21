package main

import (
	"context"
	"database/sql"
	_ "embed"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/config"
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/network"
	"github.com/Home/galaxy-mmo/internal/persistence"
	redisPersist "github.com/Home/galaxy-mmo/internal/persistence/redis"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

//go:embed static/index.html
var indexHTML []byte

// SafeConn wraps a websocket.Conn with a write mutex to prevent concurrent writes.
type SafeConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func NewSafeConn(conn *websocket.Conn) *SafeConn {
	return &SafeConn{conn: conn}
}

func (sc *SafeConn) WriteMessage(msgType int, data []byte) error {
	sc.wmu.Lock()
	defer sc.wmu.Unlock()
	return sc.conn.WriteMessage(msgType, data)
}

func (sc *SafeConn) ReadJSON(v interface{}) error {
	return sc.conn.ReadJSON(v)
}

func (sc *SafeConn) Close() error {
	return sc.conn.Close()
}

func (sc *SafeConn) RemoteAddr() interface{ String() string } {
	return sc.conn.RemoteAddr()
}

type WSManager struct {
	mu      sync.RWMutex
	clients map[*SafeConn]bool
}

func NewWSManager() *WSManager {
	return &WSManager{
		clients: make(map[*SafeConn]bool),
	}
}

func (m *WSManager) Add(conn *SafeConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[conn] = true
}

func (m *WSManager) Remove(conn *SafeConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, conn)
}

func (m *WSManager) Broadcast(data []byte) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for conn := range m.clients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			go func(c *SafeConn) {
				c.Close()
				m.Remove(c)
			}(conn)
		}
	}
}

type WSPlayer struct {
	conn     *SafeConn
	playerID domain.EntityID
	login    string
}

type WSMessage struct {
	Action           string      `json:"action"`
	Login            string      `json:"login"`
	X                float32     `json:"x"`
	Y                float32     `json:"y"`
	Active           bool        `json:"active"`
	RawTargetID      interface{} `json:"target_id"`
	Resource         string      `json:"resource"`
	Amount           int32       `json:"amount"`
	ShipType         string      `json:"ship_type"`
	VaultType        string      `json:"vault_type"`
	ActionType       string      `json:"action_type"`
	AlignWithFleetID uint64      `json:"align_with_fleet_id"`
	ShipID           uint32      `json:"ship_id"`
	Role             string      `json:"role"`
	Strategy         string      `json:"strategy"`
	// Phase 2 hangar / refit
	FittedWeapons  map[string]string `json:"fitted_weapons"`
	FittedHullmods []string          `json:"fitted_hullmods"`
	Vents          int32             `json:"vents"`
	Capacitors     int32             `json:"capacitors"`
}

func (m *WSMessage) GetTargetID() uint64 {
	switch v := m.RawTargetID.(type) {
	case float64:
		return uint64(v)
	case string:
		n, _ := strconv.ParseUint(v, 10, 64)
		return n
	case nil:
		return 0
	default:
		return 0
	}
}

type PlayerRoutingTable struct {
	mu      sync.RWMutex
	mapping map[domain.EntityID]uint32
}

func NewPlayerRoutingTable() *PlayerRoutingTable {
	return &PlayerRoutingTable{
		mapping: make(map[domain.EntityID]uint32),
	}
}

func (rt *PlayerRoutingTable) Get(playerID domain.EntityID) uint32 {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	sysID, exists := rt.mapping[playerID]
	if !exists {
		return 1 // Default to system 1
	}
	return sysID
}

func (rt *PlayerRoutingTable) Set(playerID domain.EntityID, systemID uint32) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.mapping[playerID] = systemID
}

func (rt *PlayerRoutingTable) Has(playerID domain.EntityID) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	_, exists := rt.mapping[playerID]
	return exists
}

func getPlayerSystemID(ctx context.Context, db *sql.DB, accountID uint64) (uint32, error) {
	if db == nil {
		return 1, nil
	}
	var systemID uint32
	err := db.QueryRowContext(ctx, "SELECT system_id FROM characters WHERE account_id = $1", accountID).Scan(&systemID)
	if err == sql.ErrNoRows {
		return 1, nil
	}
	if err != nil {
		return 1, err
	}
	return systemID, nil
}

func getOrCreateAccountID(ctx context.Context, db *sql.DB, login string) (uint64, error) {
	if db == nil {
		return uint64(domain.HashNameToID(login)), nil
	}
	var id uint64
	err := db.QueryRowContext(ctx, "SELECT id FROM accounts WHERE login = $1", login).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err == sql.ErrNoRows {
		// Create a new account with a mock password hash for simple web-based authentication
		defaultHash := "mock_hash"
		err = db.QueryRowContext(ctx, "INSERT INTO accounts (login, password_hash) VALUES ($1, $2) RETURNING id", login, defaultHash).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	}
	return 0, err
}

func main() {
	rand.Seed(time.Now().UnixNano())

	var wsPlayersMu sync.RWMutex
	wsPlayers := make(map[domain.EntityID]*WSPlayer)
	wsConns := make(map[*SafeConn]*WSPlayer)
	wsManager := NewWSManager()

	// 1. Logger initialization
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Info("Galaxy MMO Gateway Server starting...")

	// 2. Parse command-line flags
	configPath := flag.String("config", "configs/server.yaml", "Path to config file")
	flag.Parse()

	// 3. Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Warn("Failed to load config file, using default settings", zap.Error(err))
		cfg = &config.Config{
			Server: config.ServerConfig{Tickrate: 20, MaxPlayers: 100, Address: ":7777"},
			Redis:  config.RedisConfig{Address: "localhost:6379"},
			NATS:   config.NATSConfig{URL: "nats://localhost:4222"},
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3.5 Connect to PostgreSQL Database
	var db *sql.DB
	if cfg.Database.DSN != "" {
		db, err = sql.Open("postgres", cfg.Database.DSN)
		if err != nil {
			logger.Warn("Failed to open database connection", zap.Error(err))
		} else {
			err = db.PingContext(ctx)
			if err != nil {
				logger.Warn("Database ping failed, database is not reachable", zap.Error(err))
				db.Close()
				db = nil
			} else {
				logger.Info("Connected to PostgreSQL database in Gateway.")
			}
		}
	} else {
		logger.Info("Database DSN not specified, using fallback in-memory IDs.")
	}
	if db != nil {
		defer db.Close()
	}

	// 4. Redis Connection (Session Cache)
	var sessionCache domain.SessionCache
	var rClient *redis.Client
	if cfg.Redis.Address != "" {
		rClient = redis.NewClient(&redis.Options{
			Addr: cfg.Redis.Address,
		})
		if err := rClient.Ping(ctx).Err(); err == nil {
			logger.Info("Connected to Redis cache.")
			sessionCache = redisPersist.NewRedisSessionCache(rClient)
		} else {
			logger.Warn("Redis is not reachable. Falling back to In-Memory session cache.", zap.Error(err))
			sessionCache = persistence.NewInMemorySessionCache()
			rClient = nil
		}
	} else {
		logger.Info("Using In-Memory Session Cache")
		sessionCache = persistence.NewInMemorySessionCache()
	}

	// 5. Connect to Messaging Bus (NATS or fallback to Mock)
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

	// 6. Initialize Player Routing Table
	routingTable := NewPlayerRoutingTable()

	// 7. Instantiate Network Server (UDP gateway)
	// Passing nil GameLoop since Gateway has no local physical simulation loop
	server := network.NewServer(cfg.Server.Address, nil, sessionCache, logger)

	// Setup callbacks on the network server
	server.OnPlayerAuth = func(session *network.Session, login string) {
		playerIDVal, err := getOrCreateAccountID(ctx, db, login)
		if err != nil {
			logger.Error("Failed to get or create account ID in UDP auth", zap.String("login", login), zap.Error(err))
			playerIDVal = uint64(domain.HashNameToID(login))
		}
		playerID := domain.EntityID(playerIDVal)
		session.SetEntityID(playerID)
		session.SetAccountID(playerIDVal)

		var systemID uint32
		if routingTable.Has(playerID) {
			systemID = routingTable.Get(playerID)
		} else {
			var dbErr error
			systemID, dbErr = getPlayerSystemID(ctx, db, playerIDVal)
			if dbErr != nil {
				logger.Error("Failed to get player system ID from database", zap.Uint64("playerID", playerIDVal), zap.Error(dbErr))
			}
		}
		if strings.HasPrefix(login, "Bot_Sys") {
			parts := strings.Split(login, "_")
			if len(parts) >= 2 {
				sysPart := strings.TrimPrefix(parts[1], "Sys")
				if parsedSysID, err := strconv.ParseUint(sysPart, 10, 32); err == nil {
					systemID = uint32(parsedSysID)
				}
			}
		}
		routingTable.Set(playerID, systemID)

		logger.Info("Player authenticated, routing to system",
			zap.Uint64("playerID", uint64(playerID)),
			zap.Uint32("systemID", systemID),
			zap.String("login", login),
		)

		// Publish join/spawn command to the system node
		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     protocol.PacketType_C_AUTH_REQUEST,
			Payload:  []byte(login),
		}
		data, err := proto.Marshal(cmd)
		if err != nil {
			logger.Error("Failed to marshal player spawn command", zap.Error(err))
			return
		}

		topic := fmt.Sprintf("system.%d.input", systemID)
		if err := bus.Publish(topic, data); err != nil {
			logger.Error("Failed to publish player spawn command to NATS", zap.String("topic", topic), zap.Error(err))
		}
	}

	server.OnPlayerInput = func(playerID domain.EntityID, pType protocol.PacketType, payload []byte) {
		systemID := routingTable.Get(playerID)

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     pType,
			Payload:  payload,
		}
		data, err := proto.Marshal(cmd)
		if err != nil {
			logger.Error("Failed to marshal player input command", zap.Error(err))
			return
		}

		topic := fmt.Sprintf("system.%d.input", systemID)
		if err := bus.Publish(topic, data); err != nil {
			logger.Error("Failed to publish input command to NATS", zap.String("topic", topic), zap.Error(err))
		}
	}

	// 8. Subscribe to System Node Outputs (*.output) to receive tick snapshots
	_, err = bus.Subscribe("system.*.output", func(msg *messaging.Message) {
		// Subject pattern: system.<system_id>.output
		parts := strings.Split(msg.Topic, ".")
		if len(parts) < 3 {
			return
		}
		sysIDVal, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return
		}
		systemID := uint32(sysIDVal)

		var worldSnap protocol.WorldSnapshot
		if err := proto.Unmarshal(msg.Data, &worldSnap); err != nil {
			logger.Warn("Failed to unmarshal world snapshot from NATS", zap.Error(err))
			return
		}

		// Broadcast snapshot to WebSockets
		mOpts := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
		snapJSON, err := mOpts.Marshal(&worldSnap)
		if err == nil {
			wrappedJSON := []byte(fmt.Sprintf(`{"system_id":%d,"snapshot":%s}`, systemID, string(snapJSON)))
			wsManager.Broadcast(wrappedJSON)
		}

		// Broadcast snapshot to all players in this system
		sessions := server.GetSessions()
		for _, sess := range sessions {
			if sess.GetState() != network.StateInGame {
				continue
			}

			playerID := sess.GetEntityID()
			if routingTable.Get(playerID) == systemID {
				// Build delta snapshot for the player
				delta, ok := network.BuildDeltaSnapshotFromWorldState(&worldSnap, sess, 1500.0)
				if ok {
					server.SendUnreliable(sess, protocol.PacketType_S_DELTA_SNAPSHOT, delta)
				}
			}
		}
	})
	if err != nil {
		logger.Fatal("Failed to subscribe to system outputs", zap.Error(err))
	}

	// 9. Subscribe to transfer/migration status update notifications
	_, err = bus.Subscribe("system.routing.update", func(msg *messaging.Message) {
		// Expects PlayerMigrationPayload or similar containing player_id and new system_id
		// For simplicity, let's use a NATS message representing routing updates
		// Payload is simply "player_id,system_id"
		str := string(msg.Data)
		parts := strings.Split(str, ",")
		if len(parts) != 2 {
			return
		}
		pID, err1 := strconv.ParseUint(parts[0], 10, 64)
		sID, err2 := strconv.ParseUint(parts[1], 10, 32)
		if err1 == nil && err2 == nil {
			routingTable.Set(domain.EntityID(pID), uint32(sID))
			logger.Info("Updated routing table via NATS transfer update",
				zap.Uint64("playerID", pID),
				zap.Uint32("systemID", uint32(sID)),
			)

			wsPlayersMu.RLock()
			wp, isWS := wsPlayers[domain.EntityID(pID)]
			wsPlayersMu.RUnlock()
			if isWS {
				transitionMsg := fmt.Sprintf(`{"type":"system_transition","system_id":%d}`, sID)
				wp.conn.WriteMessage(websocket.TextMessage, []byte(transitionMsg))
			}
		}
	})
	if err != nil {
		logger.Fatal("Failed to subscribe to system routing updates", zap.Error(err))
	}

	// 9.5. Subscribe to Direct Player Responses from World Nodes
	_, err = bus.Subscribe("player.*.response", func(msg *messaging.Message) {
		parts := strings.Split(msg.Topic, ".")
		if len(parts) < 3 {
			return
		}

		pIDVal, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return
		}
		playerID := domain.EntityID(pIDVal)

		var packet protocol.Packet
		if err := proto.Unmarshal(msg.Data, &packet); err != nil {
			logger.Error("Failed to unmarshal player response packet", zap.Error(err))
			return
		}

		sess := server.GetSessionByPlayerID(playerID)
		if sess != nil {
			// Direct responses are reliable, snapshots are unreliable
			isReliable := packet.Type != protocol.PacketType_S_DELTA_SNAPSHOT
			server.SendPacketRaw(sess, packet.Type, packet.Payload, isReliable)
		}

		// Bridge to WebSocket player!
		wsPlayersMu.RLock()
		wp, isWS := wsPlayers[playerID]
		wsPlayersMu.RUnlock()
		if isWS {
			var payloadJSON []byte
			var jsonErr error
			mOpts := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
			switch packet.Type {
			case protocol.PacketType_S_INVENTORY_UPDATE:
				var update protocol.InventoryUpdate
				if proto.Unmarshal(packet.Payload, &update) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&update)
				}
			case protocol.PacketType_S_MARKET_DATA:
				var market protocol.MarketData
				if proto.Unmarshal(packet.Payload, &market) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&market)
				}
			case protocol.PacketType_S_CHAT_MESSAGE:
				var chat protocol.ChatMessage
				if proto.Unmarshal(packet.Payload, &chat) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&chat)
				}
			case protocol.PacketType_S_VAULT_STATUS:
				var vaultStatus protocol.VaultStatus
				if proto.Unmarshal(packet.Payload, &vaultStatus) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&vaultStatus)
				}
			case protocol.PacketType_S_FLEET_STATUS:
				var fleetStatus protocol.FleetStatus
				if proto.Unmarshal(packet.Payload, &fleetStatus) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&fleetStatus)
				}
			case protocol.PacketType_S_HANGAR_DATA:
				var hangar protocol.HangarData
				if proto.Unmarshal(packet.Payload, &hangar) == nil {
					payloadJSON, jsonErr = mOpts.Marshal(&hangar)
				}
			}

			if len(payloadJSON) > 0 && jsonErr == nil {
				typeStr := strings.ToLower(packet.Type.String())
				wp.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"%s","data":%s}`, typeStr, string(payloadJSON))))
			}
		}
	})
	if err != nil {
		logger.Fatal("Failed to subscribe to direct player responses", zap.Error(err))
	}

	// Web Visualizer WebSocket and HTTP Server
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTML)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		rawConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("Failed to upgrade websocket connection", zap.Error(err))
			return
		}
		conn := NewSafeConn(rawConn)
		wsManager.Add(conn)
		logger.Info("New WebVisualizer client connected", zap.String("addr", conn.RemoteAddr().String()))

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Recovered panic in WebSocket handler", zap.Any("panic", r))
				}
				wsManager.Remove(conn)
				wsPlayersMu.Lock()
				if wp, exists := wsConns[conn]; exists {
					delete(wsPlayers, wp.playerID)
					delete(wsConns, conn)
					logger.Info("WebPlayer disconnected", zap.Uint64("playerID", uint64(wp.playerID)), zap.String("login", wp.login))
				}
				wsPlayersMu.Unlock()
				conn.Close()
				logger.Info("WebVisualizer client disconnected", zap.String("addr", conn.RemoteAddr().String()))
			}()

			for {
				var msg WSMessage
				err := conn.ReadJSON(&msg)
				if err != nil {
					logger.Warn("WebSocket ReadJSON error (client disconnecting)", zap.Error(err))
					break
				}

				logger.Debug("WS action received", zap.String("action", msg.Action), zap.Uint64("targetID", msg.GetTargetID()), zap.Bool("active", msg.Active))

				switch msg.Action {
				case "auth":
					pIDVal, err := getOrCreateAccountID(ctx, db, msg.Login)
					if err != nil {
						logger.Error("Failed to get or create account ID in WS auth", zap.String("login", msg.Login), zap.Error(err))
						pIDVal = uint64(domain.HashNameToID(msg.Login))
					}
					pID := domain.EntityID(pIDVal)

					var systemID uint32
					if routingTable.Has(pID) {
						systemID = routingTable.Get(pID)
					} else {
						var dbErr error
						systemID, dbErr = getPlayerSystemID(ctx, db, pIDVal)
						if dbErr != nil {
							logger.Error("Failed to get player system ID from database", zap.Uint64("playerID", pIDVal), zap.Error(dbErr))
						}
					}
					routingTable.Set(pID, systemID)

					wsPlayersMu.Lock()
					wp := &WSPlayer{conn: conn, playerID: pID, login: msg.Login}
					wsPlayers[pID] = wp
					wsConns[conn] = wp
					wsPlayersMu.Unlock()

					logger.Info("WebPlayer authenticated", zap.Uint64("playerID", uint64(pID)), zap.String("login", msg.Login), zap.Uint32("systemID", systemID))

					authResp := &protocol.AuthResponse{
						Success:  true,
						EntityId: uint64(pID),
					}
					mOpts := protojson.MarshalOptions{UseProtoNames: true}
					respJSON, _ := mOpts.Marshal(authResp)
					conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"auth_response","data":%s}`, string(respJSON))))

					// Spawn player command to NATS
					cmdPayload := []byte(msg.Login)
					serverCmd := &protocol.ServerCommand{
						PlayerId: uint64(pID),
						Type:     protocol.PacketType_C_AUTH_REQUEST,
						Payload:  cmdPayload,
					}
					data, _ := proto.Marshal(serverCmd)
					topic := fmt.Sprintf("system.%d.input", systemID)
					if pubErr := bus.Publish(topic, data); pubErr != nil {
						logger.Error("Failed to publish WS auth to NATS", zap.Error(pubErr))
					}

				case "move":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						moveInput := &protocol.MoveInput{X: msg.X, Y: msg.Y}
						payload, _ := proto.Marshal(moveInput)
						systemID := routingTable.Get(wp.playerID)

						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_MOVE_INPUT,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS move to NATS", zap.Error(pubErr))
						}
					}

				case "shoot":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						logger.Info("WebPlayer shoot action",
							zap.Uint64("playerID", uint64(wp.playerID)),
							zap.Bool("active", msg.Active),
							zap.Uint64("targetID", msg.GetTargetID()),
						)
						shootInput := &protocol.ShootInput{Active: msg.Active, TargetId: msg.GetTargetID()}
						payload, _ := proto.Marshal(shootInput)
						systemID := routingTable.Get(wp.playerID)

						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_SHOOT_INPUT,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS shoot to NATS", zap.Error(pubErr))
						}
					}

				case "join_combat":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						joinReq := &protocol.JoinCombatRequest{
							CombatInstanceId: uint32(msg.GetTargetID()),
							AlignWithFleetId: msg.AlignWithFleetID,
						}
						payload, _ := proto.Marshal(joinReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_JOIN_COMBAT_REQUEST,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS join_combat to NATS", zap.Error(pubErr))
						}
					}

				case "set_fleet_tactics":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						setReq := &protocol.SetFleetTactics{
							Ships: []*protocol.FleetShipTactics{
								{ShipId: msg.ShipID, Role: msg.Role, Strategy: msg.Strategy},
							},
						}
						payload, _ := proto.Marshal(setReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_SET_FLEET_TACTICS,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS set_fleet_tactics to NATS", zap.Error(pubErr))
						}
					}

				case "get_hangar":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_GET_HANGAR,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS get_hangar to NATS", zap.Error(pubErr))
						}
					}

				case "fit_ship":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						fitReq := &protocol.FitShipRequest{
							ShipId:         msg.ShipID,
							FittedWeapons:  msg.FittedWeapons,
							FittedHullmods: msg.FittedHullmods,
							Vents:          msg.Vents,
							Capacitors:     msg.Capacitors,
						}
						payload, _ := proto.Marshal(fitReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_FIT_SHIP,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS fit_ship to NATS", zap.Error(pubErr))
						}
					}

				case "mine":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						logger.Info("WebPlayer mine action",
							zap.Uint64("playerID", uint64(wp.playerID)),
							zap.Bool("active", msg.Active),
							zap.Uint64("targetID", msg.GetTargetID()),
						)
						mineInput := &protocol.MineInput{Active: msg.Active, TargetId: msg.GetTargetID()}
						payload, _ := proto.Marshal(mineInput)
						systemID := routingTable.Get(wp.playerID)

						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_MINE_INPUT,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS mine to NATS", zap.Error(pubErr))
						}
					}

				case "refine":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						refineReq := &protocol.StartRefineRequest{
							StationId: msg.GetTargetID(),
						}
						payload, _ := proto.Marshal(refineReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_START_REFINE_REQUEST,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS refine to NATS", zap.Error(pubErr))
						}
					}

				case "build_ship":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						buildReq := &protocol.BuildShipRequest{
							StationId: msg.GetTargetID(),
							ShipType:  msg.ShipType,
						}
						payload, _ := proto.Marshal(buildReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_BUILD_SHIP_REQUEST,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS build ship to NATS", zap.Error(pubErr))
						}
					}

				case "buy":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						buyReq := &protocol.BuyInput{
							StationId: msg.GetTargetID(),
							Resource:  msg.Resource,
							Amount:    msg.Amount,
						}
						payload, _ := proto.Marshal(buyReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_BUY_INPUT,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS buy trade to NATS", zap.Error(pubErr))
						}
					}

				case "sell":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						sellReq := &protocol.SellInput{
							StationId: msg.GetTargetID(),
							Resource:  msg.Resource,
							Amount:    msg.Amount,
						}
						payload, _ := proto.Marshal(sellReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_SELL_INPUT,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS sell trade to NATS", zap.Error(pubErr))
						}
					}

				case "vault_action":
					wsPlayersMu.RLock()
					wp, exists := wsConns[conn]
					wsPlayersMu.RUnlock()
					if exists {
						systemID := routingTable.Get(wp.playerID)
						vaultReq := &protocol.VaultAction{
							StationId:  msg.GetTargetID(),
							Resource:   msg.Resource,
							Amount:     msg.Amount,
							VaultType:  msg.VaultType,
							ActionType: msg.ActionType,
						}
						payload, _ := proto.Marshal(vaultReq)
						serverCmd := &protocol.ServerCommand{
							PlayerId: uint64(wp.playerID),
							Type:     protocol.PacketType_C_VAULT_ACTION,
							Payload:  payload,
						}
						data, _ := proto.Marshal(serverCmd)
						if pubErr := bus.Publish(fmt.Sprintf("system.%d.input", systemID), data); pubErr != nil {
							logger.Error("Failed to publish WS vault action to NATS", zap.Error(pubErr))
						}
					}
				}
			}
		}()
	})

	http.HandleFunc("/api/spawn-bot", func(w http.ResponseWriter, r *http.Request) {
		sysIDStr := r.URL.Query().Get("system_id")
		if _, err := strconv.Atoi(sysIDStr); err != nil {
			sysIDStr = "1"
		}

		botName := fmt.Sprintf("Bot_Sys%s_%d", sysIDStr, rand.Intn(10000))

		botPath := filepath.Join("bin", "botclient.exe")
		if _, err := os.Stat(botPath); os.IsNotExist(err) {
			botPath = "botclient.exe"
		}

		cmd := exec.Command(botPath, botName)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}

		if err := cmd.Start(); err != nil {
			logger.Error("Failed to start bot client", zap.Error(err))
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		logger.Info("Spawned bot client via API", zap.String("name", botName), zap.Int("pid", cmd.Process.Pid))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"pid":%d}`, cmd.Process.Pid)
	})

	go func() {
		logger.Info("Starting Web Visualizer HTTP server on http://localhost:8080/")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Error("Web Visualizer HTTP server failed", zap.Error(err))
		}
	}()

	// 10. Start UDP Network Server
	if err := server.Start(ctx); err != nil {
		logger.Fatal("Failed to start UDP server", zap.Error(err))
	}

	logger.Info("Gateway Server fully active and routing packets.")

	// 11. Wait for termination
	<-ctx.Done()
	logger.Info("Termination signal received. Shutting down gateway...")

	server.Stop()
	if rClient != nil {
		rClient.Close()
	}

	logger.Info("Gateway Server shutdown complete.")
}
