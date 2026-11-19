package httpapi

import (
	"errors"
	"net/http"

	"github.com/mrramdn/tenggat/api/internal/store"
)

func (s *Server) getTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeTugasNotFound(w)
		return
	}
	t, err := s.Store.GetTugas(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTugasJSON(t, s.Now()))
}
