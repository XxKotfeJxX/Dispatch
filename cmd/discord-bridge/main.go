package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

type bridgeConfig struct {
	Token                string
	IngressURL           string
	IngressSecret        string
	AllowedUsers         map[string]bool
	AllowedChannels      map[string]bool
	MessageContentIntent bool
	Acknowledge          bool
}

func main() {
	config, err := loadBridgeConfig()
	if err != nil {
		slog.Error("Discord bridge configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	session, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		slog.Error("create Discord session", "error", err)
		os.Exit(1)
	}
	session.Identify.Intents = discordgo.IntentsDirectMessages | discordgo.IntentsGuildMessages
	if config.MessageContentIntent {
		session.Identify.Intents |= discordgo.IntentsMessageContent
	}
	client := &http.Client{Timeout: 10 * time.Second}
	session.AddHandler(func(discord *discordgo.Session, event *discordgo.MessageCreate) {
		if event.Author == nil || event.Author.Bot || !config.AllowedUsers[event.Author.ID] {
			return
		}
		if len(config.AllowedChannels) > 0 && !config.AllowedChannels[event.ChannelID] {
			return
		}
		if strings.TrimSpace(event.Content) == "" {
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"id": event.ID, "type": "message", "content": event.Content,
			"channel_id": event.ChannelID, "guild_id": event.GuildID,
			"timestamp": event.Timestamp,
			"author":    map[string]string{"id": event.Author.ID, "username": event.Author.Username},
		})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.IngressURL, bytes.NewReader(payload))
		if err == nil {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+config.IngressSecret)
			var response *http.Response
			response, err = client.Do(request)
			if response != nil {
				_ = response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					err = errors.New(response.Status)
				}
			}
		}
		if err != nil {
			slog.Error("forward Discord message", "message_id", event.ID, "error", err)
			return
		}
		slog.Info("Discord message accepted", "message_id", event.ID, "channel_id", event.ChannelID)
		if config.Acknowledge {
			_, _ = discord.ChannelMessageSendReply(event.ChannelID, "Accepted by Dispatch.", event.Reference())
		}
	})
	session.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		slog.Info("Discord bridge connected", "bot", ready.User.Username)
	})
	if err := session.Open(); err != nil {
		slog.Error("connect Discord Gateway", "error", err)
		os.Exit(1)
	}
	defer session.Close()
	<-ctx.Done()
}

func loadBridgeConfig() (bridgeConfig, error) {
	value := bridgeConfig{
		Token:                os.Getenv("DISCORD_BOT_TOKEN"),
		IngressURL:           envValue("DISCORD_INGRESS_URL", "http://api:8080/ingest/v1/discord"),
		IngressSecret:        os.Getenv("DISCORD_INGRESS_SECRET"),
		AllowedUsers:         splitSet(os.Getenv("DISCORD_ALLOWED_USER_IDS")),
		AllowedChannels:      splitSet(os.Getenv("DISCORD_ALLOWED_CHANNEL_IDS")),
		MessageContentIntent: boolValue("DISCORD_MESSAGE_CONTENT_INTENT", false),
		Acknowledge:          boolValue("DISCORD_ACKNOWLEDGE", true),
	}
	if value.Token == "" || value.IngressSecret == "" || len(value.AllowedUsers) == 0 {
		return value, errors.New("DISCORD_BOT_TOKEN, DISCORD_INGRESS_SECRET and DISCORD_ALLOWED_USER_IDS are required")
	}
	return value, nil
}

func splitSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result[item] = true
		}
	}
	return result
}

func envValue(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func boolValue(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}
