package token

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateFormat(t *testing.T) {
	tok := Generate()

	if len(tok) != 47 {
		t.Fatalf("len(token) = %d, want 47", len(tok))
	}
	rest, ok := strings.CutPrefix(tok, "kji_")
	if !ok {
		t.Fatalf("token %q does not start with kji_", tok)
	}
	b, err := base64.RawURLEncoding.DecodeString(rest)
	if err != nil {
		t.Fatalf("token body is not raw base64url: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("decoded body is %d bytes, want 32", len(b))
	}
}

func TestGenerateUnique(t *testing.T) {
	tok1 := Generate()
	tok2 := Generate()

	if tok1 == tok2 {
		t.Errorf("tokens generated are not unique")
	}
}

func TestHashLength(t *testing.T) {
	hash := Hash("hello")

	if len(hash) != 32 {
		t.Errorf("hash length is not 32")
	}
}

func TestHashDeterministic(t *testing.T) {
	hash1 := Hash("hello")
	hash2 := Hash("hello")

	if !bytes.Equal(hash1, hash2) {
		t.Errorf("hash function is not deterministic")
	}
}

func TestHashKnownValue(t *testing.T) {
	// From: printf 'kji_test' | shasum -a 256
	const want = "5599f803da8e2d09efe14fc0c1568508b663ef5e07f0e722d29a761f5dbc1b33"

	got := Hash("kji_test")
	if len(got) != 32 {
		t.Fatalf("len(Hash) = %d, want 32 raw bytes", len(got))
	}
	if gotHex := hex.EncodeToString(got); gotHex != want {
		t.Errorf("Hash(%q) = %s, want %s", "kji_test", gotHex, want)
	}
}
