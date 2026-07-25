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
		if !ok || item.Out || strings.TrimSpace(item.Message) == "" {
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
	subject := telegramMessageSubject(entities, message)
	digest := sha256.Sum256([]byte(connection.ID + ":" + externalID))
	item := notification.Notification{
		IdempotencyKey: "con_" + hex.EncodeToString(digest[:]),
		RecipientID:    connection.RecipientID,
		EventType:      "telegram.message",
		Subject:        subject,
		Body:           message.Message,
		Metadata: map[string]any{
			"connector":  "telegram",
			"message_id": message.ID,
			"peer":       peer,
		},
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

func telegramMessageSubject(entities tg.Entities, message *tg.Message) string {
	switch sender := message.FromID.(type) {
	case *tg.PeerUser:
		if user := entities.Users[sender.UserID]; user != nil {
			return telegramUserLabel(user)
		}
	case *tg.PeerChannel:
		if channel := entities.Channels[sender.ChannelID]; channel != nil && channel.Title != "" {
			return channel.Title
		}
	case *tg.PeerChat:
		if chat := entities.Chats[sender.ChatID]; chat != nil && chat.Title != "" {
			return chat.Title
		}
	}
	return "Telegram message"
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
