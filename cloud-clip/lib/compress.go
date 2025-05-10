package lib

import (
	"net/http"
	"strings"

	"compress/gzip"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

/**
*** FILE: compress.go
***   handle response compression
**/

// compressionMiddleware adds support for `Content-Encoding: zstd`, `gzip`, and `br` (Brotli).
func compressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding := r.Header.Get("Accept-Encoding")

		if strings.Contains(acceptEncoding, "br") {
			// Handle Brotli encoding
			encoder := brotli.NewWriter(w)
			defer encoder.Close()

			w.Header().Set("Content-Encoding", "br")
			w.Header().Del("Content-Length") // Content length cannot be known with compression
			next.ServeHTTP(&compressedResponseWriter{ResponseWriter: w, writer: encoder}, r)
			return
		} else if strings.Contains(acceptEncoding, "zstd") {
			// Handle zstd encoding
			encoder, err := zstd.NewWriter(w)
			if err != nil {
				http.Error(w, "failed to create zstd writer", http.StatusInternalServerError)
				return
			}
			defer encoder.Close()

			w.Header().Set("Content-Encoding", "zstd")
			w.Header().Del("Content-Length") // Content length cannot be known with compression
			next.ServeHTTP(&compressedResponseWriter{ResponseWriter: w, writer: encoder}, r)
			return
		} else if strings.Contains(acceptEncoding, "gzip") {
			// Handle gzip encoding
			encoder := gzip.NewWriter(w)
			defer encoder.Close()

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length") // Content length cannot be known with compression
			next.ServeHTTP(&compressedResponseWriter{ResponseWriter: w, writer: encoder}, r)
			return
		}

		// Fallback to normal handler if no supported encoding is found
		next.ServeHTTP(w, r)
	})
}

// compressedResponseWriter wraps the standard ResponseWriter to support compression.
type compressedResponseWriter struct {
	http.ResponseWriter
	writer interface {
		Write([]byte) (int, error)
	}
}

func (crw *compressedResponseWriter) Write(b []byte) (int, error) {
	return crw.writer.Write(b)
}
