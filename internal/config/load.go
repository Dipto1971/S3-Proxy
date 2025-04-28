// internal/config/load.go
package config

import (
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads YAML config from a path.
func Load(path string) (*Config, error) {
    abs, _ := filepath.Abs(path)
    b, err := ioutil.ReadFile(abs)
    if err != nil {
        return nil, err
    }
    var cfg Config
    if err := yaml.Unmarshal(b, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
