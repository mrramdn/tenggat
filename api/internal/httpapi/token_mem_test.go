package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/mrramdn/tenggat/api/internal/store"
)

type memToken struct {
	tok  store.TokenAkses
	hash string
}

type memTokenList struct {
	entries []memToken
	seq     int
}

var (
	memTokenMu sync.Mutex
	memTokens  = map[*memStore]*memTokenList{}
)

func (m *memStore) tokenList() *memTokenList {
	memTokenMu.Lock()
	defer memTokenMu.Unlock()
	if memTokens[m] == nil {
		memTokens[m] = &memTokenList{}
	}
	return memTokens[m]
}

func (m *memStore) ListTokens(_ context.Context, userID string) ([]store.TokenAkses, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.TokenAkses{}
	if _, ok := m.users[userID]; !ok {
		return out, nil
	}
	entries := m.tokenList().entries
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].tok.PenggunaID == userID {
			out = append(out, entries[i].tok)
		}
	}
	return out, nil
}

func (m *memStore) CreateToken(_ context.Context, userID string, in store.TokenInput) (store.TokenAkses, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.tokenList()
	count := 0
	for _, e := range list.entries {
		if e.tok.PenggunaID == userID {
			count++
		}
	}
	if count >= store.MaxTokensPerUser {
		return store.TokenAkses{}, store.ErrTokenLimit
	}
	list.seq++
	tok := store.TokenAkses{
		ID:          m.newID(),
		PenggunaID:  userID,
		Nama:        in.Nama,
		Awalan:      in.Awalan,
		Cakupan:     in.Cakupan,
		Dibuat:      testNow.Add(time.Duration(list.seq) * time.Millisecond),
		Kedaluwarsa: in.Kedaluwarsa,
	}
	list.entries = append(list.entries, memToken{tok: tok, hash: in.Hash})
	return tok, nil
}

func (m *memStore) DeleteToken(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.tokenList()
	for i, e := range list.entries {
		if e.tok.ID == id && e.tok.PenggunaID == userID {
			list.entries = append(list.entries[:i], list.entries[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *memStore) UserByToken(_ context.Context, hash string, now time.Time) (store.Pengguna, store.TokenAkses, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.tokenList()
	for i := range list.entries {
		e := &list.entries[i]
		if e.hash != hash {
			continue
		}
		if e.tok.Kedaluwarsa != nil && !e.tok.Kedaluwarsa.After(now) {
			return store.Pengguna{}, store.TokenAkses{}, store.ErrNotFound
		}
		u, ok := m.users[e.tok.PenggunaID]
		if !ok {
			return store.Pengguna{}, store.TokenAkses{}, store.ErrNotFound
		}
		if e.tok.TerakhirDipakai == nil || now.Sub(*e.tok.TerakhirDipakai) >= time.Minute {
			at := now
			e.tok.TerakhirDipakai = &at
		}
		return u, e.tok, nil
	}
	return store.Pengguna{}, store.TokenAkses{}, store.ErrNotFound
}
