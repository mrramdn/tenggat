package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range rt.headers {
		clone.Header.Set(k, v)
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

type recordedCall struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

func connectClient(t *testing.T, srvURL string, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	httpClient := &http.Client{
		Transport: &headerRoundTripper{headers: headers},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             srvURL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty CallToolResult.Content: %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

func TestForwardedHeaders(t *testing.T) {
	var mu sync.Mutex
	var got recordedCall

	fakeAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = recordedCall{
			Method: r.Method,
			Path:   r.URL.RequestURI(),
			Header: r.Header.Clone(),
			Body:   body,
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"klien":[]}`))
	})

	h := New(Config{
		API: fakeAPI,
		Verify: func(_ context.Context, tok string, _ *http.Request) (*auth.TokenInfo, error) {
			return &auth.TokenInfo{UserID: "u1", Scopes: []string{"tulis"}}, nil
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	cs := connectClient(t, srv.URL, map[string]string{
		"Authorization":   "Bearer tgt_rahasia_uji",
		"X-Forwarded-For": "203.0.113.9, 10.0.0.7",
		"X-Custom-Secret": "jangan-diteruskan",
	})

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "daftar_klien",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool daftar_klien: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", textOf(t, res))
	}

	mu.Lock()
	defer mu.Unlock()
	if got.Method != http.MethodGet || got.Path != "/api/klien" {
		t.Errorf("call = %s %s, want GET /api/klien", got.Method, got.Path)
	}
	if gotAuth := got.Header.Get("Authorization"); gotAuth != "Bearer tgt_rahasia_uji" {
		t.Errorf("Authorization = %q, want Bearer tgt_rahasia_uji", gotAuth)
	}
	if gotXFF := got.Header.Get("X-Forwarded-For"); gotXFF != "203.0.113.9, 10.0.0.7" {
		t.Errorf("X-Forwarded-For = %q, want 203.0.113.9, 10.0.0.7", gotXFF)
	}
	if gotCustom := got.Header.Get("X-Custom-Secret"); gotCustom != "" {
		t.Errorf("X-Custom-Secret forwarded = %q, want empty", gotCustom)
	}
}

func TestPartialInputsOmitAbsentFields(t *testing.T) {
	var mu sync.Mutex
	var calls []recordedCall

	fakeAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, recordedCall{
			Method: r.Method,
			Path:   r.URL.RequestURI(),
			Body:   body,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"00000000-0000-4000-8000-000000000001","nama":"Uji","judul":"Uji","jenis":"tagihan","kedaluwarsa":"2026-11-20","jatuh_tempo":"2026-11-20","tanggal":"2026-11-09"}`))
	})

	h := New(Config{
		API: fakeAPI,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	cs := connectClient(t, srv.URL, nil)

	tests := []struct {
		tool     string
		args     map[string]any
		wantPath string
		wantKeys []string
	}{
		{
			tool:     "tambah_item",
			args:     map[string]any{"jenis": "sertifikat", "host": "studiorana.co.id"},
			wantPath: "/api/item",
			wantKeys: []string{"jenis", "host"},
		},
		{
			tool:     "tambah_tugas",
			args:     map[string]any{"judul": "Cek backup", "jatuh_tempo": "2026-12-01"},
			wantPath: "/api/tugas",
			wantKeys: []string{"judul", "jatuh_tempo"},
		},
		{
			tool:     "selesaikan",
			args:     map[string]any{"asal": "item", "id": "00000000-0000-4000-8000-000000000001", "dari": "2026-11-20"},
			wantPath: "/api/item/00000000-0000-4000-8000-000000000001/perpanjang",
			wantKeys: []string{"dari"},
		},
		{
			tool:     "selesaikan",
			args:     map[string]any{"asal": "tugas", "id": "00000000-0000-4000-8000-000000000002", "dari": "2026-11-20"},
			wantPath: "/api/tugas/00000000-0000-4000-8000-000000000002/selesai",
			wantKeys: []string{"dari"},
		},
		{
			tool:     "catat_pekerjaan",
			args:     map[string]any{"jenis": "pekerjaan", "teks": "Update plugin WordPress"},
			wantPath: "/api/aktivitas",
			wantKeys: []string{"jenis", "teks"},
		},
	}

	for _, tc := range tests {
		mu.Lock()
		calls = nil
		mu.Unlock()

		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      tc.tool,
			Arguments: tc.args,
		})
		if err != nil {
			t.Fatalf("%s: CallTool error: %v", tc.tool, err)
		}
		if res.IsError {
			t.Fatalf("%s: unexpected tool error: %s", tc.tool, textOf(t, res))
		}

		mu.Lock()
		if len(calls) != 1 {
			mu.Unlock()
			t.Fatalf("%s: got %d REST calls, want 1", tc.tool, len(calls))
		}
		call := calls[0]
		mu.Unlock()

		if call.Path != tc.wantPath {
			t.Errorf("%s: path = %q, want %q", tc.tool, call.Path, tc.wantPath)
		}
		var bodyMap map[string]any
		if err := json.Unmarshal(call.Body, &bodyMap); err != nil {
			t.Fatalf("%s: unmarshal body %q: %v", tc.tool, call.Body, err)
		}
		if len(bodyMap) != len(tc.wantKeys) {
			t.Errorf("%s: body keys = %v (raw %s), want only %v", tc.tool, bodyMap, call.Body, tc.wantKeys)
		}
		for _, k := range tc.wantKeys {
			if _, ok := bodyMap[k]; !ok {
				t.Errorf("%s: missing expected key %q in %s", tc.tool, k, call.Body)
			}
		}
	}
}

func TestUbahItemKedaluwarsaOmissionRule(t *testing.T) {
	const itemID = "00000000-0000-4000-8000-000000000010"

	cases := []struct {
		name            string
		sumber          string
		args            map[string]any
		wantKedaluwarsa string
		wantPresent     bool
	}{
		{
			name:            "manual keeps stored kedaluwarsa when omitted",
			sumber:          "manual",
			args:            map[string]any{"id": itemID, "catatan": "Catatan baru"},
			wantKedaluwarsa: "2026-11-28",
			wantPresent:     true,
		},
		{
			name:            "manual uses new kedaluwarsa when provided",
			sumber:          "manual",
			args:            map[string]any{"id": itemID, "kedaluwarsa": "2027-11-28"},
			wantKedaluwarsa: "2027-11-28",
			wantPresent:     true,
		},
		{
			name:        "server omits kedaluwarsa",
			sumber:      "server",
			args:        map[string]any{"id": itemID, "catatan": "Sertifikat utama"},
			wantPresent: false,
		},
		{
			name:        "registri omits kedaluwarsa",
			sumber:      "registri",
			args:        map[string]any{"id": itemID, "catatan": "Domain utama"},
			wantPresent: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var putBody []byte

			fakeAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					_, _ = w.Write([]byte(`{"id":"` + itemID + `","nama":"studiorana.co.id","jenis":"domain","klien":"Studio Rana","klien_id":"00000000-0000-4000-8000-000000000099","host":"studiorana.co.id","kedaluwarsa":"2026-11-28","sumber":"` + tc.sumber + `","mata_uang":"IDR","catatan":"Lama"}`))
				case http.MethodPut:
					b, _ := io.ReadAll(r.Body)
					mu.Lock()
					putBody = b
					mu.Unlock()
					_, _ = w.Write([]byte(`{"id":"` + itemID + `","nama":"studiorana.co.id","jenis":"domain","kedaluwarsa":"2026-11-28","sumber":"` + tc.sumber + `","mata_uang":"IDR","catatan":"Baru"}`))
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})

			h := New(Config{
				API: fakeAPI,
				Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			srv := httptest.NewServer(h)
			defer srv.Close()

			cs := connectClient(t, srv.URL, nil)
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "ubah_item",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool ubah_item: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %s", textOf(t, res))
			}

			mu.Lock()
			raw := putBody
			mu.Unlock()

			var bodyMap map[string]any
			if err := json.Unmarshal(raw, &bodyMap); err != nil {
				t.Fatalf("unmarshal PUT body %q: %v", raw, err)
			}
			gotVal, present := bodyMap["kedaluwarsa"]
			if present != tc.wantPresent {
				t.Fatalf("sumber=%s: kedaluwarsa present=%v (body=%s), want %v", tc.sumber, present, raw, tc.wantPresent)
			}
			if tc.wantPresent && gotVal != tc.wantKedaluwarsa {
				t.Errorf("sumber=%s: kedaluwarsa = %v, want %q", tc.sumber, gotVal, tc.wantKedaluwarsa)
			}
			if bodyMap["nama"] != "studiorana.co.id" || bodyMap["host"] != "studiorana.co.id" {
				t.Errorf("merged fields missing from PUT body: %s", raw)
			}
		})
	}
}

func TestStatusMapping(t *testing.T) {
	// Pin direct Go error return for 5xx and CallToolResult{IsError: true} for 4xx,
	// plus verify what the SDK client sees over the wire for each status.
	if _, _, err := mapResponse(http.StatusInternalServerError, []byte(`{"galat":"Database mati."}`), nil); err == nil {
		t.Fatal("mapResponse(500) returned nil error, want non-nil Go error")
	}
	if _, _, err := mapResponse(http.StatusServiceUnavailable, []byte(`{"galat":"Sibuk."}`), nil); err == nil {
		t.Fatal("mapResponse(503) returned nil error, want non-nil Go error")
	}

	var mu sync.Mutex
	nextStatus := http.StatusOK
	nextBody := `{"klien":[]}`

	fakeAPI := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		st, body := nextStatus, nextBody
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = w.Write([]byte(body))
	})

	e := &env{api: fakeAPI}
	h := New(Config{
		API: fakeAPI,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	cs := connectClient(t, srv.URL, nil)

	// 1. 2xx -> structured content + one-line summary, IsError false.
	mu.Lock()
	nextStatus = http.StatusOK
	nextBody = `{"klien":[{"id":"00000000-0000-4000-8000-000000000001","nama":"Studio Rana"}]}`
	mu.Unlock()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "daftar_klien"})
	if err != nil {
		t.Fatalf("200 CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("200 IsError = true: %s", textOf(t, res))
	}
	if summary := textOf(t, res); summary != "1 klien." || strings.Contains(summary, "\n") {
		t.Errorf("200 summary = %q, want one-line \"1 klien.\"", summary)
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok || len(sc["klien"].([]any)) != 1 {
		t.Errorf("200 StructuredContent = %#v", res.StructuredContent)
	}

	// 2. 422 -> IsError with galat plus each field: message.
	mu.Lock()
	nextStatus = http.StatusUnprocessableEntity
	nextBody = `{"galat":"Periksa kembali isian Anda.","field":{"biaya":"Biaya harus angka 0 atau lebih.","mata_uang":"Pilih IDR atau USD."}}`
	mu.Unlock()

	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "tambah_item",
		Arguments: map[string]any{"jenis": "tagihan"},
	})
	if err != nil {
		t.Fatalf("422 CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("422 IsError = false, want true")
	}
	got422 := textOf(t, res)
	for _, want := range []string{
		"Periksa kembali isian Anda.",
		"biaya: Biaya harus angka 0 atau lebih.",
		"mata_uang: Pilih IDR atau USD.",
	} {
		if !strings.Contains(got422, want) {
			t.Errorf("422 text = %q, missing %q", got422, want)
		}
	}

	// 3. 403, 404, 409, 429 -> IsError with expected message.
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusForbidden, `{"galat":"Token ini hanya untuk membaca."}`, "Token ini hanya untuk membaca."},
		{http.StatusNotFound, `{"galat":"Aset tidak ditemukan."}`, "Aset tidak ditemukan."},
		{http.StatusConflict, `{"galat":"Aset ini baru saja diubah. Muat ulang lalu coba lagi."}`, "Data berubah; panggil agenda lagi."},
		{http.StatusTooManyRequests, `{"galat":"Terlalu banyak permintaan dalam satu menit. Coba beberapa saat lagi."}`, "Terlalu banyak permintaan dalam satu menit. Coba beberapa saat lagi."},
	} {
		mu.Lock()
		nextStatus = tc.status
		nextBody = tc.body
		mu.Unlock()

		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "daftar_klien"})
		if err != nil {
			t.Fatalf("%d CallTool: %v", tc.status, err)
		}
		if !res.IsError {
			t.Errorf("%d IsError = false, want true", tc.status)
		}
		if got := textOf(t, res); got != tc.want {
			t.Errorf("%d text = %q, want %q", tc.status, got, tc.want)
		}
	}

	// 4. 5xx -> handler returns a Go error (and SDK client sees IsError=true).
	mu.Lock()
	nextStatus = http.StatusInternalServerError
	nextBody = `{"galat":"Terjadi kesalahan di server. Coba beberapa saat lagi."}`
	mu.Unlock()

	if _, _, err := e.daftarKlien(t.Context(), &mcp.CallToolRequest{}, daftarKlienInput{}); err == nil {
		t.Fatal("500 handler call returned nil error, want non-nil Go error")
	}
	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "daftar_klien"})
	if err != nil {
		t.Fatalf("500 CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("500 client CallToolResult.IsError = false, want true")
	}
}
