package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config stores the user-editable configuration for DevMem.
type Config struct {
	Version       string                  `json:"version"`
	Modules       map[string]ModuleConfig `json:"modules,omitempty"`
	Ignore        []string                `json:"ignore"`
	AIModel       string                  `json:"ai_model"`
	MaxConcurrent int                     `json:"max_concurrent"`
}

// ModuleConfig maps a module name to one or more root paths.
type ModuleConfig struct {
	Paths []string `json:"paths"`
}

// State tracks the latest persisted runtime state.
type State struct {
	InitialisedAt string `json:"initialised_at"`
	LastCommit    string `json:"last_commit"`
	LastCapture   string `json:"last_capture,omitempty"`
	ModuleCount   int    `json:"module_count"`
}

// DefaultConfig returns the baseline configuration.
func DefaultConfig() *Config {
	return &Config{
		Version:       "1",
		Modules:       map[string]ModuleConfig{},
		Ignore:        []string{"node_modules", "vendor", ".git", "dist", "build", "__pycache__", ".venv", "*.lock", "*.sum", "*.min.js"},
		AIModel:       "claude-sonnet-4-20250514",
		MaxConcurrent: 5,
	}
}

// EnsureDirs creates all DevMem directories if they do not already exist.
func EnsureDirs(repoRoot string) error {
	paths := []string{
		filepath.Join(repoRoot, ".devmem"),
		filepath.Join(repoRoot, ".devmem", "docs"),
		filepath.Join(repoRoot, ".devmem", "docs", "modules"),
		filepath.Join(repoRoot, ".devmem", "changelog"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", p, err)
		}
	}
	return nil
}

// LoadConfig loads .devmem/config.json or returns default config when missing.
func LoadConfig(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, ".devmem", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]ModuleConfig{}
	}
	if cfg.Version == "" {
		cfg.Version = "1"
	}
	if cfg.AIModel == "" {
		cfg.AIModel = "claude-sonnet-4-20250514"
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 5
	}
	if len(cfg.Ignore) == 0 {
		cfg.Ignore = DefaultConfig().Ignore
	}
	return &cfg, nil
}

// WriteConfig writes .devmem/config.json atomically.
func WriteConfig(repoRoot string, cfg *Config) error {
	if err := EnsureDirs(repoRoot); err != nil {
		return err
	}
	path := filepath.Join(repoRoot, ".devmem", "config.json")
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return atomicWrite(path, b, 0o644)
}

// LoadState loads .devmem/state.json.
func LoadState(repoRoot string) (*State, error) {
	path := filepath.Join(repoRoot, ".devmem", "state.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// WriteState writes .devmem/state.json atomically.
func WriteState(repoRoot string, s *State) error {
	if err := EnsureDirs(repoRoot); err != nil {
		return err
	}
	path := filepath.Join(repoRoot, ".devmem", "state.json")
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return atomicWrite(path, b, 0o644)
}

// WriteTree writes .devmem/tree.json atomically.
func WriteTree(repoRoot string, tree any) error {
	if err := EnsureDirs(repoRoot); err != nil {
		return err
	}
	path := filepath.Join(repoRoot, ".devmem", "tree.json")
	b, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tree: %w", err)
	}
	return atomicWrite(path, b, 0o644)
}

func atomicWrite(path string, payload []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, mode); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
