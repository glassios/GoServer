package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/network"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	serverAddr := "127.0.0.1:7777"
	var botName string
	if len(os.Args) > 1 {
		botName = os.Args[1]
	} else {
		botName = fmt.Sprintf("Bot_%d", rand.Intn(10000))
	}

	fmt.Printf("[%s] Connecting to server at %s...\n", botName, serverAddr)

	rAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		panic(err)
	}

	conn, err := net.DialUDP("udp", nil, rAddr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// 1. Authenticate
	authReq, err := proto.Marshal(&protocol.AuthRequest{
		Login:    botName,
		Password: "password",
	})
	if err != nil {
		panic(err)
	}

	// Wrap and send reliable auth packet
	packetData, err := network.WrapPacket(1, 0, 0, protocol.PacketType_C_AUTH_REQUEST, authReq)
	if err != nil {
		panic(err)
	}

	_, err = conn.Write(packetData)
	if err != nil {
		panic(err)
	}

	// Wait for Auth Response
	buf := make([]byte, 2048)
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		panic(err)
	}

	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Auth timeout. Is the server running?")
		return
	}

	respPacket, err := network.UnwrapPacket(buf[:n])
	if err != nil {
		panic(err)
	}

	if respPacket.Type != protocol.PacketType_S_AUTH_RESPONSE {
		fmt.Printf("Expected Auth Response, got: %s\n", respPacket.Type.String())
		return
	}

	var authResp protocol.AuthResponse
	if err := proto.Unmarshal(respPacket.Payload, &authResp); err != nil {
		panic(err)
	}

	if !authResp.Success {
		fmt.Printf("Auth failed: %s\n", authResp.ErrorMessage)
		return
	}

	fmt.Printf("[%s] Auth Successful! EntityID: %d. SessionToken: %s\n", botName, authResp.EntityId, authResp.SessionToken)

	// Reset read deadline to non-blocking or large duration
	_ = conn.SetReadDeadline(time.Time{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start reading server snapshots in a background thread
	go func() {
		readBuf := make([]byte, 65535)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := conn.Read(readBuf)
				if err != nil {
					return
				}

				packet, err := network.UnwrapPacket(readBuf[:n])
				if err != nil {
					continue
				}

				if packet.Type == protocol.PacketType_S_DELTA_SNAPSHOT {
					var delta protocol.DeltaSnapshot
					if err := proto.Unmarshal(packet.Payload, &delta); err == nil {
						// Process entities to find asteroids
						for _, ent := range delta.UpdatedEntities {
							if ent.EntityId == authResp.EntityId {
								fmt.Printf("[%s] POS: (%.1f, %.1f), HP: %d/%d, SH: %d/%d\n",
									botName, ent.X, ent.Y, ent.Hp, ent.MaxHp, ent.Shield, ent.MaxShield)
							}
							// If we see an asteroid close by, we can mine it
							if ent.EntityType == 2 { // EntityType Asteroid (from entity.go, wait: 2 is Asteroid: Player=0, NPC=1, Asteroid=2)
								dist := float32(0.0) // simplified range check or assume we can mine
								_ = dist
							}
						}
					}
				}
			}
		}
	}()

	// Game Tick Simulation Loop (Send inputs to server)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	seq := uint32(2)

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[%s] Disconnecting...\n", botName)
			return
		case <-ticker.C:
			seq++
			// Random movement input
			movePayload, _ := proto.Marshal(&protocol.MoveInput{
				X: (rand.Float32() - 0.5) * 50,
				Y: (rand.Float32() - 0.5) * 50,
			})

			ack, ackBitfield := uint32(0), uint32(0) // simple unreliable input
			movePacket, _ := network.WrapPacket(0, ack, ackBitfield, protocol.PacketType_C_MOVE_INPUT, movePayload)
			
			_, _ = conn.Write(movePacket)
		}
	}
}
