package main

import (
	"fmt"
	"os"
	"strings"
)

type appConfig struct {
	BotToken                string
	GAPropertyID            string
	GAOAuthClientSecretFile string
	GAOAuthTokenFile        string
	GAOAuthClientSecretJSON string
	GAOAuthTokenJSON        string
	OAuthBootstrap          bool

	UpdateMode string

	WebhookListenAddr string
	WebhookPath       string
	WebhookURL        string
	WebhookSecret     string
}

const (
	updateModePolling = "polling"
	updateModeWebhook = "webhook"
)

func loadConfigFromEnv() (appConfig, error) {
	cfg := appConfig{
		BotToken:                os.Getenv("BOT_TOKEN"),
		GAPropertyID:            os.Getenv("GA4_PROPERTY_ID"),
		GAOAuthClientSecretFile: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET_FILE"),
		GAOAuthTokenFile:        os.Getenv("GOOGLE_OAUTH_TOKEN_FILE"),
		GAOAuthClientSecretJSON: strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET_JSON")),
		GAOAuthTokenJSON:        strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_TOKEN_JSON")),
		OAuthBootstrap:          strings.EqualFold(os.Getenv("OAUTH_BOOTSTRAP"), "1"),

		UpdateMode:        strings.ToLower(strings.TrimSpace(getEnvOrDefault("TELEGRAM_UPDATE_MODE", updateModePolling))),
		WebhookListenAddr: getEnvOrDefault("TELEGRAM_WEBHOOK_LISTEN_ADDR", ":8080"),
		WebhookPath:       getEnvOrDefault("TELEGRAM_WEBHOOK_PATH", "POST /telegram/webhook"),
		WebhookURL:        strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_URL")),
		WebhookSecret:     strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")),
	}

	if cfg.OAuthBootstrap {
		if cfg.GAOAuthClientSecretJSON == "" && cfg.GAOAuthClientSecretFile == "" {
			return cfg, fmt.Errorf("OAUTH_BOOTSTRAP=1 requires GOOGLE_OAUTH_CLIENT_SECRET_JSON or GOOGLE_OAUTH_CLIENT_SECRET_FILE")
		}
		return cfg, nil
	}

	if cfg.BotToken == "" || cfg.GAPropertyID == "" {
		return cfg, fmt.Errorf("missing required env: BOT_TOKEN, GA4_PROPERTY_ID")
	}
	if cfg.GAOAuthClientSecretJSON == "" && cfg.GAOAuthClientSecretFile == "" {
		return cfg, fmt.Errorf("missing required OAuth env: GOOGLE_OAUTH_CLIENT_SECRET_JSON or GOOGLE_OAUTH_CLIENT_SECRET_FILE")
	}
	if cfg.GAOAuthTokenJSON == "" && cfg.GAOAuthTokenFile == "" {
		return cfg, fmt.Errorf("missing required OAuth env: GOOGLE_OAUTH_TOKEN_JSON or GOOGLE_OAUTH_TOKEN_FILE")
	}

	if cfg.UpdateMode != updateModePolling && cfg.UpdateMode != updateModeWebhook {
		return cfg, fmt.Errorf("invalid TELEGRAM_UPDATE_MODE=%q, expected polling or webhook", cfg.UpdateMode)
	}

	if cfg.UpdateMode == updateModeWebhook && cfg.WebhookURL == "" {
		return cfg, fmt.Errorf("TELEGRAM_UPDATE_MODE=webhook requires TELEGRAM_WEBHOOK_URL")
	}

	return cfg, nil
}

func getEnvOrDefault(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}
