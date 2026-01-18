package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Monekx/hyprlink/internal/config"
	"github.com/gorilla/websocket"
)

// === Структуры данных ===

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
	Payload  string  `json:"payload,omitempty"` // Для команд
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

// === Глобальные переменные ===

var (
	currentPin string
	pinMutex   sync.Mutex

	// Храним активные WS соединения
	clients = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex

	// Конфигурация
	configMu       sync.RWMutex
	currentConfig  *config.UIConfig
	currentActions map[string]string

	// Канал для передачи данных от телефона к CLI (через HTTP запрос)
	getChan = make(chan map[string]interface{}, 1) // Буфер 1, так как запрос синхронный

	// Настройка WebSocket
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Разрешаем всем (удобно для локальной разработки)
		},
	}
)

// === Основные функции сервера ===

// UpdateConfig обновляет текущую конфигурацию в памяти
func UpdateConfig(cfg *config.UIConfig, actions map[string]string) {
	configMu.Lock()
	defer configMu.Unlock()
	currentConfig = cfg
	currentActions = actions
}

// StartServer запускает HTTP сервер с поддержкой WebSockets
func StartServer(port int, cfg *config.UIConfig, actions map[string]string) {
	UpdateConfig(cfg, actions)

	// Роутинг
	http.HandleFunc("/ws", handleWsConnection)
	http.HandleFunc("/api/get", handleGetAPI) // Endpoint для CLI команд

	// Запуск фоновых процессов
	go startUpdateLoop()
	go watchClipboard()
	go watchMediaStatus()

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting WebSocket server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe error:", err)
	}
}

// === Обработка WebSocket соединений ===

func handleWsConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	log.Printf("Client connected: %s", conn.RemoteAddr())

	// 1. Читаем запрос авторизации
	var req Request
	if err := conn.ReadJSON(&req); err != nil {
		log.Println("Read error:", err)
		return
	}

	// 2. Логика авторизации
	home, _ := os.UserHomeDir()
	trustedPath := filepath.Join(home, ".config", "hyprlink", "trusted_devices.json")
	// Создаем папку если нет
	os.MkdirAll(filepath.Dir(trustedPath), 0755) 
	
	trustedDevices, _ := config.LoadTrustedDevices(trustedPath)
	isAuthorized := false
	var newID, newToken string

	// Проверка по токену
	if req.DeviceID != "" && req.Token != "" {
		if dev, ok := trustedDevices[req.DeviceID]; ok && dev.Token == req.Token {
			isAuthorized = true
			newID = req.DeviceID
			newToken = req.Token
		}
	}

	// Если не авторизован по токену, проверяем PIN
	if !isAuthorized {
		// Если PIN не прислан, генерируем его и просим клиента ввести
		if req.Pin == "" {
			pin := generateAndNotifyPin()
			conn.WriteJSON(Response{Status: "unauthorized", Message: "PIN_REQUIRED"})
			
			// Ждем ответа с PIN (с таймаутом)
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			var authReq Request
			if err := conn.ReadJSON(&authReq); err != nil {
				return
			}
			conn.SetReadDeadline(time.Time{}) // Сброс таймаута

			if authReq.Pin == pin && pin != "" {
				isAuthorized = true
				newID = "phone-" + config.GenerateToken()[:8]
				newToken = config.GenerateToken()
				config.SaveTrustedDevice(trustedPath, config.TrustedDevice{
					ID: newID, Token: newToken, Name: "Android Device",
				})
			}
		} else {
			// Если клиент сразу прислал PIN (например, повторная попытка)
			if req.Pin == currentPin && currentPin != "" {
				isAuthorized = true
				newID = "phone-" + config.GenerateToken()[:8]
				newToken = config.GenerateToken()
				config.SaveTrustedDevice(trustedPath, config.TrustedDevice{
					ID: newID, Token: newToken, Name: "Android Device",
				})
			}
		}
	}

	if !isAuthorized {
		conn.WriteJSON(Response{Status: "error", Message: "INVALID_PIN"})
		return
	}

	// 3. Регистрация успешного соединения
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
	}()

	// 4. Отправка конфига
	configMu.RLock()
	cfg := currentConfig
	configMu.RUnlock()

	resp := Response{Status: "authorized", Type: "auth", DeviceID: newID, Token: newToken}
	// Проверяем хэш конфига, если нужно обновить
	if req.Hash != cfg.Hash {
		resp.Status = "update" // Клиент поймет, что нужно обновить конфиг
		resp.Config = cfg
	}
	conn.WriteJSON(resp)

	// 5. Основной цикл чтения сообщений
	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS Error: %v", err)
			}
			break
		}

		t, _ := msg["type"].(string)
		
		// Обработка ответа на sys_info (для CLI)
		if t == "sys_info" {
			select {
			case getChan <- msg:
			case <-time.After(1 * time.Second):
				// Никто не ждет
			}
			continue
		}

		handleIncomingMap(msg)
	}
}

// === Обработка HTTP запросов (для CLI) ===

func handleGetAPI(w http.ResponseWriter, r *http.Request) {
	// Очищаем старые данные из канала
	select {
	case <-getChan:
	default:
	}

	// Ищем подключенного клиента
	clientsMu.Lock()
	var activeConn *websocket.Conn
	for conn := range clients {
		activeConn = conn
		break // Берем первого попавшегося
	}
	clientsMu.Unlock()

	if activeConn == nil {
		http.Error(w, `{"error": "No devices connected"}`, http.StatusServiceUnavailable)
		return
	}

	targetID := r.URL.Query().Get("id")
	
	// Отправляем запрос на телефон через WS
	req := Request{Type: "get_request", ID: targetID}
	if err := activeConn.WriteJSON(req); err != nil {
		http.Error(w, `{"error": "Failed to send request to device"}`, http.StatusInternalServerError)
		return
	}

	// Ждем ответа
	select {
	case data := <-getChan:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	case <-time.After(5 * time.Second):
		http.Error(w, `{"error": "Timeout waiting for device"}`, http.StatusGatewayTimeout)
	}
}

// === Вспомогательная логика ===

func generateAndNotifyPin() string {
	pinMutex.Lock()
	defer pinMutex.Unlock()
	currentPin = fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	exec.Command("notify-send", "-a", "HyprLink", "Запрос подключения",
		fmt.Sprintf("Введите PIN-код на устройстве: %s", currentPin)).Run()
	return currentPin
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
		// Pings обрабатываются на уровне протокола WS, но можно оставить
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

func broadcastUpdate(resp Response) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	
	for conn := range clients {
		if err := conn.WriteJSON(resp); err != nil {
			log.Printf("Write error: %v", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

// BroadcastUpdate - публичный метод для уведомления об изменении конфига
func BroadcastUpdate(cfg *config.UIConfig) {
	broadcastUpdate(Response{Type: "update_layout", Status: "update", Config: cfg})
}

// === Фоновые задачи ===

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