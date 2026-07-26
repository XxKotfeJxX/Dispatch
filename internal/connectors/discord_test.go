package connectors

import "testing"

func TestDiscordEventMatchesManagedConnection(t *testing.T) {
	connection := Connection{
		ConnectorID: "discord",
		Enabled:     true,
		Config: map[string]string{
			"discord_user_id": "user-1",
			"guild_id":        "guild-1",
			"mode":            DiscordModeMentions,
		},
	}
	if !DiscordEventMatches(connection, DiscordEvent{
		ID: "dm", Author: DiscordAuthor{ID: "user-1"},
	}) {
		t.Fatal("direct message from the connected Discord user should match")
	}
	if !DiscordEventMatches(connection, DiscordEvent{
		ID: "mention", GuildID: "guild-1",
		Mentions: []DiscordAuthor{{ID: "user-1"}},
	}) {
		t.Fatal("mention in the installed guild should match")
	}
	if DiscordEventMatches(connection, DiscordEvent{
		ID: "other", GuildID: "guild-2", MentionEveryone: true,
	}) {
		t.Fatal("a different guild must not match")
	}
}

func TestDiscordAllModeMatchesInstalledGuildOnly(t *testing.T) {
	connection := Connection{
		ConnectorID: "discord", Enabled: true,
		Config: map[string]string{"guild_id": "guild-1", "mode": DiscordModeAll},
	}
	if !DiscordEventMatches(connection, DiscordEvent{GuildID: "guild-1"}) {
		t.Fatal("all mode should accept the configured guild")
	}
	if DiscordEventMatches(connection, DiscordEvent{GuildID: "guild-2"}) {
		t.Fatal("all mode must reject another guild")
	}
}
