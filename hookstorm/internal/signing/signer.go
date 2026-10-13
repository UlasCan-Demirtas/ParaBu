package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignPayload generates an HMAC SHA256 signature for the given payload and secret.
// This matches ParaBu's webhook and API signature strategy.
func SignPayload(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature checks if the provided signature matches the payload and secret.
func VerifySignature(payload []byte, signature, secret string) bool {
	expectedSig := SignPayload(payload, secret)
	// Use hmac.Equal to avoid timing attacks
	return hmac.Equal([]byte(expectedSig), []byte(signature))
}
