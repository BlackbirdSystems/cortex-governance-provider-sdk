package dynamicprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) wrapStreamDownload(w http.ResponseWriter, r *http.Request) {
	var input FetchDownloadInput
	s.mustDecode(w, r, &input)

	slog.Info("Dynamic provider receiving stream-download request",
		slog.String("provider", s.Meta.Name),
		slog.String("resource_id", input.Download.ResourceID),
		slog.String("filename", input.Download.Filename),
		slog.String("content_type", input.Download.ContentType),
	)

	if s.Callbacks.StreamDownload == nil {
		slog.Warn("Dynamic provider stream-download callback not implemented", slog.String("provider", s.Meta.Name))
		s.writeError(w, r, http.StatusNotFound, "stream-download not implemented")
		return
	}

	stream, size, filename, contentType, err := s.Callbacks.StreamDownload(input)
	if err != nil {
		slog.Error("Dynamic provider StreamDownload callback failed",
			slog.String("provider", s.Meta.Name),
			slog.String("resource_id", input.Download.ResourceID),
			slog.String("filename", input.Download.Filename),
			slog.String("content_type", input.Download.ContentType),
			slog.String("error", err.Error()),
		)
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	defer stream.Close()

	slog.Info("Dynamic provider stream-download callback returned stream",
		slog.String("provider", s.Meta.Name),
		slog.String("resource_id", input.Download.ResourceID),
		slog.String("filename", filename),
		slog.String("content_type", contentType),
		slog.Int64("expected_size", size),
	)

	if filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	w.Header().Set(DynamicProviderTimestampHeader, timestamp)
	w.Header().Set(DynamicProviderKeyHeader, s.Meta.Name)
	w.Header().Add("Trailer", DynamicProviderSignatureHeader)

	w.WriteHeader(http.StatusOK)
	hasher := sha256.New()
	copied, err := io.Copy(io.MultiWriter(w, hasher), stream)
	if err != nil {
		slog.Error("Dynamic provider failed to write stream download response",
			slog.String("provider", s.Meta.Name),
			slog.String("resource_id", input.Download.ResourceID),
			slog.String("filename", filename),
			slog.String("content_type", contentType),
			slog.Int64("expected_size", size),
			slog.Int64("bytes_written", copied),
			slog.String("error", err.Error()),
		)
	} else {
		digest := hex.EncodeToString(hasher.Sum(nil))
		w.Header().Set(DynamicProviderSignatureHeader, SignResponseDigest(r.Method, r.URL.Path, timestamp, http.StatusOK, digest, s.Secret))
		if size > 0 && copied != size {
			slog.Warn("Dynamic provider stream-download completed with unexpected byte count",
				slog.String("provider", s.Meta.Name),
				slog.String("resource_id", input.Download.ResourceID),
				slog.String("filename", filename),
				slog.String("content_type", contentType),
				slog.Int64("expected_size", size),
				slog.Int64("bytes_written", copied),
			)
		}
		slog.Info("Dynamic provider completed stream-download response",
			slog.String("provider", s.Meta.Name),
			slog.String("resource_id", input.Download.ResourceID),
			slog.String("filename", filename),
			slog.String("content_type", contentType),
			slog.Int64("expected_size", size),
			slog.Int64("bytes_written", copied),
			slog.String("response_signature", "trailer"),
		)
	}
}
