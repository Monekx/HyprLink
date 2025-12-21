package server

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type Beacon struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
}

func StartDiscovery(hostname string, port int) {
	// Используем порт 9999
	addr, err := net.ResolveUDPAddr("udp", "255.255.255.255:9999")
	if err != nil {
		fmt.Printf("UDP Error: %v\n", err)
		return
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		fmt.Printf("UDP Listen Error: %v\n", err)
		return
	}
	defer conn.Close()

	msg, _ := json.Marshal(Beacon{Hostname: hostname, Port: port})

	fmt.Println("UDP Discovery beacon started...")
	for {
		_, err := conn.WriteToUDP(msg, addr)
		if err != nil {
			fmt.Printf("UDP Write Error: %v\n", err)
		}
		time.Sleep(2 * time.Second) // Уменьшим интервал до 2 секунд для быстрого поиска
	}
}
