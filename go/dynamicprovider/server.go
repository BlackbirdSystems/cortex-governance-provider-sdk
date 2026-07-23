package dynamicprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type Callbacks struct {
	Tools           func() []mcp.Tool
	MapRequest      func(MapRequestInput) (MapRequestOutput, error)
	ResolveResource func(ResolveResourceInput) (ResolveResourceOutput, error)
	MapScope        func(MapScopeInput) (MapScopeOutput, error)
	Execute         func(ExecuteInput) (map[string]any, error)
	ParseSettings   func(ParseSettingsInput) (ParseSettingsOutput, error)
	ComputeBinding  func(ComputeBindingInput) (ComputeBindingOutput, error)
	IsAvailable     func(IsAvailableInput) (bool, error)
	FetchDownload   func(FetchDownloadInput) (FetchDownloadOutput, error)
	StreamDownload  func(FetchDownloadInput) (io.ReadCloser, int64, string, string, error)
}

type Server struct {
	Meta      Meta
	Secret    []byte
	Callbacks Callbacks
}

func NewServer(meta Meta, secret []byte, callbacks Callbacks) *Server {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = ProtocolVersion
	}
	return &Server{Meta: meta, Secret: secret, Callbacks: callbacks}
}

func SecretFromEnv() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("GOVERNANCE_DYNAMIC_PROVIDER_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("GOVERNANCE_SECRET"))
	}
	if secret == "" {
		return nil, fmt.Errorf("set GOVERNANCE_DYNAMIC_PROVIDER_SECRET or GOVERNANCE_SECRET")
	}
	return []byte(secret), nil
}

func (s *Server) ServeUDS(socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o666); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}
	return http.Serve(listener, s.withHMAC(s.routes()))
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.writeText(w, r, http.StatusOK, "ok")
	})
	mux.HandleFunc("/v1/provider/meta", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, r, s.Meta)
	})
	mux.HandleFunc("/v1/provider/tools", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, r, ToolsResponse{Tools: s.Callbacks.Tools()})
	})
	mux.HandleFunc("/v1/provider/map-request", s.wrapMapRequest)
	mux.HandleFunc("/v1/provider/resolve-resource", s.wrapResolveResource)
	mux.HandleFunc("/v1/provider/map-scope", s.wrapMapScope)
	mux.HandleFunc("/v1/provider/execute", s.wrapExecute)
	mux.HandleFunc("/v1/provider/parse-settings", s.wrapParseSettings)
	mux.HandleFunc("/v1/provider/compute-binding", s.wrapComputeBinding)
	mux.HandleFunc("/v1/provider/is-available", s.wrapIsAvailable)
	mux.HandleFunc("/v1/provider/fetch-download", s.wrapFetchDownload)
	mux.HandleFunc("/v1/provider/stream-download", s.wrapStreamDownload)
	return mux
}

func (s *Server) wrapMapRequest(w http.ResponseWriter, r *http.Request) {
	var input MapRequestInput
	s.mustDecode(w, r, &input)
	output, err := s.Callbacks.MapRequest(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) wrapResolveResource(w http.ResponseWriter, r *http.Request) {
	var input ResolveResourceInput
	s.mustDecode(w, r, &input)
	output, err := s.Callbacks.ResolveResource(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) wrapMapScope(w http.ResponseWriter, r *http.Request) {
	var input MapScopeInput
	s.mustDecode(w, r, &input)
	output, err := s.Callbacks.MapScope(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) wrapExecute(w http.ResponseWriter, r *http.Request) {
	var input ExecuteInput
	s.mustDecode(w, r, &input)
	output, err := s.Callbacks.Execute(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, map[string]any{"result": output})
}

func (s *Server) wrapParseSettings(w http.ResponseWriter, r *http.Request) {
	var input ParseSettingsInput
	s.mustDecode(w, r, &input)

	if s.Meta.BindingSchema != nil {
		for _, schemaInput := range s.Meta.BindingSchema.Inputs {
			if schemaInput.Hash && schemaInput.EnvKey != "" {
				settingVal, ok := input.Settings[schemaInput.SettingKey]
				if !ok {
					key := schemaInput.SettingKey
					if key == "" {
						key = schemaInput.Name
					}
					settingVal, ok = input.Settings[key]
				}

				hashVal, _ := settingVal.(string)
				hashVal = strings.TrimSpace(hashVal)
				if hashVal == "" && schemaInput.Required {
					s.writeError(w, r, http.StatusUnauthorized, fmt.Sprintf("missing required setting: %s", schemaInput.Name))
					return
				}

				if hashVal != "" {
					actualSecret := strings.TrimSpace(os.Getenv(schemaInput.EnvKey))
					if actualSecret == "" {
						s.writeError(w, r, http.StatusInternalServerError, fmt.Sprintf("provider error: environment variable %s is not set on the agent", schemaInput.EnvKey))
						return
					}

					h := sha256.New()
					h.Write([]byte(actualSecret))
					computedHash := hex.EncodeToString(h.Sum(nil))

					if !strings.EqualFold(computedHash, hashVal) {
						s.writeError(w, r, http.StatusUnauthorized, fmt.Sprintf("unauthorized: provided hash for %s does not match the agent's hosted secret", schemaInput.Name))
						return
					}
				}
			}
		}
	}

	output, err := s.Callbacks.ParseSettings(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) wrapComputeBinding(w http.ResponseWriter, r *http.Request) {
	var input ComputeBindingInput
	s.mustDecode(w, r, &input)
	output, err := s.Callbacks.ComputeBinding(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) wrapIsAvailable(w http.ResponseWriter, r *http.Request) {
	var input IsAvailableInput
	s.mustDecode(w, r, &input)
	ok, err := s.Callbacks.IsAvailable(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, map[string]bool{"ok": ok})
}

func (s *Server) wrapFetchDownload(w http.ResponseWriter, r *http.Request) {
	var input FetchDownloadInput
	s.mustDecode(w, r, &input)
	if s.Callbacks.FetchDownload == nil {
		s.writeError(w, r, http.StatusNotFound, "fetch-download not implemented")
		return
	}
	output, err := s.Callbacks.FetchDownload(input)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, r, output)
}

func (s *Server) mustDecode(w http.ResponseWriter, r *http.Request, out interface{}) {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		panic(http.ErrAbortHandler)
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	slog.Error("governance provider error response",
		slog.String("path", r.URL.Path),
		slog.Int("status", status),
		slog.String("error", msg),
	)
	s.writeSigned(w, r, status, "text/plain; charset=utf-8", []byte(msg+"\n"))
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, body interface{}) {
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	s.writeSigned(w, r, http.StatusOK, "application/json", data)
}

func (s *Server) writeText(w http.ResponseWriter, r *http.Request, status int, body string) {
	s.writeSigned(w, r, status, "text/plain", []byte(body))
}

func (s *Server) writeSigned(w http.ResponseWriter, r *http.Request, status int, contentType string, data []byte) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	w.Header().Set(DynamicProviderTimestampHeader, timestamp)
	w.Header().Set(DynamicProviderKeyHeader, s.Meta.Name)
	w.Header().Set(DynamicProviderSignatureHeader, SignResponse(r.Method, r.URL.Path, timestamp, status, data, s.Secret))
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (s *Server) withHMAC(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if r.URL.Path != "/healthz" {
			timestamp := strings.TrimSpace(r.Header.Get(DynamicProviderTimestampHeader))
			signature := strings.TrimSpace(r.Header.Get(DynamicProviderSignatureHeader))
			key := strings.TrimSpace(r.Header.Get(DynamicProviderKeyHeader))
			if key != s.Meta.Name || timestamp == "" || signature == "" {
				s.writeError(w, r, http.StatusUnauthorized, "missing HMAC headers")
				return
			}
			if !VerifyRequestSignature(r.Method, r.URL.Path, timestamp, body, signature, s.Secret) {
				s.writeError(w, r, http.StatusUnauthorized, "invalid HMAC signature")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
