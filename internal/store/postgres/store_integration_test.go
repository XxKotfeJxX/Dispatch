package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestNotificationIdempotencyAndJobClaim(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	person := recipient.Recipient{
		ID: "rec_test_" + suffix, Name: "Integration", Email: "integration@example.test",
		Preferences: recipient.Preferences{DefaultChannels: []string{"email"}},
	}
	if err := store.CreateRecipient(ctx, &person); err != nil {
		t.Fatal(err)
	}
	item := notification.Notification{
		IdempotencyKey: "integration-" + suffix, RecipientID: person.ID,
		EventType: "integration.test", Subject: "Test", Body: "Test",
	}
	created, err := store.CreateNotification(ctx, &item)
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	duplicate := notification.Notification{
		IdempotencyKey: item.IdempotencyKey, RecipientID: person.ID,
		EventType: item.EventType, Subject: item.Subject, Body: item.Body,
	}
	created, err = store.CreateNotification(ctx, &duplicate)
	if err != nil || created || duplicate.ID != item.ID {
		t.Fatalf("duplicate create: created=%v id=%q err=%v", created, duplicate.ID, err)
	}
	job, err := store.ClaimJob(ctx, "integration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.LockedBy != "integration-worker" {
		t.Fatalf("job was not leased: %#v", job)
	}
	if err := store.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverStaleJob(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	id := "job_test_" + uuid.NewString()
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO jobs(id,type,payload_json,status,max_attempts,available_at,locked_at,locked_by)
		VALUES ($1,'test','{}','running',3,now(),now()-interval '1 hour','gone')`, id); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RecoverStaleJobs(ctx, 5*time.Minute)
	if err != nil || recovered < 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
}
