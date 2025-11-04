package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

const (
	envConfigPath = "AGENCIA_CONFIG"
)

// Duration wraps time.Duration to provide YAML decoding with "set" semantics so
// defaults can be preserved when the user omits a field.
type Duration struct {
	value time.Duration
	set   bool
}

// DurationFrom constructs a Duration that is explicitly set to the provided value.
func DurationFrom(d time.Duration) Duration {
	return Duration{value: d, set: true}
}

// Duration returns the underlying duration value.
func (d Duration) Duration() time.Duration {
	return d.value
}

// IsSet reports whether the duration was explicitly configured.
func (d Duration) IsSet() bool {
	return d.set
}

// UnmarshalYAML parses duration strings (e.g. "30s", "1m") or numeric seconds.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar value, got kind %d", value.Kind)
	}

	// Try to decode as a string duration first.
	var asString string
	if err := value.Decode(&asString); err == nil {
		parsed, err := time.ParseDuration(asString)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", asString, err)
		}
		d.value = parsed
		d.set = true
		return nil
	}

	// Fall back to interpreting numeric values as seconds.
	var asFloat float64
	if err := value.Decode(&asFloat); err == nil {
		d.value = time.Duration(asFloat * float64(time.Second))
		d.set = true
		return nil
	}

	return fmt.Errorf("duration should be a string (e.g. \"30s\") or number of seconds, got %q", value.Value)
}

type OpenAIConfig struct {
	Enabled               bool     `yaml:"enabled"`
	APIBase               string   `yaml:"api_base"`
	Organization          string   `yaml:"organization"`
	Model                 string   `yaml:"model"`
	Temperature           *float32 `yaml:"temperature"`
	MaxTokens             int      `yaml:"max_tokens"`
	RequestTimeout        Duration `yaml:"request_timeout"`
	RateLimitPerMinute    int      `yaml:"rate_limit_per_minute"`
	MaxConcurrentRequests int      `yaml:"max_concurrent_requests"`
	MaxCallsPerRun        int      `yaml:"max_calls_per_run"`
	RetryAttempts         int      `yaml:"retry_attempts"`
	RetryInitialBackoff   Duration `yaml:"retry_initial_backoff"`
	RetryMaxBackoff       Duration `yaml:"retry_max_backoff"`
	LogPrompts            bool     `yaml:"log_prompts"`
}

type Config struct {
	OpenAI OpenAIConfig `yaml:"openai"`

	sourcePath string
}

func defaultConfig() *Config {
	defaultTemp := float32(0.2)
	return &Config{
		OpenAI: OpenAIConfig{
			Enabled:               true,
			Model:                 openai.GPT4o,
			Temperature:           &defaultTemp,
			RequestTimeout:        DurationFrom(60 * time.Second),
			RetryAttempts:         2,
			RetryInitialBackoff:   DurationFrom(2 * time.Second),
			RetryMaxBackoff:       DurationFrom(15 * time.Second),
			MaxConcurrentRequests: 0,
			MaxCallsPerRun:        0,
		},
	}
}

func (c *Config) applyDefaults() {
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = openai.GPT4o
	}
	if c.OpenAI.Temperature == nil {
		defaultTemp := float32(0.2)
		c.OpenAI.Temperature = &defaultTemp
	}
	if !c.OpenAI.RequestTimeout.IsSet() {
		c.OpenAI.RequestTimeout = DurationFrom(60 * time.Second)
	}
	if c.OpenAI.RetryAttempts < 0 {
		c.OpenAI.RetryAttempts = 0
	}
	if !c.OpenAI.RetryInitialBackoff.IsSet() {
		c.OpenAI.RetryInitialBackoff = DurationFrom(2 * time.Second)
	}
	if !c.OpenAI.RetryMaxBackoff.IsSet() {
		c.OpenAI.RetryMaxBackoff = DurationFrom(15 * time.Second)
	}
}

func (c *Config) validate() error {
	if c.OpenAI.Temperature != nil {
		if *c.OpenAI.Temperature < 0 || *c.OpenAI.Temperature > 2 {
			return fmt.Errorf("openai.temperature must be between 0 and 2, got %f", *c.OpenAI.Temperature)
		}
	}
	if c.OpenAI.MaxTokens < 0 {
		return errors.New("openai.max_tokens cannot be negative")
	}
	if c.OpenAI.RateLimitPerMinute < 0 {
		return errors.New("openai.rate_limit_per_minute cannot be negative")
	}
	if c.OpenAI.MaxConcurrentRequests < 0 {
		return errors.New("openai.max_concurrent_requests cannot be negative")
	}
	if c.OpenAI.MaxCallsPerRun < 0 {
		return errors.New("openai.max_calls_per_run cannot be negative")
	}
	if c.OpenAI.RetryAttempts < 0 {
		return errors.New("openai.retry_attempts cannot be negative")
	}
	backoffMin := c.OpenAI.RetryInitialBackoff.Duration()
	backoffMax := c.OpenAI.RetryMaxBackoff.Duration()
	if backoffMax > 0 && backoffMin > backoffMax {
		return fmt.Errorf("openai.retry_max_backoff (%s) must be greater than retry_initial_backoff (%s)", backoffMax, backoffMin)
	}
	return nil
}

var (
	cfgMu  sync.RWMutex
	cached *Config
	cfgErr error
	loaded bool
)

// Get returns the cached configuration, loading it from disk on the first call.
func Get() (*Config, error) {
	cfgMu.RLock()
	if loaded {
		defer cfgMu.RUnlock()
		return cached, cfgErr
	}
	cfgMu.RUnlock()
	return Reload()
}

// Reload forces the configuration to be reloaded from disk.
func Reload() (*Config, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	cfg, err := load()
	cached = cfg
	cfgErr = err
	loaded = true
	return cached, cfgErr
}

// SetForTests overrides the cached configuration. Intended for tests.
func SetForTests(cfg *Config) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	cached = cfg
	cfgErr = nil
	loaded = true
}

func load() (*Config, error) {
	cfg := defaultConfig()

	path, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}
	if path == "" {
		cfg.applyDefaults()
		return cfg, cfg.validate()
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	cfg.sourcePath = path
	cfg.applyDefaults()
	return cfg, cfg.validate()
}

func resolveConfigPath() (string, error) {
	explicit := strings.TrimSpace(os.Getenv(envConfigPath))
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Clean(explicit)
		}
		if _, err := os.Stat(explicit); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("config file %s (from %s) not found", explicit, envConfigPath)
			}
			return "", fmt.Errorf("cannot access config file %s: %w", explicit, err)
		}
		return explicit, nil
	}

	candidates := []string{
		"agencia.config.yaml",
		"agencia.config.yml",
		filepath.Join("config", "agencia.yaml"),
		filepath.Join("config", "agencia.yml"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("checking config file %s: %w", candidate, err)
		}
	}
	return "", nil
}
