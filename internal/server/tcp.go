package server

import (
	"context"
	"encoding/json"
	"fmt"
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

	getChan = make(chan map[string]interface{}, 10)
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
	go watchMediaStatus()
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleSession(conn)
	}
}

func handleSession(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	var firstReq Request
	if err := decoder.Decode(&firstReq); err != nil {
		conn.Close()
		return
	}

	if firstReq.Type == "get_request" {
		handleGetRequest(conn, firstReq)
		return
	}

	encoder := json.NewEncoder(conn)
	home, _ := os.UserHomeDir()
	trustedPath := filepath.Join(home, ".config", "hyprlink", "trusted_devices.json")
	os.MkdirAll(filepath.Dir(trustedPath), 0755)
	trustedDevices, _ := config.LoadTrustedDevices(trustedPath)

	isAuthorized := false
	var newID, newToken string
	if firstReq.DeviceID != "" && firstReq.Token != "" {
		if dev, ok := trustedDevices[firstReq.DeviceID]; ok && dev.Token == firstReq.Token {
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
	if firstReq.Hash != cfg.Hash {
		resp.Status = "update"
		resp.Config = cfg
	}
	encoder.Encode(resp)

	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return
		}

		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			continue
		}

		t, _ := data["type"].(string)
		if t == "sys_info" {
			fmt.Println("DEBUG: Received sys_info from phone")
			select {
			case getChan <- data:
			default:
				fmt.Println("DEBUG: getChan is full, skipping")
			}
			continue
		}

		handleIncomingMap(data)
	}
}

func handleIncomingMap(data map[string]interface{}) {
	t, _ := data["type"].(string)
	switch t {
	case "action":
		id, _ := data["id"].(string)
		val, _ := data["value"].(float64)
		go handleAction(id, val)
	case "clipboard":
		content, _ := data["content"].(string)
		if clean := strings.TrimSpace(content); clean != "" {
			go exec.Command("bash", "-c", fmt.Sprintf("echo -n %q | wl-copy", clean)).Run()
		}
	case "notification":
		app, _ := data["app"].(string)
		title, _ := data["title"].(string)
		content, _ := data["content"].(string)
		go exec.Command("notify-send", "-a", app, title, content).Run()
	case "ping":
		// Просто игнорируем пинг
	}
}

func handleGetRequest(conn net.Conn, req Request) {
	mu.Lock()
	var phoneEncoder *json.Encoder
	for _, enc := range clients {
		phoneEncoder = enc
		break
	}
	mu.Unlock()

	if phoneEncoder == nil {
		json.NewEncoder(conn).Encode(map[string]string{"error": "No devices connected"})
		conn.Close()
		return
	}

	for len(getChan) > 0 {
		<-getChan
	}

	fmt.Println("DEBUG: Sending get_request to phone...")
	phoneEncoder.Encode(req)

	select {
	case stats := <-getChan:
		fmt.Println("DEBUG: Forwarding stats to CLI")
		json.NewEncoder(conn).Encode(stats)
	case <-time.After(7 * time.Second):
		fmt.Println("DEBUG: Timeout reached")
		json.NewEncoder(conn).Encode(map[string]string{"error": "Timeout waiting for phone"})
	}
	conn.Close()
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
	posFloat, _ := strconv.ParseFloat(strings.TrimSpace(string(posRaw)), 64)
	posMs := int64(posFloat * 1000)
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
		Value:    float64(posMs),
		Duration: durMs,
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
		time.Sleep(1 * time.Second)
	}
}

func BroadcastUpdate(cfg *config.UIConfig) {
	broadcastUpdate(Response{Type: "update_layout", Status: "update", Config: cfg})
}
