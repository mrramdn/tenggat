package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestAktivitasManualEntries(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "log@contoh.id")
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Studio Rana"}`, c))

	rec := do(t, h, "POST", "/api/aktivitas", `{"klien_id":"`+k.ID+`","jenis":"pekerjaan","teks":" Ganti tema WordPress ","tanggal":"2026-11-02","durasi_menit":90}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	a := decode[aktivitasJSON](t, rec)
	if a.Teks != "Ganti tema WordPress" || a.Klien != "Studio Rana" || deref(a.KlienID) != k.ID || a.Tanggal != "2026-11-02" ||
		a.DurasiMenit == nil || *a.DurasiMenit != 90 || string(a.Data) != "null" {
		t.Errorf("created %+v", a)
	}
	note := decode[aktivitasJSON](t, do(t, h, "POST", "/api/aktivitas", `{"klien_id":"`+k.ID+`","jenis":"catatan","teks":"Bu Rani minta laporan tiap tanggal 1"}`, c))
	if note.Tanggal != "2026-11-09" {
		t.Errorf("default date %s, want today in Jakarta", note.Tanggal)
	}

	for body, field := range map[string]string{
		`{"jenis":"aset_baru","teks":"x"}`:                               "jenis",
		`{"jenis":"catatan","teks":"  "}`:                                "teks",
		`{"jenis":"catatan","teks":"` + strings.Repeat("x", 2001) + `"}`: "teks",
		`{"jenis":"pekerjaan","teks":"x","tanggal":"2026-11-10"}`:        "tanggal",
		`{"jenis":"pekerjaan","teks":"x","durasi_menit":0}`:              "durasi_menit",
		`{"jenis":"catatan","teks":"x","klien_id":"bukan-uuid"}`:         "klien_id",
	} {
		rec := do(t, h, "POST", "/api/aktivitas", body, c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field[field] == "" {
			t.Errorf("%s: %d %v, want 422 on %s", body, rec.Code, e.Field, field)
		}
	}

	// An automatic entry cannot be deleted; a note can.
	do(t, h, "POST", "/api/item", `{"nama":"VPS","jenis":"langganan","klien_id":"`+k.ID+`","kedaluwarsa":"2026-12-01"}`, c)
	list := decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas?klien_id="+k.ID, "", c))
	if len(list.Aktivitas) != 3 || list.Aktivitas[0].Jenis != "aset_baru" || list.Lagi {
		t.Fatalf("list %+v", list)
	}
	if rec := do(t, h, "DELETE", "/api/aktivitas/"+list.Aktivitas[0].ID, "", c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("delete automatic entry: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/aktivitas/"+note.ID, "", c); rec.Code != http.StatusNoContent {
		t.Errorf("delete note: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/aktivitas/"+note.ID, "", c); rec.Code != http.StatusNotFound {
		t.Errorf("delete twice: %d", rec.Code)
	}
}

func TestAktivitasPaging(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "log@contoh.id")
	for i := range 5 {
		do(t, h, "POST", "/api/aktivitas", `{"jenis":"catatan","teks":"Catatan `+string(rune('1'+i))+`"}`, c)
	}
	page := decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas?batas=2", "", c))
	if len(page.Aktivitas) != 2 || !page.Lagi || page.Aktivitas[0].Teks != "Catatan 5" {
		t.Fatalf("page 1 %+v", page)
	}
	page = decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas?batas=2&sebelum="+page.Aktivitas[1].ID, "", c))
	if len(page.Aktivitas) != 2 || !page.Lagi || page.Aktivitas[0].Teks != "Catatan 3" {
		t.Fatalf("page 2 %+v", page)
	}
	page = decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas?batas=2&sebelum="+page.Aktivitas[1].ID, "", c))
	if len(page.Aktivitas) != 1 || page.Lagi || page.Aktivitas[0].Teks != "Catatan 1" {
		t.Fatalf("page 3 %+v", page)
	}
	for _, q := range []string{"batas=0", "batas=101", "sebelum=kemarin", "klien_id=x"} {
		if rec := do(t, h, "GET", "/api/aktivitas?"+q, "", c); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("?%s: %d", q, rec.Code)
		}
	}
	if rec := do(t, h, "GET", "/api/aktivitas?sebelum=2026-11-09T10:00:00%2B07:00", "", c); rec.Code != http.StatusOK {
		t.Errorf("RFC 3339 cursor: %d %s", rec.Code, rec.Body)
	}
}

func TestKlienStatus(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "status@contoh.id")
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Studio Rana"}`, c))
	if k.Status != "aktif" {
		t.Errorf("new client status %q", k.Status)
	}
	rec := do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Studio Rana","status":"jeda"}`, c)
	if rec.Code != http.StatusOK || decode[klienJSON](t, rec).Status != "jeda" {
		t.Errorf("set jeda: %d %s", rec.Code, rec.Body)
	}
	// A PUT without status keeps it, so older clients of the API change nothing.
	rec = do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Studio Rana Kreatif"}`, c)
	if rec.Code != http.StatusOK || decode[klienJSON](t, rec).Status != "jeda" {
		t.Errorf("PUT without status: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Studio Rana","status":"arsip"}`, c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown status: %d", rec.Code)
	}
}
