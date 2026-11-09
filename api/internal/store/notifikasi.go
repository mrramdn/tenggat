package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// WebhookUser is an account with a notification webhook.
type WebhookUser struct {
	ID         string
	Nama       string
	WebhookURL string
}

// Webhook returns a user's notification webhook URL (nil when off) and the
// Jakarta date of the last daily digest.
func (s *Store) Webhook(ctx context.Context, userID string) (*string, *time.Time, error) {
	var url *string
	var last *time.Time
	err := s.pool.QueryRow(ctx, `SELECT webhook_url, webhook_terakhir FROM pengguna WHERE id = $1`, userID).Scan(&url, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return url, last, err
}

// SetWebhook stores a user's webhook URL; nil turns notifications off.
func (s *Store) SetWebhook(ctx context.Context, userID string, url *string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE pengguna SET webhook_url = $2 WHERE id = $1`, userID, url)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DigestDue lists the accounts with a webhook whose daily digest has not run on this date.
func (s *Store) DigestDue(ctx context.Context, today time.Time) ([]WebhookUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, nama, webhook_url FROM pengguna
		WHERE webhook_url IS NOT NULL AND (webhook_terakhir IS NULL OR webhook_terakhir < $1)
		ORDER BY id`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookUser{}
	for rows.Next() {
		var u WebhookUser
		if err := rows.Scan(&u.ID, &u.Nama, &u.WebhookURL); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ClaimDigest marks a user's digest of this date as done and reports whether
// this call was the one that did it, so two API processes, or a restart, never
// send the same day twice.
func (s *Store) ClaimDigest(ctx context.Context, userID string, today time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE pengguna SET webhook_terakhir = $2
		WHERE id = $1 AND webhook_url IS NOT NULL AND (webhook_terakhir IS NULL OR webhook_terakhir < $2)`, userID, today)
	return err == nil && tag.RowsAffected() == 1, err
}
