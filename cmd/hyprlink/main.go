package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/Monekx/hyprlink/internal/config"
	"github.com/Monekx/hyprlink/internal/input"
	"github.com/Monekx/hyprlink/internal/server"
)

func setupDefaultConfig(configDir string) {
	if _, err := os.Stat(filepath.Join(configDir, "main.yaml")); err == nil {
		return
	}
	if _, err := os.Stat(filepath.Join(configDir, "main.json")); err == nil {
		return
	}

	potentialPaths := []string{
		"examples",
		"../../examples",
		"/usr/share/hyprlink/examples",
	}

	var sourcePath string
	for _, p := range potentialPaths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(p, "main.yaml")); err == nil {
				sourcePath = p
				break
			}
		}
	}

	if sourcePath == "" {
		fmt.Println("Warning: No default configuration found.")
		return
	}

	fmt.Printf("Initial setup: copying config from %s to %s\n", sourcePath, configDir)
	exec.Command("cp", "-r", sourcePath+"/.", configDir).Run()
}

func main() {
	mode := flag.String("mode", "serve", "serve | build | get")
	port := flag.Int("port", 8080, "Port")
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
			log.Printf("Error loading config: %v\n", err)
		}

		// Запускаем вотчер конфига
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
		if err := input.InitMouse(); err != nil {
			log.Println("Warning: Mouse control will not work:", err)
		}
		defer input.Close()

		if fullCfg != nil {
			fmt.Printf("HyprLink: %s (Hash: %s)\n", fullCfg.UI.Hostname, fullCfg.UI.Hash)
			server.UpdateConfig(&fullCfg.UI, fullCfg.Actions)
		} else {
			server.UpdateConfig(&config.UIConfig{}, make(map[string]string))
		}

		// Запускаем UDP Discovery (если он нужен, раскомментируй)
		go server.ListenForDevices(*port)

		// Запускаем WS сервер
		server.StartServer(*port, &fullCfg.UI, fullCfg.Actions)

	case "get":
		// Теперь get работает через HTTP API, так как сервер на WebSockets
		url := fmt.Sprintf("http://localhost:%d/api/get?id=%s", *port, *target)
		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("Failed to connect to hyprlink server: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("Error reading response:", err)
		}

		// Пытаемся отформатировать JSON красиво
		var prettyJSON map[string]interface{}
		if err := json.Unmarshal(body, &prettyJSON); err == nil {
			output, _ := json.MarshalIndent(prettyJSON, "", "  ")
			fmt.Println(string(output))
		} else {
			// Если не JSON (например ошибка 500 текстом)
			fmt.Println(string(body))
		}
	}
}
