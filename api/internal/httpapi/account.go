package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/auth"
	"github.com/mrramdn/tenggat/api/internal/store"
)

const (
	sessionCookie = "tenggat_sesi"
	sessionTTL    = 30 * 24 * time.Hour
)

// accountJSON wraps the signed-in user for the client.
type accountJSON struct {
	Pengguna store.Pengguna `json:"pengguna"`
}

type signupRequest struct {
	Nama      string `json:"nama"`
	Email     string `json:"email"`
	KataSandi string `json:"kata_sandi"`
}

type loginRequest struct {
	Email     string `json:"email"`
	KataSandi string `json:"kata_sandi"`
}

// authedHandler is a handler that runs only for a signed-in user.
type authedHandler func(http.ResponseWriter, *http.Request, store.Pengguna)

// user looks up the session cookie and answers 401 when there is no valid session.
func (s *Server) user(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "Silakan masuk terlebih dahulu.", nil)
			return
		}
		u, err := s.Store.UserBySession(r.Context(), auth.HashToken(c.Value), s.Now())
		if errors.Is(err, store.ErrNotFound) {
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "Sesi Anda berakhir. Silakan masuk lagi.", nil)
			return
		}
		if err != nil {
			s.internal(w, r, err)
			return
		}
		next(w, r, u)
	}
}

// sameOrigin refuses requests that change data when the browser says they come
// from another site. The session cookie is SameSite=Lax already; this is a second
// line. Requests without an Origin header (curl, scripts) are let through.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeError(w, http.StatusForbidden, "Permintaan dari situs lain ditolak.", nil)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	nama := strings.TrimSpace(req.Nama)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	fields := map[string]string{}
	// An empty field asks for the value; a limit is named only when the value breaks it.
	switch n := utf8.RuneCountInString(nama); {
	case n == 0:
		fields["nama"] = "Isi nama Anda."
	case n < 2:
		fields["nama"] = "Nama minimal 2 karakter."
	case n > 80:
		fields["nama"] = "Nama maksimal 80 karakter."
	}
	switch {
	case email == "":
		fields["email"] = "Isi email Anda."
	case !validEmail(email):
		fields["email"] = "Tulis email yang valid, misalnya nama@contoh.id."
	}
	switch n := utf8.RuneCountInString(req.KataSandi); {
	case n == 0:
		fields["kata_sandi"] = "Isi kata sandi Anda."
	case n < 8:
		fields["kata_sandi"] = "Kata sandi minimal 8 karakter."
	case n > 128:
		fields["kata_sandi"] = "Kata sandi maksimal 128 karakter."
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	hash, err := auth.HashPassword(req.KataSandi)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	u, err := s.Store.CreateUser(r.Context(), nama, email, hash)
	if errors.Is(err, store.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "Email ini sudah terdaftar.",
			map[string]string{"email": "Email ini sudah terdaftar. Silakan masuk."})
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if !s.startSession(w, r, u) {
		return
	}
	writeJSON(w, http.StatusCreated, accountJSON{Pengguna: u})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	fields := map[string]string{}
	switch {
	case email == "":
		fields["email"] = "Isi email Anda."
	case !validEmail(email):
		fields["email"] = "Tulis email yang valid, misalnya nama@contoh.id."
	}
	if req.KataSandi == "" {
		fields["kata_sandi"] = "Isi kata sandi Anda."
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		auth.SpendTime(req.KataSandi)
		writeError(w, http.StatusUnauthorized, "Email atau kata sandi salah.", nil)
		return
	case err != nil:
		s.internal(w, r, err)
		return
	case !auth.CheckPassword(u.HashSandi, req.KataSandi):
		writeError(w, http.StatusUnauthorized, "Email atau kata sandi salah.", nil)
		return
	}
	if !s.startSession(w, r, u) {
		return
	}
	writeJSON(w, http.StatusOK, accountJSON{Pengguna: u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.Store.DeleteSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			s.internal(w, r, err)
			return
		}
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// deleteMe removes the account with all its assets and sessions: the right to
// have personal data deleted (UU 27/2022 Pasal 43).
func (s *Server) deleteMe(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	if err := s.Store.DeleteUser(r.Context(), u.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// startSession stores a new session and sets its cookie. It writes the error
// response itself and returns false when that fails.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.Pengguna) bool {
	token, hash, err := auth.NewToken()
	if err != nil {
		s.internal(w, r, err)
		return false
	}
	expires := s.Now().Add(sessionTTL)
	if err := s.Store.CreateSession(r.Context(), hash, u.ID, expires); err != nil {
		s.internal(w, r, err)
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return true
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

// validEmail accepts a plain address with a dot in the domain, nothing else.
func validEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return false
	}
	at := strings.LastIndex(email, "@")
	return at > 0 && strings.Contains(email[at+1:], ".")
}
