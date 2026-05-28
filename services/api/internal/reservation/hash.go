package reservation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns a deterministic HMAC-SHA256 digest of the booking operation
// identity. It is the Redis hold-key suffix and the idempotency_hash log
// field. The raw client idempotency key never appears in Redis, logs, or
// admin payloads — only this hash does.
func Hash(secret []byte, op, eventID, actorID, idempotencyKey string) string {
	mac := hmac.New(sha256.New, secret)
	writeField(mac, op)
	writeField(mac, eventID)
	writeField(mac, actorID)
	writeField(mac, idempotencyKey)
	return hex.EncodeToString(mac.Sum(nil))
}

// ActorHash returns a digest scoped to actor identity only. Safe to log for
// replay/debug correlation.
func ActorHash(secret []byte, actorID string) string {
	mac := hmac.New(sha256.New, secret)
	writeField(mac, "actor")
	writeField(mac, actorID)
	return hex.EncodeToString(mac.Sum(nil))
}

type hasher interface{ Write([]byte) (int, error) }

func writeField(h hasher, s string) {
	_, _ = h.Write([]byte(s))
	_, _ = h.Write([]byte{0})
}
