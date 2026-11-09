package httpapi

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/mrramdn/tenggat/api/internal/store"
)

// The memStore's tasks and activity entries, kept beside it like the calendar
// tokens so the memStore type in server_test.go stays as it is. They are
// guarded by the memStore's own mutex.
type memRuang struct {
	tugas     map[string]store.Tugas
	aktivitas []store.Aktivitas // in insertion order
	seq       int
}

var (
	memRuangMu sync.Mutex
	memRuangs  = map[*memStore]*memRuang{}
)

func (m *memStore) ruang() *memRuang {
	memRuangMu.Lock()
	defer memRuangMu.Unlock()
	if memRuangs[m] == nil {
		memRuangs[m] = &memRuang{tugas: map[string]store.Tugas{}}
	}
	return memRuangs[m]
}

func (m *memStore) klienName(id *string) string {
	if id == nil {
		return ""
	}
	return m.klien[*id].Nama
}

func (m *memStore) ListTugas(_ context.Context, userID string, f store.TugasFilter) ([]store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.Tugas{}
	for _, t := range m.ruang().tugas {
		if t.PenggunaID != userID || (f.KlienID != "" && deref(t.KlienID) != f.KlienID) ||
			(f.Sampai != nil && t.JatuhTempo.After(*f.Sampai)) {
			continue
		}
		t.Klien = m.klienName(t.KlienID)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JatuhTempo.Before(out[j].JatuhTempo) })
	return out, nil
}

func (m *memStore) GetTugas(_ context.Context, userID, id string) (store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ruang().tugas[id]
	if !ok || t.PenggunaID != userID {
		return store.Tugas{}, store.ErrNotFound
	}
	t.Klien = m.klienName(t.KlienID)
	return t, nil
}

func (m *memStore) putTugas(t store.Tugas, in store.TugasInput) store.Tugas {
	t.KlienID, t.Judul, t.Catatan, t.UlangJumlah, t.UlangSatuan, t.JatuhTempo, t.Biaya =
		in.KlienID, in.Judul, in.Catatan, in.UlangJumlah, in.UlangSatuan, in.JatuhTempo, in.Biaya
	t.Diubah = testNow
	m.ruang().tugas[t.ID] = t
	t.Klien = m.klienName(t.KlienID)
	return t
}

func (m *memStore) CreateTugas(_ context.Context, userID string, in store.TugasInput) (store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.putTugas(store.Tugas{ID: m.newID(), PenggunaID: userID, Dibuat: testNow}, in), nil
}

func (m *memStore) UpdateTugas(_ context.Context, userID, id string, in store.TugasInput) (store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ruang().tugas[id]
	if !ok || t.PenggunaID != userID {
		return store.Tugas{}, store.ErrNotFound
	}
	return m.putTugas(t, in), nil
}

func (m *memStore) CompleteTugas(_ context.Context, userID, id string, from, next, at time.Time) (store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ruang().tugas[id]
	if !ok || t.PenggunaID != userID || !t.JatuhTempo.Equal(from) {
		return store.Tugas{}, store.ErrNotFound
	}
	t.JatuhTempo, t.TerakhirSelesai = next, &at
	m.ruang().tugas[id] = t
	t.Klien = m.klienName(t.KlienID)
	return t, nil
}

func (m *memStore) DeleteTugas(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ruang().tugas[id]
	if !ok || t.PenggunaID != userID {
		return store.ErrNotFound
	}
	delete(m.ruang().tugas, id)
	return nil
}

func (m *memStore) ListAktivitas(_ context.Context, userID string, f store.AktivitasFilter) ([]store.Aktivitas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.ruang().aktivitas
	// Newest first; the insertion order stands in for (dibuat, id).
	cut := len(all)
	if f.SebelumID != "" {
		cut = -1
		for i, a := range all {
			if a.ID == f.SebelumID && a.PenggunaID == userID {
				cut = i
			}
		}
	}
	out := []store.Aktivitas{}
	for i := cut - 1; i >= 0; i-- {
		a := all[i]
		if a.PenggunaID != userID || (f.KlienID != "" && deref(a.KlienID) != f.KlienID) ||
			(f.SebelumWaktu != nil && !a.Dibuat.Before(*f.SebelumWaktu)) ||
			(len(f.Jenis) > 0 && !slices.Contains(f.Jenis, a.Jenis)) ||
			(f.Dari != nil && a.Tanggal.Before(*f.Dari)) || (f.Sampai != nil && a.Tanggal.After(*f.Sampai)) {
			continue
		}
		a.Klien = m.klienName(a.KlienID)
		out = append(out, a)
		if f.Batas > 0 && len(out) == f.Batas {
			break
		}
	}
	return out, nil
}

func (m *memStore) AddAktivitas(_ context.Context, userID string, in store.AktivitasInput) (store.Aktivitas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.ruang()
	if in.SekaliSehari {
		for _, a := range r.aktivitas {
			if deref(a.ItemID) == deref(in.ItemID) && a.Jenis == in.Jenis && a.Tanggal.Equal(in.Tanggal) {
				return store.Aktivitas{}, store.ErrNotFound
			}
		}
	}
	r.seq++
	a := store.Aktivitas{ID: m.newID(), PenggunaID: userID, KlienID: in.KlienID, ItemID: in.ItemID, TugasID: in.TugasID,
		Jenis: in.Jenis, Teks: in.Teks, Tanggal: in.Tanggal, DurasiMenit: in.DurasiMenit, Data: in.Data,
		Dibuat: testNow.Add(time.Duration(r.seq) * time.Millisecond)}
	r.aktivitas = append(r.aktivitas, a)
	a.Klien = m.klienName(a.KlienID)
	return a, nil
}

func (m *memStore) DeleteAktivitas(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.ruang()
	for i, a := range r.aktivitas {
		if a.ID != id || a.PenggunaID != userID {
			continue
		}
		if a.Jenis != "catatan" && a.Jenis != "pekerjaan" {
			return store.ErrAktivitasOtomatis
		}
		r.aktivitas = append(r.aktivitas[:i], r.aktivitas[i+1:]...)
		return nil
	}
	return store.ErrNotFound
}

// aktivitasOf lists a user's entries of one kind, oldest first.
func (m *memStore) aktivitasOf(jenis string) []store.Aktivitas {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.Aktivitas
	for _, a := range m.ruang().aktivitas {
		if a.Jenis == jenis {
			out = append(out, a)
		}
	}
	return out
}
