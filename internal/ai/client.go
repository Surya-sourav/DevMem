package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps interactions with the Anthropic messages API.
type Client struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

// Message is one message in an Anthropic request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is the Anthropic request payload.
type Request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
}

// Response is the Anthropic response payload subset used by DevMem.
type Response struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock is one content block in the response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewClient creates a new Anthropic API client.
func NewClient(apiKey, model string) *Client {
	return &Client{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.anthropic.com/v1/messages",
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Call sends one prompt pair and returns the first text content block.
func (c *Client) Call(ctx context.Context, system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	payload := Request{
		Model:     c.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []Message{{Role: "user", Content: user}},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal AI request: %w", err)
	}

	backoff := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(b))
		if err != nil {
			return "", fmt.Errorf("create AI request: %w", err)
		}
		req.Header.Set("x-api-key", c.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if attempt == 3 {
				return "", fmt.Errorf("send AI request: %w", err)
			}
			time.Sleep(backoff[attempt])
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read AI response: %w", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			var out Response
			if err := json.Unmarshal(body, &out); err != nil {
				return "", fmt.Errorf("parse AI response: %w", err)
			}
			if len(out.Content) == 0 {
				return "", fmt.Errorf("AI response did not contain content blocks")
			}
			return out.Content[0].Text, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt == 3 {
				return "", fmt.Errorf("AI request failed after retries (status %d): %s", resp.StatusCode, string(body))
			}
			time.Sleep(backoff[attempt])
			continue
		}

		return "", fmt.Errorf("AI request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return "", fmt.Errorf("AI request failed unexpectedly")
}

// AnalyseModule calls AI to produce one module analysis.
func (c *Client) AnalyseModule(ctx context.Context, input ModulePromptInput) (*ModuleAnalysis, error) {
	system, user, err := RenderModulePrompts(input)
	if err != nil {
		return nil, fmt.Errorf("render module prompt: %w", err)
	}
	raw, err := c.Call(ctx, system, user, 1600)
	if err != nil {
		return nil, err
	}
	parsed, err := ValidateModuleAnalysis(raw)
	if err != nil {
		return nil, fmt.Errorf("validate module analysis: %w; raw response: %s", err, raw)
	}
	return parsed, nil
}

// GenerateMaster calls AI to produce system-wide architecture analysis.
func (c *Client) GenerateMaster(ctx context.Context, input MasterPromptInput) (*MasterAnalysis, error) {
	system, user, err := RenderMasterPrompts(input)
	if err != nil {
		return nil, fmt.Errorf("render master prompt: %w", err)
	}
	raw, err := c.Call(ctx, system, user, 2200)
	if err != nil {
		return nil, err
	}
	parsed, err := ValidateMasterAnalysis(raw)
	if err != nil {
		return nil, fmt.Errorf("validate master analysis: %w; raw response: %s", err, raw)
	}
	return parsed, nil
}

// ClassifyChange calls AI to classify a git change.
func (c *Client) ClassifyChange(ctx context.Context, input ChangePromptInput) (*ChangeClassification, error) {
	system, user, err := RenderChangePrompts(input)
	if err != nil {
		return nil, fmt.Errorf("render change prompt: %w", err)
	}
	raw, err := c.Call(ctx, system, user, 700)
	if err != nil {
		return nil, err
	}
	parsed, err := ValidateChangeClassification(raw)
	if err != nil {
		return nil, fmt.Errorf("validate change classification: %w; raw response: %s", err, raw)
	}
	return parsed, nil
}

// PatchModuleDoc calls AI for targeted section patch suggestions.
func (c *Client) PatchModuleDoc(ctx context.Context, input PatchPromptInput) (*DocPatch, error) {
	system, user, err := RenderPatchPrompts(input)
	if err != nil {
		return nil, fmt.Errorf("render patch prompt: %w", err)
	}
	raw, err := c.Call(ctx, system, user, 1200)
	if err != nil {
		return nil, err
	}
	parsed, err := ValidateDocPatch(raw)
	if err != nil {
		return nil, fmt.Errorf("validate doc patch: %w; raw response: %s", err, raw)
	}
	return parsed, nil
}
