package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		if r.Header.Get("x-api-key") != "" {
			t.Error("x-api-key should not be sent to OpenAI")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("bad request body: %v", err)
		}
		if req["model"] != "gpt-5-mini" || req["instructions"] != "sys" || req["input"] != "hello" {
			t.Errorf("unexpected request: %s", body)
		}
		_, _ = io.WriteString(w, `{"status":"completed","output":[
			{"type":"reasoning","summary":[]},
			{"type":"message","content":[{"type":"output_text","text":"{\"ok\":"},{"type":"output_text","text":"true}"}]}
		]}`)
	}))
	defer srv.Close()

	c := NewClient(ProviderOpenAI, "sk-test", "gpt-5-mini")
	c.BaseURL = srv.URL
	got, err := c.Call(context.Background(), "sys", "hello", 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("got %q", got)
	}
}

func TestOpenAIIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`)
	}))
	defer srv.Close()

	c := NewClient(ProviderOpenAI, "sk-test", "gpt-5-mini")
	c.BaseURL = srv.URL
	_, err := c.Call(context.Background(), "sys", "hello", 100)
	if err == nil || !strings.Contains(err.Error(), "max_output_tokens") {
		t.Fatalf("expected incomplete error, got %v", err)
	}
}

func TestAnthropicCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic headers: %v", r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		var req Request
		_ = json.Unmarshal(body, &req)
		if req.MaxTokens != 100 || req.System != "sys" || len(req.Messages) != 1 {
			t.Errorf("unexpected request: %s", body)
		}
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"hi"}]}`)
	}))
	defer srv.Close()

	c := NewClient(ProviderAnthropic, "sk-ant", "claude")
	c.BaseURL = srv.URL
	got, err := c.Call(context.Background(), "sys", "hello", 100)
	if err != nil || got != "hi" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestClientErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided"}}`)
	}))
	defer srv.Close()

	c := NewClient(ProviderOpenAI, "bad", "gpt-5-mini")
	c.BaseURL = srv.URL
	_, err := c.Call(context.Background(), "sys", "hello", 100)
	if err == nil || err.Error() != "OpenAI rejected the request (401): Incorrect API key provided" {
		t.Fatalf("got %v", err)
	}
}
