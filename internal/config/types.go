// internal/config/types.go
package config

import "os"

type MultiSourceString struct {
	Data   string `yaml:"data"`
	EnvVar string `yaml:"env_var"`
}

func (m MultiSourceString) Get() string {
	if m.Data != "" {
		return m.Data
	}
	if m.EnvVar != "" {
		return os.Getenv(m.EnvVar)
	}
	return ""
}
