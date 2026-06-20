package dynamicprovider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func SignRequest(method, path, timestamp string, body, secret []byte) string {
	digest := sha256.Sum256(body)
	payload := strings.Join([]string{
		"cortex-dynamic-provider-request",
		ProtocolVersion,
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		strings.TrimSpace(timestamp),
		hex.EncodeToString(digest[:]),
	}, "\n")
	return computeHMAC(secret, payload)
}

func SignResponse(method, path, timestamp string, statusCode int, body, secret []byte) string {
	digest := sha256.Sum256(body)
	payload := strings.Join([]string{
		"cortex-dynamic-provider-response",
		ProtocolVersion,
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		fmt.Sprintf("%d", statusCode),
		strings.TrimSpace(timestamp),
		hex.EncodeToString(digest[:]),
	}, "\n")
	return computeHMAC(secret, payload)
}

func VerifyRequestSignature(method, path, timestamp string, body []byte, signature string, secret []byte) bool {
	expected := SignRequest(method, path, timestamp, body, secret)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func computeHMAC(secret []byte, payload string) string {
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}
