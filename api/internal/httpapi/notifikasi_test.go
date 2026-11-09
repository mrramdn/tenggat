package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	slackURL    = "https://hooks.slack.com/services/T0001/B0002/RahasiaSlackXyZ9"
	discordURL  = "https://discord.com/api/webhooks/123456789/Rahasia-Discord_tok3n"
	telegramURL = "https://api.telegram.org/bot123456:RahasiaTelegramTokenAbc/sendMessage?chat_id=-1001234567"
)

func TestParseWebhookAllowlist(t *testing.T) {
	for raw, layanan := range map[string]string{
		slackURL:   "slack",
		discordURL: "discord",
		"https://discordapp.com/api/webhooks/1/abc_DEF-1": "discord",
		telegramURL: "telegram",
		"https://api.telegram.org/bot42:Abc_def-123/sendMessage?chat_id=@kanal_tenggat": "telegram",
		"  " + slackURL + "  ": "slack",
	} {
		got, err := parseWebhook(raw)
		if err != nil || got.Layanan != layanan {
			t.Errorf("%q: %v %+v, want %s", raw, err, got, layanan)
		}
	}
	for _, raw := range []string{
		"",
		"http://hooks.slack.com/services/T0001/B0002/abc",          // not HTTPS
		"https://hooks.slack.com.evil.example/services/T/B/abc",    // look-alike host
		"https://evil.example/services/T0001/B0002/abc",            // other host
		"https://hooks.slack.com:8443/services/T0001/B0002/abc",    // port
		"https://user:pw@hooks.slack.com/services/T0001/B0002/abc", // user info
		"https://hooks.slack.com/services/T0001/B0002/abc#x",       // fragment
		"https://hooks.slack.com/services/T0001/B0002/abc?x=1",     // query
		"https://hooks.slack.com/services/T0001/../../redirect",    // path tricks
		"https://hooks.slack.com/workflows/T0001/B0002/abc",
		"https://discord.com/api/webhooks/abc/def",
		"https://discord.com/api/webhooks/1/abc/github",
		"https://canary.discord.com/api/webhooks/1/abc",
		"https://api.telegram.org/bot42:abc/sendMessage",                     // no chat
		"https://api.telegram.org/bot42:abc/sendMessage?chat_id=1&text=halo", // extra parameter
		"https://api.telegram.org/bot42:abc/sendMessage?chat_id=1&chat_id=2", // two chats
		"https://api.telegram.org/bot42:abc/getUpdates?chat_id=1",            // other method
		"https://api.telegram.org/file/bot42:abc/sendMessage?chat_id=1",
		"https://127.0.0.1/services/T0001/B0002/abc",
		"javascript:alert(1)",
		"https://hooks.slack.com/services/T0001/B0002/" + strings.Repeat("a", 500),
	} {
		if got, err := parseWebhook(raw); err == nil {
			t.Errorf("%q accepted as %+v", raw, got)
		}
	}
}

// capturingTransport sends every request to a test server instead of the real
// service, keeping the original URL for the test to check.
type capturingTransport struct {
	mu     sync.Mutex
	target *url.URL
	urls   []string
	bodies []string
}

func (c *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	c.mu.Lock()
	c.urls = append(c.urls, req.URL.String())
	c.bodies = append(c.bodies, string(body))
	c.mu.Unlock()
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host, out.Host = c.target.Scheme, c.target.Host, ""
	out.Body = io.NopCloser(bytes.NewReader(body))
	return http.DefaultTransport.RoundTrip(out)
}

func (c *capturingTransport) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.bodies...)
}

// newWebhookServer is a test server whose webhook client posts to handler.
func newWebhookServer(t *testing.T, handler http.HandlerFunc) (*Server, *memStore, *capturingTransport, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	tr := &capturingTransport{target: target}
	s, st := newTestServer()
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	s.Webhook = &http.Client{Transport: tr, Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s, st, tr, &logs
}

func TestNotifikasiSettings(t *testing.T) {
	s, _, tr, logs := newWebhookServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := s.Routes()
	a := signUp(t, h, "a@contoh.id")
	b := signUp(t, h, "b@contoh.id")

	off := decode[notifikasiJSON](t, do(t, h, "GET", "/api/notifikasi", "", a))
	if off.Aktif || off.Layanan != nil || off.Tujuan != nil || off.Terakhir != nil {
		t.Errorf("before PUT %+v", off)
	}
	rec := do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+discordURL+`"}`, a)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Rahasia") {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	got := decode[notifikasiJSON](t, do(t, h, "GET", "/api/notifikasi", "", a))
	if !got.Aktif || deref(got.Layanan) != "discord" || deref(got.Tujuan) != "discord.com/…ok3n" {
		t.Errorf("after PUT %+v", got)
	}
	if strings.Contains(do(t, h, "GET", "/api/notifikasi", "", a).Body.String(), "Rahasia") {
		t.Error("GET returns the webhook secret")
	}
	tg := decode[notifikasiJSON](t, do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+telegramURL+`"}`, a))
	if deref(tg.Layanan) != "telegram" || deref(tg.Tujuan) != "api.telegram.org/…nAbc, chat …567" {
		t.Errorf("telegram %+v", tg)
	}

	// Another user sees nothing and cannot test A's webhook.
	if got := decode[notifikasiJSON](t, do(t, h, "GET", "/api/notifikasi", "", b)); got.Aktif {
		t.Errorf("user B sees %+v", got)
	}
	if rec := do(t, h, "POST", "/api/notifikasi/uji", "", b); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("user B test without webhook: %d", rec.Code)
	}

	for _, body := range []string{`{"webhook_url":"https://evil.example/hook"}`, `{"webhook_url":""}`} {
		rec := do(t, h, "PUT", "/api/notifikasi", body, a)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["webhook_url"] == "" {
			t.Errorf("PUT %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := do(t, h, "DELETE", "/api/notifikasi", "", a); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
	if got := decode[notifikasiJSON](t, do(t, h, "GET", "/api/notifikasi", "", a)); got.Aktif {
		t.Errorf("after DELETE %+v", got)
	}
	for _, r := range []struct{ method, path string }{{"GET", "/api/notifikasi"}, {"PUT", "/api/notifikasi"},
		{"DELETE", "/api/notifikasi"}, {"POST", "/api/notifikasi/uji"}} {
		if rec := do(t, h, r.method, r.path, `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without session: %d", r.method, r.path, rec.Code)
		}
	}
	if strings.Contains(logs.String(), "Rahasia") {
		t.Errorf("the webhook URL reached the log: %s", logs)
	}
	if len(tr.sent()) != 0 {
		t.Errorf("settings sent %d messages", len(tr.sent()))
	}
}

func TestNotifikasiTestMessage(t *testing.T) {
	status := http.StatusOK
	var mu sync.Mutex
	s, _, tr, logs := newWebhookServer(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if status == http.StatusFound {
			w.Header().Set("Location", "https://evil.example/")
		}
		w.WriteHeader(status)
	})
	h := s.Routes()
	c := signUp(t, h, "uji@contoh.id")
	for _, tc := range []struct{ url, want string }{
		{slackURL, `{"text":"Pesan uji dari Tenggat.`},
		{discordURL, `{"allowed_mentions":{"parse":[]},"content":"Pesan uji dari Tenggat.`},
		{telegramURL, `{"chat_id":"-1001234567","disable_web_page_preview":true,"text":"Pesan uji dari Tenggat.`},
	} {
		do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+tc.url+`"}`, c)
		rec := do(t, h, "POST", "/api/notifikasi/uji", "", c)
		if rec.Code != http.StatusOK {
			t.Fatalf("test message: %d %s", rec.Code, rec.Body)
		}
		sent := tr.sent()
		if last := sent[len(sent)-1]; !strings.HasPrefix(last, tc.want) {
			t.Errorf("payload %s, want prefix %s", last, tc.want)
		}
	}
	if got := tr.urls[2]; got != "https://api.telegram.org/bot123456:RahasiaTelegramTokenAbc/sendMessage" {
		t.Errorf("telegram posted to %s", got)
	}

	for code, want := range map[int]string{
		http.StatusNotFound:            "Layanan menolak pesan (status 404). Periksa URL webhook.",
		http.StatusFound:               "Layanan webhook mengalihkan permintaan. Periksa URL webhook.",
		http.StatusInternalServerError: "Layanan webhook sedang bermasalah (status 500). Coba lagi nanti.",
	} {
		mu.Lock()
		status = code
		mu.Unlock()
		n := len(tr.sent())
		rec := do(t, h, "POST", "/api/notifikasi/uji", "", c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusBadGateway || e.Galat != want {
			t.Errorf("status %d: %d %s", code, rec.Code, rec.Body)
		}
		if len(tr.sent()) != n+1 {
			t.Errorf("status %d: the redirect was followed or nothing was sent", code)
		}
	}
	if strings.Contains(logs.String(), "Rahasia") || !strings.Contains(logs.String(), "webhook gagal") {
		t.Errorf("log: %s", logs)
	}
}

func TestNotifikasiUnreachable(t *testing.T) {
	s, _ := newTestServer()
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	s.Webhook = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	h := s.Routes()
	c := signUp(t, h, "mati@contoh.id")
	do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+slackURL+`"}`, c)
	rec := do(t, h, "POST", "/api/notifikasi/uji", "", c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusBadGateway || !strings.HasPrefix(e.Galat, "Layanan webhook tidak dapat dihubungi") {
		t.Errorf("unreachable: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(logs.String(), "Rahasia") || strings.Contains(rec.Body.String(), "Rahasia") {
		t.Errorf("the URL leaked: %s %s", logs.String(), rec.Body)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDailyDigest(t *testing.T) {
	s, _, tr, _ := newWebhookServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	s.AppURL = "https://tenggat.example"
	h := s.Routes()
	a := signUp(t, h, "a@contoh.id")
	b := signUp(t, h, "b@contoh.id")
	signUp(t, h, "c@contoh.id") // no webhook: never sent anything
	do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+slackURL+`"}`, a)
	do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+discordURL+`"}`, b)

	// Today is Monday 9 Nov 2026 in Jakarta.
	for _, body := range []string{
		`{"nama":"Domain lama","jenis":"lainnya","klien":"Studio Rana","kedaluwarsa":"2026-11-06"}`, // 3 days late
		`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-11-16"}`,                             // 7 days
		`{"nama":"Hosting","jenis":"langganan","kedaluwarsa":"2026-11-17"}`,                         // 8 days: not listed
		`{"nama":"Token API","jenis":"token","kedaluwarsa":"2026-11-09"}`,                           // today
	} {
		do(t, h, "POST", "/api/item", body, a)
	}
	do(t, h, "POST", "/api/tugas", `{"judul":"Backup","klien":"Studio Rana","jatuh_tempo":"2026-11-10"}`, a)
	do(t, h, "POST", "/api/tugas", `{"judul":"Audit","jatuh_tempo":"2026-12-10"}`, a)
	// B has only something far away: nothing to send.
	do(t, h, "POST", "/api/item", `{"nama":"Hosting B","jenis":"langganan","kedaluwarsa":"2027-01-01"}`, b)

	early := time.Date(2026, 11, 9, 0, 59, 0, 0, time.UTC) // 07.59 WIB
	if n := s.sendDigests(context.Background(), early); n != 0 || len(tr.sent()) != 0 {
		t.Fatalf("before 08.00: %d sent", n)
	}
	at8 := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC) // 08.00 WIB
	if n := s.sendDigests(context.Background(), at8); n != 1 {
		t.Fatalf("08.00: %d sent, want 1", n)
	}
	want := "Tenggat, ringkasan 9 Nov 2026\n\nLewat tanggal:\n- Domain lama (Studio Rana): lewat 3 hari, 6 Nov 2026" +
		"\n\nJatuh tempo 7 hari ke depan:\n- Token API: hari ini\n- Tugas Backup (Studio Rana): besok, 10 Nov 2026" +
		"\n- VPS: 7 hari lagi, 16 Nov 2026\n\nBuka Tenggat: https://tenggat.example/app"
	if got := tr.sent()[0]; got != mustJSON(map[string]string{"text": want}) {
		t.Errorf("digest\n%s\nwant\n%s", got, mustJSON(map[string]string{"text": want}))
	}
	// Once per day, also after a restart (the claim is in the store); the next day again.
	if n := s.sendDigests(context.Background(), at8.Add(3*time.Hour)); n != 0 {
		t.Errorf("second run the same day sent %d", n)
	}
	for _, c := range []*http.Cookie{a, b} {
		if got := decode[notifikasiJSON](t, do(t, h, "GET", "/api/notifikasi", "", c)); deref(got.Terakhir) != "2026-11-09" {
			t.Errorf("terakhir %+v", got)
		}
	}
	if n := s.sendDigests(context.Background(), at8.Add(24*time.Hour)); n != 1 {
		t.Errorf("next day sent %d, want 1", n)
	}
	if len(tr.sent()) != 2 {
		t.Errorf("%d messages in all, want 2", len(tr.sent()))
	}
}

func TestDigestIsCapped(t *testing.T) {
	s, _, tr, _ := newWebhookServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	h := s.Routes()
	c := signUp(t, h, "banyak@contoh.id")
	do(t, h, "PUT", "/api/notifikasi", `{"webhook_url":"`+slackURL+`"}`, c)
	for i := range 25 {
		do(t, h, "POST", "/api/item", `{"nama":"Aset `+string(rune('A'+i))+`","jenis":"lainnya","kedaluwarsa":"2026-11-10"}`, c)
	}
	s.sendDigests(context.Background(), time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC))
	got := tr.sent()
	if len(got) != 1 || !strings.Contains(got[0], "dan 5 lainnya.") || strings.Contains(got[0], "Buka Tenggat") {
		t.Errorf("capped digest %v", got)
	}
}
