package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// NotifikasiStore is the part of Store that keeps each user's own webhook.
type NotifikasiStore interface {
	Webhook(ctx context.Context, userID string) (*string, *time.Time, error)
	SetWebhook(ctx context.Context, userID string, url *string) error
	DigestDue(ctx context.Context, today time.Time) ([]store.WebhookUser, error)
	ClaimDigest(ctx context.Context, userID string, today time.Time) (bool, error)
}

const (
	webhookTimeout   = 8 * time.Second
	webhookMaxURL    = 512
	webhookMaxText   = 1900 // Discord takes 2000 characters per message
	webhookTestLimit = 5    // test messages per minute per client
	digestHour       = 8    // the daily digest goes out from 08.00 in Jakarta
	digestDays       = 7    // overdue, or due within this many days
	digestMaxLines   = 20
)

// webhookTarget is a validated webhook: which service, where to POST, and for
// Telegram the chat.
type webhookTarget struct {
	Layanan string // slack, discord or telegram
	Post    string // the URL to POST to (Telegram's without its query)
	ChatID  string
	Tujuan  string // a masked form that is safe to show and log
}

var (
	slackPath    = regexp.MustCompile(`^/services/[A-Za-z0-9]+/[A-Za-z0-9]+/[A-Za-z0-9]+$`)
	discordPath  = regexp.MustCompile(`^/api/webhooks/[0-9]+/[A-Za-z0-9_-]+$`)
	telegramPath = regexp.MustCompile(`^/bot[0-9]+:[A-Za-z0-9_-]+/sendMessage$`)
	telegramChat = regexp.MustCompile(`^(-?[0-9]+|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
)

var errWebhookURL = errors.New("Gunakan URL webhook Slack (https://hooks.slack.com/services/...), Discord " +
	"(https://discord.com/api/webhooks/...), atau Telegram (https://api.telegram.org/bot<token>/sendMessage?chat_id=<id>).")

// parseWebhook accepts only the three services' own webhook URLs over HTTPS:
// no other host, port, user info or fragment, so the server never posts to an
// address the user picked.
func parseWebhook(raw string) (webhookTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > webhookMaxURL {
		return webhookTarget{}, errWebhookURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Fragment != "" ||
		u.RawFragment != "" || u.Port() != "" || u.Host != u.Hostname() {
		return webhookTarget{}, errWebhookURL
	}
	host := strings.ToLower(u.Host)
	switch {
	case host == "hooks.slack.com" && slackPath.MatchString(u.Path) && u.RawQuery == "" && !u.ForceQuery:
		return webhookTarget{Layanan: "slack", Post: "https://" + host + u.Path, Tujuan: mask(host, u.Path)}, nil
	case (host == "discord.com" || host == "discordapp.com") && discordPath.MatchString(u.Path) && u.RawQuery == "" &&
		!u.ForceQuery:
		return webhookTarget{Layanan: "discord", Post: "https://" + host + u.Path, Tujuan: mask(host, u.Path)}, nil
	case host == "api.telegram.org" && telegramPath.MatchString(u.Path):
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(q) != 1 || len(q["chat_id"]) != 1 || !telegramChat.MatchString(q.Get("chat_id")) {
			return webhookTarget{}, errWebhookURL
		}
		chat := q.Get("chat_id")
		token := strings.TrimSuffix(u.Path, "/sendMessage")
		return webhookTarget{Layanan: "telegram", Post: "https://" + host + u.Path, ChatID: chat,
			Tujuan: mask(host, token) + ", chat …" + tail(chat, 3)}, nil
	}
	return webhookTarget{}, errWebhookURL
}

// mask shows the host and the last four characters of the secret path.
func mask(host, path string) string {
	return host + "/…" + tail(path, 4)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// webhookError is a failed send: Pesan is for the user, Detail only for the
// log. Neither ever holds the URL.
type webhookError struct {
	Pesan  string
	Detail string
}

func (e *webhookError) Error() string { return e.Pesan + " (" + e.Detail + ")" }

// webhookClient posts with an 8 second timeout and never follows a redirect.
func (s *Server) webhookClient() *http.Client {
	if s.Webhook != nil {
		return s.Webhook
	}
	return &http.Client{
		Timeout:       webhookTimeout,
		Transport:     &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: webhookTimeout},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// sendWebhook posts one plain-text message in the shape each service expects.
func (s *Server) sendWebhook(ctx context.Context, t webhookTarget, text string) error {
	if utf8.RuneCountInString(text) > webhookMaxText {
		text = string([]rune(text)[:webhookMaxText-1]) + "…"
	}
	var payload any
	switch t.Layanan {
	case "slack":
		payload = map[string]string{"text": text}
	case "discord":
		payload = map[string]any{"content": text, "allowed_mentions": map[string]any{"parse": []string{}}}
	case "telegram":
		payload = map[string]any{"chat_id": t.ChatID, "text": text, "disable_web_page_preview": true}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Post, bytes.NewReader(body))
	if err != nil {
		return &webhookError{Pesan: "URL webhook tidak dapat dipakai.", Detail: "permintaan tidak dapat dibuat"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tenggat")
	resp, err := s.webhookClient().Do(req)
	if err != nil {
		detail := "tidak terhubung"
		var ue *url.Error
		if errors.As(err, &ue) {
			detail = ue.Err.Error() // url.Error itself carries the URL, its cause does not
		}
		return &webhookError{Pesan: "Layanan webhook tidak dapat dihubungi. Coba lagi sebentar lagi.", Detail: detail}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return &webhookError{Pesan: "Layanan webhook mengalihkan permintaan. Periksa URL webhook.",
			Detail: "status " + strconv.Itoa(resp.StatusCode)}
	case resp.StatusCode < 500:
		return &webhookError{Pesan: fmt.Sprintf("Layanan menolak pesan (status %d). Periksa URL webhook.", resp.StatusCode),
			Detail: "status " + strconv.Itoa(resp.StatusCode)}
	default:
		return &webhookError{Pesan: fmt.Sprintf("Layanan webhook sedang bermasalah (status %d). Coba lagi nanti.", resp.StatusCode),
			Detail: "status " + strconv.Itoa(resp.StatusCode)}
	}
}

// notifikasiJSON is GET /api/notifikasi: never the URL itself.
type notifikasiJSON struct {
	Aktif    bool    `json:"aktif"`
	Layanan  *string `json:"layanan"`
	Tujuan   *string `json:"tujuan"`
	Terakhir *string `json:"terakhir"` // TTTT-BB-HH of the last daily digest run, null before the first
}

func toNotifikasiJSON(raw *string, last *time.Time) notifikasiJSON {
	var out notifikasiJSON
	if last != nil {
		d := last.Format(time.DateOnly)
		out.Terakhir = &d
	}
	if raw == nil {
		return out
	}
	t, err := parseWebhook(*raw)
	if err != nil {
		return out // stored before a rule changed: shown as off, and never used
	}
	out.Aktif, out.Layanan, out.Tujuan = true, &t.Layanan, &t.Tujuan
	return out
}

func (s *Server) getNotifikasi(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	raw, last, err := s.Store.Webhook(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, toNotifikasiJSON(raw, last))
}

func (s *Server) putNotifikasi(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req struct {
		WebhookURL string `json:"webhook_url"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := parseWebhook(req.WebhookURL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "URL webhook tidak dapat dipakai.",
			map[string]string{"webhook_url": err.Error()})
		return
	}
	raw := strings.TrimSpace(req.WebhookURL)
	if err := s.Store.SetWebhook(r.Context(), u.ID, &raw); err != nil {
		s.internal(w, r, err)
		return
	}
	s.Log.Info("webhook diatur", "pengguna", u.ID, "layanan", t.Layanan)
	_, last, err := s.Store.Webhook(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, toNotifikasiJSON(&raw, last))
}

func (s *Server) deleteNotifikasi(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	if err := s.Store.SetWebhook(r.Context(), u.ID, nil); err != nil {
		s.internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testNotifikasi sends one short message so the user sees the webhook works.
func (s *Server) testNotifikasi(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	raw, _, err := s.Store.Webhook(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	var t webhookTarget
	if raw != nil {
		t, err = parseWebhook(*raw)
	}
	if raw == nil || err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Atur URL webhook terlebih dahulu.", nil)
		return
	}
	text := "Pesan uji dari Tenggat. Ringkasan harian untuk akun " + u.Nama + " akan dikirim ke sini setiap pukul 08.00 WIB."
	if err := s.sendWebhook(r.Context(), t, text); err != nil {
		s.logWebhookError(u.ID, t, err)
		msg := "Pesan uji tidak terkirim."
		var we *webhookError
		if errors.As(err, &we) {
			msg = we.Pesan
		}
		writeError(w, http.StatusBadGateway, msg, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pesan": "Pesan uji terkirim."})
}

func (s *Server) logWebhookError(userID string, t webhookTarget, err error) {
	detail := "tidak diketahui"
	var we *webhookError
	if errors.As(err, &we) {
		detail = we.Detail
	}
	s.Log.Error("webhook gagal", "pengguna", userID, "layanan", t.Layanan, "tujuan", t.Tujuan, "galat", detail)
}

// digestText lists a user's assets and tasks that are overdue or due within
// seven days, most urgent first, or returns "" when there are none.
func (s *Server) digestText(ctx context.Context, userID string, now time.Time) (string, error) {
	today := expiry.DateOf(now)
	type line struct {
		date time.Time
		text string
	}
	var overdue, soon []line
	add := func(nama, klien string, date time.Time) {
		days := expiry.DaysLeft(date, now)
		if days > digestDays {
			return
		}
		label := nama
		if klien != "" {
			label += " (" + klien + ")"
		}
		switch {
		case days < 0:
			overdue = append(overdue, line{date, "- " + label + ": lewat " + strconv.Itoa(-days) + " hari, " + tanggalID(date)})
		case days == 0:
			soon = append(soon, line{date, "- " + label + ": hari ini"})
		case days == 1:
			soon = append(soon, line{date, "- " + label + ": besok, " + tanggalID(date)})
		default:
			soon = append(soon, line{date, "- " + label + ": " + strconv.Itoa(days) + " hari lagi, " + tanggalID(date)})
		}
	}
	items, err := s.Store.List(ctx, userID, store.Filter{}, now)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		add(it.Nama, it.Klien, it.Kedaluwarsa)
	}
	limit := today.AddDate(0, 0, digestDays)
	tasks, err := s.Store.ListTugas(ctx, userID, store.TugasFilter{Sampai: &limit})
	if err != nil {
		return "", err
	}
	for _, t := range tasks {
		add("Tugas "+t.Judul, t.Klien, t.JatuhTempo)
	}
	if len(overdue)+len(soon) == 0 {
		return "", nil
	}
	byDate := func(l []line) {
		for i := 1; i < len(l); i++ {
			for j := i; j > 0 && l[j].date.Before(l[j-1].date); j-- {
				l[j], l[j-1] = l[j-1], l[j]
			}
		}
	}
	byDate(overdue)
	byDate(soon)
	var b strings.Builder
	b.WriteString("Tenggat, ringkasan " + tanggalID(today))
	written, total := 0, len(overdue)+len(soon)
	section := func(title string, l []line) {
		if len(l) == 0 || written >= digestMaxLines {
			return
		}
		b.WriteString("\n\n" + title)
		for _, x := range l {
			if written >= digestMaxLines {
				break
			}
			b.WriteString("\n" + x.text)
			written++
		}
	}
	section("Lewat:", overdue)
	section("Jatuh tempo 7 hari ke depan:", soon)
	if total > written {
		b.WriteString("\n…dan " + strconv.Itoa(total-written) + " lainnya.")
	}
	if s.AppURL != "" {
		b.WriteString("\n\nBuka Tenggat: " + s.AppURL + "/app")
	}
	return b.String(), nil
}

// sendDigests sends today's digest to every user with a webhook who has not had
// it yet, from 08.00 in Jakarta. A day is claimed before sending, so it is never
// sent twice; a day with nothing due is claimed and nothing is sent.
func (s *Server) sendDigests(ctx context.Context, now time.Time) (sent int) {
	if now.In(expiry.Jakarta).Hour() < digestHour {
		return 0
	}
	today := expiry.DateOf(now)
	users, err := s.Store.DigestDue(ctx, today)
	if err != nil {
		s.Log.Error("ringkasan harian gagal membaca daftar", "galat", err)
		return 0
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return sent
		}
		t, err := parseWebhook(u.WebhookURL)
		if err != nil {
			continue
		}
		text, err := s.digestText(ctx, u.ID, now)
		if err != nil {
			s.Log.Error("ringkasan harian gagal disusun", "pengguna", u.ID, "galat", err)
			continue
		}
		claimed, err := s.Store.ClaimDigest(ctx, u.ID, today)
		if err != nil {
			s.Log.Error("ringkasan harian gagal ditandai", "pengguna", u.ID, "galat", err)
			continue
		}
		if !claimed || text == "" {
			continue
		}
		if err := s.sendWebhook(ctx, t, text); err != nil {
			s.logWebhookError(u.ID, t, err)
			continue
		}
		sent++
		s.Log.Info("ringkasan harian terkirim", "pengguna", u.ID, "layanan", t.Layanan)
	}
	return sent
}

// RunDigests checks every few minutes whether a daily digest is due, until ctx
// ends. Checking often instead of sleeping until 08.00 means a server started
// later in the day still sends that day's digest.
func (s *Server) RunDigests(ctx context.Context, every time.Duration) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.sendDigests(ctx, s.Now())
		timer.Reset(every)
	}
}
