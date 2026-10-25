package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type appConfig struct {
	Theme string `json:"theme"`
}

func configPath() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	} else if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_CONFIG_HOME must be an absolute path")
	}
	return filepath.Join(root, "glance", "config.json"), nil
}

func loadThemePreference() (string, error) {
	path, err := configPath()
	if err != nil {
		return defaultThemeID, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultThemeID, nil
	}
	if err != nil {
		return defaultThemeID, err
	}
	var config appConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return defaultThemeID, fmt.Errorf("read %s: %w", path, err)
	}
	if _, ok := themeByID(config.Theme); !ok {
		return defaultThemeID, fmt.Errorf("unknown saved theme %q", config.Theme)
	}
	return config.Theme, nil
}

func saveThemePreference(id string) error {
	if _, ok := themeByID(id); !ok {
		return fmt.Errorf("unknown theme %q", id)
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".config-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if err := json.NewEncoder(temp).Encode(appConfig{Theme: id}); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
