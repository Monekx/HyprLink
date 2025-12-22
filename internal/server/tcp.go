package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Monekx/hyprlink/internal/config"
)

type Request struct {
	Type     string  `json:"type"`
	Pin      string  `json:"pin,omitempty"`
	DeviceID string  `json:"device_id,omitempty"`
	Token    string  `json:"token,omitempty"`
	Hash     string  `json:"hash,omitempty"`
	ID       string  `json:"id,omitempty"`
	Value    float64 `json:"value,omitempty"`
	Content  string  `json:"content,omitempty"`
	Title    string  `json:"title,omitempty"`
	App      string  `json:"app,omitempty"`
}

type Response struct {
	Type     string           `json:"type,omitempty"`
	Status   string           `json:"status,omitempty"`
	DeviceID string           `json:"device_id,omitempty"`
	Token    string           `json:"token,omitempty"`
	Message  string           `json:"message,omitempty"`
	Config   *config.UIConfig `json:"config,omitempty"`
	ID       string           `json:"id,omitempty"`
	Value    float64          `json:"value,omitempty"`
	Content  string           `json:"content,omitempty"`
	App      string           `json:"app,omitempty"`
	Duration int64            `json:"duration,omitempty"`
}

var (
	currentPin string
	pinMutex   sync.Mutex
	clients    = make(map[net.Conn]*json.Encoder)
	mu         sync.Mutex

	configMu       sync.RWMutex
	currentConfig  *config.UIConfig
	currentActions map[string]string
)

func generateAndNotifyPin() string {
	pinMutex.Lock()
	defer pinMutex.Unlock()

	currentPin = fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	exec.Command("notify-send", "-a", "HyprLink", "Запрос подключения",
		fmt.Sprintf("Введите PIN-код на устройстве: %s", currentPin)).Run()

	return currentPin
}

func UpdateConfig(cfg *config.UIConfig, actions map[string]string) {
	configMu.Lock()
	defer configMu.Unlock()
	currentConfig = cfg
	currentActions = actions
}

func StartTCPServer(port int, cfg *config.UIConfig, actions map[string]string) {
	UpdateConfig(cfg, actions)

	lc := net.ListenConfig{KeepAlive: 10 * time.Second}
	ln, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return
	}

	go startUpdateLoop()
	go watchClipboard()
	go watchMediaStatus() // Фоновый мониторинг плеера

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleSession(conn)
	}
}

func handleSession(conn net.Conn) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var req Request
	if err := decoder.Decode(&req); err != nil {
		conn.Close()
		return
	}

	// Получаем домашнюю директорию пользователя динамически
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	// Формируем полный путь
	trustedPath := filepath.Join(home, ".config", "hyprlink", "trusted_devices.json")

	// Гарантируем, что папка существует
	os.MkdirAll(filepath.Dir(trustedPath), 0755)
	trustedDevices, _ := config.LoadTrustedDevices(trustedPath)

	isAuthorized := false
	var newID, newToken string

	if req.DeviceID != "" && req.Token != "" {
		if dev, ok := trustedDevices[req.DeviceID]; ok && dev.Token == req.Token {
			isAuthorized = true
		}
	}

	if !isAuthorized {
		pin := generateAndNotifyPin()
		encoder.Encode(Response{Status: "unauthorized", Message: "PIN_REQUIRED"})

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var authReq Request
		if err := decoder.Decode(&authReq); err != nil {
			conn.Close()
			return
		}

		if authReq.Pin == pin && pin != "" {
			isAuthorized = true
			newID = "phone-" + config.GenerateToken()[:8]
			newToken = config.GenerateToken()
			config.SaveTrustedDevice(trustedPath, config.TrustedDevice{
				ID: newID, Token: newToken, Name: "Android Device",
			})
		}
	}

	if !isAuthorized {
		encoder.Encode(Response{Status: "error", Message: "INVALID_PIN"})
		conn.Close()
		return
	}

	conn.SetReadDeadline(time.Time{})

	mu.Lock()
	clients[conn] = encoder
	mu.Unlock()

	// При подключении сразу отправляем текущий статус медиа
	go broadcastMediaStatus()

	defer func() {
		mu.Lock()
		delete(clients, conn)
		mu.Unlock()
		conn.Close()
	}()

	configMu.RLock()
	cfg := currentConfig
	configMu.RUnlock()

	resp := Response{Status: "ok", DeviceID: newID, Token: newToken}
	if req.Hash != cfg.Hash {
		resp.Status = "update"
		resp.Config = cfg
	}
	encoder.Encode(resp)

	for {
		var action Request
		if err := decoder.Decode(&action); err != nil {
			return
		}
		switch action.Type {
		case "action":
			go handleAction(action.ID, action.Value)
		case "clipboard":
			clean := strings.TrimSpace(action.Content)
			if clean != "" {
				go exec.Command("bash", "-c", fmt.Sprintf("echo -n %q | wl-copy", clean)).Run()
			}
		case "notification":
			go func(a, t, c string) {
				exec.Command("notify-send", "-a", a, t, c).Run()
			}(action.App, action.Title, action.Content)
		case "ping":
		}
	}
}

func handleAction(actionID string, actionValue float64) {
	switch actionID {
	case "media_play":
		exec.Command("playerctl", "play").Run()
		broadcastMediaStatus()
		return
	case "media_pause":
		exec.Command("playerctl", "pause").Run()
		broadcastMediaStatus()
		return
	case "media_next":
		exec.Command("playerctl", "next").Run()
		broadcastMediaStatus()
		return
	case "media_prev":
		exec.Command("playerctl", "previous").Run()
		broadcastMediaStatus()
		return
	case "media_seek":
		// playerctl принимает значение в секундах
		exec.Command("playerctl", "position", fmt.Sprintf("%f", actionValue)).Run()
		broadcastMediaStatus()
		return
	}

	configMu.RLock()
	actions := currentActions
	configMu.RUnlock()

	if cmdStr, ok := actions[actionID]; ok {
		valStr := fmt.Sprintf("%.0f", actionValue)
		finalCmd := strings.ReplaceAll(cmdStr, "{v}", valStr)
		exec.Command("/bin/bash", "-c", finalCmd).Run()
	}
}

func broadcastMediaStatus() {
	title, _ := exec.Command("playerctl", "metadata", "title").Output()
	artist, _ := exec.Command("playerctl", "metadata", "artist").Output()
	status, _ := exec.Command("playerctl", "status").Output()

	posRaw, _ := exec.Command("playerctl", "position").Output()
	durRaw, _ := exec.Command("playerctl", "metadata", "mpris:length").Output()

	t := strings.TrimSpace(string(title))
	a := strings.TrimSpace(string(artist))
	s := strings.ToLower(strings.TrimSpace(string(status)))

	// playerctl position возвращает секунды (float) -> в мс
	posFloat, _ := strconv.ParseFloat(strings.TrimSpace(string(posRaw)), 64)
	posMs := int64(posFloat * 1000)

	// mpris:length обычно в микросекундах -> в мс
	durUs, _ := strconv.ParseInt(strings.TrimSpace(string(durRaw)), 10, 64)
	durMs := durUs / 1000

	if t == "" {
		t = "Ничего не воспроизводится"
		posMs = 0
		durMs = 0
	}

	broadcastUpdate(Response{
		Type:     "media_info",
		Content:  t,
		App:      a,
		Status:   s,
		Value:    float64(posMs), // Текущая позиция в мс
		Duration: durMs,          // Длительность в мс
	})
}

func broadcastUpdate(resp Response) {
	mu.Lock()
	var badConns []net.Conn
	for conn, encoder := range clients {
		if err := encoder.Encode(resp); err != nil {
			badConns = append(badConns, conn)
		}
	}
	for _, conn := range badConns {
		delete(clients, conn)
		conn.Close()
	}
	mu.Unlock()
}

func startUpdateLoop() {
	for {
		configMu.RLock()
		cfg := currentConfig
		configMu.RUnlock()

		if cfg != nil {
			for _, profile := range cfg.Profiles {
				scanModules(profile.Modules)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func scanModules(modules []config.Module) {
	for _, mod := range modules {
		if mod.Source != "" {
			out, err := exec.Command("/bin/bash", "-c", mod.Source).Output()
			if err == nil {
				strVal := strings.TrimSpace(string(out))
				if val, err := strconv.ParseFloat(strings.ReplaceAll(strVal, ",", "."), 64); err == nil {
					broadcastUpdate(Response{Type: "update", ID: mod.ID, Value: val})
				} else {
					broadcastUpdate(Response{Type: "update", ID: mod.ID, Content: strVal})
				}
			}
		}
		if mod.Children != nil {
			scanModules(mod.Children)
		}
	}
}

func watchClipboard() {
	var lastClip string
	for {
		out, err := exec.Command("wl-paste", "--no-newline").Output()
		if err == nil {
			curr := strings.TrimSpace(string(out))
			if curr != lastClip && curr != "" {
				lastClip = curr
				broadcastUpdate(Response{Type: "clipboard", Content: curr})
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func watchMediaStatus() {
	for {
		broadcastMediaStatus()
		time.Sleep(1 * time.Second) // Раз в секунду достаточно, Android сам интерполирует
	}
}

func BroadcastUpdate(cfg *config.UIConfig) {
	broadcastUpdate(Response{Type: "update_layout", Status: "update", Config: cfg})
}
