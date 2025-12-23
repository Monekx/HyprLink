package server

import (
	"encoding/json"
	"fmt"
	"net"
)

// Beacon структура теперь используется для десериализации данных от Android
type Beacon struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
}

func ListenForDevices(tcpPort int) {
	addr, err := net.ResolveUDPAddr("udp", ":9999")
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		var beacon Beacon
		if err := json.Unmarshal(buf[:n], &beacon); err != nil {
			continue
		}

		// Отправляем HYPRLINK_ACK и порт, на котором висит TCP сервер
		ack := []byte(fmt.Sprintf("HYPRLINK_ACK|%d", tcpPort))
		conn.WriteToUDP(ack, remoteAddr)

		fmt.Printf("Device %s found at %s. Ack sent.\n", beacon.Hostname, remoteAddr.IP.String())
	}
}
