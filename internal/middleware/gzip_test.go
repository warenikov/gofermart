package middleware_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mw "github.com/warenikov/gofermart/internal/middleware"
)

func gzipBytes(t *testing.T, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(payload))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestDecompressGzip_NoHeader_PassesThrough(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("plain"))
	var seen string
	mw.DecompressGzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = string(body)
	})).ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, "plain", seen)
}

func TestDecompressGzip_ValidGzip_Unwraps(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(gzipBytes(t, `{"a":1}`)))
	req.Header.Set("Content-Encoding", "gzip")
	var seen string
	mw.DecompressGzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = string(body)
		assert.Empty(t, r.Header.Get("Content-Encoding"), "Content-Encoding должен быть убран")
	})).ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, `{"a":1}`, seen)
}

func TestDecompressGzip_InvalidGzip_Returns400(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-a-gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	rr := httptest.NewRecorder()
	mw.DecompressGzip(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("хендлер не должен быть вызван")
	})).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDecompressGzip_OtherEncoding_PassesThrough(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("compressed-with-br"))
	req.Header.Set("Content-Encoding", "br")
	var seen string
	mw.DecompressGzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = string(body)
		assert.Equal(t, "br", r.Header.Get("Content-Encoding"))
	})).ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, "compressed-with-br", seen)
}
