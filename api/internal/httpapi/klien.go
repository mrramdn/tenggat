package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/store"
)

// KlienStore is the part of store.Store that keeps clients.
type KlienStore interface {
	ListKlien(ctx context.Context, userID string, now time.Time) ([]store.KlienRingkas, error)
	GetKlien(ctx context.Context, userID, id string) (store.Klien, error)
	CreateKlien(ctx context.Context, userID, nama, catatan string) (store.Klien, error)
	UpdateKlien(ctx context.Context, userID, id, nama, catatan string) (store.Klien, error)
	DeleteKlien(ctx context.Context, userID, id string) error
	EnsureKlien(ctx context.Context, userID, nama string) (store.Klien, error)
}

// klienJSON is the API form of a client with figures over its assets.
type klienJSON struct {
	ID          string  `json:"id"`
	Nama        string  `json:"nama"`
	Catatan     string  `json:"catatan"`
	JumlahAset  int     `json:"jumlah_aset"`
	Terdekat    *string `json:"terdekat"`      // TTTT-BB-HH, null without assets
	Biaya30Hari int64   `json:"biaya_30_hari"` // rupiah due from today to 30 days ahead
}

func toKlienJSON(k store.KlienRingkas) klienJSON {
	out := klienJSON{ID: k.ID, Nama: k.Nama, Catatan: k.Catatan, JumlahAset: k.JumlahAset, Biaya30Hari: k.Biaya30Hari}
	if k.Terdekat != nil {
		d := k.Terdekat.Format(time.DateOnly)
		out.Terdekat = &d
	}
	return out
}

// klienRequest is the body of POST /api/klien and PUT /api/klien/{id}.
type klienRequest struct {
	Nama    string `json:"nama"`
	Catatan string `json:"catatan"`
}

func (req klienRequest) validate() (nama, catatan string, fields map[string]string) {
	nama, catatan = strings.TrimSpace(req.Nama), strings.TrimSpace(req.Catatan)
	fields = map[string]string{}
	switch n := utf8.RuneCountInString(nama); {
	case n == 0:
		fields["nama"] = "Isi nama klien."
	case n > 80:
		fields["nama"] = "Nama klien paling panjang 80 karakter."
	}
	if utf8.RuneCountInString(catatan) > 500 {
		fields["catatan"] = "Catatan paling panjang 500 karakter."
	}
	if len(fields) == 0 {
		fields = nil
	}
	return nama, catatan, fields
}

func writeKlienNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Klien tidak ditemukan.", nil)
}

func writeKlienTaken(w http.ResponseWriter) {
	writeError(w, http.StatusConflict, "Nama klien sudah dipakai.",
		map[string]string{"nama": "Anda sudah punya klien dengan nama ini."})
}

func (s *Server) listKlien(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	s.cachedRead(w, r, u.ID, "klien", func(now time.Time) (any, error) {
		list, err := s.Store.ListKlien(r.Context(), u.ID, now)
		if err != nil {
			return nil, err
		}
		out := make([]klienJSON, 0, len(list))
		for _, k := range list {
			out = append(out, toKlienJSON(k))
		}
		return map[string]any{"klien": out}, nil
	})
}

func (s *Server) createKlien(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req klienRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	nama, catatan, fields := req.validate()
	if fields != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	k, err := s.Store.CreateKlien(r.Context(), u.ID, nama, catatan)
	if errors.Is(err, store.ErrKlienTaken) {
		writeKlienTaken(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.Header().Set("Location", "/api/klien/"+k.ID)
	writeJSON(w, http.StatusCreated, toKlienJSON(store.KlienRingkas{Klien: k}))
}

func (s *Server) updateKlien(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeKlienNotFound(w)
		return
	}
	var req klienRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	nama, catatan, fields := req.validate()
	if fields != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	_, err := s.Store.UpdateKlien(r.Context(), u.ID, id, nama, catatan)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeKlienNotFound(w)
		return
	case errors.Is(err, store.ErrKlienTaken):
		writeKlienTaken(w)
		return
	case err != nil:
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	// Answer with the same figures GET /api/klien shows for this client.
	list, err := s.Store.ListKlien(r.Context(), u.ID, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	for _, k := range list {
		if k.ID == id {
			writeJSON(w, http.StatusOK, toKlienJSON(k))
			return
		}
	}
	writeKlienNotFound(w) // deleted between the two queries
}

func (s *Server) deleteKlien(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeKlienNotFound(w)
		return
	}
	err := s.Store.DeleteKlien(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeKlienNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// resolveKlien sets in.KlienID and in.Klien from an item request. A klien_id
// wins over a name and must be one of the user's clients; a name that is not
// a client yet becomes one; an empty name means no client.
func (s *Server) resolveKlien(ctx context.Context, userID string, req itemRequest, in *store.Input) (map[string]string, error) {
	if req.KlienID != nil && *req.KlienID != "" {
		if !uuidPattern.MatchString(*req.KlienID) {
			return map[string]string{"klien": "Klien tidak ditemukan."}, nil
		}
		k, err := s.Store.GetKlien(ctx, userID, *req.KlienID)
		if errors.Is(err, store.ErrNotFound) {
			return map[string]string{"klien": "Klien tidak ditemukan."}, nil
		}
		if err != nil {
			return nil, err
		}
		in.KlienID, in.Klien = &k.ID, k.Nama
		return nil, nil
	}
	if in.Klien == "" {
		in.KlienID = nil
		return nil, nil
	}
	k, err := s.Store.EnsureKlien(ctx, userID, in.Klien)
	if err != nil {
		return nil, err
	}
	in.KlienID, in.Klien = &k.ID, k.Nama
	return nil, nil
}

// prepareItem validates an item request and resolves its client. It writes the
// error response and returns false when the item cannot be stored.
func (s *Server) prepareItem(w http.ResponseWriter, r *http.Request, userID string, req itemRequest) (store.Input, bool) {
	in, fields := s.prepare(r.Context(), req)
	if fields == nil {
		var err error
		if fields, err = s.resolveKlien(r.Context(), userID, req, &in); err != nil {
			s.internal(w, r, err)
			return in, false
		}
	}
	if fields != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return in, false
	}
	return in, true
}
