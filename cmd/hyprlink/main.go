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
	// 1. Если конфиг уже существует (YAML или JSON), ничего не делаем
	if _, err := os.Stat(filepath.Join(configDir, "main.yaml")); err == nil {
		return
	}
	if _, err := os.Stat(filepath.Join(configDir, "main.json")); err == nil {
		return
	}

	// 2. Определяем список мест, где могут лежать примеры
	// Порядок важен: сначала локальные (для разработки), потом системные
	potentialPaths := []string{
		"examples",                     // В текущей папке (корень проекта)
		"../../examples",               // Если запускаем через go run cmd/hyprlink/main.go
		"/usr/share/hyprlink/examples", // Установленный в систему пакет
	}

	var sourcePath string
	for _, p := range potentialPaths {
		// Проверяем, существует ли папка и есть ли в ней main.yaml
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(p, "main.yaml")); err == nil {
				sourcePath = p
				break
			}
		}
	}

	if sourcePath == "" {
		fmt.Println("Warning: No default configuration (examples) found. Checked local and system paths.")
		fmt.Println("Please create ~/.config/hyprlink/main.yaml manually.")
		return
	}

	fmt.Printf("Initial setup: copying default config from %s to %s\n", sourcePath, configDir)

	// Используем cp -r для рекурсивного копирования (важно для папки modules)
	// Добавляем /. в конце sourcePath, чтобы скопировать содержимое папки, а не саму папку examples
	cmd := exec.Command("cp", "-r", sourcePath+"/.", configDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error copying default config: %v\nOutput: %s\n", err, string(output))
	}
}

func main() {
	mode := flag.String("mode", "serve", "serve | build | get")
	port := flag.Int("port", 8080, "TCP Port")
	target := flag.String("target", "all", "Target for get mode")
	flag.Parse()

	switch *mode {
	case "serve":
		var mu sync.RWMutex
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		configDir := filepath.Join(home, ".config", "hyprlink")
		os.MkdirAll(configDir, 0755)

		setupDefaultConfig(configDir)

		fullCfg, err := config.BuildFullConfig(configDir)
		if err != nil {
			// Если конфиг битый или его нет, не падаем сразу, а пробуем подождать
			log.Printf("Error loading config: %v\n", err)
		}

		// Запускаем вотчер
		config.WatchConfig(configDir, func() {
			newCfg, err := config.BuildFullConfig(configDir)
			if err == nil {
				mu.Lock()
				fullCfg = newCfg
				mu.Unlock()
				fmt.Printf("Config reloaded, new hash: %s\n", fullCfg.UI.Hash)
				server.UpdateConfig(&fullCfg.UI, fullCfg.Actions)
				server.BroadcastUpdate(&fullCfg.UI)
			} else {
				fmt.Printf("Error reloading config: %v\n", err)
			}
		})

		if fullCfg != nil {
			fmt.Printf("HyprLink: %s (Hash: %s)\n", fullCfg.UI.Hostname, fullCfg.UI.Hash)
			server.UpdateConfig(&fullCfg.UI, fullCfg.Actions)
		} else {
			fmt.Println("HyprLink started without valid config. Waiting for changes...")
			// Инициализируем пустыми значениями, чтобы сервер не упал
			server.UpdateConfig(&config.UIConfig{}, make(map[string]string))
		}

		go server.ListenForDevices(*port)
		server.StartTCPServer(*port, &fullCfg.UI, fullCfg.Actions)

	case "get":
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", *port))
		if err != nil {
			log.Fatal("Is hyprlink serve running?")
		}
		defer conn.Close()

		req := map[string]string{
			"type": "get_request",
			"id":   *target,
		}
		json.NewEncoder(conn).Encode(req)

		var response map[string]interface{}
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			log.Fatal("Error reading response: ", err)
		}

		output, _ := json.MarshalIndent(response, "", "  ")
		fmt.Println(string(output))
	}
}
