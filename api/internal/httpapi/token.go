package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/mrramdn/tenggat/api/internal/store"
)

// TokenStore is the part of Store that keeps personal access tokens.
type TokenStore interface {
	ListTokens(ctx context.Context, userID string) ([]store.TokenAkses, error)
	CreateToken(ctx context.Context, userID string, in store.TokenInput) (store.TokenAkses, error)
	DeleteToken(ctx context.Context, userID, id string) error
	UserByToken(ctx context.Context, hash string, now time.Time) (store.Pengguna, store.TokenAkses, error)
}

func (s *Server) listTokens(w http.ResponseWriter, _ *http.Request, _ store.Pengguna) {
	writeError(w, http.StatusNotImplemented, "Belum tersedia.", nil)
}

func (s *Server) createToken(w http.ResponseWriter, _ *http.Request, _ store.Pengguna) {
	writeError(w, http.StatusNotImplemented, "Belum tersedia.", nil)
}

func (s *Server) deleteToken(w http.ResponseWriter, _ *http.Request, _ store.Pengguna) {
	writeError(w, http.StatusNotImplemented, "Belum tersedia.", nil)
}
