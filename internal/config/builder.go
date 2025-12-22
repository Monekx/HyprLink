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
		Profiles: []Tab{},
	}
	actions := make(map[string]string)

	cssData, _ := os.ReadFile(filepath.Join(basePath, "style.css"))
	ui.CSS = string(cssData)

	for _, p := range main.Profiles {
		tab := Tab{Name: p.Name, Modules: []Module{}}
		for _, modName := range p.Modules {
			modPath := filepath.Join(basePath, "modules", modName+".json")
			modData, err := os.ReadFile(modPath)
			if err != nil {
				continue
			}

			var mod Module
			if err := json.Unmarshal(modData, &mod); err == nil {
				extractActions(mod, actions)
				tab.Modules = append(tab.Modules, mod)
			}
		}
		ui.Profiles = append(ui.Profiles, tab)
	}

	uiBytes, _ := json.Marshal(ui)
	hash := sha256.Sum256(uiBytes)
	ui.Hash = hex.EncodeToString(hash[:])

	// После сборки всех профилей и извлечения действий:
	actionsPath := filepath.Join(basePath, "actions.json")
	actionsData, _ := json.MarshalIndent(actions, "", "  ")
	os.WriteFile(actionsPath, actionsData, 0644) // Записываем карту в отдельный файл

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
