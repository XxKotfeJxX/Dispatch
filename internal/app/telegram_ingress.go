package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"dispatch/internal/connectors"
	"dispatch/internal/ingress"
	"dispatch/internal/notification"
	"dispatch/internal/store/postgres"
)

func (worker *Worker) telegramAccountLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	running := map[string]context.CancelFunc{}
	finished := make(chan string, 16)

	syncConnections := func() {
		items, err := worker.Store.ListConnectorConnections(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				worker.Logger.Error("list Telegram connections", "error", err)
			}
			return
		}
		wanted := map[string]connectors.Connection{}
		for _, item := range items {
			if item.ConnectorID == "telegram" && item.Enabled {
				wanted[item.ID] = item
			}
		}
		for id, cancel := range running {
			if _, ok := wanted[id]; !ok {
				cancel()
				delete(running, id)
			}
		}
		for id, item := range wanted {
			if _, ok := running[id]; ok {
				continue
			}
			connectionCtx, cancel := context.WithCancel(ctx)
			running[id] = cancel
			go func(connection connectors.Connection) {
				err := worker.runTelegramAccount(connectionCtx, connection)
				if err != nil && !errors.Is(err, context.Canceled) {
					worker.Logger.Error("Telegram account stopped",
						"connection_id", connection.ID, "error", err)
					_ = worker.Store.UpdateConnectorState(
						context.Background(), connection.ID, "error", "", err.Error(), false,
					)
				}
				select {
				case finished <- connection.ID:
				case <-ctx.Done():
				}
			}(item)
		}
	}

	syncConnections()
	for {
		select {
		case <-ctx.Done():
			for _, cancel := range running {
				cancel()
			}
			return
		case id := <-finished:
			delete(running, id)
		case <-ticker.C:
			syncConnections()
		}
	}
}

func (worker *Worker) runTelegramAccount(
	ctx context.Context,
	connection connectors.Connection,
) error {
	_, cipher, err := worker.Store.GetConnectorConnection(ctx, connection.ID)
	if err != nil {
		return err
	}
	credentials, err := decryptConnectorCredentials(worker.Config.Connectors.EncryptionKey, cipher)
	if err != nil {
		return err
	}
	sessionData, err := base64.StdEncoding.DecodeString(credentials.Values["session"])
	if err != nil || len(sessionData) == 0 {
		return errors.New("Telegram session is missing or invalid")
	}
	storage := &telegramMemorySession{data: sessionData}
	var persistMu sync.Mutex
	storage.onStore = func(data []byte) error {
		persistMu.Lock()
		defer persistMu.Unlock()
		credentials.Values["session"] = base64.StdEncoding.EncodeToString(data)
		updatedCipher, encryptErr := encryptConnectorCredentials(
			worker.Config.Connectors.EncryptionKey, credentials,
		)
		if encryptErr != nil {
			return encryptErr
		}
		persistCtx, cancel := context.WithTimeout(context.Background(), worker.Config.ProviderTimeout)
		defer cancel()
		return worker.Store.UpdateConnectorCredentials(persistCtx, connection.ID, updatedCipher, nil)
	}

	dispatcher := tg.NewUpdateDispatcher()
	handle := func(ctx context.Context, entities tg.Entities, message tg.MessageClass) error {
		item, ok := message.(*tg.Message)
		if !ok || item.Out {
			return nil
		}
		return worker.enqueueTelegramMessage(ctx, connection, entities, item)
	}
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
		return handle(ctx, entities, update.Message)
	})
	dispatcher.OnNewChannelMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewChannelMessage) error {
		return handle(ctx, entities, update.Message)
	})

	client := telegram.NewClient(
		worker.Config.Connectors.TelegramAPIID,
		worker.Config.Connectors.TelegramAPIHash,
		telegram.Options{
			SessionStorage: storage,
			UpdateHandler:  dispatcher,
			Device: telegram.DeviceConfig{
				DeviceModel: "Dispatch", SystemVersion: "self-hosted",
				AppVersion: "1.2", SystemLangCode: "en", LangCode: "en",
			},
		},
	)
	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("verify Telegram session: %w", err)
		}
		if !status.Authorized {
			return errors.New("Telegram session is no longer authorized")
		}
		if err := worker.Store.UpdateConnectorState(
			ctx, connection.ID, "connected", telegramUserLabel(status.User), "", true,
		); err != nil && !errors.Is(err, postgres.ErrNotFound) {
			return err
		}
		return telegram.RunUntilCanceled(ctx, client)
	})
}

func (worker *Worker) enqueueTelegramMessage(
	ctx context.Context,
	connection connectors.Connection,
	entities tg.Entities,
	message *tg.Message,
) error {
	peer := fmt.Sprintf("%T:%v", message.PeerID, message.PeerID)
	externalID := fmt.Sprintf("%s:%d", peer, message.ID)
	metadata := telegramMessageMetadata(entities, message)
	body := strings.TrimSpace(message.Message)
	if body == "" {
		body = telegramMediaMessage(message.Media)
	}
	if body == "" {
		return nil
	}
	subject := "Telegram message"
	if sender, ok := metadata["sender"].(string); ok && sender != "" {
		subject = sender
	}
	digest := sha256.Sum256([]byte(connection.ID + ":" + externalID))
	item := notification.Notification{
		IdempotencyKey: "con_" + hex.EncodeToString(digest[:]),
		RecipientID:    connection.RecipientID,
		EventType:      "telegram.message",
		Subject:        subject,
		Body:           body,
		Metadata:       connectors.ConnectionMetadata(connection, metadata),
	}
	if _, err := worker.Store.CreateNotification(ctx, &item); err != nil {
		return err
	}
	if err := worker.Store.RecordConnectorEvent(
		ctx, connection.ID, externalID, item.ID, item.EventType,
	); err != nil {
		return err
	}
	return worker.Store.TouchConnectorEvent(ctx, connection.ID)
}

func telegramMessageMetadata(entities tg.Entities, message *tg.Message) map[string]any {
	_, forwarded := message.GetFwdFrom()
	result := map[string]any{
		"connector":     "telegram",
		"message_id":    message.ID,
		"peer":          fmt.Sprintf("%T:%v", message.PeerID, message.PeerID),
		"timestamp":     time.Unix(int64(message.Date), 0).UTC().Format(time.RFC3339),
		"mentioned":     message.Mentioned,
		"silent":        message.Silent,
		"pinned":        message.Pinned,
		"forwarded":     forwarded,
		"has_media":     message.Media != nil,
		"grouped_id":    message.GroupedID,
		"views":         message.Views,
		"forward_count": message.Forwards,
		"text_length":   len([]rune(message.Message)),
	}
	if message.Media != nil {
		result["media_type"] = telegramMediaType(message.Media)
	}
	switch sender := message.FromID.(type) {
	case *tg.PeerUser:
		if user := entities.Users[sender.UserID]; user != nil {
			label := telegramMessageUserLabel(user)
			result["sender"] = label
			result["sender_id"] = user.ID
			result["sender_username"] = user.Username
			result["author_username"] = user.Username
			result["sender_first_name"] = user.FirstName
			result["sender_last_name"] = user.LastName
			result["sender_is_bot"] = user.Bot
			result["sender_is_verified"] = user.Verified
			result["sender_is_premium"] = user.Premium
		}
	case *tg.PeerChannel:
		if channel := entities.Channels[sender.ChannelID]; channel != nil && channel.Title != "" {
			result["sender"] = channel.Title
			result["sender_id"] = channel.ID
			result["sender_username"] = channel.Username
			result["author_username"] = channel.Username
		}
	case *tg.PeerChat:
		if chat := entities.Chats[sender.ChatID]; chat != nil && chat.Title != "" {
			result["sender"] = chat.Title
			result["sender_id"] = chat.ID
		}
	}
	if result["sender"] == nil {
		result["sender"] = "Telegram message"
	}
	switch peer := message.PeerID.(type) {
	case *tg.PeerUser:
		result["chat_id"] = peer.UserID
		result["chat_type"] = "private"
		if user := entities.Users[peer.UserID]; user != nil {
			result["chat_title"] = telegramMessageUserLabel(user)
			result["chat_username"] = user.Username
		}
	case *tg.PeerChat:
		result["chat_id"] = peer.ChatID
		result["chat_type"] = "group"
		if chat := entities.Chats[peer.ChatID]; chat != nil {
			result["chat_title"] = chat.Title
		}
	case *tg.PeerChannel:
		result["chat_id"] = peer.ChannelID
		result["chat_type"] = "channel"
		if channel := entities.Channels[peer.ChannelID]; channel != nil {
			result["chat_title"] = channel.Title
			result["chat_username"] = channel.Username
			if channel.Megagroup {
				result["chat_type"] = "supergroup"
			}
			if channel.Username != "" {
				result["url"] = fmt.Sprintf("https://t.me/%s/%d", channel.Username, message.ID)
			}
		}
	}
	if result["chat_title"] == nil {
		result["chat_title"] = "Telegram"
	}
	return result
}

func telegramMessageUserLabel(user *tg.User) string {
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name != "" {
		return name
	}
	return telegramUserLabel(user)
}

func telegramMediaMessage(media tg.MessageMediaClass) string {
	if media == nil {
		return ""
	}
	mediaType := telegramMediaType(media)
	if mediaType == "" {
		return "Telegram media attachment"
	}
	return "Telegram " + strings.ReplaceAll(mediaType, "_", " ")
}

func telegramMediaType(media tg.MessageMediaClass) string {
	value := fmt.Sprintf("%T", media)
	value = strings.TrimPrefix(value, "*tg.MessageMedia")
	if value == "" || value == "Empty" {
		return ""
	}
	var result strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}

func decryptConnectorCredentials(
	key string,
	cipher []byte,
) (connectors.Credentials, error) {
	result := connectors.Credentials{Values: map[string]string{}}
	raw, err := ingress.DecryptSecret(key, cipher)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(raw), &result)
	return result, err
}

func encryptConnectorCredentials(
	key string,
	credentials connectors.Credentials,
) ([]byte, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	return ingress.EncryptSecret(key, string(raw))
}
