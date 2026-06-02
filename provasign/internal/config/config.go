package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type JiraProjectMapping struct {
	Project       string `yaml:"project"`
	Domain        string `yaml:"domain"`
	TriggerStatus string `yaml:"trigger_status"`
	AutoApprove   bool   `yaml:"auto_approve"`
}

type JiraConfig struct {
	URL              string               `yaml:"url"`
	Token            string               `yaml:"token"`
	WebhookSecret    string               `yaml:"webhook_secret"`
	RelayProjectField string              `yaml:"provasign_project_field"` // Jira custom field name, e.g. "Relay Project"
	ProjectMappings  []JiraProjectMapping `yaml:"project_mappings"`
}

type GitHubConfig struct {
	Token         string `yaml:"token"`
	WebhookSecret string `yaml:"webhook_secret"`
	LabelTrigger  string `yaml:"label_trigger"`
	AutoApprove   bool   `yaml:"auto_approve"`
}

type Config struct {
	Port         int          `yaml:"port"`
	GroveURL     string       `yaml:"grove_url"`
	DBURL        string       `yaml:"db_url"`
	TokenFile    string       `yaml:"token_file"`
	GSThreshold  float64      `yaml:"gs_threshold"`
	Jira         JiraConfig   `yaml:"jira"`
	GitHub       GitHubConfig `yaml:"github"`
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	applyEnv(cfg)
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Port:        9000,
		GroveURL:    "http://localhost:7777",
		DBURL:       "postgres://localhost/relay",
		TokenFile:   ".provasign/.token",
		GSThreshold: 0.70,
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("RELAY_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("GROVE_URL"); v != "" {
		cfg.GroveURL = v
	}
	if v := os.Getenv("RELAY_DB_URL"); v != "" {
		cfg.DBURL = v
	}
	if v := os.Getenv("RELAY_GS_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.GSThreshold = f
		}
	}
	if v := os.Getenv("JIRA_API_TOKEN"); v != "" {
		cfg.Jira.Token = v
	}
	if v := os.Getenv("JIRA_WEBHOOK_SECRET"); v != "" {
		cfg.Jira.WebhookSecret = v
	}
	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		cfg.GitHub.Token = v
	}
	if v := os.Getenv("GITHUB_WEBHOOK_SECRET"); v != "" {
		cfg.GitHub.WebhookSecret = v
	}
}
