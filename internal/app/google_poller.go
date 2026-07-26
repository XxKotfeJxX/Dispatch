package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

func (worker *Worker) googleLoop(ctx context.Context) {
	worker.pollGoogleConnections(ctx)
	ticker := time.NewTicker(worker.Config.Connectors.GooglePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			worker.pollGoogleConnections(ctx)
		}
	}
}

func (worker *Worker) pollGoogleConnections(ctx context.Context) {
	items, err := worker.Store.ListConnectorConnections(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			worker.Logger.Error("list Google connections", "error", err)
		}
		return
	}
	for _, item := range items {
		if item.ConnectorID != "google" || !item.Enabled {
			continue
		}
		modules := connectors.GoogleModules(item.Config["modules"])
		if len(modules) == 0 {
			modules = []string{connectors.GoogleModuleGmail}
		}
		succeeded := 0
		moduleErrors := make([]string, 0)
		for _, module := range modules {
			var moduleErr error
			switch module {
			case connectors.GoogleModuleGmail:
				moduleErr = worker.pollGmailConnection(ctx, item.ID)
			case connectors.GoogleModuleCalendar:
				moduleErr = worker.pollGoogleCalendar(ctx, item.ID)
			case connectors.GoogleModuleDrive:
				moduleErr = worker.pollGoogleDrive(ctx, item.ID)
			case connectors.GoogleModuleTasks:
				moduleErr = worker.pollGoogleTasks(ctx, item.ID)
			case connectors.GoogleModuleChat:
				moduleErr = worker.pollGoogleChat(ctx, item.ID)
			}
			if moduleErr == nil {
				succeeded++
				continue
			}
			if errors.Is(moduleErr, context.Canceled) {
				return
			}
			worker.Logger.Warn(
				"poll Google module",
				"connection_id", item.ID,
				"module", module,
				"error", moduleErr,
			)
			moduleErrors = append(moduleErrors, module+": "+moduleErr.Error())
		}
		if len(moduleErrors) == 0 {
			_ = worker.Store.UpdateConnectorState(ctx, item.ID, "connected", "", "", true)
			continue
		}
		lastError := strings.Join(moduleErrors, "; ")
		if len(lastError) > 2000 {
			lastError = lastError[:2000]
		}
		status := "connected"
		if succeeded == 0 {
			status = "error"
		}
		_ = worker.Store.UpdateConnectorState(ctx, item.ID, status, "", lastError, true)
	}
}

func (worker *Worker) loadGoogleConnection(
	ctx context.Context,
	connectionID string,
) (connectors.Connection, connectors.Credentials, error) {
	connection, cipher, err := worker.Store.GetConnectorConnection(ctx, connectionID)
	if err != nil {
		return connection, connectors.Credentials{}, err
	}
	credentials, err := worker.googleCredentials(cipher)
	if err != nil {
		return connection, credentials, err
	}
	credentials, changed, err := worker.refreshGoogleCredentials(ctx, credentials)
	if err != nil {
		return connection, credentials, err
	}
	if changed {
		if err := worker.saveGoogleCredentials(
			ctx, connection.ID, credentials,
		); err != nil {
			return connection, credentials, err
		}
	}
	return connection, credentials, nil
}

func (worker *Worker) googleJSON(
	ctx context.Context,
	method, baseURL, path, accessToken string,
	body any,
	target any,
) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	requestCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx, method, strings.TrimRight(baseURL, "/")+path, reader,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: worker.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf(
			"Google API returned %s: %s",
			response.Status,
			strings.TrimSpace(string(raw)),
		)
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target)
}
