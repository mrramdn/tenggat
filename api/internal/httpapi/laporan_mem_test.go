package httpapi

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// The memStore's report notes and link tokens, kept beside it like the
// calendar tokens. LaporanAktif is kept on the client itself.
var (
	memLaporanMu sync.Mutex
	memCatatan   = map[*memStore]map[string]string{} // klien id + month → text
	memReports   = map[*memStore]map[string]string{} // token hash → klien id
)

func (m *memStore) CatatanLaporan(_ context.Context, klienID string, bulan time.Time) (string, error) {
	memLaporanMu.Lock()
	defer memLaporanMu.Unlock()
	return memCatatan[m][klienID+bulan.Format("2006-01")], nil
}

func (m *memStore) SetCatatanLaporan(ctx context.Context, userID, klienID string, bulan time.Time, teks string) error {
	if _, err := m.GetKlien(ctx, userID, klienID); err != nil {
		return err
	}
	memLaporanMu.Lock()
	defer memLaporanMu.Unlock()
	if memCatatan[m] == nil {
		memCatatan[m] = map[string]string{}
	}
	memCatatan[m][klienID+bulan.Format("2006-01")] = teks
	return nil
}

func (m *memStore) setReport(userID, klienID, hash string) error {
	m.mu.Lock()
	k, ok := m.klien[klienID]
	if !ok || k.PenggunaID != userID {
		m.mu.Unlock()
		return store.ErrNotFound
	}
	k.LaporanAktif = hash != ""
	m.klien[klienID] = k
	m.mu.Unlock()
	memLaporanMu.Lock()
	defer memLaporanMu.Unlock()
	if memReports[m] == nil {
		memReports[m] = map[string]string{}
	}
	for h, id := range memReports[m] {
		if id == klienID {
			delete(memReports[m], h)
		}
	}
	if hash != "" {
		memReports[m][hash] = klienID
	}
	return nil
}

func (m *memStore) SetReportToken(_ context.Context, userID, klienID, hash string) error {
	return m.setReport(userID, klienID, hash)
}

func (m *memStore) DeleteReportToken(_ context.Context, userID, klienID string) error {
	return m.setReport(userID, klienID, "")
}

func (m *memStore) KlienByReportToken(_ context.Context, hash string) (store.Klien, store.Pengguna, error) {
	memLaporanMu.Lock()
	id, ok := memReports[m][hash]
	memLaporanMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	k, found := m.klien[id]
	if !ok || !found {
		return store.Klien{}, store.Pengguna{}, store.ErrNotFound
	}
	return k, m.users[k.PenggunaID], nil
}

func (m *memStore) AwalLaporan(_ context.Context, klienID string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.klien[klienID]
	if !ok {
		return time.Time{}, store.ErrNotFound
	}
	awal := expiry.DateOf(k.Dibuat)
	for _, a := range m.ruang().aktivitas {
		if deref(a.KlienID) == klienID && a.Tanggal.Before(awal) &&
			slices.Contains([]string{"pekerjaan", "tugas_selesai", "aset_diperpanjang", "cek_diperbarui"}, a.Jenis) {
			awal = a.Tanggal
		}
	}
	return awal, nil
}
