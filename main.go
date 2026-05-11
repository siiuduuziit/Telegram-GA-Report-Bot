package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := loadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	if cfg.OAuthBootstrap {
		if err := bootstrapOAuthToken(ctx, cfg.GAOAuthClientSecretFile, cfg.GAOAuthTokenFile); err != nil {
			log.Fatal(err)
		}
		log.Println("OAuth token bootstrap completed. You can now run bot normally.")
		return
	}

	gaSvc, err := newGAService(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := runTelegramBot(ctx, cfg, gaSvc); err != nil {
		log.Fatal(err)
	}
}
