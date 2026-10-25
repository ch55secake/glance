package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestThemeConfigUsesXDGOrDotConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := configPath()
	if err != nil || path != filepath.Join(root, "glance", "config.json") {
		t.Fatalf("XDG config path = %q, %v", path, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", root)
	path, err = configPath()
	if err != nil || path != filepath.Join(root, ".config", "glance", "config.json") {
		t.Fatalf("default config path = %q, %v", path, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	if _, err := configPath(); err == nil {
		t.Fatal("accepted a relative XDG_CONFIG_HOME")
	}
}

func TestThemeConfigPersistsOnlyAValidThemeWithPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if got, err := loadThemePreference(); err != nil || got != defaultThemeID {
		t.Fatalf("missing config = %q, %v", got, err)
	}
	if err := saveThemePreference("dracula"); err != nil {
		t.Fatal(err)
	}
	got, err := loadThemePreference()
	if err != nil || got != "dracula" {
		t.Fatalf("saved theme = %q, %v", got, err)
	}
	path, _ := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil || len(config) != 1 || config["theme"] != "dracula" {
		t.Fatalf("config contains unexpected values: %s, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %v, %v; want 0600", info, err)
	}
	if err := saveThemePreference("not-a-theme"); err == nil {
		t.Fatal("saved an unknown theme")
	}
}

func TestInvalidThemeConfigFallsBackToDefault(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, _ := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"sunset"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadThemePreference(); err == nil || got != defaultThemeID {
		t.Fatalf("unknown saved theme = %q, %v; want fallback and warning", got, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadThemePreference(); err == nil || got != defaultThemeID {
		t.Fatalf("invalid JSON theme = %q, %v; want fallback and warning", got, err)
	}
}
