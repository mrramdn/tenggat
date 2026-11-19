// Package kurs fetches daily reference exchange rates from Frankfurter (ECB)
// and turns foreign-currency minor units into approximate whole rupiah for
// display only.
package kurs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/mrramdn/tenggat/api/internal/uang"
)

const (
	// frankfurterURL is fixed by design so user input can never steer outbound requests.
	frankfurterURL = "https://api.frankfurter.dev/v1/latest?base=USD&symbols=IDR,SGD"

	// NamaSumber identifies the reference rate source in API responses.
	NamaSumber = "Frankfurter (kurs referensi ECB)"

	defaultTimeout = 10 * time.Second
	maxBodyBytes   = 16 << 10 // 16 KiB; a real response is ~100 bytes
)

// Kurs holds daily reference exchange rates as rupiah per one unit of each
// supported foreign currency (USD, SGD).
type Kurs struct {
	Tanggal string             `json:"tanggal"`
	PerIDR  map[string]float64 `json:"per_idr"`
}

// Sumber reads the latest exchange rates.
type Sumber interface {
	Ambil(ctx context.Context) (Kurs, error)
}

// KeRupiah converts minor units of mataUang into whole rupiah, rounded half up.
// IDR returns minor unchanged; an unknown currency or missing rate returns false.
func (k Kurs) KeRupiah(minor int64, mataUang string) (int64, bool) {
	if !uang.Valid(mataUang) {
		return 0, false
	}
	if mataUang == "IDR" {
		return minor, true
	}
	rate, ok := k.PerIDR[mataUang]
	if !ok || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0, false
	}
	scale := math.Pow10(uang.Exponent(mataUang))
	rupiah := (float64(minor) * rate) / scale
	if math.IsNaN(rupiah) || math.IsInf(rupiah, 0) || rupiah > float64(math.MaxInt64) || rupiah < float64(math.MinInt64) {
		return 0, false
	}
	// Stored costs are non-negative, where math.Round rounds ties half up
	// without the floating-point edge case of math.Floor(x + 0.5).
	return int64(math.Round(rupiah)), true
}

// Frankfurter reads USD and SGD reference rates from api.frankfurter.dev.
type Frankfurter struct {
	// url and client are unexported so callers outside tests cannot alter the fixed endpoint.
	url     string
	client  *http.Client
	maxBody int64
}

// New returns a Frankfurter source pointed at the fixed HTTPS endpoint with a
// 10 second timeout and no redirects followed.
func New() *Frankfurter {
	return &Frankfurter{
		url:     frankfurterURL,
		client:  newHTTPClient(defaultTimeout, nil),
		maxBody: maxBodyBytes,
	}
}

func newHTTPClient(timeout time.Duration, transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type frankfurterResponse struct {
	Base  string             `json:"base"`
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

// Ambil fetches the latest USD and SGD rates in rupiah.
func (f *Frankfurter) Ambil(ctx context.Context) (Kurs, error) {
	endpoint := f.url
	if endpoint == "" {
		endpoint = frankfurterURL
	}
	client := f.client
	if client == nil {
		client = newHTTPClient(defaultTimeout, nil)
	}
	limit := f.maxBody
	if limit <= 0 {
		limit = maxBodyBytes
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Kurs{}, fmt.Errorf("kurs: buat permintaan: %w", err)
	}
	req.Header.Set("User-Agent", "Tenggat")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Kurs{}, fmt.Errorf("kurs: gagal menghubungi sumber: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, limit))
		return Kurs{}, fmt.Errorf("kurs: status HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Kurs{}, fmt.Errorf("kurs: baca respons: %w", err)
	}
	if int64(len(body)) > limit {
		return Kurs{}, errors.New("kurs: respons terlalu besar")
	}

	return parseFrankfurter(body)
}

func parseFrankfurter(body []byte) (Kurs, error) {
	var raw frankfurterResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return Kurs{}, fmt.Errorf("kurs: JSON tidak valid: %w", err)
	}
	if raw.Base != "USD" {
		return Kurs{}, fmt.Errorf("kurs: base %q bukan USD", raw.Base)
	}
	if _, err := time.Parse(time.DateOnly, raw.Date); err != nil {
		return Kurs{}, fmt.Errorf("kurs: tanggal tidak valid: %q", raw.Date)
	}
	idr, okIDR := raw.Rates["IDR"]
	sgd, okSGD := raw.Rates["SGD"]
	if !okIDR || !okSGD || !positiveFinite(idr) || !positiveFinite(sgd) {
		return Kurs{}, errors.New("kurs: nilai tukar IDR atau SGD tidak valid")
	}
	sgdPerIDR := idr / sgd
	if !positiveFinite(sgdPerIDR) {
		return Kurs{}, errors.New("kurs: hasil konversi SGD tidak valid")
	}
	return Kurs{
		Tanggal: raw.Date,
		PerIDR: map[string]float64{
			"USD": idr,
			"SGD": sgdPerIDR,
		},
	}, nil
}

func positiveFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}
