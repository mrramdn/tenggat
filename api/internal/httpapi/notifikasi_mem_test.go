package httpapi

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/mrramdn/tenggat/api/internal/store"
)

// The memStore's webhooks, kept beside it like the calendar tokens.
type memWebhook struct {
	url  *string
	last *time.Time
}

var (
	memWebhookMu sync.Mutex
	memWebhooks  = map[*memStore]map[string]memWebhook{} // by user id
)

func (m *memStore) webhooks() map[string]memWebhook {
	if memWebhooks[m] == nil {
		memWebhooks[m] = map[string]memWebhook{}
	}
	return memWebhooks[m]
}

func (m *memStore) Webhook(_ context.Context, userID string) (*string, *time.Time, error) {
	memWebhookMu.Lock()
	defer memWebhookMu.Unlock()
	w := m.webhooks()[userID]
	return w.url, w.last, nil
}

func (m *memStore) SetWebhook(_ context.Context, userID string, url *string) error {
	memWebhookMu.Lock()
	defer memWebhookMu.Unlock()
	w := m.webhooks()[userID]
	w.url = url
	m.webhooks()[userID] = w
	return nil
}

func (m *memStore) DigestDue(_ context.Context, today time.Time) ([]store.WebhookUser, error) {
	m.mu.Lock()
	users := m.users
	m.mu.Unlock()
	memWebhookMu.Lock()
	defer memWebhookMu.Unlock()
	out := []store.WebhookUser{}
	for id, w := range m.webhooks() {
		if w.url != nil && (w.last == nil || w.last.Before(today)) {
			out = append(out, store.WebhookUser{ID: id, Nama: users[id].Nama, WebhookURL: *w.url})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memStore) ClaimDigest(_ context.Context, userID string, today time.Time) (bool, error) {
	memWebhookMu.Lock()
	defer memWebhookMu.Unlock()
	w := m.webhooks()[userID]
	if w.url == nil || (w.last != nil && !w.last.Before(today)) {
		return false, nil
	}
	w.last = &today
	m.webhooks()[userID] = w
	return true, nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
