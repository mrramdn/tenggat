package httpapi

import (
	"context"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

func (m *memStore) PerpanjangItem(_ context.Context, userID, id string, p store.Perpanjangan) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok || it.PenggunaID != userID || !it.Kedaluwarsa.Equal(p.Dari) || it.SelesaiPada != nil {
		return store.Item{}, store.ErrNotFound
	}
	if p.Sampai != nil {
		it.Kedaluwarsa = *p.Sampai
	}
	if p.Lunas {
		at := p.At
		it.SelesaiPada = &at
	}
	it.Diubah = p.At
	m.items[id] = it

	r := m.ruang()
	r.seq++
	klienID := p.Aktivitas.KlienID
	if klienID == nil {
		klienID = it.KlienID
	}
	tgl := p.Aktivitas.Tanggal
	if tgl.IsZero() {
		tgl = expiry.DateOf(p.At)
	}
	itemID := it.ID
	a := store.Aktivitas{
		ID:          m.newID(),
		PenggunaID:  userID,
		KlienID:     klienID,
		Klien:       m.klienName(klienID),
		ItemID:      &itemID,
		Jenis:       p.Aktivitas.Jenis,
		Teks:        p.Aktivitas.Teks,
		Tanggal:     tgl,
		DurasiMenit: p.Aktivitas.DurasiMenit,
		Jumlah:      p.Aktivitas.Jumlah,
		MataUang:    p.Aktivitas.MataUang,
		Data:        p.Aktivitas.Data,
		Dibuat:      testNow.Add(time.Duration(r.seq) * time.Millisecond),
	}
	r.aktivitas = append(r.aktivitas, a)
	it.Klien = m.klienName(it.KlienID)
	return it, nil
}
