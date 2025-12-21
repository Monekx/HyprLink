package config

type Module struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	View           string            `json:"view,omitempty"`
	Label          string            `json:"label,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	UpdateInterval string            `json:"update_interval,omitempty"`
	Source         string            `json:"source,omitempty"`
	Action         string            `json:"action,omitempty"`
	Styles         map[string]string `json:"styles,omitempty"`
	Children       []Module          `json:"children,omitempty"`
}

type MainConfig struct {
	Hostname string   `json:"hostname"`
	Pin      string   `json:"pin"`
	Modules  []string `json:"modules"`
}

type UIConfig struct {
	Hostname string   `json:"hostname"`
	Hash     string   `json:"hash"`
	Modules  []Module `json:"modules"`
	CSS      string   `json:"css"`
}
