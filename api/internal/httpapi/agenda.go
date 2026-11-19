package httpapi

import (
	"net/http"

	"github.com/mrramdn/tenggat/api/internal/store"
)

func (s *Server) getAgenda(w http.ResponseWriter, _ *http.Request, _ store.Pengguna) {
	writeError(w, http.StatusNotImplemented, "Belum tersedia.", nil)
}
