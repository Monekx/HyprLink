package config

import (
	"log"

	"github.com/fsnotify/fsnotify"
)

func WatchConfig(basePath string, onWrite func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					onWrite()
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
	watcher.Add(basePath + "/modules")
}
