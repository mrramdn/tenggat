package httpapi

import (
	"context"
	"net/http"

	"github.com/mrramdn/tenggat/api/internal/store"
)

// PerpanjangStore is the part of Store that records item renewals and bill payments.
type PerpanjangStore interface {
	PerpanjangItem(ctx context.Context, userID, id string, p store.Perpanjangan) (store.Item, error)
}

func (s *Server) perpanjangItem(w http.ResponseWriter, _ *http.Request, _ store.Pengguna) {
	writeError(w, http.StatusNotImplemented, "Belum tersedia.", nil)
}
