package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmurray/pkl-proxy/gen/appconfig"
)

// configFiles is the priority-ordered list of config file names to try.
var configFiles = []struct {
	name   string
	loader func(path string) (*appconfig.AppConfig, error)
}{
	{"config.pklbin", loadPkl},
	{"config.pkl", loadPkl},
	{"config.json", loadJSON},
}

func loadConfig(configDir string) (*appconfig.AppConfig, error) {
	for _, cf := range configFiles {
		path := filepath.Join(configDir, cf.name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		cfg, err := cf.loader(path)
		if err != nil {
			return nil, err
		}
		applyDefaults(cfg)
		return cfg, nil
	}
	return nil, fmt.Errorf("no config file found in %s (tried config.pklbin, config.pkl, config.json)", configDir)
}

func loadPkl(path string) (*appconfig.AppConfig, error) {
	cfg, err := appconfig.LoadFromPath(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("error loading pkl config %s: %w", path, err)
	}
	return &cfg, nil
}

func loadJSON(path string) (*appconfig.AppConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening json config %s: %w", path, err)
	}
	defer f.Close()

	var cfg appconfig.AppConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("error decoding json config %s: %w", path, err)
	}
	return &cfg, nil
}

func applyDefaults(cfg *appconfig.AppConfig) {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = "localhost:9443"
	}
}
