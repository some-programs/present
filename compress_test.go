package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestAcceptedEncoding(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   string
	}{
		{"", ""},
		{"gzip", "gzip"},
		{"GZIP", "gzip"},
		{"x-gzip", "gzip"},
		{"gzip, deflate, br, zstd", "zstd"},
		{"zstd;q=0.5, gzip", "gzip"},
		{"zstd, gzip;q=1.0", "zstd"},
		{"zstd;q=0, gzip", "gzip"},
		{"gzip;q=0", ""},
		{"gzip; q=0.000, zstd ;q=0", ""},
		{"*", "zstd"},
		{"*;q=0.1, gzip", "gzip"},
		{"deflate, br", ""},
		{"identity", ""},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if tt.header != "" {
			r.Header.Set("Accept-Encoding", tt.header)
		}
		got := ""
		if i := acceptedEncoding(r); i >= 0 {
			got = encoders[i].coding
		}
		if got != tt.want {
			t.Errorf("acceptedEncoding(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestCompressible(t *testing.T) {
	for _, tt := range []struct {
		ct   string
		want bool
	}{
		{"text/html; charset=utf-8", true},
		{"text/css; charset=utf-8", true},
		{"text/javascript; charset=utf-8", true},
		{"image/svg+xml", true},
		{"application/json", true},
		{"application/x-tar", true},
		{"font/ttf", true},
		{"text/event-stream", false},
		{"image/png", false},
		{"image/jpeg", false},
		{"video/webm", false},
		{"font/woff2", false},
		{"", false},
	} {
		if got := compressible(tt.ct); got != tt.want {
			t.Errorf("compressible(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

// compressGet serves a request for path with h wrapped by compressHandler, with
// the request headers in hdr.
func compressGet(h http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	compressHandler(h).ServeHTTP(w, r)
	return w
}

// decode returns b decoded from the content coding.
func decode(t *testing.T, coding string, b []byte) string {
	t.Helper()
	var r io.Reader
	switch coding {
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		r = zr
	case "zstd":
		zr, err := zstd.NewReader(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()
		r = zr
	default:
		t.Fatalf("unknown coding %q", coding)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCompressHandler(t *testing.T) {
	page := "<!DOCTYPE html><html><body>" + strings.Repeat("<p>Hello, slides.</p>\n", 200) + "</body></html>"
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 4096)...)
	dir := t.TempDir()
	for name, data := range map[string]string{
		"a.css":  strings.Repeat("body { color: red; }\n", 200),
		"s.css":  "p{}",
		"a.png":  string(png),
		"a.webm": strings.Repeat("\x1aE\xdf\xa3", 1024),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/files/", http.StripPrefix("/files", http.FileServer(http.Dir(dir))))
	// The page is written without a Content-Type, as dirHandler does.
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(page))
	})
	mux.HandleFunc("/etag", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		http.ServeContent(w, r, "x.js", time.Time{}, strings.NewReader(strings.Repeat("let x = 1;\n", 200)))
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("bad ", 1000), http.StatusInternalServerError)
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
	})
	gz := map[string]string{"Accept-Encoding": "gzip, deflate"}

	for _, tt := range []struct{ accept, coding string }{
		{"gzip, deflate", "gzip"},
		{"gzip, deflate, br, zstd", "zstd"},
	} {
		t.Run("sniffed html "+tt.coding, func(t *testing.T) {
			w := compressGet(mux, "/page", map[string]string{"Accept-Encoding": tt.accept})
			if got := w.Header().Get("Content-Encoding"); got != tt.coding {
				t.Fatalf("Content-Encoding = %q, want %s", got, tt.coding)
			}
			if got, want := w.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding", got)
			}
			if w.Body.Len() >= len(page) {
				t.Errorf("compressed body is %d bytes, page is %d", w.Body.Len(), len(page))
			}
			if got := decode(t, tt.coding, w.Body.Bytes()); got != page {
				t.Errorf("body differs from the page")
			}
		})
	}

	t.Run("not accepted", func(t *testing.T) {
		w := compressGet(mux, "/page", map[string]string{"Accept-Encoding": "gzip;q=0, br"})
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}
		if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("Vary = %q, want Accept-Encoding", got)
		}
		if w.Body.String() != page {
			t.Errorf("body differs from the page")
		}
	})

	t.Run("file", func(t *testing.T) {
		w := compressGet(mux, "/files/a.css", gz)
		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		for _, h := range []string{"Content-Length", "Accept-Ranges"} {
			if got := w.Header().Get(h); got != "" {
				t.Errorf("%s = %q, want none", h, got)
			}
		}
		if got := decode(t, "gzip", w.Body.Bytes()); !strings.HasPrefix(got, "body { color: red; }") {
			t.Errorf("body = %.40q…", got)
		}
	})

	t.Run("weak etag", func(t *testing.T) {
		w := compressGet(mux, "/etag", gz)
		if got := w.Header().Get("ETag"); got != `W/"abc"` {
			t.Errorf("ETag = %q, want W/\"abc\"", got)
		}
		// The browser sends back the weak ETag.
		w = compressGet(mux, "/etag", map[string]string{"Accept-Encoding": "gzip", "If-None-Match": `W/"abc"`})
		if w.Code != http.StatusNotModified {
			t.Errorf("status for a matching ETag = %d, want 304", w.Code)
		}
		if w.Body.Len() != 0 || w.Header().Get("Content-Encoding") != "" {
			t.Errorf("304 response has body %q, Content-Encoding %q", w.Body, w.Header().Get("Content-Encoding"))
		}
	})

	for _, tt := range []struct {
		name, path string
		hdr        map[string]string
		code       int
	}{
		{"small", "/files/s.css", gz, http.StatusOK},
		{"image", "/files/a.png", gz, http.StatusOK},
		{"video", "/files/a.webm", gz, http.StatusOK},
		{"range", "/files/a.css", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-99"}, http.StatusPartialContent},
		{"error", "/error", gz, http.StatusInternalServerError},
		{"empty", "/empty", gz, http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := compressGet(mux, tt.path, tt.hdr)
			if w.Code != tt.code {
				t.Errorf("status = %d, want %d", w.Code, tt.code)
			}
			if got := w.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("Content-Encoding = %q, want none", got)
			}
			if bytes.HasPrefix(w.Body.Bytes(), []byte("\x1f\x8b")) || bytes.HasPrefix(w.Body.Bytes(), []byte("\x28\xb5\x2f\xfd")) {
				t.Errorf("body is compressed")
			}
		})
	}
}

// TestCompressHandlerStream checks that server-sent events pass through
// uncompressed and that flushing reaches the client.
func TestCompressHandlerStream(t *testing.T) {
	srv := httptest.NewServer(compressHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: change\ndata: change\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
		<-r.Context().Done()
	})))
	defer srv.Close()
	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
	buf := make([]byte, 64)
	n, err := io.ReadAtLeast(res.Body, buf, len("event: change\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); !strings.HasPrefix(got, "event: change\n") {
		t.Errorf("stream starts with %q", got)
	}
}

// TestCompressHandlerFlush checks that a compressed response that is
// flushed reaches the client before the handler returns.
func TestCompressHandlerFlush(t *testing.T) {
	for _, coding := range []string{"gzip", "zstd"} {
		t.Run(coding, func(t *testing.T) {
			done := make(chan struct{})
			srv := httptest.NewServer(compressHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				io.WriteString(w, "first\n")
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Error(err)
				}
				<-done
				io.WriteString(w, "second\n")
			})))
			defer srv.Close()
			req, err := http.NewRequest("GET", srv.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", coding)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if got := res.Header.Get("Content-Encoding"); got != coding {
				t.Fatalf("Content-Encoding = %q, want %s", got, coding)
			}
			var body io.Reader
			if coding == "gzip" {
				zr, err := gzip.NewReader(res.Body)
				if err != nil {
					t.Fatal(err)
				}
				body = zr
			} else {
				zr, err := zstd.NewReader(res.Body, zstd.WithDecoderConcurrency(1))
				if err != nil {
					t.Fatal(err)
				}
				defer zr.Close()
				body = zr
			}
			buf := make([]byte, len("first\n"))
			if _, err := io.ReadFull(body, buf); err != nil {
				t.Fatal(err)
			}
			close(done)
			rest, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(buf) + string(rest); got != "first\nsecond\n" {
				t.Errorf("body = %q", got)
			}
		})
	}
}
