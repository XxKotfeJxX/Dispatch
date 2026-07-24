package postgres

import (
	"context"
	"encoding/json"

	"dispatch/internal/audit"
)

type executor interface {
	Exec(context.Context, string, ...any) (pgconnCommandTag, error)
}

func (store *Store) AddAudit(ctx context.Context, entityType, entityID, eventType string, details map[string]any) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
		VALUES ($1,$2,$3,$4)`, entityType, entityID, eventType, details)
	return err
}

func (store *Store) AuditEvents(ctx context.Context, entityType, entityID string) ([]audit.Event, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,entity_type,entity_id,event_type,details_json,created_at
		FROM audit_events WHERE entity_type=$1 AND entity_id=$2 ORDER BY id`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}

func (store *Store) AuditEventsAfter(ctx context.Context, afterID int64, limit int) ([]audit.Event, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id,entity_type,entity_id,event_type,details_json,created_at
		FROM audit_events WHERE id>$1 ORDER BY id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}

func scanAuditRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]audit.Event, error) {
	result := make([]audit.Event, 0)
	for rows.Next() {
		var item audit.Event
		var raw []byte
		if err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.EventType, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Details); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
