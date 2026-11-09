package auth

import (
	"strings"
	"testing"
)

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("rahasia-panjang-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected format: %s", h)
	}
	if !CheckPassword(h, "rahasia-panjang-1") {
		t.Error("correct password rejected")
	}
	if CheckPassword(h, "rahasia-panjang-2") {
		t.Error("wrong password accepted")
	}
	h2, _ := HashPassword("rahasia-panjang-1")
	if h == h2 {
		t.Error("two hashes of one password are equal: salt is not random")
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=0,t=2,p=1$c2FsdA$a2V5"} {
		if CheckPassword(bad, "x") {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 || HashToken(tok) != hash || len(hash) != 64 {
		t.Errorf("token %q hash %q", tok, hash)
	}
	tok2, _, _ := NewToken()
	if tok == tok2 {
		t.Error("tokens repeat")
	}
}
