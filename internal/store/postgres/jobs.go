package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dispatch/internal/jobs"
)

func (store *Store) ClaimJob(ctx context.Context, workerID string) (jobs.Job, error) {
	var job jobs.Job
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		var payload []byte
		err := tx.QueryRow(ctx, `
			SELECT id,type,payload_json,status,attempt_count,max_attempts,available_at,
			       locked_at,COALESCE(locked_by,''),COALESCE(last_error,''),created_at,updated_at
			FROM jobs
			WHERE status='queued' AND available_at<=now()
			ORDER BY available_at,created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1`).Scan(
			&job.ID, &job.Type, &payload, &job.Status, &job.AttemptCount, &job.MaxAttempts,
			&job.AvailableAt, &job.LockedAt, &job.LockedBy, &job.LastError,
			&job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return notFound(err)
		}
		job.AttemptCount++
		job.Status, job.LockedBy = "running", workerID
		now := time.Now().UTC()
		job.LockedAt, job.UpdatedAt = &now, now
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET status='running',attempt_count=attempt_count+1,
			    locked_at=$2,locked_by=$3,updated_at=$2 WHERE id=$1`,
			job.ID, now, workerID); err != nil {
			return err
		}
		return json.Unmarshal(payload, &job.Payload)
	})
	return job, err
}

func (store *Store) CompleteJob(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE jobs SET status='completed',locked_at=NULL,locked_by=NULL,updated_at=now()
		WHERE id=$1 AND status='running'`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

func (store *Store) RetryJob(ctx context.Context, job jobs.Job, failure error, delay time.Duration) (bool, error) {
	exhausted := job.AttemptCount >= job.MaxAttempts
	status := "queued"
	if exhausted {
		status = "failed"
	}
	command, err := store.pool.Exec(ctx, `
		UPDATE jobs SET status=$2,available_at=$3,locked_at=NULL,locked_by=NULL,
		    last_error=$4,updated_at=now() WHERE id=$1 AND status='running'`,
		job.ID, status, time.Now().UTC().Add(delay), safeError(failure))
	if err != nil {
		return exhausted, err
	}
	if command.RowsAffected() == 0 {
		return exhausted, ErrInvalidState
	}
	return exhausted, nil
}

func (store *Store) RecoverStaleJobs(ctx context.Context, lockTimeout time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-lockTimeout)
	command, err := store.pool.Exec(ctx, `
		UPDATE jobs SET status='queued',available_at=now(),locked_at=NULL,locked_by=NULL,
		    last_error='worker lock expired and was recovered',updated_at=now()
		WHERE status='running' AND locked_at<$1`, cutoff)
	if err != nil {
		return 0, err
	}
	return command.RowsAffected(), nil
}

func (store *Store) EnqueueProcessNotification(ctx context.Context, notificationID string) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO jobs(id,type,payload_json,status,max_attempts,available_at)
		VALUES ($1,'process_notification',jsonb_build_object('notification_id',$2::text),'queued',5,now())`,
		"job_"+uuid.NewString(), notificationID)
	return err
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func payloadString(job jobs.Job, key string) (string, error) {
	value := fmt.Sprint(job.Payload[key])
	if value == "" || value == "<nil>" {
		return "", fmt.Errorf("job payload is missing %s", key)
	}
	return value, nil
}
