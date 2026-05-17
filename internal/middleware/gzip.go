package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// DecompressGzip распаковывает тело запроса, если клиент прислал Content-Encoding: gzip.
// На некорректный gzip — 400. Остальные кодировки (br, deflate) пропускаются как есть.
func DecompressGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc := r.Header.Get("Content-Encoding")
		if enc == "" || !strings.Contains(enc, "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		defer func() { _ = gz.Close() }()

		r.Body = io.NopCloser(gz)
		r.Header.Del("Content-Encoding")
		r.Header.Del("Content-Length")

		next.ServeHTTP(w, r)
	})
}
