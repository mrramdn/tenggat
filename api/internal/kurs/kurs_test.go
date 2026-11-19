package kurs

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSource(srv *httptest.Server, timeout time.Duration) *Frankfurter {
	return &Frankfurter{
		url:     srv.URL,
		client:  newHTTPClient(timeout, srv.Client().Transport),
		maxBody: maxBodyBytes,
	}
}

func TestFrankfurterAmbilOK(t *testing.T) {
	var gotUA string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":1.2896}}`))
	}))
	defer srv.Close()

	k, err := testSource(srv, 5*time.Second).Ambil(t.Context())
	if err != nil {
		t.Fatalf("Ambil: %v", err)
	}
	if gotUA != "Tenggat" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "Tenggat")
	}
	if k.Tanggal != "2026-11-18" {
		t.Errorf("Tanggal = %q, want 2026-11-18", k.Tanggal)
	}
	if k.PerIDR["USD"] != 18254 {
		t.Errorf("PerIDR[USD] = %v, want 18254", k.PerIDR["USD"])
	}
	wantSGD := 18254.0 / 1.2896
	if math.Abs(k.PerIDR["SGD"]-wantSGD) > 1e-9 {
		t.Errorf("PerIDR[SGD] = %v, want %v", k.PerIDR["SGD"], wantSGD)
	}
}

func TestFrankfurterRejectsRedirects(t *testing.T) {
	followed := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed = true
		_, _ = w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":1.2896}}`))
	}))
	defer target.Close()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	if _, err := testSource(srv, 5*time.Second).Ambil(t.Context()); err == nil {
		t.Fatal("Ambil on 302 redirect succeeded, want error")
	}
	if followed {
		t.Error("redirect target was contacted")
	}
}

func TestFrankfurterRejectsBadResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusBadGateway, `{"error":"upstream"}`},
		{"not found", http.StatusNotFound, `not found`},
		{"bad json", http.StatusOK, `{not json`},
		{"wrong base", http.StatusOK, `{"amount":1.0,"base":"EUR","date":"2026-11-18","rates":{"IDR":18254,"SGD":1.2896}}`},
		{"missing date", http.StatusOK, `{"amount":1.0,"base":"USD","date":"","rates":{"IDR":18254,"SGD":1.2896}}`},
		{"bad date", http.StatusOK, `{"amount":1.0,"base":"USD","date":"18-11-2026","rates":{"IDR":18254,"SGD":1.2896}}`},
		{"missing IDR", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"SGD":1.2896}}`},
		{"missing SGD", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254}}`},
		{"zero IDR", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":0,"SGD":1.2896}}`},
		{"negative IDR", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":-100,"SGD":1.2896}}`},
		{"zero SGD", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":0}}`},
		{"negative SGD", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":-1.2}}`},
		{"oversized body", http.StatusOK, `{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":1.2896},"pad":"` + strings.Repeat("x", maxBodyBytes+10) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if got, err := testSource(srv, 5*time.Second).Ambil(t.Context()); err == nil {
				t.Errorf("Ambil = %+v, nil; want error", got)
			}
		})
	}
}

func TestFrankfurterTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-11-18","rates":{"IDR":18254,"SGD":1.2896}}`))
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if _, err := testSource(srv, 50*time.Millisecond).Ambil(ctx); err == nil {
		t.Fatal("Ambil on slow server succeeded, want timeout error")
	}
}

func TestNewUsesFixedHTTPSURL(t *testing.T) {
	f := New()
	if f.url != frankfurterURL || !strings.HasPrefix(f.url, "https://") {
		t.Errorf("url = %q, want fixed HTTPS URL %q", f.url, frankfurterURL)
	}
	if f.client == nil || f.client.Timeout != 10*time.Second {
		t.Errorf("client timeout = %v, want 10s", f.client)
	}
}

func TestKeRupiah(t *testing.T) {
	k := Kurs{
		Tanggal: "2026-11-18",
		PerIDR: map[string]float64{
			"USD": 18254,
			"SGD": 14155.2,
		},
	}

	cases := []struct {
		name  string
		kurs  Kurs
		minor int64
		mu    string
		want  int64
		ok    bool
	}{
		{"IDR exact with rates", k, 350000, "IDR", 350000, true},
		{"IDR exact without rates", Kurs{}, 350000, "IDR", 350000, true},
		{"IDR zero", Kurs{}, 0, "IDR", 0, true},
		{"USD whole", k, 2050, "USD", 374207, true},
		{"USD zero", k, 0, "USD", 0, true},
		{"USD round down", k, 1, "USD", 183, true},                                                    // 182.54 -> 183
		{"USD half up from even", Kurs{PerIDR: map[string]float64{"USD": 1050}}, 5, "USD", 53, true},  // 52.5 -> 53 (not 52)
		{"USD half up from odd", Kurs{PerIDR: map[string]float64{"USD": 1070}}, 5, "USD", 54, true},   // 53.5 -> 54
		{"USD half up 0.5", Kurs{PerIDR: map[string]float64{"USD": 50}}, 1, "USD", 1, true},           // 0.5 -> 1
		{"USD below half", Kurs{PerIDR: map[string]float64{"USD": 49}}, 1, "USD", 0, true},            // 0.49 -> 0
		{"SGD fractional rate", k, 2050, "SGD", 290182, true},                                         // 20.50 * 14155.2 = 290181.6 -> 290182
		{"SGD missing rate", Kurs{PerIDR: map[string]float64{"USD": 18254}}, 2050, "SGD", 0, false},   // missing SGD
		{"USD missing rate", Kurs{}, 2050, "USD", 0, false},                                           // empty Kurs
		{"USD zero rate", Kurs{PerIDR: map[string]float64{"USD": 0}}, 2050, "USD", 0, false},          // non-positive
		{"USD negative rate", Kurs{PerIDR: map[string]float64{"USD": -100}}, 2050, "USD", 0, false},   // negative
		{"USD NaN rate", Kurs{PerIDR: map[string]float64{"USD": math.NaN()}}, 2050, "USD", 0, false},  // NaN
		{"USD Inf rate", Kurs{PerIDR: map[string]float64{"USD": math.Inf(1)}}, 2050, "USD", 0, false}, // +Inf
		{"unknown currency", k, 1000, "EUR", 0, false},                                                // unknown
		{"empty currency", k, 1000, "", 0, false},                                                     // empty
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.kurs.KeRupiah(tc.minor, tc.mu)
			if ok != tc.ok || got != tc.want {
				t.Errorf("KeRupiah(%d, %q) = %d, %v; want %d, %v", tc.minor, tc.mu, got, ok, tc.want, tc.ok)
			}
		})
	}
}
