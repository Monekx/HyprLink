package config

import "gopkg.in/yaml.v3"

type MainConfig struct {
	Hostname string    `yaml:"hostname"`
	Profiles []Profile `yaml:"profiles"`
}

type Profile struct {
	Name    string   `yaml:"name"`
	Modules []Module `yaml:"modules"`
	Import  string   `yaml:"import,omitempty"`
}

func (p *Profile) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		p.Import = value.Value
		return nil
	}
	type alias Profile
	return value.Decode((*alias)(p))
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
	ID       string   `json:"id" yaml:"id"`
	Type     string   `json:"type" yaml:"type"`
	Label    string   `json:"label,omitempty" yaml:"label,omitempty"`
	View     string   `json:"view,omitempty" yaml:"view,omitempty"`
	Icon     string   `json:"icon,omitempty" yaml:"icon,omitempty"`
	Children []Module `json:"children,omitempty" yaml:"children,omitempty"`

	Action       string `json:"action,omitempty" yaml:"-"`
	ConfigAction string `json:"-" yaml:"action,omitempty"`

	Source string `json:"-" yaml:"source,omitempty"`
	Import string `json:"-" yaml:"import,omitempty"`
}

func (m *Module) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		m.Import = value.Value
		return nil
	}
	type alias Module
	return value.Decode((*alias)(m))
}

type TrustedDevice struct {
	ID    string `yaml:"id"`
	Token string `yaml:"token"`
	Name  string `yaml:"name"`
}
