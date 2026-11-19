package httpapi

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/auth"
)

type tokenCreatedJSON struct {
	Token   tokenJSON `json:"token"`
	Rahasia string    `json:"rahasia"`
}

type tokenListJSON struct {
	Token []tokenJSON `json:"token"`
}

// doBearer sends a request with an Authorization header and optional cookies.
func doBearer(t *testing.T, h http.Handler, method, path, body, authHeader string, cookie ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", authHeader)
	for _, c := range cookie {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mintToken(t *testing.T, h http.Handler, c *http.Cookie, body string) tokenCreatedJSON {
	t.Helper()
	rec := do(t, h, "POST", "/api/token", body, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint token %s: %d %s", body, rec.Code, rec.Body)
	}
	return decode[tokenCreatedJSON](t, rec)
}

func TestTokenCRUDExpiryAndSecretVisibility(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "token-a@contoh.id")
	b := signUp(t, h, "token-b@contoh.id")

	// Default expiry is 90 days when berlaku_hari is omitted.
	rec := do(t, h, "POST", "/api/token", `{"nama":"  Skrip Backup  ","cakupan":"baca"}`, a)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create default token: %d %s", rec.Code, rec.Body)
	}
	t1 := decode[tokenCreatedJSON](t, rec)
	if rec.Header().Get("Location") != "/api/token/"+t1.Token.ID {
		t.Errorf("Location = %q, want /api/token/%s", rec.Header().Get("Location"), t1.Token.ID)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(t1.Rahasia, "tgt_") || len(t1.Rahasia) != 47 || t1.Token.Awalan != t1.Rahasia[:8] {
		t.Errorf("secret = %q, awalan = %q", t1.Rahasia, t1.Token.Awalan)
	}
	if t1.Token.Nama != "Skrip Backup" || t1.Token.Cakupan != "baca" || t1.Token.TerakhirDipakai != nil {
		t.Errorf("token = %+v", t1.Token)
	}
	wantDefaultExp := testNow.Add(90 * 24 * time.Hour)
	if t1.Token.Kedaluwarsa == nil || !t1.Token.Kedaluwarsa.Equal(wantDefaultExp) {
		t.Errorf("default expiry = %v, want %v", t1.Token.Kedaluwarsa, wantDefaultExp)
	}

	// Explicit null means no expiry.
	t2 := mintToken(t, h, a, `{"nama":"Claude Desktop","cakupan":"tulis","berlaku_hari":null}`)
	if t2.Token.Kedaluwarsa != nil {
		t.Errorf("null berlaku_hari: kedaluwarsa = %v, want nil", t2.Token.Kedaluwarsa)
	}

	// Custom days in 1..365.
	t3 := mintToken(t, h, a, `{"nama":"CI Deploy","cakupan":"tulis","berlaku_hari":30}`)
	want30 := testNow.Add(30 * 24 * time.Hour)
	if t3.Token.Kedaluwarsa == nil || !t3.Token.Kedaluwarsa.Equal(want30) {
		t.Errorf("30-day expiry = %v, want %v", t3.Token.Kedaluwarsa, want30)
	}

	// Invalid inputs: 0 and 366 days, bad name, bad scope.
	for _, tc := range []struct {
		body  string
		field string
	}{
		{`{"nama":"Nol Hari","cakupan":"baca","berlaku_hari":0}`, "berlaku_hari"},
		{`{"nama":"Terlalu Lama","cakupan":"baca","berlaku_hari":366}`, "berlaku_hari"},
		{`{"nama":"Negatif","cakupan":"baca","berlaku_hari":-1}`, "berlaku_hari"},
		{`{"nama":"Bukan Angka","cakupan":"baca","berlaku_hari":"90"}`, "berlaku_hari"},
		{`{"nama":"   ","cakupan":"baca"}`, "nama"},
		{`{"nama":"` + strings.Repeat("a", 61) + `","cakupan":"baca"}`, "nama"},
		{`{"nama":"Cakupan Salah","cakupan":"admin"}`, "cakupan"},
	} {
		bad := do(t, h, "POST", "/api/token", tc.body, a)
		if bad.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, want 422", tc.body, bad.Code)
			continue
		}
		if e := decode[apiError](t, bad); e.Field[tc.field] == "" {
			t.Errorf("%s: missing field %q in %v", tc.body, tc.field, e.Field)
		}
	}

	// The secret appears only in the 201 response and never in GET /api/token.
	listRec := do(t, h, "GET", "/api/token", "", a)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list tokens: %d %s", listRec.Code, listRec.Body)
	}
	rawList := listRec.Body.String()
	for _, secret := range []string{t1.Rahasia, t2.Rahasia, t3.Rahasia, `"rahasia"`} {
		if strings.Contains(rawList, secret) {
			t.Errorf("GET /api/token leaked %q in %s", secret, rawList)
		}
	}
	listed := decode[tokenListJSON](t, listRec)
	if len(listed.Token) != 3 || listed.Token[0].ID != t3.Token.ID || listed.Token[1].ID != t2.Token.ID || listed.Token[2].ID != t1.Token.ID {
		t.Errorf("listed tokens = %+v", listed.Token)
	}

	// Another user sees an empty list and gets 404 deleting A's token.
	if other := decode[tokenListJSON](t, do(t, h, "GET", "/api/token", "", b)); len(other.Token) != 0 {
		t.Errorf("user B tokens = %+v", other.Token)
	}
	if rec := do(t, h, "DELETE", "/api/token/"+t1.Token.ID, "", b); rec.Code != http.StatusNotFound {
		t.Errorf("user B delete A's token: %d, want 404", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/token/bukan-uuid", "", a); rec.Code != http.StatusNotFound {
		t.Errorf("delete non-uuid: %d, want 404", rec.Code)
	}

	// Cap at 20 tokens per account; deleting frees a slot.
	for i := 3; i < 20; i++ {
		mintToken(t, h, a, `{"nama":"Token `+strconv.Itoa(i+1)+`","cakupan":"baca"}`)
	}
	over := do(t, h, "POST", "/api/token", `{"nama":"Token 21","cakupan":"baca"}`, a)
	if e := decode[apiError](t, over); over.Code != http.StatusConflict || e.Galat != "Batas 20 token tercapai. Cabut token yang tidak dipakai." {
		t.Errorf("21st token: %d %+v", over.Code, e)
	}
	if rec := do(t, h, "DELETE", "/api/token/"+t3.Token.ID, "", a); rec.Code != http.StatusNoContent {
		t.Fatalf("delete token: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/token/"+t3.Token.ID, "", a); rec.Code != http.StatusNotFound {
		t.Errorf("second delete: %d, want 404", rec.Code)
	}
	mintToken(t, h, a, `{"nama":"Pengganti","cakupan":"baca"}`)
}

func TestBearerAuthAndScopes(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "bearer@contoh.id")

	baca := mintToken(t, h, c, `{"nama":"Baca Saja","cakupan":"baca"}`)
	tulis := mintToken(t, h, c, `{"nama":"Baca Tulis","cakupan":"tulis"}`)

	// A tulis token can POST (create a client) and GET.
	createRec := doBearer(t, h, "POST", "/api/klien", `{"nama":"Toko Kue Mawar"}`, "Bearer "+tulis.Rahasia)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/klien with tulis token: %d %s", createRec.Code, createRec.Body)
	}
	klien := decode[klienJSON](t, createRec)

	getRec := doBearer(t, h, "GET", "/api/klien", "", "Bearer "+baca.Rahasia)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/klien with baca token: %d %s", getRec.Code, getRec.Body)
	}
	if list := decode[struct{ Klien []klienJSON }](t, getRec); len(list.Klien) != 1 || list.Klien[0].ID != klien.ID {
		t.Errorf("GET /api/klien = %+v", list.Klien)
	}
	if headRec := doBearer(t, h, "HEAD", "/api/klien", "", "Bearer "+baca.Rahasia); headRec.Code != http.StatusOK {
		t.Errorf("HEAD /api/klien with baca token: %d, want 200", headRec.Code)
	}

	// terakhir_dipakai is stamped after using the tokens.
	listed := decode[tokenListJSON](t, do(t, h, "GET", "/api/token", "", c))
	for _, tok := range listed.Token {
		if tok.TerakhirDipakai == nil || !tok.TerakhirDipakai.Equal(testNow) {
			t.Errorf("token %s terakhir_dipakai = %v, want %v", tok.Nama, tok.TerakhirDipakai, testNow)
		}
	}

	// A baca token gets 403 with WWW-Authenticate on POST, PUT, DELETE.
	for _, r := range []struct{ method, path, body string }{
		{"POST", "/api/klien", `{"nama":"Klien Baru"}`},
		{"PUT", "/api/klien/" + klien.ID, `{"nama":"Diubah"}`},
		{"DELETE", "/api/klien/" + klien.ID, ""},
		{"POST", "/api/item", `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01"}`},
	} {
		rec := doBearer(t, h, r.method, r.path, r.body, "Bearer "+baca.Rahasia)
		if rec.Code != http.StatusForbidden {
			t.Errorf("baca token %s %s: %d, want 403", r.method, r.path, rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer error="insufficient_scope", scope="tulis"` {
			t.Errorf("baca token %s %s WWW-Authenticate = %q", r.method, r.path, got)
		}
		if e := decode[apiError](t, rec); e.Galat != "Token ini hanya untuk membaca." {
			t.Errorf("baca token %s %s galat = %q", r.method, r.path, e.Galat)
		}
	}

	// Server.lookupToken resolves a raw token directly.
	if u, tok, err := s.lookupToken(t.Context(), tulis.Rahasia); err != nil || u.Email != "bearer@contoh.id" || tok.ID != tulis.Token.ID {
		t.Errorf("lookupToken = %+v, %+v, %v", u, tok, err)
	}
}

func TestRevokedExpiredAndMalformedTokensAndPrecedence(t *testing.T) {
	s, _ := newTestServer()
	now := testNow
	s.Now = func() time.Time { return now }
	h := s.Routes()

	a := signUp(t, h, "alice@contoh.id")
	b := signUp(t, h, "bob@contoh.id")

	tokA := mintToken(t, h, a, `{"nama":"Token Alice","cakupan":"tulis","berlaku_hari":1}`)
	revoked := mintToken(t, h, a, `{"nama":"Akan Dicabut","cakupan":"tulis"}`)

	// Bearer wins over a valid session cookie of another user.
	rec := doBearer(t, h, "GET", "/api/saya", "", "Bearer "+tokA.Rahasia, b)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/saya with Bearer + B cookie: %d %s", rec.Code, rec.Body)
	}
	if me := decode[accountJSON](t, rec); me.Pengguna.Email != "alice@contoh.id" {
		t.Errorf("Bearer + B cookie signed in as %q, want alice@contoh.id", me.Pengguna.Email)
	}

	// Revoke one token and expire the other.
	if del := do(t, h, "DELETE", "/api/token/"+revoked.Token.ID, "", a); del.Code != http.StatusNoContent {
		t.Fatalf("revoke token: %d", del.Code)
	}
	now = testNow.Add(25 * time.Hour)

	for name, header := range map[string]string{
		"revoked":            "Bearer " + revoked.Rahasia,
		"expired":            "Bearer " + tokA.Rahasia,
		"unknown":            "Bearer tgt_tidak_dikenal_123456789012345678901234567",
		"bearer no token":    "Bearer",
		"bearer empty token": "Bearer ",
		"basic scheme":       "Basic dXNlcjpwYXNz",
		"lowercase scheme":   "bearer " + tokA.Rahasia,
		"without tgt prefix": "Bearer bukan_tgt_1234567890",
	} {
		// Send the valid session cookie along with the bad header: it must still
		// answer 401 with WWW-Authenticate and must never clear the session cookie.
		rec := doBearer(t, h, "GET", "/api/item", "", header, a)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="tenggat", error="invalid_token"` {
			t.Errorf("%s: WWW-Authenticate = %q", name, got)
		}
		if e := decode[apiError](t, rec); e.Galat != "Token akses tidak dikenal, kedaluwarsa, atau sudah dicabut." {
			t.Errorf("%s: galat = %q", name, e.Galat)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: set cookies %+v; session cookie must not be cleared", name, rec.Result().Cookies())
		}
	}

	// Alice's session cookie still works after all the failed Bearer requests.
	if rec := do(t, h, "GET", "/api/saya", "", a); rec.Code != http.StatusOK {
		t.Errorf("cookie after failed Bearer requests: %d, want 200", rec.Code)
	}
}

func TestSessionOnlyRoutesRejectBearerAndWorkWithCookie(t *testing.T) {
	s, _, _, _ := newWebhookServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := s.Routes()
	c := signUp(t, h, "sesi-saja@contoh.id")

	tulis := mintToken(t, h, c, `{"nama":"Token Penuh","cakupan":"tulis"}`)
	baca := mintToken(t, h, c, `{"nama":"Token Baca","cakupan":"baca"}`)
	victim := mintToken(t, h, c, `{"nama":"Token Target","cakupan":"baca"}`)
	klien := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Klien Laporan"}`, c))

	routes := []struct {
		method     string
		path       string
		body       string
		cookieCode int
	}{
		{"GET", "/api/token", "", http.StatusOK},
		{"POST", "/api/token", `{"nama":"Dari Cookie","cakupan":"baca"}`, http.StatusCreated},
		{"DELETE", "/api/token/" + victim.Token.ID, "", http.StatusNoContent},
		{"PUT", "/api/notifikasi", `{"webhook_url":"` + slackURL + `"}`, http.StatusOK},
		{"POST", "/api/notifikasi/uji", "", http.StatusOK},
		{"DELETE", "/api/notifikasi", "", http.StatusNoContent},
		{"POST", "/api/kalender/tautan", "", http.StatusCreated},
		{"DELETE", "/api/kalender/tautan", "", http.StatusNoContent},
		{"POST", "/api/klien/" + klien.ID + "/laporan/tautan", "", http.StatusCreated},
		{"DELETE", "/api/klien/" + klien.ID + "/laporan/tautan", "", http.StatusNoContent},
		{"DELETE", "/api/saya", "", http.StatusNoContent},
	}

	for _, r := range routes {
		for _, tok := range []string{tulis.Rahasia, baca.Rahasia} {
			rec := doBearer(t, h, r.method, r.path, r.body, "Bearer "+tok)
			if rec.Code != http.StatusForbidden {
				t.Errorf("Bearer %s %s: %d, want 403 (%s)", r.method, r.path, rec.Code, rec.Body)
			}
			if e := decode[apiError](t, rec); e.Galat != "Token akses tidak dapat dipakai untuk tindakan ini." {
				t.Errorf("Bearer %s %s galat = %q", r.method, r.path, e.Galat)
			}
		}
		rec := do(t, h, r.method, r.path, r.body, c)
		if rec.Code != r.cookieCode {
			t.Errorf("Cookie %s %s: %d, want %d (%s)", r.method, r.path, rec.Code, r.cookieCode, rec.Body)
		}
	}
}

func TestBearerRateLimitKeyAndFailedLookupBucket(t *testing.T) {
	s, _ := newTestServer()
	mc := newMemCache()
	s.Cache = mc
	s.LimitPerMinute = 2
	h := s.Routes()
	c := signUp(t, h, "limit-token@contoh.id")

	tokA := mintToken(t, h, c, `{"nama":"Token A","cakupan":"tulis"}`)
	tokB := mintToken(t, h, c, `{"nama":"Token B","cakupan":"tulis"}`)

	reqA := httptest.NewRequest("POST", "/api/item", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokA.Rahasia)
	keyA := clientKey(reqA)
	wantKeyA := "t:" + auth.HashToken(tokA.Rahasia)[:16]
	if keyA != wantKeyA || !strings.HasPrefix(keyA, "t:") {
		t.Fatalf("clientKey(Bearer) = %q, want %q", keyA, wantKeyA)
	}

	body := `{"nama":"Aset","jenis":"token","kedaluwarsa":"2027-01-01"}`
	for i := range 2 {
		if rec := doBearer(t, h, "POST", "/api/item", body, "Bearer "+tokA.Rahasia); rec.Code != http.StatusCreated {
			t.Fatalf("tokA request %d: %d", i+1, rec.Code)
		}
	}
	if rec := doBearer(t, h, "POST", "/api/item", body, "Bearer "+tokA.Rahasia); rec.Code != http.StatusTooManyRequests {
		t.Errorf("tokA 3rd request: %d, want 429", rec.Code)
	}

	// Inspect memCache to confirm the rate-limit bucket key uses the t: token prefix.
	mc.mu.Lock()
	countA := mc.counts["tulis"+wantKeyA]
	mc.mu.Unlock()
	if countA != 3 {
		t.Errorf("memCache counts[%q] = %d, want 3", "tulis"+wantKeyA, countA)
	}

	// Token B from the same IP has its own t: key and is not rate-limited by Token A.
	if rec := doBearer(t, h, "POST", "/api/item", body, "Bearer "+tokB.Rahasia); rec.Code != http.StatusCreated {
		t.Errorf("tokB request from same IP: %d, want 201", rec.Code)
	}

	// Failed Bearer lookups count in token_gagal by client IP, 20 per minute.
	for i := range 20 {
		rec := doBearer(t, h, "GET", "/api/item", "", "Bearer tgt_salah_"+strconv.Itoa(i))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("failed lookup %d: %d, want 401", i+1, rec.Code)
		}
	}
	over := doBearer(t, h, "GET", "/api/item", "", "Bearer tgt_salah_21")
	if over.Code != http.StatusTooManyRequests || over.Header().Get("Retry-After") == "" {
		t.Errorf("21st failed lookup: %d Retry-After=%q, want 429", over.Code, over.Header().Get("Retry-After"))
	}
}

func TestLogsNeverContainTokenSecret(t *testing.T) {
	s, _ := newTestServer()
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	h := s.Routes()
	c := signUp(t, h, "log@contoh.id")

	tulis := mintToken(t, h, c, `{"nama":"Token Log","cakupan":"tulis"}`)
	baca := mintToken(t, h, c, `{"nama":"Token Baca Log","cakupan":"baca"}`)

	do(t, h, "GET", "/api/token", "", c)
	doBearer(t, h, "GET", "/api/item", "", "Bearer "+tulis.Rahasia)
	doBearer(t, h, "POST", "/api/klien", `{"nama":"Klien Log"}`, "Bearer "+tulis.Rahasia)
	doBearer(t, h, "POST", "/api/klien", `{"nama":"Ditolak"}`, "Bearer "+baca.Rahasia)
	doBearer(t, h, "GET", "/api/token", "", "Bearer "+tulis.Rahasia)
	doBearer(t, h, "GET", "/api/item", "", "Bearer tgt_rahasia_tidak_valid_1234567890")
	do(t, h, "DELETE", "/api/token/"+tulis.Token.ID, "", c)

	out := logs.String()
	if out == "" {
		t.Fatal("expected request log output, got empty buffer")
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "tgt_") {
			t.Errorf("log line contains tgt_: %s", line)
		}
	}
}

// A made-up token must not buy a fresh rate-limit bucket on routes that never
// check it: ten wrong passwords are the limit whatever the Authorization header says.
func TestBearerHeaderDoesNotBypassSignInLimit(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	signUp(t, h, "korban@contoh.id")
	body := `{"email":"korban@contoh.id","kata_sandi":"tebakan-salah"}`
	last := 0
	for i := range 12 {
		rec := doBearer(t, h, "POST", "/api/sesi", body, fmt.Sprintf("Bearer tgt_palsu%038d", i))
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("12th sign-in attempt with a new made-up token each time: %d, want 429", last)
	}
}
