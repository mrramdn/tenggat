package store

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestTokenRoundTripAndScoping(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")

	// Empty list is non-nil.
	if list, err := s.ListTokens(ctx, u.ID); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("ListTokens(empty) = %v, %v; want empty non-nil slice", list, err)
	}

	exp := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	t1, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:    "Skrip Backup",
		Hash:    "hash-token-1",
		Awalan:  "tgt_abcd",
		Cakupan: "baca",
	})
	if err != nil {
		t.Fatalf("CreateToken(t1): %v", err)
	}
	if t1.ID == "" || t1.PenggunaID != u.ID || t1.Nama != "Skrip Backup" || t1.Awalan != "tgt_abcd" ||
		t1.Cakupan != "baca" || t1.Dibuat.IsZero() || t1.TerakhirDipakai != nil || t1.Kedaluwarsa != nil {
		t.Errorf("CreateToken(t1) = %+v", t1)
	}

	t2, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:        "Claude Desktop",
		Hash:        "hash-token-2",
		Awalan:      "tgt_efgh",
		Cakupan:     "tulis",
		Kedaluwarsa: &exp,
	})
	if err != nil {
		t.Fatalf("CreateToken(t2): %v", err)
	}
	if t2.Cakupan != "tulis" || !sama(t2.Kedaluwarsa, &exp) {
		t.Errorf("CreateToken(t2) = %+v", t2)
	}

	list, err := s.ListTokens(ctx, u.ID)
	if err != nil || len(list) != 2 || list[0].ID != t2.ID || list[1].ID != t1.ID {
		t.Errorf("ListTokens = %+v, %v; want [t2, t1]", list, err)
	}
	if otherList, err := s.ListTokens(ctx, other.ID); err != nil || len(otherList) != 0 {
		t.Errorf("ListTokens(other) = %+v, %v", otherList, err)
	}

	// Another user cannot delete Rana's token.
	if err := s.DeleteToken(ctx, other.ID, t1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteToken(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteToken(ctx, u.ID, t1.ID); err != nil {
		t.Fatalf("DeleteToken(t1): %v", err)
	}
	if err := s.DeleteToken(ctx, u.ID, t1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteToken(t1) err = %v, want ErrNotFound", err)
	}
	if list, err = s.ListTokens(ctx, u.ID); err != nil || len(list) != 1 || list[0].ID != t2.ID {
		t.Errorf("ListTokens after delete = %+v, %v", list, err)
	}
}

func TestTokenLimit(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")

	var firstID string
	for i := range MaxTokensPerUser {
		tok, err := s.CreateToken(ctx, u.ID, TokenInput{
			Nama:    "Token " + strconv.Itoa(i+1),
			Hash:    "hash-rana-" + strconv.Itoa(i+1),
			Awalan:  "tgt_0000",
			Cakupan: "baca",
		})
		if err != nil {
			t.Fatalf("CreateToken #%d: %v", i+1, err)
		}
		if i == 0 {
			firstID = tok.ID
		}
	}

	// The 21st token is rejected with ErrTokenLimit.
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:    "Token 21",
		Hash:    "hash-rana-21",
		Awalan:  "tgt_0021",
		Cakupan: "baca",
	}); !errors.Is(err, ErrTokenLimit) {
		t.Errorf("21st CreateToken err = %v, want ErrTokenLimit", err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM token_akses WHERE pengguna_id = $1`, u.ID); n != MaxTokensPerUser {
		t.Errorf("token count = %d, want %d", n, MaxTokensPerUser)
	}

	// Another user can still create tokens.
	if _, err := s.CreateToken(ctx, other.ID, TokenInput{
		Nama:    "Token Budi",
		Hash:    "hash-budi-1",
		Awalan:  "tgt_budi",
		Cakupan: "tulis",
	}); err != nil {
		t.Errorf("CreateToken(other user): %v", err)
	}

	// Deleting one token frees a slot for Rana.
	if err := s.DeleteToken(ctx, u.ID, firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:    "Token 21",
		Hash:    "hash-rana-21",
		Awalan:  "tgt_0021",
		Cakupan: "baca",
	}); err != nil {
		t.Errorf("CreateToken after deleting one: %v", err)
	}
}

func TestUserByTokenExpiryAndTerakhirDipakai(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	noExp, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:    "Tanpa Kedaluwarsa",
		Hash:    "hash-no-exp",
		Awalan:  "tgt_noex",
		Cakupan: "tulis",
	})
	if err != nil {
		t.Fatal(err)
	}
	future := now.Add(time.Hour)
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:        "Belum Habis",
		Hash:        "hash-future",
		Awalan:      "tgt_futr",
		Cakupan:     "baca",
		Kedaluwarsa: &future,
	}); err != nil {
		t.Fatal(err)
	}
	exact := now
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:        "Tepat Habis",
		Hash:        "hash-exact",
		Awalan:      "tgt_exct",
		Cakupan:     "baca",
		Kedaluwarsa: &exact,
	}); err != nil {
		t.Fatal(err)
	}
	past := now.Add(-time.Second)
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{
		Nama:        "Sudah Habis",
		Hash:        "hash-past",
		Awalan:      "tgt_past",
		Cakupan:     "baca",
		Kedaluwarsa: &past,
	}); err != nil {
		t.Fatal(err)
	}

	// Expired or unknown tokens return ErrNotFound.
	for _, h := range []string{"hash-exact", "hash-past", "hash-unknown"} {
		if _, _, err := s.UserByToken(ctx, h, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("UserByToken(%q) err = %v, want ErrNotFound", h, err)
		}
	}

	// Non-expired token with Kedaluwarsa in the future succeeds.
	if gotU, gotT, err := s.UserByToken(ctx, "hash-future", now); err != nil || gotU != u || gotT.Cakupan != "baca" {
		t.Errorf("UserByToken(future) = %+v, %+v, %v", gotU, gotT, err)
	}

	// First use stamps terakhir_dipakai = now.
	gotU, gotT, err := s.UserByToken(ctx, "hash-no-exp", now)
	if err != nil || gotU != u || gotT.ID != noExp.ID || !sama(gotT.TerakhirDipakai, &now) {
		t.Fatalf("first UserByToken = %+v, %+v, %v", gotU, gotT, err)
	}

	// A second use 30 seconds later (< 1 minute) keeps the original terakhir_dipakai.
	withinMinute := now.Add(30 * time.Second)
	_, gotT, err = s.UserByToken(ctx, "hash-no-exp", withinMinute)
	if err != nil || !sama(gotT.TerakhirDipakai, &now) {
		t.Errorf("UserByToken(+30s) TerakhirDipakai = %v, %v; want %v", gotT.TerakhirDipakai, err, now)
	}
	var dbLast *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT terakhir_dipakai FROM token_akses WHERE id = $1`, noExp.ID).Scan(&dbLast); err != nil || !sama(dbLast, &now) {
		t.Errorf("DB terakhir_dipakai(+30s) = %v, %v; want %v", dbLast, err, now)
	}

	// Use at exactly +1 minute updates terakhir_dipakai.
	afterMinute := now.Add(time.Minute)
	_, gotT, err = s.UserByToken(ctx, "hash-no-exp", afterMinute)
	if err != nil || !sama(gotT.TerakhirDipakai, &afterMinute) {
		t.Errorf("UserByToken(+1m) TerakhirDipakai = %v, %v; want %v", gotT.TerakhirDipakai, err, afterMinute)
	}
	if err := s.pool.QueryRow(ctx, `SELECT terakhir_dipakai FROM token_akses WHERE id = $1`, noExp.ID).Scan(&dbLast); err != nil || !sama(dbLast, &afterMinute) {
		t.Errorf("DB terakhir_dipakai(+1m) = %v, %v; want %v", dbLast, err, afterMinute)
	}
}

func TestTokenConstraintsAndCascade(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")

	_, err := s.CreateToken(ctx, u.ID, TokenInput{Nama: "", Hash: "h1", Awalan: "tgt_1111", Cakupan: "baca"})
	mustCheckViolation(t, err, "token_akses_nama_check")
	_, err = s.CreateToken(ctx, u.ID, TokenInput{Nama: "1234567890123456789012345678901234567890123456789012345678901", Hash: "h2", Awalan: "tgt_2222", Cakupan: "baca"})
	mustCheckViolation(t, err, "token_akses_nama_check")
	_, err = s.CreateToken(ctx, u.ID, TokenInput{Nama: "Valid", Hash: "h3", Awalan: "tgt_3333", Cakupan: "admin"})
	mustCheckViolation(t, err, "token_akses_cakupan_check")

	if _, err := s.CreateToken(ctx, u.ID, TokenInput{Nama: "Token 1", Hash: "hash-unik", Awalan: "tgt_unik", Cakupan: "baca"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateToken(ctx, u.ID, TokenInput{Nama: "Token 2", Hash: "hash-unik", Awalan: "tgt_unik", Cakupan: "baca"}); err == nil {
		t.Errorf("CreateToken(duplicate hash) succeeded")
	} else if code, constraint := kodeSQL(err); code != "23505" || constraint != "token_akses_hash_unik" {
		t.Errorf("duplicate hash err = %v (%s, %s), want 23505 on token_akses_hash_unik", err, code, constraint)
	}

	// Deleting the user cascades to token_akses.
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM token_akses`); n != 0 {
		t.Errorf("token_akses has %d rows after DeleteUser, want 0", n)
	}
}
