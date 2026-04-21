// Package config loads the global (~/.acorn/config.toml) and per-project
// (.acorn/config.toml) config files. Project overrides global.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	ServerURL string `toml:"server"`
	TeamKey   string `toml:"team_key"`
	User      string `toml:"user"`
	Theme     string `toml:"theme"`
	// PlanMode starts the session in plan mode when true (Shift+Tab toggles).
	PlanMode bool `toml:"plan_mode"`
}

// Load reads ~/.acorn/config.toml and cwd/.acorn/config.toml, with the
// per-project file overriding the global.
func Load(cwd string) (*Config, error) {
	cfg := &Config{Theme: "default", User: defaultUser()}

	if home, err := os.UserHomeDir(); err == nil {
		global := filepath.Join(home, ".acorn", "config.toml")
		if err := mergeIfExists(global, cfg); err != nil {
			return nil, fmt.Errorf("global config: %w", err)
		}
	}

	local := filepath.Join(cwd, ".acorn", "config.toml")
	if err := mergeIfExists(local, cfg); err != nil {
		return nil, fmt.Errorf("local config: %w", err)
	}
	return cfg, nil
}

func mergeIfExists(path string, dst *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, err = toml.Decode(string(data), dst)
	return err
}

func defaultUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "operator"
}
