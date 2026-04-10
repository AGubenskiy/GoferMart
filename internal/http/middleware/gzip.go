package middleware

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

func RequestDecompressor(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			reader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			defer func() {
				if err := reader.Close(); err != nil {
					logCloseError(log, "close gzip request reader", r, err)
				}
			}()

			originalBody := r.Body
			defer func() {
				if err := originalBody.Close(); err != nil {
					logCloseError(log, "close original request body", r, err)
				}
			}()

			r.Body = &readCloser{
				Reader: reader, // Нужно ли ограничивать максимальный размер в теории можно использовать атаку Zip Bomb
				Closer: originalBody,
			}
			r.Header.Del("Content-Encoding")

			next.ServeHTTP(w, r)
		})
	}
}

func ResponseCompressor(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Add("Vary", "Accept-Encoding")

			compressedWriter := newGzipResponseWriter(w)
			defer func() {
				if err := compressedWriter.Close(); err != nil {
					logCloseError(log, "close gzip response writer", r, err)
				}
			}()

			next.ServeHTTP(compressedWriter, r)
		})
	}
}

func logCloseError(log *slog.Logger, message string, r *http.Request, err error) {
	if log == nil {
		return
	}

	log.Error(message, "method", r.Method, "path", r.URL.Path, "error", err)
}

type readCloser struct {
	io.Reader
	io.Closer
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	compressing bool
	statusCode  int
}

func newGzipResponseWriter(w http.ResponseWriter) *gzipResponseWriter {
	return &gzipResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (w *gzipResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	if !shouldWriteBody(statusCode) {
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *gzipResponseWriter) Write(payload []byte) (int, error) {
	if !w.compressing {
		w.startCompression()
	}

	return w.writer.Write(payload)
}

func (w *gzipResponseWriter) Close() error {
	if !w.compressing {
		if shouldWriteBody(w.statusCode) {
			w.ResponseWriter.WriteHeader(w.statusCode)
		}
		return nil
	}

	return w.writer.Close()
}

func (w *gzipResponseWriter) startCompression() {
	if w.compressing {
		return
	}

	w.compressing = true
	headers := w.ResponseWriter.Header()
	headers.Set("Content-Encoding", "gzip")
	headers.Del("Content-Length")
	w.ResponseWriter.WriteHeader(w.statusCode)
	w.writer = gzip.NewWriter(w.ResponseWriter)
}

func shouldWriteBody(statusCode int) bool {
	return statusCode >= 200 && statusCode != http.StatusNoContent && statusCode != http.StatusNotModified
}
