package network

import (
	"net"
	"sync"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type SessionState int

const (
	StateConnecting SessionState = iota
	StateAuthenticating
	StateInGame
	StateDisconnecting
)

func (s SessionState) String() string {
	switch s {
	case StateConnecting:
		return "Connecting"
	case StateAuthenticating:
		return "Authenticating"
	case StateInGame:
		return "InGame"
	case StateDisconnecting:
		return "Disconnecting"
	default:
		return "Unknown"
	}
}

type Session struct {
	Addr              *net.UDPAddr
	EntityID          domain.EntityID
	AccountID         uint64
	State             SessionState
	Tracker           *ReliabilityTracker
	LastActive        time.Time
	previouslyVisible map[domain.EntityID]struct{}
	mutex             sync.RWMutex
}

func NewSession(addr *net.UDPAddr) *Session {
	return &Session{
		Addr:              addr,
		State:             StateConnecting,
		Tracker:           NewReliabilityTracker(),
		LastActive:        time.Now(),
		previouslyVisible: make(map[domain.EntityID]struct{}),
	}
}

func (s *Session) GetState() SessionState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.State
}

func (s *Session) SetState(state SessionState) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.State = state
}

func (s *Session) SetEntityID(id domain.EntityID) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.EntityID = id
}

func (s *Session) GetEntityID() domain.EntityID {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.EntityID
}

func (s *Session) SetAccountID(id uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.AccountID = id
}

func (s *Session) GetAccountID() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.AccountID
}

func (s *Session) Touch() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.LastActive = time.Now()
}

func (s *Session) IsTimedOut(timeout time.Duration) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return time.Since(s.LastActive) > timeout
}

func (s *Session) GetPreviouslyVisible() map[domain.EntityID]struct{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	res := make(map[domain.EntityID]struct{}, len(s.previouslyVisible))
	for k := range s.previouslyVisible {
		res[k] = struct{}{}
	}
	return res
}

func (s *Session) SetPreviouslyVisible(visible map[domain.EntityID]struct{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.previouslyVisible = make(map[domain.EntityID]struct{}, len(visible))
	for k := range visible {
		s.previouslyVisible[k] = struct{}{}
	}
}
