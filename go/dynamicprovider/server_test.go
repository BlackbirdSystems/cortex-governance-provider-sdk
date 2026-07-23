package dynamicprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamDownloadSignsResponseTrailer(t *testing.T) {
	secret := []byte("test-secret")
	body := "streamed content"
	server := NewServer(Meta{Name: "sample"}, secret, Callbacks{
		StreamDownload: func(FetchDownloadInput) (io.ReadCloser, int64, string, string, error) {
			return io.NopCloser(strings.NewReader(body)), int64(len(body)), "out.txt", "text/plain", nil
		},
	})

	requestBody, err := json.Marshal(FetchDownloadInput{})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/provider/stream-download", bytes.NewReader(requestBody))
	rr := httptest.NewRecorder()

	server.wrapStreamDownload(rr, req)

	resp := rr.Result()
	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if string(gotBody) != body {
		t.Fatalf("unexpected response body %q", string(gotBody))
	}
	timestamp := strings.TrimSpace(resp.Header.Get(DynamicProviderTimestampHeader))
	if timestamp == "" {
		t.Fatal("missing response timestamp")
	}
	if key := resp.Header.Get(DynamicProviderKeyHeader); key != "sample" {
		t.Fatalf("expected key sample, got %q", key)
	}
	signature := strings.TrimSpace(resp.Trailer.Get(DynamicProviderSignatureHeader))
	if signature == "" {
		t.Fatal("missing response signature trailer")
	}
	digest := sha256.Sum256(gotBody)
	expected := SignResponseDigest(req.Method, req.URL.Path, timestamp, http.StatusOK, hex.EncodeToString(digest[:]), secret)
	if signature != expected {
		t.Fatalf("unexpected response signature %q, want %q", signature, expected)
	}
}
