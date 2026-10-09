package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
)

// Providers lists the supported AI providers in display order.
var Providers = []string{ProviderAnthropic, ProviderOpenAI}

// ValidProvider reports whether p is a supported provider.
func ValidProvider(p string) bool {
	return p == ProviderAnthropic || p == ProviderOpenAI
}

// ProviderLabel returns the human-readable name for a provider.
func ProviderLabel(p string) string {
	if p == ProviderOpenAI {
		return "OpenAI"
	}
	return "Anthropic"
}

// DefaultModel returns the model used when none is configured.
func DefaultModel(p string) string {
	if p == ProviderOpenAI {
		return "gpt-5-mini"
	}
	return "claude-sonnet-4-20250514"
}

// Client wraps interactions with the Anthropic and OpenAI APIs.
type Client struct {
	Provider   string
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

type openAIRequest struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions,omitempty"`
	Input        string `json:"input"`
}

type openAIResponse struct {
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

// NewClient creates a new API client for the given provider.
func NewClient(provider, apiKey, model string) *Client {
	baseURL := "https://api.anthropic.com/v1/messages"
	if provider == ProviderOpenAI {
		baseURL = "https://api.openai.com/v1/responses"
	}
	return &Client{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
		BaseURL:  baseURL,
		HTTPClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

// Call sends one prompt pair and returns the response text.
func (c *Client) Call(ctx context.Context, system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	b, err := c.marshalRequest(system, user, maxTokens)
	if err != nil {
		return "", fmt.Errorf("marshal AI request: %w", err)
	}

	backoff := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(b))
		if err != nil {
			return "", fmt.Errorf("create AI request: %w", err)
		}
		c.setHeaders(req)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if attempt == 3 || ctx.Err() != nil {
				return "", fmt.Errorf("send AI request: %w", err)
			}
			if err := sleep(ctx, backoff[attempt]); err != nil {
				return "", err
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read AI response: %w", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			return c.parseResponse(body)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt == 3 {
				return "", fmt.Errorf("AI request failed after retries (%d): %s", resp.StatusCode, apiErrorMessage(body))
			}
			if err := sleep(ctx, backoff[attempt]); err != nil {
				return "", err
			}
			continue
		}

		return "", fmt.Errorf("%s rejected the request (%d): %s", ProviderLabel(c.Provider), resp.StatusCode, apiErrorMessage(body))
	}

	return "", fmt.Errorf("AI request failed unexpectedly")
}

func (c *Client) marshalRequest(system, user string, maxTokens int) ([]byte, error) {
	if c.Provider == ProviderOpenAI {
		// GPT-5 models spend part of the output budget on reasoning, so a
		// tight max_output_tokens tends to return empty answers. Leave it unset.
		return json.Marshal(openAIRequest{
			Model:        c.Model,
			Instructions: system,
			Input:        user,
		})
	}
	return json.Marshal(Request{
		Model:     c.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []Message{{Role: "user", Content: user}},
	})
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("content-type", "application/json")
	if c.Provider == ProviderOpenAI {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (c *Client) parseResponse(body []byte) (string, error) {
	if c.Provider == ProviderOpenAI {
		var out openAIResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return "", fmt.Errorf("parse AI response: %w", err)
		}
		var text strings.Builder
		for _, item := range out.Output {
			if item.Type != "message" {
				continue
			}
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text.WriteString(part.Text)
				}
			}
		}
		if text.Len() == 0 {
			if out.IncompleteDetails != nil && out.IncompleteDetails.Reason != "" {
				return "", fmt.Errorf("AI response was incomplete: %s", out.IncompleteDetails.Reason)
			}
			return "", fmt.Errorf("AI response did not contain any text (status %q)", out.Status)
		}
		return text.String(), nil
	}

	var out Response
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse AI response: %w", err)
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("AI response did not contain content blocks")
	}
	return out.Content[0].Text, nil
}

// apiErrorMessage pulls error.message out of an API error body. Both
// providers use that shape; anything else is returned as-is.
func apiErrorMessage(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(body))
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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
