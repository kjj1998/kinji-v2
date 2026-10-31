// Package token creates and hashes the bearer tokens the iPhone Shortcut sends to ingest.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Prefix starts every token, so a leaked token is easy to grep for and to flag in secret scanners.
const Prefix = "kji_"

// Generate returns a new ingest token: Prefix followed by 32 random bytes from crypto/rand,
// base64url-encoded without padding (47 characters in total). The caller shows it once and
// stores only Hash(token).
func Generate() string {
	b := make([]byte, 32)
	rand.Read(b)
	encodedString := base64.RawURLEncoding.EncodeToString(b)

	return Prefix + encodedString
}

// Hash returns the SHA-256 digest of the full token, prefix included, as 32 raw bytes.
// This is the value stored in and looked up from api_tokens.token_hash.
func Hash(token string) []byte {
	hash := sha256.Sum256([]byte(token))

	return hash[:]
}
