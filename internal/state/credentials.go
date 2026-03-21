package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type credentials struct {
	AnthropicAPIKey string `json:"anthropic_api_key"`
}

// LoadAPIKey loads the persisted Anthropic API key from .devmem/credentials.json.
func LoadAPIKey(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, ".devmem", "credentials.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read credentials: %w", err)
	}
	var c credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}
	return c.AnthropicAPIKey, nil
}

// SaveAPIKey persists the Anthropic API key to .devmem/credentials.json.
func SaveAPIKey(repoRoot, apiKey string) error {
	if err := EnsureDirs(repoRoot); err != nil {
		return err
	}
	path := filepath.Join(repoRoot, ".devmem", "credentials.json")
	b, err := json.MarshalIndent(credentials{AnthropicAPIKey: apiKey}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return atomicWrite(path, b, 0o600)
}
