package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/connectors"
	"dispatch/internal/ingress"
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

func TestMailpitRecipientSetupCompletion(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	codeHash := []byte("code-" + suffix)
	setup := recipient.Setup{
		ID: "rst_" + suffix, Kind: "mailpit", Name: "Local inbox",
		Target: "test-" + suffix + "@dispatch.local", CodeHash: codeHash,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := store.CreateRecipientSetup(ctx, &setup); err != nil {
		t.Fatal(err)
	}
	person, err := store.CompleteMailpitRecipientSetup(ctx, setup.ID, codeHash)
	if err != nil {
		t.Fatal(err)
	}
	if person.DestinationType != "mailpit" || person.Email != setup.Target ||
		len(person.Preferences.DefaultChannels) != 1 ||
		person.Preferences.DefaultChannels[0] != "email" {
		t.Fatalf("recipient = %#v", person)
	}
	completed, err := store.GetRecipientSetup(ctx, setup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.RecipientID != person.ID {
		t.Fatalf("setup = %#v", completed)
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

func TestIngressSourceLifecycle(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	person := recipient.Recipient{
		ID: "rec_ingress_" + suffix, Name: "Ingress Integration", Email: "ingress@example.test",
		Preferences: recipient.Preferences{DefaultChannels: []string{"email"}},
	}
	if err := store.CreateRecipient(ctx, &person); err != nil {
		t.Fatal(err)
	}
	input := ingress.CreateInput{
		Name: "Integration source", Slug: "integration-" + suffix,
		Provider: "generic", RecipientID: person.ID, AuthMode: ingress.AuthBearer,
		Mapping: ingress.Mapping{
			IDPath: "id", EventTypePath: "type", BodyPath: "body",
			DefaultEventType: "integration.ingress", DefaultSubject: "Integration",
		},
	}
	secretHash := []byte("test hash")
	source, err := store.CreateIngressSource(ctx, input, secretHash, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteIngressSource(ctx, source.ID) })

	got, hash, cipher, err := store.GetIngressSourceBySlug(ctx, source.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != source.ID || got.Mapping.BodyPath != "body" ||
		string(hash) != string(secretHash) || len(cipher) != 0 {
		t.Fatalf("unexpected source: %#v hash=%q cipher=%q", got, hash, cipher)
	}
	if err := store.SetIngressSourceEnabled(ctx, source.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _, _, err = store.GetIngressSourceBySlug(ctx, source.Slug)
	if err != nil || got.Enabled {
		t.Fatalf("source enabled=%v err=%v", got.Enabled, err)
	}
}

func TestConnectorConnectionLifecycle(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	person := recipient.Recipient{
		ID: "rec_connector_" + suffix, Name: "Connector Integration",
		Email:       "connector@example.test",
		Preferences: recipient.Preferences{DefaultChannels: []string{"email"}},
	}
	if err := store.CreateRecipient(ctx, &person); err != nil {
		t.Fatal(err)
	}
	item, err := store.CreateConnectorConnection(ctx, connectors.CreateInput{
		ConnectorID: "demo", Name: "Demo", RecipientID: person.ID,
		Config: map[string]string{"mode": "test"},
	}, "connected", "Local demo", []byte("ciphertext"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteConnectorConnection(ctx, item.ID) })
	got, cipher, err := store.GetConnectorConnection(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConnectorID != "demo" || got.Config["mode"] != "test" ||
		string(cipher) != "ciphertext" {
		t.Fatalf("unexpected connection: %#v cipher=%q", got, cipher)
	}
	if err := store.UpdateConnectorState(
		ctx, item.ID, "disabled", "", "", true,
	); err != nil {
		t.Fatal(err)
	}
	got, _, err = store.GetConnectorConnection(ctx, item.ID)
	if err != nil || got.Enabled || got.Status != "disabled" {
		t.Fatalf("connection enabled=%v status=%q err=%v", got.Enabled, got.Status, err)
	}
}
