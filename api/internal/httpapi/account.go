package httpapi

import (
	"context"
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

// sessionOnlyRoutes lists endpoints that a personal access token may never
// call: a token must not raise its own power, destroy the account, publish
// data on a public link, or point outbound webhook posts at a URL.
var sessionOnlyRoutes = []struct {
	method string
	path   string
}{
	// Token management: a token must not list, mint, or revoke tokens to raise its own power.
	{http.MethodGet, "/api/token"},
	{http.MethodHead, "/api/token"},
	{http.MethodPost, "/api/token"},
	{http.MethodDelete, "/api/token/{id}"},
	// Account deletion: a leaked token must not wipe the account and all its data.
	{http.MethodDelete, "/api/saya"},
	// Outbound webhooks: a token must not configure, remove, or trigger posts to a webhook URL.
	{http.MethodPut, "/api/notifikasi"},
	{http.MethodDelete, "/api/notifikasi"},
	{http.MethodPost, "/api/notifikasi/uji"},
	// Public calendar link: a token must not publish or revoke the unauthenticated calendar feed.
	{http.MethodPost, "/api/kalender/tautan"},
	{http.MethodDelete, "/api/kalender/tautan"},
	// Public report link: a token must not publish or revoke a client's read-only report link.
	{http.MethodPost, "/api/klien/{id}/laporan/tautan"},
	{http.MethodDelete, "/api/klien/{id}/laporan/tautan"},
}

func isSessionOnly(r *http.Request) bool {
	pat := r.URL.Path
	if _, p, ok := strings.Cut(r.Pattern, " "); ok {
		pat = p
	}
	for _, route := range sessionOnlyRoutes {
		if route.method == r.Method && matchRoute(route.path, pat) {
			return true
		}
	}
	return false
}

func matchRoute(pattern, path string) bool {
	if pattern == path {
		return true
	}
	pSegs := strings.Split(pattern, "/")
	uSegs := strings.Split(path, "/")
	if len(pSegs) != len(uSegs) {
		return false
	}
	for i := range pSegs {
		if strings.HasPrefix(pSegs[i], "{") && strings.HasSuffix(pSegs[i], "}") {
			if uSegs[i] == "" {
				return false
			}
			continue
		}
		if pSegs[i] != uSegs[i] {
			return false
		}
	}
	return true
}

// lookupToken looks up a raw personal access token ("tgt_...") and returns its
// owner and token row, or store.ErrNotFound when the prefix is wrong or the
// token is unknown, expired, or revoked.
func (s *Server) lookupToken(ctx context.Context, raw string) (store.Pengguna, store.TokenAkses, error) {
	if !strings.HasPrefix(raw, "tgt_") {
		return store.Pengguna{}, store.TokenAkses{}, store.ErrNotFound
	}
	return s.Store.UserByToken(ctx, auth.HashToken(raw), s.Now())
}

// authenticate resolves the caller from an Authorization: Bearer header or,
// when no Authorization header is sent, from the session cookie. For a Bearer
// token it also returns the token row; for a cookie session the token pointer
// is nil. It writes the error response itself and returns false on failure.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (store.Pengguna, *store.TokenAkses, bool) {
	if _, hasAuth := r.Header["Authorization"]; hasAuth {
		raw, ok := bearerToken(r)
		var u store.Pengguna
		var tok store.TokenAkses
		err := store.ErrNotFound
		if ok {
			u, tok, err = s.lookupToken(r.Context(), raw)
		}
		if errors.Is(err, store.ErrNotFound) {
			if !s.allow(w, r, "token_gagal", clientIP(r), tokenFailPerMinute) {
				return store.Pengguna{}, nil, false
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="tenggat", error="invalid_token"`)
			writeError(w, http.StatusUnauthorized, "Token akses tidak dikenal, kedaluwarsa, atau sudah dicabut.", nil)
			return store.Pengguna{}, nil, false
		}
		if err != nil {
			s.internal(w, r, err)
			return store.Pengguna{}, nil, false
		}
		return u, &tok, true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "Silakan masuk terlebih dahulu.", nil)
		return store.Pengguna{}, nil, false
	}
	u, err := s.Store.UserBySession(r.Context(), auth.HashToken(c.Value), s.Now())
	if errors.Is(err, store.ErrNotFound) {
		s.clearCookie(w)
		writeError(w, http.StatusUnauthorized, "Sesi Anda berakhir. Silakan masuk lagi.", nil)
		return store.Pengguna{}, nil, false
	}
	if err != nil {
		s.internal(w, r, err)
		return store.Pengguna{}, nil, false
	}
	return u, nil, true
}

// user authenticates the request by Bearer token or session cookie and checks
// token restrictions before running next.
func (s *Server) user(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, tok, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		if tok != nil {
			if isSessionOnly(r) {
				writeError(w, http.StatusForbidden, "Token akses tidak dapat dipakai untuk tindakan ini.", nil)
				return
			}
			if tok.Cakupan == "baca" && r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="tulis"`)
				writeError(w, http.StatusForbidden, "Token ini hanya untuk membaca.", nil)
				return
			}
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
