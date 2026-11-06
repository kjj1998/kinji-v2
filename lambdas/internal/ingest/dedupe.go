package ingest

import (
	"crypto/sha256"

	"github.com/google/uuid"
)

// dedupeVersion tags the key scheme so a later scheme (hashing selected fields instead of the
// raw body) can never produce the same key as this one. Existing rows keep their old keys.
const dedupeVersion = "v1"

// dedupeKey returns the milestone 1 dedupe key: SHA-256 of user ID, source, and the raw body.
// It catches exact resends of the same payload by the same user. Fields are separated so
// different inputs can't run together into the same bytes: the user ID is a fixed 16 bytes,
// and a NUL byte ends the source, which is allowlisted and never contains one.
func dedupeKey(userID uuid.UUID, source string, body []byte) []byte {
	h := sha256.New()
	h.Write([]byte(dedupeVersion))
	h.Write([]byte{0})
	h.Write(userID[:])
	h.Write([]byte(source))
	h.Write([]byte{0})
	h.Write(body)
	return h.Sum(nil)
}
