package config

type MainConfig struct {
	Hostname string    `json:"hostname"`
	Profiles []Profile `json:"profiles"`
}

type Profile struct {
	Name    string   `json:"name"`
	Modules []string `json:"modules"`
}

type UIConfig struct {
	Hostname string `json:"hostname"`
	Hash     string `json:"hash"`
	Profiles []Tab  `json:"profiles"`
	CSS      string `json:"css,omitempty"`
}

type Tab struct {
	Name    string   `json:"name"`
	Modules []Module `json:"modules"`
}

type Module struct {
	ID       string   `json:"id"` // Изменил Id на ID для соответствия builder.go
	Type     string   `json:"type"`
	Label    string   `json:"label,omitempty"`
	View     string   `json:"view,omitempty"`
	Icon     string   `json:"icon,omitempty"`
	Children []Module `json:"children,omitempty"`
	Action   string   `json:"action,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type TrustedDevice struct {
	ID    string `json:"id"`
	Token string `json:"token"`
	Name  string `json:"name"`
}
