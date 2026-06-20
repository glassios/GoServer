package network

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// WrapPacket encodes data into a framed protocol.Packet bytes.
func WrapPacket(seq, ack, ackBitfield uint32, pType protocol.PacketType, payload []byte) ([]byte, error) {
	packet := &protocol.Packet{
		Sequence:    seq,
		Ack:         ack,
		AckBitfield: ackBitfield,
		Type:        pType,
		Payload:     payload,
	}

	data, err := proto.Marshal(packet)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet: %w", err)
	}

	return data, nil
}

// UnwrapPacket parses raw bytes into a protocol.Packet.
func UnwrapPacket(data []byte) (*protocol.Packet, error) {
	var packet protocol.Packet
	if err := proto.Unmarshal(data, &packet); err != nil {
		return nil, fmt.Errorf("failed to unmarshal packet: %w", err)
	}

	return &packet, nil
}
