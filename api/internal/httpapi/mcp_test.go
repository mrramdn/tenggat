package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (rt *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if rt.token != "" {
		clone.Header.Set("Authorization", "Bearer "+rt.token)
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func connectMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	httpClient := &http.Client{
		Transport: &bearerRoundTripper{token: token},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "tenggat-test", Version: "1.0.0"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
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

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty CallToolResult.Content: %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] = %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

func decodeStructured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal StructuredContent %s: %v", raw, err)
	}
	return v
}

func TestMCPToolsAnnotationsAndScopes(t *testing.T) {
	s, _ := newTestServer()
	s.MCP = true
	h := s.Routes()
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := signUp(t, h, "mcp-tools@contoh.id")
	tulis := mintToken(t, h, c, `{"nama":"Penuh","cakupan":"tulis"}`)
	baca := mintToken(t, h, c, `{"nama":"Baca","cakupan":"baca"}`)

	csFull := connectMCP(t, srv.URL+"/mcp", tulis.Rahasia)
	listFull, err := csFull.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools full: %v", err)
	}

	type wantAnn struct {
		readOnly    bool
		destructive bool
		idempotent  bool
		openWorld   bool
	}
	wantAll := map[string]wantAnn{
		"agenda":          {readOnly: true, destructive: false, idempotent: false, openWorld: false},
		"daftar_klien":    {readOnly: true, destructive: false, idempotent: false, openWorld: false},
		"cek_host":        {readOnly: true, destructive: false, idempotent: false, openWorld: true},
		"tambah_item":     {readOnly: false, destructive: false, idempotent: false, openWorld: true},
		"tambah_tugas":    {readOnly: false, destructive: false, idempotent: false, openWorld: false},
		"ubah_item":       {readOnly: false, destructive: false, idempotent: true, openWorld: true},
		"ubah_tugas":      {readOnly: false, destructive: false, idempotent: true, openWorld: false},
		"selesaikan":      {readOnly: false, destructive: false, idempotent: true, openWorld: false},
		"catat_pekerjaan": {readOnly: false, destructive: false, idempotent: false, openWorld: false},
		"hapus":           {readOnly: false, destructive: true, idempotent: true, openWorld: false},
	}

	if len(listFull.Tools) != len(wantAll) {
		t.Fatalf("full token listed %d tools, want %d", len(listFull.Tools), len(wantAll))
	}
	for _, tool := range listFull.Tools {
		want, ok := wantAll[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		ann := tool.Annotations
		if ann == nil || ann.DestructiveHint == nil || ann.OpenWorldHint == nil {
			t.Fatalf("tool %q has nil annotation pointer: %+v", tool.Name, ann)
		}
		if ann.ReadOnlyHint != want.readOnly ||
			*ann.DestructiveHint != want.destructive ||
			ann.IdempotentHint != want.idempotent ||
			*ann.OpenWorldHint != want.openWorld {
			t.Errorf("tool %q annotations = (readOnly=%v, destructive=%v, idempotent=%v, openWorld=%v), want %+v",
				tool.Name, ann.ReadOnlyHint, *ann.DestructiveHint, ann.IdempotentHint, *ann.OpenWorldHint, want)
		}
	}

	// A baca token sees only the three read tools.
	csRead := connectMCP(t, srv.URL+"/mcp", baca.Rahasia)
	listRead, err := csRead.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools baca: %v", err)
	}
	wantRead := map[string]bool{"agenda": true, "daftar_klien": true, "cek_host": true}
	if len(listRead.Tools) != len(wantRead) {
		t.Fatalf("baca token listed %d tools, want 3: %+v", len(listRead.Tools), listRead.Tools)
	}
	for _, tool := range listRead.Tools {
		if !wantRead[tool.Name] {
			t.Errorf("baca token listed non-read tool %q", tool.Name)
		}
	}
}

func TestMCPEndToEndWorkflowAndErrors(t *testing.T) {
	s, _, _ := newRegistryServer()
	s.MCP = true
	h := s.Routes()
	srv := httptest.NewServer(h)
	defer srv.Close()

	cA := signUp(t, h, "mcp-a@contoh.id")
	cB := signUp(t, h, "mcp-b@contoh.id")
	tokA := mintToken(t, h, cA, `{"nama":"Claude A","cakupan":"tulis"}`)
	tokB := mintToken(t, h, cB, `{"nama":"Claude B","cakupan":"tulis"}`)

	csA := connectMCP(t, srv.URL+"/mcp", tokA.Rahasia)
	csB := connectMCP(t, srv.URL+"/mcp", tokB.Rahasia)

	// 1. tambah_item for a monthly USD bill.
	addRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "tambah_item",
		Arguments: map[string]any{
			"nama":         "Claude Pro",
			"jenis":        "tagihan",
			"klien":        "Studio Rana",
			"kedaluwarsa":  "2026-11-20",
			"biaya":        20.5,
			"mata_uang":    "USD",
			"ulang_jumlah": 1,
			"ulang_satuan": "bulan",
		},
	})
	if err != nil {
		t.Fatalf("CallTool tambah_item: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("tambah_item error: %s", toolText(t, addRes))
	}
	bill := decodeStructured[itemJSON](t, addRes)
	if bill.Nama != "Claude Pro" || bill.Jenis != "tagihan" || bill.MataUang != "USD" ||
		bill.Biaya == nil || string(*bill.Biaya) != "20.5" || bill.Kedaluwarsa != "2026-11-20" {
		t.Fatalf("created bill = %+v", bill)
	}

	// 2. agenda shows it (and resolves client NAME via /api/klien).
	agRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "agenda",
		Arguments: map[string]any{"klien": "studio rana"},
	})
	if err != nil {
		t.Fatalf("CallTool agenda: %v", err)
	}
	if agRes.IsError {
		t.Fatalf("agenda error: %s", toolText(t, agRes))
	}
	ag := decodeStructured[agendaResponse](t, agRes)
	if len(ag.Agenda) != 1 || ag.Agenda[0].ID != bill.ID || ag.Agenda[0].Tanggal != "2026-11-20" ||
		ag.Agenda[0].MataUang != "USD" || ag.Agenda[0].Biaya == nil || string(*ag.Agenda[0].Biaya) != "20.5" {
		t.Fatalf("agenda = %+v", ag)
	}

	// daftar_klien shows Studio Rana with 30-day USD cost.
	klienRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{Name: "daftar_klien"})
	if err != nil || klienRes.IsError {
		t.Fatalf("daftar_klien: err=%v, res=%+v", err, klienRes)
	}
	klienList := decodeStructured[struct {
		Klien []klienJSON `json:"klien"`
	}](t, klienRes)
	if len(klienList.Klien) != 1 || string(klienList.Klien[0].Biaya30HariPer["USD"]) != "20.5" {
		t.Errorf("daftar_klien = %+v", klienList.Klien)
	}

	// 3. selesaikan moves the bill forward by one month (to 2026-12-20).
	selRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "selesaikan",
		Arguments: map[string]any{
			"asal":    "item",
			"id":      bill.ID,
			"dari":    "2026-11-20",
			"catatan": "Kartu kredit",
		},
	})
	if err != nil {
		t.Fatalf("CallTool selesaikan: %v", err)
	}
	if selRes.IsError {
		t.Fatalf("selesaikan error: %s", toolText(t, selRes))
	}
	renewed := decodeStructured[itemJSON](t, selRes)
	if renewed.Kedaluwarsa != "2026-12-20" {
		t.Errorf("renewed kedaluwarsa = %q, want 2026-12-20", renewed.Kedaluwarsa)
	}

	// Replaying selesaikan with the old dari ("2026-11-20") is a tool error (409).
	replayRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "selesaikan",
		Arguments: map[string]any{
			"asal": "item",
			"id":   bill.ID,
			"dari": "2026-11-20",
		},
	})
	if err != nil {
		t.Fatalf("CallTool selesaikan replay: %v", err)
	}
	if !replayRes.IsError {
		t.Fatal("replay with old dari: IsError = false, want true")
	}
	if got := toolText(t, replayRes); !strings.Contains(got, "Data berubah; panggil agenda lagi.") {
		t.Errorf("replay error text = %q", got)
	}

	// 4. A 422 comes back as a tool error naming the field.
	badRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "tambah_item",
		Arguments: map[string]any{
			"nama":        "Tagihan Salah",
			"jenis":       "tagihan",
			"kedaluwarsa": "2026-12-01",
			"biaya":       1.5,
			"mata_uang":   "IDR",
		},
	})
	if err != nil {
		t.Fatalf("CallTool 422 tambah_item: %v", err)
	}
	if !badRes.IsError {
		t.Fatal("422 tambah_item: IsError = false, want true")
	}
	if got := toolText(t, badRes); !strings.Contains(got, "biaya:") {
		t.Errorf("422 tool error = %q, want field name \"biaya:\"", got)
	}

	// 5. Another user's id is "not found" (404).
	otherRes, err := csB.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "ubah_item",
		Arguments: map[string]any{
			"id":   bill.ID,
			"nama": "Diambil",
		},
	})
	if err != nil {
		t.Fatalf("CallTool user B ubah_item: %v", err)
	}
	if !otherRes.IsError || !strings.Contains(toolText(t, otherRes), "tidak ditemukan") {
		t.Errorf("user B ubah_item on A's id: IsError=%v text=%q", otherRes.IsError, toolText(t, otherRes))
	}

	// 6. Exercise remaining tools: cek_host, tambah_tugas, ubah_tugas, ubah_item, catat_pekerjaan, hapus.
	hostRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "cek_host",
		Arguments: map[string]any{"host": "toko.co.id"},
	})
	if err != nil || hostRes.IsError {
		t.Fatalf("cek_host: err=%v res=%+v", err, hostRes)
	}

	taskRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "tambah_tugas",
		Arguments: map[string]any{
			"judul":       "Kirim laporan",
			"jatuh_tempo": "2026-11-25",
			"klien":       "Studio Rana",
			"sekali":      true,
		},
	})
	if err != nil || taskRes.IsError {
		t.Fatalf("tambah_tugas: err=%v text=%v", err, toolText(t, taskRes))
	}
	task := decodeStructured[tugasJSON](t, taskRes)

	updTaskRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "ubah_tugas",
		Arguments: map[string]any{
			"id":      task.ID,
			"catatan": "Kirim lewat WhatsApp",
		},
	})
	if err != nil || updTaskRes.IsError {
		t.Fatalf("ubah_tugas: err=%v text=%v", err, toolText(t, updTaskRes))
	}

	updItemRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "ubah_item",
		Arguments: map[string]any{
			"id":    bill.ID,
			"biaya": 25.0,
		},
	})
	if err != nil || updItemRes.IsError {
		t.Fatalf("ubah_item: err=%v text=%v", err, toolText(t, updItemRes))
	}

	workRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "catat_pekerjaan",
		Arguments: map[string]any{
			"jenis":        "pekerjaan",
			"teks":         "Perbarui konfigurasi nginx",
			"klien_id":     *bill.KlienID,
			"durasi_menit": 45,
		},
	})
	if err != nil || workRes.IsError {
		t.Fatalf("catat_pekerjaan: err=%v text=%v", err, toolText(t, workRes))
	}

	delTaskRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "hapus",
		Arguments: map[string]any{"asal": "tugas", "id": task.ID},
	})
	if err != nil || delTaskRes.IsError {
		t.Fatalf("hapus tugas: err=%v text=%v", err, toolText(t, delTaskRes))
	}

	delItemRes, err := csA.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "hapus",
		Arguments: map[string]any{"asal": "item", "id": bill.ID},
	})
	if err != nil || delItemRes.IsError {
		t.Fatalf("hapus item: err=%v text=%v", err, toolText(t, delItemRes))
	}
}

func TestMCPHTTPAuthMethodAndDisabledMount(t *testing.T) {
	// With MCP enabled:
	s, _ := newTestServer()
	s.MCP = true
	h := s.Routes()
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := signUp(t, h, "mcp-http@contoh.id")
	tok := mintToken(t, h, c, `{"nama":"Token","cakupan":"tulis"}`)

	// No token -> HTTP 401 with WWW-Authenticate: Bearer ... (and no resource_metadata by default).
	noAuthReq, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	noAuthReq.Header.Set("Content-Type", "application/json")
	noAuthResp, err := http.DefaultClient.Do(noAuthReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = noAuthResp.Body.Close()
	if noAuthResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /mcp without token: status %d, want 401", noAuthResp.StatusCode)
	}
	wwwAuth := noAuthResp.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(wwwAuth, "Bearer") {
		t.Errorf("POST /mcp without token: WWW-Authenticate = %q, want Bearer ...", wwwAuth)
	}
	if strings.Contains(wwwAuth, "resource_metadata") {
		t.Errorf("WWW-Authenticate advertised resource_metadata by default: %q", wwwAuth)
	}

	// GET /mcp -> 405 Method Not Allowed (also regression test for pattern conflict with GET /).
	getReq, err := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	getReq.Header.Set("Authorization", "Bearer "+tok.Rahasia)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp with MCP=true: status %d, want 405", getResp.StatusCode)
	}

	// DELETE /mcp -> 405 Method Not Allowed in stateless mode.
	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	delReq.Header.Set("Authorization", "Bearer "+tok.Rahasia)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /mcp with MCP=true: status %d, want 405", delResp.StatusCode)
	}

	// With MCP false, /mcp is 404.
	sOff, _ := newTestServer()
	sOff.MCP = false
	srvOff := httptest.NewServer(sOff.Routes())
	defer srvOff.Close()

	offResp, err := http.Get(srvOff.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	_ = offResp.Body.Close()
	if offResp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /mcp with MCP=false: status %d, want 404", offResp.StatusCode)
	}
}
