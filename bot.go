package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

func runTelegramBot(ctx context.Context, cfg appConfig, gaSvc *analyticsdata.Service) error {
	bot, err := telego.NewBot(cfg.BotToken)
	if err != nil {
		return err
	}

	updates, startMsg, err := newUpdateSource(ctx, cfg, bot)
	if err != nil {
		return err
	}

	h, err := th.NewBotHandler(bot, updates)
	if err != nil {
		return err
	}

	h.Handle(func(ctx *th.Context, update telego.Update) error {
		if update.Message == nil {
			return nil
		}

		text := strings.TrimSpace(strings.ToLower(update.Message.Text))
		if !isUpdateCommand(text) {
			return nil
		}

		reply := buildReport(ctx, gaSvc, cfg.GAPropertyID)
		_, _ = ctx.Bot().SendMessage(ctx, tu.Message(tu.ID(update.Message.Chat.ID), reply))
		return nil
	})

	log.Println(startMsg)
	return h.Start()
}

func isUpdateCommand(text string) bool {
	switch text {
	case "/update", "/update@telegram_ga_report_bot", "hey bot update me":
		return true
	default:
		return false
	}
}

func newUpdateSource(ctx context.Context, cfg appConfig, bot *telego.Bot) (<-chan telego.Update, string, error) {
	if cfg.UpdateMode == updateModeWebhook {
		return setupWebhookUpdates(ctx, cfg, bot)
	}
	return setupPollingUpdates(ctx, bot)
}

func setupPollingUpdates(ctx context.Context, bot *telego.Bot) (<-chan telego.Update, string, error) {
	if err := bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{
		DropPendingUpdates: true,
	}); err != nil {
		return nil, "", err
	}

	updates, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	return updates, "Bot is running (long polling)...", nil
}

func setupWebhookUpdates(ctx context.Context, cfg appConfig, bot *telego.Bot) (<-chan telego.Update, string, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    cfg.WebhookListenAddr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("webhook server stopped: %v", err)
		}
	}()

	params := &telego.SetWebhookParams{
		URL:                cfg.WebhookURL,
		DropPendingUpdates: true,
	}
	if cfg.WebhookSecret != "" {
		params.SecretToken = cfg.WebhookSecret
	}

	updates, err := bot.UpdatesViaWebhook(
		ctx,
		telego.WebhookHTTPServeMux(mux, cfg.WebhookPath, cfg.WebhookSecret),
		telego.WithWebhookSet(ctx, params),
	)
	if err != nil {
		return nil, "", err
	}

	msg := fmt.Sprintf("Bot is running (webhook) listen=%s path=%s url=%s", cfg.WebhookListenAddr, cfg.WebhookPath, cfg.WebhookURL)
	return updates, msg, nil
}
