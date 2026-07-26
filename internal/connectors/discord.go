package connectors

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	DiscordModeDirectMessages = "direct_messages"
	DiscordModeMentions       = "mentions"
	DiscordModeAll            = "all"
)

type DiscordAuthor struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type DiscordEvent struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Content         string          `json:"content"`
	ChannelID       string          `json:"channel_id"`
	GuildID         string          `json:"guild_id"`
	GuildName       string          `json:"guild_name,omitempty"`
	ChannelName     string          `json:"channel_name,omitempty"`
	Timestamp       time.Time       `json:"timestamp"`
	Author          DiscordAuthor   `json:"author"`
	Mentions        []DiscordAuthor `json:"mentions,omitempty"`
	MentionRoles    []string        `json:"mention_roles,omitempty"`
	MentionEveryone bool            `json:"mention_everyone,omitempty"`
}

func DiscordBridgeKey(encryptionKey string) string {
	sum := sha256.Sum256([]byte("dispatch/discord-bridge/v1\x00" + encryptionKey))
	return hex.EncodeToString(sum[:])
}

func DiscordEventMatches(connection Connection, event DiscordEvent) bool {
	if connection.ConnectorID != "discord" || !connection.Enabled {
		return false
	}
	userID := strings.TrimSpace(connection.Config["discord_user_id"])
	if event.GuildID == "" {
		return userID != "" && event.Author.ID == userID
	}
	if guildID := strings.TrimSpace(connection.Config["guild_id"]); guildID == "" ||
		guildID != event.GuildID {
		return false
	}
	switch connection.Config["mode"] {
	case DiscordModeAll:
		return true
	case DiscordModeMentions:
		if event.MentionEveryone {
			return true
		}
		for _, mention := range event.Mentions {
			if mention.ID == userID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func NormalizeDiscordEvent(event DiscordEvent) NormalizedEvent {
	location := "direct message"
	if event.GuildID != "" {
		location = "server message"
		if event.GuildName != "" && event.ChannelName != "" {
			location = event.GuildName + " / #" + event.ChannelName
		}
	}
	body := strings.TrimSpace(event.Content)
	if body == "" {
		body = "Discord delivered a message without readable content. Enable Message Content Intent for server message text."
	}
	return NormalizedEvent{
		ExternalID: event.ID,
		EventType:  "discord.message.created",
		Subject:    "Discord: " + event.Author.Username + " in " + location,
		Body:       body,
		Metadata: map[string]any{
			"connector":        "discord",
			"author_id":        event.Author.ID,
			"author_username":  event.Author.Username,
			"channel_id":       event.ChannelID,
			"channel_name":     event.ChannelName,
			"guild_id":         event.GuildID,
			"guild_name":       event.GuildName,
			"mention_everyone": event.MentionEveryone,
			"timestamp":        event.Timestamp,
		},
	}
}
