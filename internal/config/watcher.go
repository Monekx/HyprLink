package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func WatchConfig(basePath string, onWrite func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		var mu sync.Mutex
		var timer *time.Timer

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Игнорируем actions.json и временные файлы
				name := filepath.Base(event.Name)
				if name == "actions.json" || strings.HasPrefix(name, ".") {
					continue
				}

				if event.Has(fsnotify.Write) {
					mu.Lock()
					if timer != nil {
						timer.Stop()
					}
					// Задержка 100мс, чтобы не перезагружать конфиг на каждое микро-изменение
					timer = time.AfterFunc(100*time.Millisecond, func() {
						onWrite()
					})
					mu.Unlock()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("watcher error:", err)
			}
		}
	}()

	watcher.Add(basePath)
	// Проверяем существование папки перед добавлением
	modulesPath := filepath.Join(basePath, "modules")
	if _, err := os.Stat(modulesPath); err == nil {
		watcher.Add(modulesPath)
	}
}
