package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/auth"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// TokenStore is the part of Store that keeps personal access tokens.
type TokenStore interface {
	ListTokens(ctx context.Context, userID string) ([]store.TokenAkses, error)
	CreateToken(ctx context.Context, userID string, in store.TokenInput) (store.TokenAkses, error)
	DeleteToken(ctx context.Context, userID, id string) error
	UserByToken(ctx context.Context, hash string, now time.Time) (store.Pengguna, store.TokenAkses, error)
}

const defaultTokenDays = 90

// tokenJSON is the API form of a personal access token, without its secret or hash.
type tokenJSON struct {
	ID              string     `json:"id"`
	Nama            string     `json:"nama"`
	Awalan          string     `json:"awalan"`
	Cakupan         string     `json:"cakupan"`
	Dibuat          time.Time  `json:"dibuat"`
	TerakhirDipakai *time.Time `json:"terakhir_dipakai"`
	Kedaluwarsa     *time.Time `json:"kedaluwarsa"`
}

func toTokenJSON(t store.TokenAkses) tokenJSON {
	return tokenJSON{
		ID:              t.ID,
		Nama:            t.Nama,
		Awalan:          t.Awalan,
		Cakupan:         t.Cakupan,
		Dibuat:          t.Dibuat,
		TerakhirDipakai: t.TerakhirDipakai,
		Kedaluwarsa:     t.Kedaluwarsa,
	}
}

// tokenRequest is the body of POST /api/token. BerlakuHari is kept raw so an
// omitted field (90-day default) can be told apart from an explicit null (no expiry).
type tokenRequest struct {
	Nama        string          `json:"nama"`
	Cakupan     string          `json:"cakupan"`
	BerlakuHari json.RawMessage `json:"berlaku_hari"`
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	list, err := s.Store.ListTokens(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]tokenJSON, 0, len(list))
	for _, t := range list {
		out = append(out, toTokenJSON(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": out})
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req tokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	nama := strings.TrimSpace(req.Nama)
	cakupan := strings.TrimSpace(req.Cakupan)
	fields := map[string]string{}
	switch n := utf8.RuneCountInString(nama); {
	case n == 0:
		fields["nama"] = "Isi nama token."
	case n > 60:
		fields["nama"] = "Nama token maksimal 60 karakter."
	}
	if cakupan != "baca" && cakupan != "tulis" {
		fields["cakupan"] = "Pilih baca atau tulis."
	}

	var kedaluwarsa *time.Time
	rawDays := bytes.TrimSpace(req.BerlakuHari)
	switch {
	case len(rawDays) == 0:
		exp := s.Now().Add(defaultTokenDays * 24 * time.Hour)
		kedaluwarsa = &exp
	case string(rawDays) == "null":
		kedaluwarsa = nil
	default:
		var days int
		if err := json.Unmarshal(rawDays, &days); err != nil || days < 1 || days > 365 {
			fields["berlaku_hari"] = "Masa berlaku harus 1 sampai 365 hari."
		} else {
			exp := s.Now().Add(time.Duration(days) * 24 * time.Hour)
			kedaluwarsa = &exp
		}
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}

	raw, hash, awalan, err := auth.NewPAT()
	if err != nil {
		s.internal(w, r, err)
		return
	}
	tok, err := s.Store.CreateToken(r.Context(), u.ID, store.TokenInput{
		Nama:        nama,
		Hash:        hash,
		Awalan:      awalan,
		Cakupan:     cakupan,
		Kedaluwarsa: kedaluwarsa,
	})
	if errors.Is(err, store.ErrTokenLimit) {
		writeError(w, http.StatusConflict, "Batas 20 token tercapai. Cabut token yang tidak dipakai.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.Header().Set("Location", "/api/token/"+tok.ID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, struct {
		Token   tokenJSON `json:"token"`
		Rahasia string    `json:"rahasia"`
	}{
		Token:   toTokenJSON(tok),
		Rahasia: raw,
	})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "Token tidak ditemukan.", nil)
		return
	}
	err := s.Store.DeleteToken(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Token tidak ditemukan.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}
