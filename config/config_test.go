package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
)

func TestDefaultConfig(t *testing.T) {
	t.Cleanup(func() {
		SetForTests(defaultConfig())
	})

	tempDir := t.TempDir()
	emptyPath := filepath.Join(tempDir, "empty.yaml")
	if err := os.WriteFile(emptyPath, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty config file: %v", err)
	}
	t.Setenv(envConfigPath, emptyPath)

	cfg, err := Reload()
	if err != nil {
		t.Fatalf("Reload(): unexpected error %v", err)
	}

	if cfg.OpenAI.Enabled != true {
		t.Fatalf("expected OpenAI enabled by default")
	}
	if cfg.OpenAI.Model != openai.GPT4o {
		t.Fatalf("expected default model %s, got %s", openai.GPT4o, cfg.OpenAI.Model)
	}
	if got := cfg.OpenAI.RequestTimeout.Duration(); got != time.Minute {
		t.Fatalf("expected default request timeout 1m, got %s", got)
	}
	if cfg.OpenAI.RetryAttempts != 2 {
		t.Fatalf("expected default retry attempts 2, got %d", cfg.OpenAI.RetryAttempts)
	}
}

func TestConfigFromFile(t *testing.T) {
	t.Cleanup(func() {
		SetForTests(defaultConfig())
	})

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "agencia.yml")
	custom := `
openai:
  enabled: false
  api_base: https://api.example.com/v1
  organization: test-org
  model: gpt-4.1-mini
  temperature: 1.1
  max_tokens: 1024
  request_timeout: 45s
  rate_limit_per_minute: 42
  max_concurrent_requests: 3
  max_calls_per_run: 5
  retry_attempts: 4
  retry_initial_backoff: 500ms
  retry_max_backoff: 3s
  log_prompts: true
`
	if err := os.WriteFile(configPath, []byte(custom), 0o600); err != nil {
		t.Fatalf("write custom config: %v", err)
	}
	t.Setenv(envConfigPath, configPath)

	cfg, err := Reload()
	if err != nil {
		t.Fatalf("Reload(): unexpected error %v", err)
	}

	if cfg.OpenAI.Enabled {
		t.Fatalf("expected OpenAI disabled from config")
	}
	if cfg.OpenAI.APIBase != "https://api.example.com/v1" {
		t.Fatalf("unexpected api base: %s", cfg.OpenAI.APIBase)
	}
	if cfg.OpenAI.Organization != "test-org" {
		t.Fatalf("unexpected organization: %s", cfg.OpenAI.Organization)
	}
	if cfg.OpenAI.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected model: %s", cfg.OpenAI.Model)
	}
	if cfg.OpenAI.Temperature == nil || *cfg.OpenAI.Temperature != 1.1 {
		t.Fatalf("unexpected temperature: %v", cfg.OpenAI.Temperature)
	}
	if cfg.OpenAI.MaxTokens != 1024 {
		t.Fatalf("unexpected max tokens: %d", cfg.OpenAI.MaxTokens)
	}
	if got := cfg.OpenAI.RequestTimeout.Duration(); got != 45*time.Second {
		t.Fatalf("unexpected request timeout: %s", got)
	}
	if cfg.OpenAI.RateLimitPerMinute != 42 {
		t.Fatalf("unexpected rate limit: %d", cfg.OpenAI.RateLimitPerMinute)
	}
	if cfg.OpenAI.MaxConcurrentRequests != 3 {
		t.Fatalf("unexpected max concurrent: %d", cfg.OpenAI.MaxConcurrentRequests)
	}
	if cfg.OpenAI.MaxCallsPerRun != 5 {
		t.Fatalf("unexpected max calls per run: %d", cfg.OpenAI.MaxCallsPerRun)
	}
	if cfg.OpenAI.RetryAttempts != 4 {
		t.Fatalf("unexpected retry attempts: %d", cfg.OpenAI.RetryAttempts)
	}
	if got := cfg.OpenAI.RetryInitialBackoff.Duration(); got != 500*time.Millisecond {
		t.Fatalf("unexpected retry initial backoff: %s", got)
	}
	if got := cfg.OpenAI.RetryMaxBackoff.Duration(); got != 3*time.Second {
		t.Fatalf("unexpected retry max backoff: %s", got)
	}
	if !cfg.OpenAI.LogPrompts {
		t.Fatalf("expected log_prompts to be true")
	}
}
