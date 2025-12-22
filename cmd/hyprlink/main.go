package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/Monekx/hyprlink/internal/config"
	"github.com/Monekx/hyprlink/internal/server"
)

func main() {
	mode := flag.String("mode", "serve", "serve | build | get")
	port := flag.Int("port", 8080, "TCP Port")
	target := flag.String("target", "screenshot", "Target for get mode")
	flag.Parse()

	switch *mode {
	case "serve":
		var mu sync.RWMutex
		fullCfg, err := config.BuildFullConfig("./examples")
		if err != nil {
			log.Fatal(err)
		}

		config.WatchConfig("./examples", func() {
			newCfg, err := config.BuildFullConfig("./examples")
			if err == nil {
				mu.Lock()
				fullCfg = newCfg
				mu.Unlock()
				fmt.Printf("Config reloaded, new hash: %s\n", fullCfg.UI.Hash)
				server.UpdateConfig(fullCfg.UI, fullCfg.Actions)
				server.BroadcastUpdate(fullCfg.UI)
			}
		})

		fmt.Printf("HyprLink: %s (Hash: %s)\n", fullCfg.UI.Hostname, fullCfg.UI.Hash)

		// Запуск UDP Discovery для автопоиска в сети
		go server.StartDiscovery(fullCfg.UI.Hostname, *port)

		// TCP сервер запускается и блокирует поток, внутри него уже запущены
		// циклы обновления модулей, буфера обмена и теперь медиа-статуса
		server.StartTCPServer(*port, fullCfg.UI, fullCfg.Actions)

	case "get":
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", *port))
		if err != nil {
			log.Fatal("Is hyprlink serve running?")
		}
		defer conn.Close()

		req := map[string]string{
			"type": "get_request",
			"id":   *target,
			"pin":  "1337",
		}
		json.NewEncoder(conn).Encode(req)

		fmt.Printf("Request sent: %s\n", *target)
	}
}
