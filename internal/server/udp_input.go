package server

import (
	"encoding/binary"
	"log"
	"net"

	"github.com/Monekx/hyprlink/internal/input"
)

// Протокол:
// [1 байт Type] [Данные...]
// Type 0x01 (Move):  [DX (int32 - 4 bytes)] [DY (int32 - 4 bytes)]
// Type 0x02 (Click): [Button (1 byte: 0=Left, 1=Right)]

func StartInputServer(port string) {
	addr, err := net.ResolveUDPAddr("udp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to resolve UDP input address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to start UDP input server: %v", err)
	}
	defer conn.Close()

	log.Printf("Input UDP Server listening on %s (Binary Mode)", port)

	buf := make([]byte, 1024)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		if n > 0 {
			handleBinaryInput(buf[:n])
		}
	}
}

func handleBinaryInput(data []byte) {
	if len(data) < 1 {
		return
	}

	cmd := data[0]
	switch cmd {
	case 0x01: // Move Mouse
		if len(data) >= 9 {
			dx := int32(binary.LittleEndian.Uint32(data[1:5]))
			dy := int32(binary.LittleEndian.Uint32(data[5:9]))
			input.Move(dx, dy)
		}
	case 0x02: // Click Mouse
		if len(data) >= 2 {
			if data[1] == 0 {
				input.Click("left")
			} else {
				input.Click("right")
			}
		}
	// НОВОЕ: Обработка клавиатуры
	case 0x03: // Type Text
		// Остаток пакета — это строка UTF-8
		text := string(data[1:])
		for _, char := range text {
			input.TypeChar(char)
		}
	case 0x04: // Special Keys (Backspace, Enter и т.д. отправленные как коды)
		if len(data) >= 5 {
			keyCode := int(binary.LittleEndian.Uint32(data[1:5]))
			input.PressKey(keyCode)
		}
	}
}
