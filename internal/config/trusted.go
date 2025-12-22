package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
)

func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func SaveTrustedDevice(path string, device TrustedDevice) error {
	devices, _ := LoadTrustedDevices(path)
	devices[device.ID] = device
	data, _ := json.MarshalIndent(devices, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func LoadTrustedDevices(path string) (map[string]TrustedDevice, error) {
	devices := make(map[string]TrustedDevice)
	data, err := os.ReadFile(path)
	if err != nil {
		return devices, err
	}
	json.Unmarshal(data, &devices)
	return devices, nil
}
