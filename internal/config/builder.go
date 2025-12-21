package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type FullConfig struct {
	UI      *UIConfig
	Actions map[string]string
}

func BuildFullConfig(basePath string) (*FullConfig, error) {
	mainData, err := os.ReadFile(filepath.Join(basePath, "main.json"))
	if err != nil {
		return nil, err
	}

	var main MainConfig
	if err := json.Unmarshal(mainData, &main); err != nil {
		return nil, err
	}

	ui := &UIConfig{
		Hostname: main.Hostname,
		Modules:  []Module{},
	}
	actions := make(map[string]string)

	cssData, _ := os.ReadFile(filepath.Join(basePath, "style.css"))
	ui.CSS = string(cssData)

	for _, modName := range main.Modules {
		modPath := filepath.Join(basePath, "modules", modName+".json")
		modData, err := os.ReadFile(modPath)
		if err != nil {
			continue
		}

		var mod Module
		json.Unmarshal(modData, &mod)

		extractActions(mod, actions)
		ui.Modules = append(ui.Modules, mod)
	}

	uiBytes, _ := json.Marshal(ui)
	hash := sha256.Sum256(uiBytes)
	ui.Hash = hex.EncodeToString(hash[:])

	return &FullConfig{UI: ui, Actions: actions}, nil
}

func extractActions(mod Module, actions map[string]string) {
	if mod.Action != "" {
		actions[mod.ID] = mod.Action
	}
	if mod.Children != nil {
		for _, child := range mod.Children {
			extractActions(child, actions)
		}
	}
}
