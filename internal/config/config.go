package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// XaaConfig mirrors the "xaa" section of config.json (same shape as the
// C# and TypeScript ports' appsettings.json / config.json).
type XaaConfig struct {
	ClientID        string `json:"clientId"`
	ClientSecret    string `json:"clientSecret"`
	IdpBaseURL      string `json:"idpBaseUrl"`
	McpClientID     string `json:"mcpClientId"`
	McpClientSecret string `json:"mcpClientSecret"`
	AuthServerURL   string `json:"authServerUrl"`
	McpServerURL    string `json:"mcpServerUrl"`
	ResourceURI     string `json:"resourceUri"`
	Scope           string `json:"scope"`
	RedirectURI     string `json:"redirectUri"`
	SessionSecret   string `json:"sessionSecret"`
}

// SamlConfig configures the alternate SAML SSO login path. The app runs
// fine without it — SAML routes are only mounted when Enabled is true.
type SamlConfig struct {
	Enabled        bool   `json:"enabled"`
	IdpMetadataURL string `json:"idpMetadataUrl"`
	SPCertFile     string `json:"spCertFile"`
	SPKeyFile      string `json:"spKeyFile"`
	SPRootURL      string `json:"spRootUrl"`
}

type Config struct {
	Xaa  XaaConfig  `json:"xaa"`
	Saml SamlConfig `json:"saml"`
	Port int        `json:"port"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	return &cfg, nil
}
