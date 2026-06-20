package network

import (
	"sync"
	"time"

	"github.com/Home/galaxy-mmo/pkg/protocol"
)

const (
	ResendInterval = 150 * time.Millisecond
	MaxPendingAcks = 256
)

type sentPacket struct {
	seq        uint32
	packetType protocol.PacketType
	payload    []byte
	sentTime   time.Time
	attempts   int
}

type ReliabilityTracker struct {
	mutex            sync.Mutex
	localSeq         uint32
	remoteSeq        uint32
	ackBitfield      uint32
	sentPackets      map[uint32]*sentPacket
	receivedPackets  map[uint32]time.Time
	lastReceivedTime time.Time
}

func NewReliabilityTracker() *ReliabilityTracker {
	return &ReliabilityTracker{
		sentPackets:      make(map[uint32]*sentPacket),
		receivedPackets:  make(map[uint32]time.Time),
		lastReceivedTime: time.Now(),
	}
}

// NextSequence generates and returns the next local sequence number.
func (t *ReliabilityTracker) NextSequence() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.localSeq++
	if t.localSeq == 0 {
		t.localSeq = 1
	}
	return t.localSeq
}

// RegisterSentPacket stores a reliable packet for potential retransmission.
func (t *ReliabilityTracker) RegisterSentPacket(seq uint32, pType protocol.PacketType, payload []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.sentPackets[seq] = &sentPacket{
		seq:        seq,
		packetType: pType,
		payload:    payload,
		sentTime:   time.Now(),
		attempts:   1,
	}

	// Limit map size
	if len(t.sentPackets) > MaxPendingAcks {
		// Remove oldest
		var oldestSeq uint32
		var oldestTime time.Time
		first := true
		for s, p := range t.sentPackets {
			if first || p.sentTime.Before(oldestTime) {
				oldestTime = p.sentTime
				oldestSeq = s
				first = false
			}
		}
		delete(t.sentPackets, oldestSeq)
	}
}

// ProcessIncomingHeader processes the packet sequence and ack info.
func (t *ReliabilityTracker) ProcessIncomingHeader(seq, ack, ackBitfield uint32) (isDuplicate bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.lastReceivedTime = time.Now()

	// 1. Check if incoming seq is duplicate
	if _, exists := t.receivedPackets[seq]; exists {
		return true // Duplicate packet, drop
	}
	t.receivedPackets[seq] = time.Now()

	// Clean up received packets tracker occasionally
	if len(t.receivedPackets) > MaxPendingAcks {
		var oldestSeq uint32
		var oldestTime time.Time
		first := true
		for s, tm := range t.receivedPackets {
			if first || tm.Before(oldestTime) {
				oldestTime = tm
				oldestSeq = s
				first = false
			}
		}
		delete(t.receivedPackets, oldestSeq)
	}

	// 2. Update remote sequence and ack bitfield
	if seqGreater(seq, t.remoteSeq) {
		shift := seq - t.remoteSeq
		if shift < 32 {
			t.ackBitfield = (t.ackBitfield << shift) | 1
			// Set the bit corresponding to the old remoteSeq
			t.ackBitfield |= (1 << (shift - 1))
		} else {
			t.ackBitfield = 0
		}
		t.remoteSeq = seq
	} else {
		// Late packet, update bitfield
		shift := t.remoteSeq - seq
		if shift <= 32 {
			t.ackBitfield |= (1 << (shift - 1))
		}
	}

	// 3. Process ACKs for sent packets
	// Ack the single packet
	delete(t.sentPackets, ack)

	// Ack packets in the bitfield
	for i := uint32(0); i < 32; i++ {
		if (ackBitfield & (1 << i)) != 0 {
			ackedSeq := ack - (i + 1)
			delete(t.sentPackets, ackedSeq)
		}
	}

	return false
}

// GetAckInfo returns remote sequence and ack bitfield to embed in outgoing packets.
func (t *ReliabilityTracker) GetAckInfo() (uint32, uint32) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.remoteSeq, t.ackBitfield
}

// GetPacketsToRetransmit returns packets that have timed out and need to be resent.
func (t *ReliabilityTracker) GetPacketsToRetransmit() []*sentPacket {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	var toResend []*sentPacket
	now := time.Now()

	for _, p := range t.sentPackets {
		if now.Sub(p.sentTime) >= ResendInterval {
			p.sentTime = now
			p.attempts++
			toResend = append(toResend, p)
		}
	}

	return toResend
}

func (t *ReliabilityTracker) LastReceivedTime() time.Time {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.lastReceivedTime
}

// Helper: check if s1 is greater than s2 with wrapping (32-bit range).
func seqGreater(s1, s2 uint32) bool {
	return ((s1 > s2) && (s1-s2 <= 0x7FFFFFFF)) || ((s1 < s2) && (s2-s1 > 0x7FFFFFFF))
}
