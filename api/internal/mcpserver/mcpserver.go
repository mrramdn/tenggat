// Package mcpserver exposes Tenggat's tools over the Model Context Protocol
// by calling the REST API in-process.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolTimeout  = 25 * time.Second
	maxBodyBytes = 64 << 10

	instructions = "Dates are YYYY-MM-DD in Asia/Jakarta. \"pribadi\" means personal (no client). " +
		"Money is in major units with mata_uang \"IDR\" or \"USD\". " +
		"Call agenda first to get an id and the current date to pass as dari."
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Config holds what the MCP HTTP handler needs.
type Config struct {
	API                 http.Handler
	Verify              auth.TokenVerifier
	Log                 *slog.Logger
	ResourceMetadataURL string
}

// New builds the read-only and full MCP servers once and returns an HTTP
// handler that picks between them by the bearer token's scope.
func New(cfg Config) http.Handler {
	e := &env{api: cfg.API}
	readOnly := newServer(cfg.Log)
	full := newServer(cfg.Log)
	e.registerReadTools(readOnly)
	e.registerReadTools(full)
	e.registerWriteTools(full)

	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil && slices.Contains(ti.Scopes, "baca") && !slices.Contains(ti.Scopes, "tulis") {
			return readOnly
		}
		return full
	}, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: maxBodyBytes,
		Logger:              cfg.Log,
	})

	if cfg.Verify == nil {
		return stream
	}
	authed := auth.RequireBearerToken(cfg.Verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL:    cfg.ResourceMetadataURL,
		AllowMissingExpiration: true,
	})(stream)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authed.ServeHTTP(&authHeaderWriter{ResponseWriter: w}, r)
	})
}

func newServer(log *slog.Logger) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    "tenggat",
		Version: "2.0.0",
	}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       log,
	})
}

// authHeaderWriter ensures a 401 response always carries a WWW-Authenticate:
// Bearer header even when ResourceMetadataURL is empty and the SDK middleware
// omits it.
type authHeaderWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *authHeaderWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if code == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tenggat"`)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *authHeaderWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// recorder captures an in-process HTTP response without importing net/http/httptest.
type recorder struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func newRecorder() *recorder {
	return &recorder{header: make(http.Header), status: http.StatusOK}
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.body.Write(b)
}

type env struct {
	api http.Handler
}

func (e *env) call(ctx context.Context, req *mcp.CallToolRequest, method, path string, body any) (int, []byte, error) {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req != nil && req.Extra != nil && req.Extra.Header != nil {
		if a := req.Extra.Header.Get("Authorization"); a != "" {
			httpReq.Header.Set("Authorization", a)
		}
		if xff := req.Extra.Header.Get("X-Forwarded-For"); xff != "" {
			httpReq.Header.Set("X-Forwarded-For", xff)
		}
	}
	rec := newRecorder()
	e.api.ServeHTTP(rec, httpReq)
	return rec.status, rec.body.Bytes(), nil
}

type apiError struct {
	Galat string            `json:"galat"`
	Field map[string]string `json:"field"`
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func extractErrorMessage(status int, raw []byte) string {
	var e apiError
	_ = json.Unmarshal(raw, &e)
	switch status {
	case http.StatusUnprocessableEntity:
		var parts []string
		if e.Galat != "" {
			parts = append(parts, e.Galat)
		}
		if len(e.Field) > 0 {
			keys := make([]string, 0, len(e.Field))
			for k := range e.Field {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				parts = append(parts, k+": "+e.Field[k])
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
		return "Permintaan tidak dapat diproses."
	case http.StatusConflict:
		return "Data berubah; panggil agenda lagi."
	default:
		if e.Galat != "" {
			return e.Galat
		}
		return http.StatusText(status)
	}
}

func statusErr(status int, raw []byte) error {
	var e apiError
	_ = json.Unmarshal(raw, &e)
	if e.Galat != "" {
		return fmt.Errorf("REST %d: %s", status, e.Galat)
	}
	return fmt.Errorf("REST %d", status)
}

func mapResponse(status int, raw []byte, summarize func(map[string]any) string) (*mcp.CallToolResult, any, error) {
	if status >= 200 && status < 300 {
		data := map[string]any{}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &data); err != nil {
				return nil, nil, err
			}
		}
		text := "OK"
		if summarize != nil {
			text = singleLine(summarize(data))
		}
		// Some clients show the model only the text, not the structured content,
		// so the data goes in the text as well, after the one-line summary.
		content := []mcp.Content{&mcp.TextContent{Text: text}}
		if len(data) > 0 {
			if compact, err := json.Marshal(data); err == nil {
				content = append(content, &mcp.TextContent{Text: string(compact)})
			}
		}
		return &mcp.CallToolResult{Content: content}, data, nil
	}
	if status >= 500 {
		return nil, nil, statusErr(status, raw)
	}
	return toolError(extractErrorMessage(status, raw)), nil, nil
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func strField(m map[string]any, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func toolAnnotations(readOnly, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   &openWorld,
	}
}

// Tool input types. Every optional field carries omitempty so the inferred JSON
// schema marks only fields without omitempty as required and marshalling omits
// unset fields from the REST request body.

type agendaInput struct {
	DalamHari *int   `json:"dalam_hari,omitempty" jsonschema:"Include overdue rows and rows due within this many days (0..3660)."`
	Klien     string `json:"klien,omitempty" jsonschema:"Filter by client name or client UUID."`
	Pribadi   *bool  `json:"pribadi,omitempty" jsonschema:"When true, only personal rows without a client (\"pribadi\")."`
	Status    string `json:"status,omitempty" jsonschema:"Filter by status: \"lewat\", \"kritis\", \"segera\", or \"aman\"."`
	Jenis     string `json:"jenis,omitempty" jsonschema:"Comma-separated types: \"sertifikat\", \"domain\", \"token\", \"langganan\", \"tagihan\", \"lainnya\", or \"tugas\"."`
	Q         string `json:"q,omitempty" jsonschema:"Search query matched against title, client, host, and notes."`
	Batas     *int   `json:"batas,omitempty" jsonschema:"Maximum rows to return (1..500, default 200)."`
}

type daftarKlienInput struct{}

type cekHostInput struct {
	Host string `json:"host" jsonschema:"Domain or host name to check now (for example \"mrramdn.com\")."`
}

type tambahItemInput struct {
	Nama        string   `json:"nama,omitempty" jsonschema:"Asset or bill name (optional for \"sertifikat\" and registry \"domain\", where it defaults to host)."`
	Jenis       string   `json:"jenis" jsonschema:"Item type: \"sertifikat\", \"domain\", \"token\", \"langganan\", \"tagihan\", or \"lainnya\"."`
	Klien       string   `json:"klien,omitempty" jsonschema:"Client name (new names create a client; omit for personal/\"pribadi\")."`
	KlienID     string   `json:"klien_id,omitempty" jsonschema:"Existing client UUID (overrides klien when set)."`
	Host        string   `json:"host,omitempty" jsonschema:"Domain or host name (required for \"sertifikat\", forbidden for \"tagihan\")."`
	Kedaluwarsa string   `json:"kedaluwarsa,omitempty" jsonschema:"Expiry or due date as YYYY-MM-DD (omit to read automatically for \"sertifikat\" or RDAP \"domain\")."`
	Biaya       *float64 `json:"biaya,omitempty" jsonschema:"Cost in major currency units (integer for \"IDR\", up to 2 decimals for \"USD\")."`
	MataUang    string   `json:"mata_uang,omitempty" jsonschema:"Currency code: \"IDR\" (default) or \"USD\"."`
	UlangJumlah *int     `json:"ulang_jumlah,omitempty" jsonschema:"Recurrence interval count (1..365) for manual items and bills."`
	UlangSatuan string   `json:"ulang_satuan,omitempty" jsonschema:"Recurrence unit: \"hari\", \"minggu\", \"bulan\", or \"tahun\"."`
	Catatan     string   `json:"catatan,omitempty" jsonschema:"Optional notes (up to 500 characters)."`
}

type tambahTugasInput struct {
	Judul       string   `json:"judul" jsonschema:"Task title (2..120 characters)."`
	JatuhTempo  string   `json:"jatuh_tempo" jsonschema:"Due date as YYYY-MM-DD."`
	Klien       string   `json:"klien,omitempty" jsonschema:"Client name (new names create a client; omit for personal/\"pribadi\")."`
	KlienID     string   `json:"klien_id,omitempty" jsonschema:"Existing client UUID (overrides klien when set)."`
	Sekali      bool     `json:"sekali,omitempty" jsonschema:"Set true for a one-off deadline (\"sekali\"); false (default) for a recurring task."`
	UlangJumlah *int     `json:"ulang_jumlah,omitempty" jsonschema:"Recurrence interval count (1..365, default 1 when recurring)."`
	UlangSatuan string   `json:"ulang_satuan,omitempty" jsonschema:"Recurrence unit: \"hari\", \"minggu\", \"bulan\" (default), or \"tahun\"."`
	Biaya       *float64 `json:"biaya,omitempty" jsonschema:"Optional fee in major currency units."`
	MataUang    string   `json:"mata_uang,omitempty" jsonschema:"Currency code: \"IDR\" (default) or \"USD\"."`
	Catatan     string   `json:"catatan,omitempty" jsonschema:"Optional notes (up to 500 characters)."`
}

type ubahItemInput struct {
	ID          string   `json:"id" jsonschema:"Item UUID to update (from agenda)."`
	Nama        *string  `json:"nama,omitempty" jsonschema:"New item name."`
	Jenis       *string  `json:"jenis,omitempty" jsonschema:"New item type: \"sertifikat\", \"domain\", \"token\", \"langganan\", \"tagihan\", or \"lainnya\"."`
	Klien       *string  `json:"klien,omitempty" jsonschema:"Client name (set to \"\" to make personal/\"pribadi\")."`
	KlienID     *string  `json:"klien_id,omitempty" jsonschema:"Client UUID."`
	Host        *string  `json:"host,omitempty" jsonschema:"Domain or host name."`
	Kedaluwarsa *string  `json:"kedaluwarsa,omitempty" jsonschema:"New expiry or due date as YYYY-MM-DD (for manual items and bills)."`
	Biaya       *float64 `json:"biaya,omitempty" jsonschema:"New cost in major currency units."`
	MataUang    *string  `json:"mata_uang,omitempty" jsonschema:"Currency code: \"IDR\" or \"USD\"."`
	UlangJumlah *int     `json:"ulang_jumlah,omitempty" jsonschema:"Recurrence interval count (1..365)."`
	UlangSatuan *string  `json:"ulang_satuan,omitempty" jsonschema:"Recurrence unit: \"hari\", \"minggu\", \"bulan\", \"tahun\", or \"\" to stop recurrence."`
	Catatan     *string  `json:"catatan,omitempty" jsonschema:"Notes (set to \"\" to clear)."`
}

type ubahTugasInput struct {
	ID          string   `json:"id" jsonschema:"Task UUID to update (from agenda)."`
	Judul       *string  `json:"judul,omitempty" jsonschema:"New task title (2..120 characters)."`
	JatuhTempo  *string  `json:"jatuh_tempo,omitempty" jsonschema:"New due date as YYYY-MM-DD."`
	Klien       *string  `json:"klien,omitempty" jsonschema:"Client name (set to \"\" to make personal/\"pribadi\")."`
	KlienID     *string  `json:"klien_id,omitempty" jsonschema:"Client UUID."`
	Sekali      *bool    `json:"sekali,omitempty" jsonschema:"Set true for a one-off deadline (\"sekali\"), false for a recurring task."`
	UlangJumlah *int     `json:"ulang_jumlah,omitempty" jsonschema:"Recurrence interval count (1..365)."`
	UlangSatuan *string  `json:"ulang_satuan,omitempty" jsonschema:"Recurrence unit: \"hari\", \"minggu\", \"bulan\", or \"tahun\"."`
	Biaya       *float64 `json:"biaya,omitempty" jsonschema:"Fee in major currency units."`
	MataUang    *string  `json:"mata_uang,omitempty" jsonschema:"Currency code: \"IDR\" or \"USD\"."`
	Catatan     *string  `json:"catatan,omitempty" jsonschema:"Notes (set to \"\" to clear)."`
}

type selesaikanInput struct {
	Asal        string   `json:"asal" jsonschema:"Row origin from agenda: \"item\" or \"tugas\"."`
	ID          string   `json:"id" jsonschema:"Item or task UUID from agenda."`
	Dari        string   `json:"dari" jsonschema:"Current tanggal of the row as YYYY-MM-DD (guards against concurrent changes)."`
	Sampai      string   `json:"sampai,omitempty" jsonschema:"New expiry date as YYYY-MM-DD (required for non-recurring manual non-bill items)."`
	SampaiKini  bool     `json:"sampai_kini,omitempty" jsonschema:"For a recurring \"item\" that is more than one period overdue: true means the user is paid up, so the date moves to the first period after today instead of one period on. Ask or infer from what the user said; default false."`
	Jumlah      *float64 `json:"jumlah,omitempty" jsonschema:"Amount paid in major currency units (for \"item\")."`
	MataUang    string   `json:"mata_uang,omitempty" jsonschema:"Currency code for jumlah: \"IDR\" or \"USD\"."`
	DurasiMenit *int     `json:"durasi_menit,omitempty" jsonschema:"Work duration in minutes (1..1440, for \"tugas\")."`
	Tanggal     string   `json:"tanggal,omitempty" jsonschema:"Date the payment or work happened as YYYY-MM-DD (defaults to today)."`
	Catatan     string   `json:"catatan,omitempty" jsonschema:"Optional note about the payment, renewal, or completed task."`
}

type catatPekerjaanInput struct {
	Jenis       string `json:"jenis" jsonschema:"Entry type: \"pekerjaan\" (logged work, included in client reports) or \"catatan\" (internal note)."`
	Teks        string `json:"teks" jsonschema:"Description of the work or note (1..2000 characters)."`
	KlienID     string `json:"klien_id,omitempty" jsonschema:"Client UUID to attach the entry to."`
	Tanggal     string `json:"tanggal,omitempty" jsonschema:"Work date as YYYY-MM-DD (defaults to today, never after today)."`
	DurasiMenit *int   `json:"durasi_menit,omitempty" jsonschema:"Optional work duration in minutes (1..1440)."`
}

type hapusInput struct {
	Asal string `json:"asal" jsonschema:"Row origin from agenda: \"item\" or \"tugas\"."`
	ID   string `json:"id" jsonschema:"Item or task UUID to delete permanently."`
}

func (e *env) registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "agenda",
		Description: "List upcoming and overdue items (\"item\") and tasks (\"tugas\") merged in date order, " +
			"with counts per status (\"lewat\", \"kritis\", \"segera\", \"aman\"). Also filters and searches by client, type, status, or text.",
		Annotations: toolAnnotations(true, false, false, false),
	}, e.agenda)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "daftar_klien",
		Description: "List clients with asset counts, nearest due date, and 30-day costs per currency (\"IDR\", \"USD\").",
		Annotations: toolAnnotations(true, false, false, false),
	}, e.daftarKlien)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "cek_host",
		Description: "Look up a domain's registry expiry date (RDAP) and a host's TLS certificate expiry date right now without storing anything.",
		Annotations: toolAnnotations(true, false, false, true),
	}, e.cekHost)
}

func (e *env) registerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "tambah_item",
		Description: "Add an asset or bill of jenis \"sertifikat\", \"domain\", \"token\", \"langganan\", \"tagihan\", or \"lainnya\". " +
			"Money is in major units with mata_uang \"IDR\" or \"USD\"; recurrence uses ulang_satuan \"hari\", \"minggu\", \"bulan\", or \"tahun\".",
		Annotations: toolAnnotations(false, false, false, true),
	}, e.tambahItem)

	mcp.AddTool(s, &mcp.Tool{
		Name: "tambah_tugas",
		Description: "Add a recurring maintenance task (ulang_satuan \"hari\", \"minggu\", \"bulan\", or \"tahun\") " +
			"or a one-off deadline (sekali: true).",
		Annotations: toolAnnotations(false, false, false, false),
	}, e.tambahTugas)

	mcp.AddTool(s, &mcp.Tool{
		Name: "ubah_item",
		Description: "Update fields on an existing item by id, keeping any omitted fields from the stored item. " +
			"When the stored sumber is \"server\" or \"registri\", kedaluwarsa is read by the server.",
		Annotations: toolAnnotations(false, false, true, true),
	}, e.ubahItem)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ubah_tugas",
		Description: "Update fields on an existing task by id, keeping any omitted fields from the stored task.",
		Annotations: toolAnnotations(false, false, true, false),
	}, e.ubahTugas)

	mcp.AddTool(s, &mcp.Tool{
		Name: "selesaikan",
		Description: "Mark a task done (asal \"tugas\"), or mark a bill paid or manual asset renewed (asal \"item\"). " +
			"Requires asal (\"item\" or \"tugas\"), id, and dari (the row's current tanggal from agenda).",
		Annotations: toolAnnotations(false, false, true, false),
	}, e.selesaikan)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catat_pekerjaan",
		Description: "Log work (jenis \"pekerjaan\") or an internal note (jenis \"catatan\") in the activity feed, optionally for a client.",
		Annotations: toolAnnotations(false, false, false, false),
	}, e.catatPekerjaan)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "hapus",
		Description: "Permanently delete an item (asal \"item\") or a task (asal \"tugas\") by id.",
		Annotations: toolAnnotations(false, true, true, false),
	}, e.hapus)
}

func (e *env) agenda(ctx context.Context, req *mcp.CallToolRequest, in agendaInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	klienID := strings.TrimSpace(in.Klien)
	if klienID != "" && !uuidPattern.MatchString(klienID) {
		status, raw, err := e.call(ctx, req, http.MethodGet, "/api/klien", nil)
		if err != nil {
			return nil, nil, err
		}
		if status < 200 || status >= 300 {
			return mapResponse(status, raw, nil)
		}
		var list struct {
			Klien []struct {
				ID   string `json:"id"`
				Nama string `json:"nama"`
			} `json:"klien"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, nil, err
		}
		for _, k := range list.Klien {
			if strings.EqualFold(k.Nama, klienID) {
				klienID = k.ID
				break
			}
		}
	}

	q := url.Values{}
	if in.DalamHari != nil {
		q.Set("dalam_hari", strconv.Itoa(*in.DalamHari))
	}
	if klienID != "" {
		q.Set("klien_id", klienID)
	}
	if in.Pribadi != nil {
		if *in.Pribadi {
			q.Set("pribadi", "1")
		} else {
			q.Set("pribadi", "0")
		}
	}
	if in.Status != "" {
		q.Set("status", in.Status)
	}
	if in.Jenis != "" {
		q.Set("jenis", in.Jenis)
	}
	if in.Q != "" {
		q.Set("q", in.Q)
	}
	if in.Batas != nil {
		q.Set("batas", strconv.Itoa(*in.Batas))
	}
	path := "/api/agenda"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	status, raw, err := e.call(ctx, req, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(status, raw, func(data map[string]any) string {
		jumlah, _ := data["jumlah"].(map[string]any)
		return fmt.Sprintf("%d agenda (lewat %d, kritis %d, segera %d, aman %d).",
			intField(jumlah, "total"), intField(jumlah, "lewat"), intField(jumlah, "kritis"),
			intField(jumlah, "segera"), intField(jumlah, "aman"))
	})
}

func (e *env) daftarKlien(ctx context.Context, req *mcp.CallToolRequest, _ daftarKlienInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	status, raw, err := e.call(ctx, req, http.MethodGet, "/api/klien", nil)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(status, raw, func(data map[string]any) string {
		list, _ := data["klien"].([]any)
		return fmt.Sprintf("%d klien.", len(list))
	})
}

func (e *env) cekHost(ctx context.Context, req *mcp.CallToolRequest, in cekHostInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	q := url.QueryEscape(in.Host)
	domStatus, domRaw, err := e.call(ctx, req, http.MethodGet, "/api/domain?host="+q, nil)
	if err != nil {
		return nil, nil, err
	}
	if domStatus >= 500 {
		return nil, nil, statusErr(domStatus, domRaw)
	}
	if domStatus == http.StatusUnauthorized || domStatus == http.StatusForbidden || domStatus == http.StatusTooManyRequests {
		return mapResponse(domStatus, domRaw, nil)
	}

	certStatus, certRaw, err := e.call(ctx, req, http.MethodGet, "/api/sertifikat?host="+q, nil)
	if err != nil {
		return nil, nil, err
	}
	if certStatus >= 500 {
		return nil, nil, statusErr(certStatus, certRaw)
	}
	if certStatus == http.StatusUnauthorized || certStatus == http.StatusForbidden || certStatus == http.StatusTooManyRequests {
		return mapResponse(certStatus, certRaw, nil)
	}

	out := map[string]any{"host": in.Host}
	var domText, certText string
	if domStatus >= 200 && domStatus < 300 {
		var dom map[string]any
		if err := json.Unmarshal(domRaw, &dom); err != nil {
			return nil, nil, err
		}
		out["domain"] = dom
		domText = "domain berakhir " + strField(dom, "kedaluwarsa")
	} else {
		msg := extractErrorMessage(domStatus, domRaw)
		out["domain"] = nil
		out["domain_galat"] = msg
		domText = "domain: " + singleLine(msg)
	}
	if certStatus >= 200 && certStatus < 300 {
		var cert map[string]any
		if err := json.Unmarshal(certRaw, &cert); err != nil {
			return nil, nil, err
		}
		out["sertifikat"] = cert
		certText = "sertifikat berakhir " + strField(cert, "kedaluwarsa")
	} else {
		msg := extractErrorMessage(certStatus, certRaw)
		out["sertifikat"] = nil
		out["sertifikat_galat"] = msg
		certText = "sertifikat: " + singleLine(msg)
	}

	summary := singleLine(fmt.Sprintf("Host %s: %s; %s.", in.Host, domText, certText))
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: summary}},
	}, out, nil
}

func (e *env) tambahItem(ctx context.Context, req *mcp.CallToolRequest, in tambahItemInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	status, raw, err := e.call(ctx, req, http.MethodPost, "/api/item", in)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(status, raw, func(data map[string]any) string {
		return fmt.Sprintf("Item ditambahkan: %s (%s), berakhir %s.",
			strField(data, "nama"), strField(data, "id"), strField(data, "kedaluwarsa"))
	})
}

func (e *env) tambahTugas(ctx context.Context, req *mcp.CallToolRequest, in tambahTugasInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	status, raw, err := e.call(ctx, req, http.MethodPost, "/api/tugas", in)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(status, raw, func(data map[string]any) string {
		return fmt.Sprintf("Tugas ditambahkan: %s (%s), jatuh tempo %s.",
			strField(data, "judul"), strField(data, "id"), strField(data, "jatuh_tempo"))
	})
}

type storedItem struct {
	Nama        string       `json:"nama"`
	Jenis       string       `json:"jenis"`
	Klien       string       `json:"klien"`
	KlienID     *string      `json:"klien_id"`
	Host        *string      `json:"host"`
	Kedaluwarsa string       `json:"kedaluwarsa"`
	Sumber      string       `json:"sumber"`
	Biaya       *json.Number `json:"biaya"`
	MataUang    string       `json:"mata_uang"`
	UlangJumlah *int         `json:"ulang_jumlah"`
	UlangSatuan *string      `json:"ulang_satuan"`
	Catatan     string       `json:"catatan"`
}

type itemPutBody struct {
	Nama        string  `json:"nama,omitempty"`
	Jenis       string  `json:"jenis,omitempty"`
	Klien       string  `json:"klien,omitempty"`
	KlienID     *string `json:"klien_id,omitempty"`
	Host        string  `json:"host,omitempty"`
	Kedaluwarsa string  `json:"kedaluwarsa,omitempty"`
	Biaya       any     `json:"biaya,omitempty"`
	MataUang    string  `json:"mata_uang,omitempty"`
	UlangJumlah *int    `json:"ulang_jumlah,omitempty"`
	UlangSatuan *string `json:"ulang_satuan,omitempty"`
	Catatan     string  `json:"catatan,omitempty"`
}

func (e *env) ubahItem(ctx context.Context, req *mcp.CallToolRequest, in ubahItemInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	path := "/api/item/" + url.PathEscape(in.ID)
	getStatus, getRaw, err := e.call(ctx, req, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	if getStatus < 200 || getStatus >= 300 {
		return mapResponse(getStatus, getRaw, nil)
	}
	var stored storedItem
	if err := json.Unmarshal(getRaw, &stored); err != nil {
		return nil, nil, err
	}

	body := itemPutBody{
		Nama:        stored.Nama,
		Jenis:       stored.Jenis,
		Klien:       stored.Klien,
		KlienID:     stored.KlienID,
		MataUang:    stored.MataUang,
		UlangJumlah: stored.UlangJumlah,
		UlangSatuan: stored.UlangSatuan,
		Catatan:     stored.Catatan,
	}
	if stored.Host != nil {
		body.Host = *stored.Host
	}
	if stored.Biaya != nil {
		body.Biaya = *stored.Biaya
	}

	if in.Nama != nil {
		body.Nama = *in.Nama
	}
	if in.Jenis != nil {
		body.Jenis = *in.Jenis
	}
	if in.KlienID != nil {
		if *in.KlienID == "" {
			body.KlienID = nil
			body.Klien = ""
		} else {
			body.KlienID = in.KlienID
			body.Klien = ""
		}
	} else if in.Klien != nil {
		body.KlienID = nil
		body.Klien = *in.Klien
	}
	if in.Host != nil {
		body.Host = *in.Host
	}
	if stored.Sumber != "server" && stored.Sumber != "registri" {
		body.Kedaluwarsa = stored.Kedaluwarsa
		if in.Kedaluwarsa != nil {
			body.Kedaluwarsa = *in.Kedaluwarsa
		}
	}
	if in.Biaya != nil {
		body.Biaya = *in.Biaya
	}
	if in.MataUang != nil {
		body.MataUang = *in.MataUang
	}
	if in.UlangJumlah != nil {
		body.UlangJumlah = in.UlangJumlah
	}
	if in.UlangSatuan != nil {
		body.UlangSatuan = in.UlangSatuan
		if *in.UlangSatuan == "" && in.UlangJumlah == nil {
			body.UlangJumlah = nil
		}
	}
	if in.Catatan != nil {
		body.Catatan = *in.Catatan
	}

	putStatus, putRaw, err := e.call(ctx, req, http.MethodPut, path, body)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(putStatus, putRaw, func(data map[string]any) string {
		return fmt.Sprintf("Item diubah: %s (%s), berakhir %s.",
			strField(data, "nama"), strField(data, "id"), strField(data, "kedaluwarsa"))
	})
}

type storedTugas struct {
	Judul       string       `json:"judul"`
	Catatan     string       `json:"catatan"`
	Klien       string       `json:"klien"`
	KlienID     *string      `json:"klien_id"`
	Sekali      bool         `json:"sekali"`
	UlangJumlah *int         `json:"ulang_jumlah"`
	UlangSatuan *string      `json:"ulang_satuan"`
	JatuhTempo  string       `json:"jatuh_tempo"`
	Biaya       *json.Number `json:"biaya"`
	MataUang    string       `json:"mata_uang"`
}

type tugasPutBody struct {
	Judul       string  `json:"judul,omitempty"`
	Catatan     string  `json:"catatan,omitempty"`
	Klien       string  `json:"klien,omitempty"`
	KlienID     *string `json:"klien_id,omitempty"`
	Sekali      bool    `json:"sekali,omitempty"`
	UlangJumlah *int    `json:"ulang_jumlah,omitempty"`
	UlangSatuan string  `json:"ulang_satuan,omitempty"`
	JatuhTempo  string  `json:"jatuh_tempo,omitempty"`
	Biaya       any     `json:"biaya,omitempty"`
	MataUang    string  `json:"mata_uang,omitempty"`
}

func (e *env) ubahTugas(ctx context.Context, req *mcp.CallToolRequest, in ubahTugasInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	path := "/api/tugas/" + url.PathEscape(in.ID)
	getStatus, getRaw, err := e.call(ctx, req, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	if getStatus < 200 || getStatus >= 300 {
		return mapResponse(getStatus, getRaw, nil)
	}
	var stored storedTugas
	if err := json.Unmarshal(getRaw, &stored); err != nil {
		return nil, nil, err
	}

	body := tugasPutBody{
		Judul:      stored.Judul,
		Catatan:    stored.Catatan,
		Klien:      stored.Klien,
		KlienID:    stored.KlienID,
		Sekali:     stored.Sekali,
		JatuhTempo: stored.JatuhTempo,
		MataUang:   stored.MataUang,
	}
	if stored.Biaya != nil {
		body.Biaya = *stored.Biaya
	}
	if in.Judul != nil {
		body.Judul = *in.Judul
	}
	if in.JatuhTempo != nil {
		body.JatuhTempo = *in.JatuhTempo
	}
	if in.Catatan != nil {
		body.Catatan = *in.Catatan
	}
	if in.KlienID != nil {
		if *in.KlienID == "" {
			body.KlienID = nil
			body.Klien = ""
		} else {
			body.KlienID = in.KlienID
			body.Klien = ""
		}
	} else if in.Klien != nil {
		body.KlienID = nil
		body.Klien = *in.Klien
	}
	if in.Sekali != nil {
		body.Sekali = *in.Sekali
	} else if in.UlangJumlah != nil || in.UlangSatuan != nil {
		body.Sekali = false
	}
	if !body.Sekali {
		body.UlangJumlah = stored.UlangJumlah
		if stored.UlangSatuan != nil {
			body.UlangSatuan = *stored.UlangSatuan
		}
	}
	if in.UlangJumlah != nil {
		body.UlangJumlah = in.UlangJumlah
	}
	if in.UlangSatuan != nil {
		body.UlangSatuan = *in.UlangSatuan
	}
	if in.Biaya != nil {
		body.Biaya = *in.Biaya
	}
	if in.MataUang != nil {
		body.MataUang = *in.MataUang
	}

	putStatus, putRaw, err := e.call(ctx, req, http.MethodPut, path, body)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(putStatus, putRaw, func(data map[string]any) string {
		return fmt.Sprintf("Tugas diubah: %s (%s), jatuh tempo %s.",
			strField(data, "judul"), strField(data, "id"), strField(data, "jatuh_tempo"))
	})
}

type perpanjangBody struct {
	Dari       string   `json:"dari"`
	Sampai     string   `json:"sampai,omitempty"`
	Jumlah     *float64 `json:"jumlah,omitempty"`
	MataUang   string   `json:"mata_uang,omitempty"`
	Catatan    string   `json:"catatan,omitempty"`
	Tanggal    string   `json:"tanggal,omitempty"`
	SampaiKini bool     `json:"sampai_kini,omitempty"`
}

type selesaiTugasBody struct {
	Dari        string `json:"dari"`
	Catatan     string `json:"catatan,omitempty"`
	DurasiMenit *int   `json:"durasi_menit,omitempty"`
	Tanggal     string `json:"tanggal,omitempty"`
}

func (e *env) selesaikan(ctx context.Context, req *mcp.CallToolRequest, in selesaikanInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	switch in.Asal {
	case "item":
		body := perpanjangBody{
			Dari:       in.Dari,
			Sampai:     in.Sampai,
			Jumlah:     in.Jumlah,
			MataUang:   in.MataUang,
			Catatan:    in.Catatan,
			Tanggal:    in.Tanggal,
			SampaiKini: in.SampaiKini,
		}
		status, raw, err := e.call(ctx, req, http.MethodPost, "/api/item/"+url.PathEscape(in.ID)+"/perpanjang", body)
		if err != nil {
			return nil, nil, err
		}
		return mapResponse(status, raw, func(data map[string]any) string {
			if data["selesai_pada"] != nil {
				return fmt.Sprintf("Item selesai: %s (%s), lunas.", strField(data, "nama"), strField(data, "id"))
			}
			return fmt.Sprintf("Item diperpanjang: %s (%s), berakhir %s.",
				strField(data, "nama"), strField(data, "id"), strField(data, "kedaluwarsa"))
		})
	case "tugas":
		body := selesaiTugasBody{
			Dari:        in.Dari,
			Catatan:     in.Catatan,
			DurasiMenit: in.DurasiMenit,
			Tanggal:     in.Tanggal,
		}
		status, raw, err := e.call(ctx, req, http.MethodPost, "/api/tugas/"+url.PathEscape(in.ID)+"/selesai", body)
		if err != nil {
			return nil, nil, err
		}
		return mapResponse(status, raw, func(data map[string]any) string {
			if data["selesai_pada"] != nil {
				return fmt.Sprintf("Tugas selesai: %s (%s).", strField(data, "judul"), strField(data, "id"))
			}
			return fmt.Sprintf("Tugas selesai: %s (%s), jatuh tempo berikutnya %s.",
				strField(data, "judul"), strField(data, "id"), strField(data, "jatuh_tempo"))
		})
	default:
		return toolError("asal: Pilih item atau tugas."), nil, nil
	}
}

func (e *env) catatPekerjaan(ctx context.Context, req *mcp.CallToolRequest, in catatPekerjaanInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	status, raw, err := e.call(ctx, req, http.MethodPost, "/api/aktivitas", in)
	if err != nil {
		return nil, nil, err
	}
	return mapResponse(status, raw, func(data map[string]any) string {
		return fmt.Sprintf("Aktivitas dicatat: %s (%s) tanggal %s.",
			strField(data, "jenis"), strField(data, "id"), strField(data, "tanggal"))
	})
}

func (e *env) hapus(ctx context.Context, req *mcp.CallToolRequest, in hapusInput) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	var path string
	switch in.Asal {
	case "item":
		path = "/api/item/" + url.PathEscape(in.ID)
	case "tugas":
		path = "/api/tugas/" + url.PathEscape(in.ID)
	default:
		return toolError("asal: Pilih item atau tugas."), nil, nil
	}

	status, raw, err := e.call(ctx, req, http.MethodDelete, path, nil)
	if err != nil {
		return nil, nil, err
	}
	if status >= 200 && status < 300 {
		out := map[string]any{"dihapus": true, "asal": in.Asal, "id": in.ID}
		text := singleLine(fmt.Sprintf("Dihapus: %s %s.", in.Asal, in.ID))
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, out, nil
	}
	return mapResponse(status, raw, nil)
}
