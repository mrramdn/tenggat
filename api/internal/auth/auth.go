// Package auth hashes passwords with Argon2id and makes session tokens.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters from the OWASP Password Storage Cheat Sheet (19 MiB, 2 passes,
// 1 lane): strong enough, and small enough for a container limited to 128 MB.
const (
	memoryKiB   = 19 * 1024
	iterations  = 2
	parallelism = 1
	saltLen     = 16
	keyLen      = 32
)

var b64 = base64.RawStdEncoding

// HashPassword returns a PHC-style string: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memoryKiB, parallelism, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, iterations, parallelism, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches an encoded hash, comparing in
// constant time with the parameters stored in the hash.
func CheckPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false
	}
	salt, err1 := b64.DecodeString(parts[4])
	want, err2 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// SpendTime runs one hash check against a fixed dummy hash. A login for an email
// that does not exist calls it, so it takes as long as one for an existing email
// and the response time does not reveal which emails have an account.
func SpendTime(password string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("tenggat: no such account") })
	CheckPassword(dummyHash, password)
}

// NewToken returns a random session token for the cookie and the hash to store.
func NewToken() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is the SHA-256 of a session token, as stored in the database.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
