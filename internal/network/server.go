package network

import (
	"context"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/gameloop"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

const (
	MaxPacketSize  = 1400
	SessionTimeout = 15 * time.Second
	RateLimitPPS   = 60.0 // Packets per second limit
)

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

func (b *tokenBucket) Allow(limit float64) bool {
	now := time.Now()
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.lastUpdate = now
	b.tokens += elapsed * limit
	if b.tokens > limit {
		b.tokens = limit
	}
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

type Server struct {
	addr          string
	conn          *net.UDPConn
	gameLoop      *gameloop.GameLoop
	sessionCache  domain.SessionCache
	sessions      map[string]*Session
	sessionsMu    sync.RWMutex
	rateLimiters  map[string]*tokenBucket
	limitersMu    sync.Mutex
	logger        *zap.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	OnPlayerInput func(playerID domain.EntityID, pType protocol.PacketType, payload []byte)
	OnPlayerAuth  func(session *Session, login string)
}

func NewServer(addr string, gameLoop *gameloop.GameLoop, cache domain.SessionCache, logger *zap.Logger) *Server {
	return &Server{
		addr:         addr,
		gameLoop:     gameLoop,
		sessionCache: cache,
		sessions:     make(map[string]*Session),
		rateLimiters: make(map[string]*tokenBucket),
		logger:       logger,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	lAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", lAddr)
	if err != nil {
		return err
	}
	s.conn = conn

	s.logger.Info("UDP Network Server started", zap.String("addr", s.addr))

	s.wg.Add(3)
	go s.readLoop()
	go s.retransmitLoop()
	go s.timeoutLoop()

	return nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		s.conn.Close()
	}
	s.wg.Wait()
	s.logger.Info("UDP Network Server stopped")
}

func (s *Server) checkRateLimit(addr string) bool {
	s.limitersMu.Lock()
	defer s.limitersMu.Unlock()

	bucket, exists := s.rateLimiters[addr]
	if !exists {
		bucket = &tokenBucket{tokens: RateLimitPPS, lastUpdate: time.Now()}
		s.rateLimiters[addr] = bucket
	}

	return bucket.Allow(RateLimitPPS)
}

func (s *Server) readLoop() {
	defer s.wg.Done()
	buf := make([]byte, 65535)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, rAddr, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-s.ctx.Done():
					return
				default:
					s.logger.Error("Read from UDP error", zap.Error(err))
					continue
				}
			}

			// Validate packet size
			if n > MaxPacketSize {
				s.logger.Warn("Packet size exceeds limit, dropped", zap.Int("size", n), zap.String("addr", rAddr.String()))
				continue
			}

			// Validate flood limits
			if !s.checkRateLimit(rAddr.String()) {
				s.logger.Warn("Rate limit exceeded, packet dropped", zap.String("addr", rAddr.String()))
				continue
			}

			// Clone data to avoid slice reuse issues in goroutines
			data := make([]byte, n)
			copy(data, buf[:n])

			go s.handlePacket(rAddr, data)
		}
	}
}

func (s *Server) handlePacket(addr *net.UDPAddr, data []byte) {
	packet, err := UnwrapPacket(data)
	if err != nil {
		s.logger.Warn("Failed to unwrap packet", zap.Error(err), zap.String("addr", addr.String()))
		return
	}

	session := s.getOrCreateSession(addr)
	session.Touch()

	// Reliability processing
	isDuplicate := session.Tracker.ProcessIncomingHeader(packet.Sequence, packet.Ack, packet.AckBitfield)
	if isDuplicate {
		s.logger.Debug("Duplicate packet received, dropped", zap.Uint32("seq", packet.Sequence), zap.String("addr", addr.String()))
		return
	}

	// Route packet
	s.routePacket(session, packet)
}

func (s *Server) GetSessionByPlayerID(playerID domain.EntityID) *Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for _, sess := range s.sessions {
		if sess.GetEntityID() == playerID {
			return sess
		}
	}
	return nil
}

func (s *Server) GetSessions() []*Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	res := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		res = append(res, sess)
	}
	return res
}

func (s *Server) getOrCreateSession(addr *net.UDPAddr) *Session {
	key := addr.String()
	s.sessionsMu.RLock()
	session, exists := s.sessions[key]
	s.sessionsMu.RUnlock()

	if exists {
		return session
	}

	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	// Double-check
	if session, exists = s.sessions[key]; exists {
		return session
	}

	session = NewSession(addr)
	s.sessions[key] = session
	s.logger.Info("New session registered", zap.String("addr", key))
	return session
}

func (s *Server) routePacket(session *Session, packet *protocol.Packet) {
	if packet.Type == protocol.PacketType_C_AUTH_REQUEST {
		s.handleAuthRequest(session, packet.Payload)
		return
	}
	if packet.Type == protocol.PacketType_C_PING {
		s.handlePing(session, packet.Payload)
		return
	}

	if s.gameLoop == nil && s.OnPlayerInput != nil {
		s.OnPlayerInput(session.GetEntityID(), packet.Type, packet.Payload)
		return
	}

	switch packet.Type {
	case protocol.PacketType_C_MOVE_INPUT:
		s.handleMoveInput(session, packet.Payload)
	case protocol.PacketType_C_SHOOT_INPUT:
		s.handleShootInput(session, packet.Payload)
	case protocol.PacketType_C_MINE_INPUT:
		s.handleMineInput(session, packet.Payload)
	default:
		s.logger.Warn("Unknown/unhandled packet type received", zap.String("type", packet.Type.String()))
	}
}

func (s *Server) handleAuthRequest(session *Session, payload []byte) {
	if session.GetState() != StateConnecting {
		return
	}
	session.SetState(StateAuthenticating)

	var req protocol.AuthRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		s.sendAuthResponse(session, false, 0, "Invalid auth request format")
		return
	}

	s.logger.Info("Processing auth request", zap.String("login", req.Login), zap.String("addr", session.Addr.String()))

	// Mock Authentication for MVP (verified on stage 1.10)
	// We will simulate a successful login
	session.SetState(StateInGame)
	// Bind to stable entity ID derived from login name
	playerID := domain.HashNameToID(req.Login)
	session.SetEntityID(playerID)
	session.SetAccountID(uint64(playerID))

	if s.OnPlayerAuth != nil {
		s.OnPlayerAuth(session, req.Login)
	}

	s.sendAuthResponse(session, true, uint64(session.GetEntityID()), "")
}

func (s *Server) sendAuthResponse(session *Session, success bool, entityID uint64, errMsg string) {
	resp := &protocol.AuthResponse{
		Success:      success,
		SessionToken: "token_" + session.Addr.IP.String(),
		EntityId:     entityID,
		ErrorMessage: errMsg,
	}
	s.SendReliable(session, protocol.PacketType_S_AUTH_RESPONSE, resp)
}

func (s *Server) handlePing(session *Session, payload []byte) {
	var ping protocol.Ping
	if err := proto.Unmarshal(payload, &ping); err != nil {
		return
	}

	pong := &protocol.Pong{
		Timestamp: ping.Timestamp,
	}
	s.SendUnreliable(session, protocol.PacketType_S_PONG, pong)
}

func (s *Server) handleMoveInput(session *Session, payload []byte) {
	if session.GetState() != StateInGame {
		return
	}

	var input protocol.MoveInput
	if err := proto.Unmarshal(payload, &input); err != nil {
		return
	}

	s.gameLoop.EnqueueCommand(gameloop.Command{
		PlayerID: session.GetEntityID(),
		Type:     "move",
		Payload: domain.Velocity{
			X: input.X,
			Y: input.Y,
		},
	})
}

func (s *Server) handleShootInput(session *Session, payload []byte) {
	if session.GetState() != StateInGame {
		return
	}

	var input protocol.ShootInput
	if err := proto.Unmarshal(payload, &input); err != nil {
		return
	}

	s.gameLoop.EnqueueCommand(gameloop.Command{
		PlayerID: session.GetEntityID(),
		Type:     "shoot",
		Payload: struct {
			Active   bool
			TargetID domain.EntityID
		}{
			Active:   input.Active,
			TargetID: domain.EntityID(input.TargetId),
		},
	})
}

func (s *Server) handleMineInput(session *Session, payload []byte) {
	if session.GetState() != StateInGame {
		return
	}

	var input protocol.MineInput
	if err := proto.Unmarshal(payload, &input); err != nil {
		return
	}

	s.gameLoop.EnqueueCommand(gameloop.Command{
		PlayerID: session.GetEntityID(),
		Type:     "mine",
		Payload: struct {
			Active   bool
			TargetID domain.EntityID
		}{
			Active:   input.Active,
			TargetID: domain.EntityID(input.TargetId),
		},
	})
}

func (s *Server) SendPacketRaw(session *Session, pType protocol.PacketType, payload []byte, reliable bool) {
	if reliable {
		seq := session.Tracker.NextSequence()
		session.Tracker.RegisterSentPacket(seq, pType, payload)

		ack, ackBitfield := session.Tracker.GetAckInfo()
		data, err := WrapPacket(seq, ack, ackBitfield, pType, payload)
		if err == nil {
			s.sendRaw(session.Addr, data)
		}
	} else {
		ack, ackBitfield := session.Tracker.GetAckInfo()
		data, err := WrapPacket(0, ack, ackBitfield, pType, payload)
		if err == nil {
			s.sendRaw(session.Addr, data)
		}
	}
}

func (s *Server) SendUnreliable(session *Session, pType protocol.PacketType, msg proto.Message) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Error("Failed to marshal outgoing message", zap.Error(err))
		return
	}

	ack, ackBitfield := session.Tracker.GetAckInfo()
	// Unreliable packets use seq = 0 (fire and forget)
	data, err := WrapPacket(0, ack, ackBitfield, pType, payload)
	if err != nil {
		s.logger.Error("Failed to wrap packet", zap.Error(err))
		return
	}

	s.sendRaw(session.Addr, data)
}

func (s *Server) SendReliable(session *Session, pType protocol.PacketType, msg proto.Message) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Error("Failed to marshal outgoing msg", zap.Error(err))
		return
	}

	seq := session.Tracker.NextSequence()
	session.Tracker.RegisterSentPacket(seq, pType, payload)

	ack, ackBitfield := session.Tracker.GetAckInfo()
	data, err := WrapPacket(seq, ack, ackBitfield, pType, payload)
	if err != nil {
		s.logger.Error("Failed to wrap packet", zap.Error(err))
		return
	}

	s.sendRaw(session.Addr, data)
}

func (s *Server) sendRaw(addr *net.UDPAddr, data []byte) {
	if s.conn == nil {
		return
	}
	_, err := s.conn.WriteToUDP(data, addr)
	if err != nil {
		s.logger.Error("Failed to write to UDP", zap.Error(err), zap.String("addr", addr.String()))
	}
}

// Retransmission loop

func (s *Server) retransmitLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sessionsMu.RLock()
			var activeSessions []*Session
			for _, sess := range s.sessions {
				activeSessions = append(activeSessions, sess)
			}
			s.sessionsMu.RUnlock()

			for _, session := range activeSessions {
				toResend := session.Tracker.GetPacketsToRetransmit()
				if len(toResend) == 0 {
					continue
				}

				ack, ackBitfield := session.Tracker.GetAckInfo()
				for _, p := range toResend {
					data, err := WrapPacket(p.seq, ack, ackBitfield, p.packetType, p.payload)
					if err == nil {
						s.sendRaw(session.Addr, data)
					}
				}
			}
		}
	}
}

// Timeout loop

func (s *Server) timeoutLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sessionsMu.Lock()
			var toRemove []string
			for key, sess := range s.sessions {
				if sess.IsTimedOut(SessionTimeout) {
					toRemove = append(toRemove, key)
				}
			}
			for _, key := range toRemove {
				s.logger.Info("Session timed out, removing", zap.String("addr", key))
				delete(s.sessions, key)
			}
			s.sessionsMu.Unlock()
		}
	}
}

// BindToGameLoop links the server snapshot broadcast to the GameLoop tick hook.
func (s *Server) BindToGameLoop() {
	s.gameLoop.OnSnapshot = func(tick uint64) {
		s.broadcastSnapshots(tick)
	}
}

func (s *Server) broadcastSnapshots(tick uint64) {
	s.sessionsMu.RLock()
	var activeSessions []*Session
	for _, sess := range s.sessions {
		if sess.GetState() == StateInGame {
			activeSessions = append(activeSessions, sess)
		}
	}
	s.sessionsMu.RUnlock()

	world := s.gameLoop.World()
	for _, session := range activeSessions {
		delta, ok := BuildDeltaSnapshot(world, session, tick)
		if ok {
			s.SendUnreliable(session, protocol.PacketType_S_DELTA_SNAPSHOT, delta)
		}
	}
}
