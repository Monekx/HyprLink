package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/Monekx/hyprlink/internal/config"
	"github.com/Monekx/hyprlink/internal/server"
)

func setupDefaultConfig(configDir string) {
	// Если main.json уже есть, значит конфиг настроен
	if _, err := os.Stat(filepath.Join(configDir, "main.json")); err == nil {
		return
	}

	systemDefaults := "/usr/share/hyprlink/examples"

	// Проверяем, существуют ли системные шаблоны (установленные через PKGBUILD)
	if _, err := os.Stat(systemDefaults); os.IsNotExist(err) {
		fmt.Printf("System defaults not found at %s. Please create config manually.\n", systemDefaults)
		return
	}

	fmt.Printf("Initial setup: copying default config from %s to %s\n", systemDefaults, configDir)

	// Используем системную команду cp для рекурсивного копирования содержимого
	// Символы "/." копируют содержимое папки, а не саму папку
	err := exec.Command("cp", "-r", systemDefaults+"/.", configDir).Run()
	if err != nil {
		fmt.Printf("Error copying default config: %v\n", err)
	}
}

func main() {
	mode := flag.String("mode", "serve", "serve | build | get")
	port := flag.Int("port", 8080, "TCP Port")
	target := flag.String("target", "screenshot", "Target for get mode")
	flag.Parse()

	switch *mode {
	case "serve":
		var mu sync.RWMutex

		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		configDir := filepath.Join(home, ".config", "hyprlink")

		// Создаем директорию, если её нет
		os.MkdirAll(configDir, 0755)

		// Копируем дефолтные файлы, если это первый запуск
		setupDefaultConfig(configDir)

		fullCfg, err := config.BuildFullConfig(configDir)
		if err != nil {
			log.Fatal(err)
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
