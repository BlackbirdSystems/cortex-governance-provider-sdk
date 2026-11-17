package dynamicprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

func TestWrapParseSettings_ProviderSecrets(t *testing.T) {
	t.Setenv("PROVIDER_SECRETS", "secret-key-1, secret-key-2, ntn_1234567890")

	meta := Meta{
		Name: "test-provider",
		BindingSchema: &BindingSchema{
			Inputs: []BindingInputSchema{
				{
					Name:       "fathom_api_key",
					SettingKey: "fathom_api_key",
					Secret:     true,
					EnvKey:     "FATHOM_API_KEY",
				},
				{
					Name:       "notion_api_key",
					SettingKey: "notion_api_key",
					Hash:       true,
					EnvKey:     "NOTION_API_KEY",
				},
			},
		},
	}

	secret := []byte("test-secret")
	server := NewServer(meta, secret, Callbacks{
		ParseSettings: func(input ParseSettingsInput) (ParseSettingsOutput, error) {
			return ParseSettingsOutput{}, nil
		},
	})

	// Case 1: Match raw secret token (secret-key-2)
	body, _ := json.Marshal(ParseSettingsInput{
		TenantID: "tenant-1",
		Settings: map[string]interface{}{
			"fathom_api_key": "secret-key-2",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/provider/parse-settings", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	server.wrapParseSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for raw secret-key-2, got %d: %s", rr.Code, rr.Body.String())
	}

	// Case 2: Match SHA256 hash digest of ntn_1234567890
	h := sha256.Sum256([]byte("ntn_1234567890"))
	hashDigest := hex.EncodeToString(h[:])
	body, _ = json.Marshal(ParseSettingsInput{
		TenantID: "tenant-2",
		Settings: map[string]interface{}{
			"notion_api_key": hashDigest,
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/provider/parse-settings", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	server.wrapParseSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for SHA256 hashDigest, got %d: %s", rr.Code, rr.Body.String())
	}

	// Case 3: Invalid secret token should return status 401
	body, _ = json.Marshal(ParseSettingsInput{
		TenantID: "tenant-3",
		Settings: map[string]interface{}{
			"fathom_api_key": "invalid-secret",
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/provider/parse-settings", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	server.wrapParseSettings(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for invalid secret, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWrapApprovalPreview(t *testing.T) {
	post := func(server *Server, input ApprovalPreviewInput) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/v1/provider/approval-preview", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		server.wrapApprovalPreview(rr, req)
		return rr
	}
	input := ApprovalPreviewInput{
		Request: &Request{Provider: "sample", Action: "send", Params: map[string]interface{}{"message_id": "m1"}},
	}

	t.Run("rows round-trip", func(t *testing.T) {
		server := NewServer(Meta{Name: "sample"}, []byte("s"), Callbacks{
			ApprovalPreview: func(in ApprovalPreviewInput) (ApprovalPreviewOutput, error) {
				if in.Request == nil || in.Request.Params["message_id"] != "m1" {
					t.Fatalf("request not delivered: %+v", in.Request)
				}
				return ApprovalPreviewOutput{Rows: []ApprovalPreviewRow{{Label: "Replying to Ann", Value: "hi", Replaces: "/params/message_id"}}}, nil
			},
		})
		rr := post(server, input)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		var out ApprovalPreviewOutput
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Rows) != 1 || out.Rows[0].Replaces != "/params/message_id" {
			t.Fatalf("unexpected rows: %+v", out.Rows)
		}
	})
	t.Run("no callback answers 404 so the host knows there is nothing to add", func(t *testing.T) {
		server := NewServer(Meta{Name: "sample"}, []byte("s"), Callbacks{})
		if rr := post(server, input); rr.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", rr.Code)
		}
	})
	t.Run("a callback error is a failure, not an empty result", func(t *testing.T) {
		server := NewServer(Meta{Name: "sample"}, []byte("s"), Callbacks{
			ApprovalPreview: func(ApprovalPreviewInput) (ApprovalPreviewOutput, error) {
				return ApprovalPreviewOutput{}, errors.New("upstream 403")
			},
		})
		if rr := post(server, input); rr.Code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", rr.Code)
		}
	})
}
