package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
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

		// Получаем путь к ~/.config/hyprlink
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		configDir := filepath.Join(home, ".config", "hyprlink")

		// Создаем директорию, если её нет
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			fmt.Printf("Config directory not found, creating: %s\n", configDir)
			os.MkdirAll(configDir, 0755)
			// Здесь можно добавить логику копирования дефолтных файлов из /usr/share/hyprlink
		}

		fullCfg, err := config.BuildFullConfig(configDir)
		if err != nil {
			log.Fatal("Error loading config from ", configDir, ": ", err)
		}

		config.WatchConfig(configDir, func() {
			newCfg, err := config.BuildFullConfig(configDir)
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
		go server.StartDiscovery(fullCfg.UI.Hostname, *port)
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
