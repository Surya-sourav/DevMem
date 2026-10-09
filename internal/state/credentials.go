package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type credentials struct {
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	OpenAIAPIKey    string `json:"openai_api_key,omitempty"`
}

func (c *credentials) key(provider string) *string {
	if provider == "openai" {
		return &c.OpenAIAPIKey
	}
	return &c.AnthropicAPIKey
}

// CredentialsPath returns the location of the persisted API keys.
func CredentialsPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".devmem", "credentials.json")
}

func loadCredentials(repoRoot string) (credentials, error) {
	var c credentials
	b, err := os.ReadFile(CredentialsPath(repoRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return c, fmt.Errorf("read credentials: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse credentials: %w", err)
	}
	return c, nil
}

// LoadAPIKey loads the persisted API key for a provider from .devmem/credentials.json.
func LoadAPIKey(repoRoot, provider string) (string, error) {
	c, err := loadCredentials(repoRoot)
	if err != nil {
		return "", err
	}
	return *c.key(provider), nil
}

// SaveAPIKey persists the API key for a provider, keeping keys saved for other providers.
func SaveAPIKey(repoRoot, provider, apiKey string) error {
	if err := EnsureDirs(repoRoot); err != nil {
		return err
	}
	c, err := loadCredentials(repoRoot)
	if err != nil {
		return err
	}
	*c.key(provider) = apiKey
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return atomicWrite(CredentialsPath(repoRoot), b, 0o600)
}
