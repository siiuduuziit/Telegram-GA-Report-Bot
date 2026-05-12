package main

import (
	"context"
	"log"
	"net/http"
	"os"

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

	if cfg.UpdateMode != updateModeWebhook {
		startRenderHealthServer()
	}

	if err := runTelegramBot(ctx, cfg, gaSvc); err != nil {
		log.Fatal(err)
	}
}

func startRenderHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	go func() {
		addr := ":" + port
		log.Printf("Health server listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("health server stopped: %v", err)
		}
	}()
}
