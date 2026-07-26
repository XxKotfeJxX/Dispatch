package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"

	"dispatch/internal/connectors"
)

type bridgeConfig struct {
	Token                string
	InternalURL          string
	BridgeKey            string
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
	if config.Token == "" {
		slog.Info("Discord bridge disabled; DISCORD_BOT_TOKEN is not configured")
		<-ctx.Done()
		return
	}
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
		if event.Author == nil || event.Author.Bot {
			return
		}
		guildName, channelName := "", ""
		if event.GuildID != "" {
			if guild, guildErr := discord.State.Guild(event.GuildID); guildErr == nil {
				guildName = guild.Name
			}
		}
		if channel, channelErr := discord.State.Channel(event.ChannelID); channelErr == nil {
			channelName = channel.Name
		}
		mentions := make([]connectors.DiscordAuthor, 0, len(event.Mentions))
		for _, mention := range event.Mentions {
			mentions = append(mentions, connectors.DiscordAuthor{
				ID: mention.ID, Username: mention.Username,
			})
		}
		payload, _ := json.Marshal(connectors.DiscordEvent{
			ID: event.ID, Type: "message", Content: event.Content,
			ChannelID: event.ChannelID, GuildID: event.GuildID,
			GuildName: guildName, ChannelName: channelName, Timestamp: event.Timestamp,
			Author:   connectors.DiscordAuthor{ID: event.Author.ID, Username: event.Author.Username},
			Mentions: mentions, MentionRoles: event.MentionRoles,
			MentionEveryone: event.MentionEveryone,
		})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.InternalURL, bytes.NewReader(payload))
		if err == nil {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Dispatch-Bridge-Key", config.BridgeKey)
			var response *http.Response
			response, err = client.Do(request)
			if response != nil {
				var result struct {
					Accepted int `json:"accepted"`
				}
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
				}
				_ = response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					err = errors.New(response.Status)
				} else if result.Accepted == 0 {
					slog.Debug("Discord message did not match an active connection",
						"message_id", event.ID, "channel_id", event.ChannelID)
					return
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
		InternalURL:          envValue("DISCORD_INTERNAL_URL", "http://api:8080/internal/v1/discord/events"),
		MessageContentIntent: boolValue("DISCORD_MESSAGE_CONTENT_INTENT", false),
		Acknowledge:          boolValue("DISCORD_ACKNOWLEDGE", true),
	}
	encryptionKey := strings.TrimSpace(os.Getenv("CONNECTOR_ENCRYPTION_KEY"))
	if encryptionKey == "" {
		encryptionKey = strings.TrimSpace(os.Getenv("INGRESS_ENCRYPTION_KEY"))
	}
	if encryptionKey == "" {
		encryptionKey = envValue("API_KEY", "dispatch-local-development-key")
	}
	value.BridgeKey = connectors.DiscordBridgeKey(encryptionKey)
	if value.InternalURL == "" || value.BridgeKey == "" {
		return value, errors.New("Discord internal bridge configuration is incomplete")
	}
	return value, nil
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
