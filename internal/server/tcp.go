package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Monekx/hyprlink/internal/config"
)

type Request struct {
	Type    string  `json:"type"`
	Pin     string  `json:"pin,omitempty"`
	Hash    string  `json:"hash,omitempty"`
	ID      string  `json:"id,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Content string  `json:"content,omitempty"`
}

type Response struct {
	Type    string           `json:"type,omitempty"`
	Status  string           `json:"status,omitempty"`
	Message string           `json:"message,omitempty"`
	Config  *config.UIConfig `json:"config,omitempty"`
	ID      string           `json:"id,omitempty"`
	Value   float64          `json:"value,omitempty"`
	Title   string           `json:"title,omitempty"`
	Content string           `json:"content,omitempty"`
}

var (
	clients = make(map[net.Conn]*json.Encoder)
	mu      sync.Mutex
)

func StartTCPServer(port int, cfg *config.UIConfig, pin string, actions map[string]string) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return
	}

	go startUpdateLoop(cfg)
	go watchClipboard()

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleSession(conn, cfg, pin, actions)
	}
}

func watchClipboard() {
	var lastClip string
	for {
		out, err := exec.Command("wl-paste", "--no-newline").Output()
		if err == nil {
			curr := string(out)
			if curr != lastClip && curr != "" {
				lastClip = curr
				broadcastClipboard(curr)
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func broadcastClipboard(content string) {
	mu.Lock()
	defer mu.Unlock()
	resp := Response{Type: "clipboard", Content: content}
	for _, encoder := range clients {
		encoder.Encode(resp)
	}
}

func startUpdateLoop(cfg *config.UIConfig) {
	for {
		for _, mod := range cfg.Modules {
			if mod.Source != "" {
				out, err := exec.Command("/bin/bash", "-c", mod.Source).Output()
				if err == nil {
					strVal := strings.TrimSpace(string(out))
					val, err := strconv.ParseFloat(strings.ReplaceAll(strVal, ",", "."), 64)
					if err == nil {
						broadcastValue(mod.ID, val)
					}
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func broadcastValue(id string, value float64) {
	mu.Lock()
	defer mu.Unlock()
	resp := Response{Type: "update", ID: id, Value: value}
	for conn, encoder := range clients {
		if err := encoder.Encode(resp); err != nil {
			conn.Close()
			delete(clients, conn)
		}
	}
}

func handleSession(conn net.Conn, cfg *config.UIConfig, pin string, actions map[string]string) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	var req Request
	if err := decoder.Decode(&req); err != nil {
		conn.Close()
		return
	}

	if req.Pin != pin {
		encoder.Encode(Response{Status: "error", Message: "Wrong PIN"})
		conn.Close()
		return
	}

	mu.Lock()
	clients[conn] = encoder
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(clients, conn)
		mu.Unlock()
		conn.Close()
	}()

	status := "ok"
	if req.Hash != cfg.Hash {
		status = "update"
	}
	encoder.Encode(Response{Status: status, Config: cfg})

	for {
		var action Request
		if err := decoder.Decode(&action); err != nil {
			break
		}
		switch action.Type {
		case "action":
			if cmdStr, ok := actions[action.ID]; ok {
				valStr := fmt.Sprintf("%.0f", action.Value)
				finalCmd := strings.ReplaceAll(cmdStr, "{v}", valStr)
				go exec.Command("/bin/bash", "-c", finalCmd).Run()
			}
		case "clipboard":
			go exec.Command("bash", "-c", fmt.Sprintf("echo -n %q | wl-copy", action.Content)).Run()
		}
	}
}

func SendNotification(title, message string) {
	mu.Lock()
	defer mu.Unlock()
	resp := Response{
		Type:    "notification",
		Title:   title,
		Message: message,
	}
	for _, encoder := range clients {
		encoder.Encode(resp)
	}
}

func BroadcastUpdate(cfg *config.UIConfig) {
	mu.Lock()
	defer mu.Unlock()
	resp := Response{
		Type:   "update_layout",
		Status: "update",
		Config: cfg,
	}
	for _, encoder := range clients {
		encoder.Encode(resp)
	}
}
