package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/trace"
)

type captureRequestBody struct {
	io.ReadCloser
	bytes.Buffer
}

func (c *captureRequestBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		_, _ = c.Buffer.Write(p[:n])
	}
	return n, err
}

type captureResponseBody struct {
	header http.Header
	bytes.Buffer
}

func (c *captureResponseBody) Write(p []byte) (int, error) {
	if isTextBody(c.header.Get("Content-Type")) {
		_, _ = c.Buffer.Write(p)
	}
	return len(p), nil
}

func isTextBody(contentType string) bool {
	return strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "application/problem+json") ||
		strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "text/")
}

// Keep individual Loki entries small while preserving the complete text body.
func logBodyChunks(ctx context.Context, requestID, message string, body []byte) {
	if len(body) == 0 {
		return
	}
	const chunkSize = 16 * 1024
	span := trace.SpanContextFromContext(ctx)
	attrs := []any{"requestId", requestID}
	if span.IsValid() {
		attrs = append(attrs, "traceId", span.TraceID().String(), "spanId", span.SpanID().String())
	}
	if !utf8.Valid(body) {
		for i, start := 0, 0; start < len(body); i, start = i+1, start+chunkSize {
			end := min(start+chunkSize, len(body))
			slog.InfoContext(ctx, message, append(attrs, "part", i+1, "encoding", "base64", "body", base64.StdEncoding.EncodeToString(body[start:end]))...)
		}
		return
	}
	for i, start := 0, 0; start < len(body); i++ {
		end := min(start+chunkSize, len(body))
		for end < len(body) && !utf8.RuneStart(body[end]) {
			end--
		}
		slog.InfoContext(ctx, message, append(attrs, "part", i+1, "body", string(body[start:end]))...)
		start = end
	}
}
