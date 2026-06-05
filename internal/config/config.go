package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultProviderName        = "chutes"
	OpenRouterProviderName     = "openrouter"
	DefaultChutesBaseURL       = "https://llm.chutes.ai/v1"
	DefaultOpenRouterBaseURL   = "https://openrouter.ai/api/v1"
	DefaultChutesAPIKeyEnv     = "CHUTES_API_TOKEN"
	DefaultOpenRouterAPIKeyEnv = "OPENROUTER_API_KEY"
	DefaultChutesModel         = "Qwen/Qwen3.6-27B-TEE"
	DefaultOpenRouterModel     = "~openai/gpt-latest"
	DefaultWorkspace           = "."
	DefaultShellTimeoutSeconds = 20
	DefaultHTTPTimeoutSeconds  = 20
	DefaultToolOutputBytes     = 32768
)

// ProviderProfile stores the launch-time settings for one model provider.
type ProviderProfile struct {
	Name                string            `json:"name"`
	BaseURL             string            `json:"base_url"`
	APIKeyEnv           string            `json:"api_key_env"`
	Model               string            `json:"model"`
	ExtraHeaders        map[string]string `json:"extra_headers,omitempty"`
	SupportsStreaming   bool              `json:"supports_streaming"`
	SupportsToolCalling bool              `json:"supports_tool_calling"`
}

// TimeoutsConfig stores guardrails for bounded external work.
type TimeoutsConfig struct {
	ShellSeconds int `json:"shell_seconds"`
	HTTPSeconds  int `json:"http_seconds"`
}

// OutputLimitsConfig stores byte caps for tool output reinjection.
type OutputLimitsConfig struct {
	ToolBytes int `json:"tool_bytes"`
}

// Config contains the persisted launch defaults for a single CLI session.
type Config struct {
	Workspace       string                     `json:"workspace"`
	DefaultProvider string                     `json:"default_provider"`
	Streaming       bool                       `json:"streaming"`
	Timeouts        TimeoutsConfig             `json:"timeouts"`
	OutputLimits    OutputLimitsConfig         `json:"output_limits"`
	Providers       map[string]ProviderProfile `json:"providers"`
}

type rawConfig struct {
	Workspace       string                        `json:"workspace"`
	LegacyWorkspace string                        `json:"default_workspace"`
	DefaultProvider string                        `json:"default_provider"`
	Streaming       *bool                         `json:"streaming"`
	Timeouts        *rawTimeoutsConfig            `json:"timeouts"`
	OutputLimits    *rawOutputLimitsConfig        `json:"output_limits"`
	Providers       map[string]rawProviderProfile `json:"providers"`
}

type rawProviderProfile struct {
	Name                string            `json:"name"`
	BaseURL             string            `json:"base_url"`
	APIKeyEnv           string            `json:"api_key_env"`
	Model               string            `json:"model"`
	ExtraHeaders        map[string]string `json:"extra_headers"`
	SupportsStreaming   *bool             `json:"supports_streaming"`
	SupportsToolCalling *bool             `json:"supports_tool_calling"`
}

type rawTimeoutsConfig struct {
	ShellSeconds *int `json:"shell_seconds"`
	HTTPSeconds  *int `json:"http_seconds"`
}

type rawOutputLimitsConfig struct {
	ToolBytes *int `json:"tool_bytes"`
}

// Default returns the baseline provider layout described in the feature plan.
func Default() Config {
	return Config{
		Workspace:       DefaultWorkspace,
		DefaultProvider: DefaultProviderName,
		Streaming:       true,
		Timeouts: TimeoutsConfig{
			ShellSeconds: DefaultShellTimeoutSeconds,
			HTTPSeconds:  DefaultHTTPTimeoutSeconds,
		},
		OutputLimits: OutputLimitsConfig{
			ToolBytes: DefaultToolOutputBytes,
		},
		Providers: map[string]ProviderProfile{
			DefaultProviderName: {
				Name:                DefaultProviderName,
				BaseURL:             DefaultChutesBaseURL,
				APIKeyEnv:           DefaultChutesAPIKeyEnv,
				Model:               DefaultChutesModel,
				SupportsStreaming:   true,
				SupportsToolCalling: true,
			},
			OpenRouterProviderName: {
				Name:                OpenRouterProviderName,
				BaseURL:             DefaultOpenRouterBaseURL,
				APIKeyEnv:           DefaultOpenRouterAPIKeyEnv,
				Model:               DefaultOpenRouterModel,
				SupportsStreaming:   true,
				SupportsToolCalling: true,
			},
		},
	}
}

// DefaultConfigPath resolves the standard config file location.
func DefaultConfigPath() (string, error) {
	baseDir := os.Getenv("XDG_CONFIG_HOME")
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(baseDir, "goagent", "config.json"), nil
}

// ResolveConfigPath normalizes an explicit or implicit config path.
func ResolveConfigPath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return "", err
		}
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}

	return absPath, nil
}

// Load reads, normalizes, and validates the JSON config file.
func Load(path string) (Config, error) {
	configPath, err := ResolveConfigPath(path)
	if err != nil {
		return Config{}, err
	}

	file, err := os.Open(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()

	var raw rawConfig
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("decode config file: %w", err)
	}

	config := Default()
	mergeRawConfig(&config, raw)

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

// Validate checks whether the config supports the v1 runtime contract.
func (c Config) Validate() error {
	if err := validateWorkspace(c.Workspace); err != nil {
		return err
	}
	if !c.Streaming {
		return errors.New("streaming must remain enabled in v1")
	}
	if c.Timeouts.ShellSeconds <= 0 {
		return errors.New("timeouts.shell_seconds must be greater than zero")
	}
	if c.Timeouts.HTTPSeconds <= 0 {
		return errors.New("timeouts.http_seconds must be greater than zero")
	}
	if c.OutputLimits.ToolBytes <= 0 {
		return errors.New("output_limits.tool_bytes must be greater than zero")
	}
	if len(c.Providers) == 0 {
		return errors.New("providers must define at least one profile")
	}

	defaultProvider, err := c.ResolveProviderName("")
	if err != nil {
		return err
	}

	for key, profile := range c.Providers {
		if err := validateProviderProfile(key, profile); err != nil {
			return err
		}
	}

	if _, ok := c.Providers[defaultProvider]; !ok {
		return fmt.Errorf("default provider %q is not configured", defaultProvider)
	}

	return nil
}

// ResolveWorkspace applies CLI override, config default, and cwd fallback rules.
func (c Config) ResolveWorkspace(override string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}

	workspace := strings.TrimSpace(override)
	if workspace == "" {
		workspace = strings.TrimSpace(c.Workspace)
	}
	if workspace == "" {
		workspace = DefaultWorkspace
	}

	if !filepath.IsAbs(workspace) {
		workspace = filepath.Join(cwd, workspace)
	}

	absPath, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %s", absPath)
	}

	return absPath, nil
}

// ResolveProviderName returns the launch provider name after applying defaults.
func (c Config) ResolveProviderName(override string) (string, error) {
	providerName := strings.TrimSpace(override)
	if providerName == "" {
		providerName = strings.TrimSpace(c.DefaultProvider)
	}
	if providerName == "" {
		providerName = DefaultProviderName
	}
	if _, ok := c.Providers[providerName]; !ok {
		return "", fmt.Errorf("provider %q is not configured", providerName)
	}

	return providerName, nil
}

// Provider resolves a named profile, falling back to the configured default.
func (c Config) Provider(name string) (ProviderProfile, bool) {
	resolvedName, err := c.ResolveProviderName(name)
	if err != nil {
		return ProviderProfile{}, false
	}

	profile, ok := c.Providers[resolvedName]
	return profile, ok
}

func mergeRawConfig(config *Config, raw rawConfig) {
	if raw.Workspace != "" {
		config.Workspace = raw.Workspace
	} else if raw.LegacyWorkspace != "" {
		config.Workspace = raw.LegacyWorkspace
	}
	if raw.DefaultProvider != "" {
		config.DefaultProvider = raw.DefaultProvider
	}
	if raw.Streaming != nil {
		config.Streaming = *raw.Streaming
	}
	if raw.Timeouts != nil {
		if raw.Timeouts.ShellSeconds != nil {
			config.Timeouts.ShellSeconds = *raw.Timeouts.ShellSeconds
		}
		if raw.Timeouts.HTTPSeconds != nil {
			config.Timeouts.HTTPSeconds = *raw.Timeouts.HTTPSeconds
		}
	}
	if raw.OutputLimits != nil && raw.OutputLimits.ToolBytes != nil {
		config.OutputLimits.ToolBytes = *raw.OutputLimits.ToolBytes
	}
	for key, rawProfile := range raw.Providers {
		baseProfile, ok := config.Providers[key]
		if !ok {
			baseProfile = ProviderProfile{
				Name:                key,
				SupportsStreaming:   true,
				SupportsToolCalling: true,
			}
		}

		if rawProfile.Name != "" {
			baseProfile.Name = rawProfile.Name
		} else if baseProfile.Name == "" {
			baseProfile.Name = key
		}
		if rawProfile.BaseURL != "" {
			baseProfile.BaseURL = rawProfile.BaseURL
		}
		if rawProfile.APIKeyEnv != "" {
			baseProfile.APIKeyEnv = rawProfile.APIKeyEnv
		}
		if rawProfile.Model != "" {
			baseProfile.Model = rawProfile.Model
		}
		if rawProfile.ExtraHeaders != nil {
			baseProfile.ExtraHeaders = cloneStringMap(rawProfile.ExtraHeaders)
		}
		if rawProfile.SupportsStreaming != nil {
			baseProfile.SupportsStreaming = *rawProfile.SupportsStreaming
		}
		if rawProfile.SupportsToolCalling != nil {
			baseProfile.SupportsToolCalling = *rawProfile.SupportsToolCalling
		}

		config.Providers[key] = baseProfile
	}
}

func validateWorkspace(workspace string) error {
	if strings.TrimSpace(workspace) == "" {
		return errors.New("workspace must not be empty")
	}

	return nil
}

func validateProviderProfile(key string, profile ProviderProfile) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("provider name must not be empty")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("provider %q must set name", key)
	}
	if profile.Name != key {
		return fmt.Errorf("provider %q has mismatched name %q", key, profile.Name)
	}
	if strings.TrimSpace(profile.Model) == "" {
		return fmt.Errorf("provider %q must set model", key)
	}
	if strings.TrimSpace(profile.APIKeyEnv) == "" {
		return fmt.Errorf("provider %q must set api_key_env", key)
	}
	if !profile.SupportsStreaming {
		return fmt.Errorf("provider %q must support streaming", key)
	}
	if !profile.SupportsToolCalling {
		return fmt.Errorf("provider %q must support tool calling", key)
	}

	parsedURL, err := url.Parse(profile.BaseURL)
	if err != nil {
		return fmt.Errorf("provider %q has invalid base_url: %w", key, err)
	}
	if !parsedURL.IsAbs() {
		return fmt.Errorf("provider %q base_url must be absolute", key)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("provider %q base_url must use http or https", key)
	}

	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}
