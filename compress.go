package main

import (
	"cmp"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// compressMinSize is the smallest response, by its Content-Length, that
// compressHandler compresses. Compressing less saves too little to pay
// for the work.
const compressMinSize = 1024

// An encoder compresses what is written to it to the writer it was last
// reset to.
type encoder interface {
	io.WriteCloser
	Flush() error
	Reset(w io.Writer)
}

// encoders holds a pool of encoders for each content coding that
// compressHandler uses, in the order it prefers them when the client
// accepts several equally.
var encoders = []struct {
	coding string
	pool   sync.Pool
}{
	{coding: "zstd", pool: sync.Pool{New: func() any {
		// RFC 9659 lets HTTP clients refuse windows larger than 8 MB.
		e, err := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedBetterCompression),
			zstd.WithEncoderConcurrency(1),
			zstd.WithWindowSize(8<<20))
		if err != nil {
			panic(err)
		}
		return e
	}}},
	{coding: "gzip", pool: sync.Pool{New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, 6)
		return w
	}}},
}

// compressHandler returns a handler that serves the responses of h
// compressed with zstd or gzip, whichever the client accepts, preferring
// zstd, if they are of a type that compresses well, such as HTML, CSS,
// JavaScript, SVG and the fonts archive. Responses other than 200 OK, such
// as partial content for a range request, and responses h has encoded
// itself are sent unchanged. So are server-sent events, which gain little
// from compression and are sent a bit at a time, and WebSocket
// connections, which h hijacks.
func compressHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			h.ServeHTTP(w, r)
			return
		}
		cw := &compressResponseWriter{ResponseWriter: w, coding: acceptedEncoding(r)}
		h.ServeHTTP(cw, r)
		// A handler that panics, such as with http.ErrAbortHandler,
		// leaves the response unfinished, so that the client does not
		// take it for a complete one.
		cw.close()
	})
}

// acceptedEncoding returns the index in encoders of the content coding
// with the highest q value in the Accept-Encoding header of r, or -1 if r
// accepts none of them.
func acceptedEncoding(r *http.Request) int {
	q := make(map[string]float64)
	for _, v := range r.Header.Values("Accept-Encoding") {
		for coding := range strings.SplitSeq(v, ",") {
			coding, params, _ := strings.Cut(coding, ";")
			coding = strings.ToLower(strings.TrimSpace(coding))
			if coding == "x-gzip" {
				coding = "gzip"
			}
			weight := 1.0
			for p := range strings.SplitSeq(params, ";") {
				name, value, _ := strings.Cut(p, "=")
				if strings.EqualFold(strings.TrimSpace(name), "q") {
					if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
						weight = f
					}
				}
			}
			q[coding] = weight
		}
	}
	best, bestQ := -1, 0.0
	for i := range encoders {
		w, ok := q[encoders[i].coding]
		if !ok {
			w = q["*"]
		}
		if w > bestQ {
			best, bestQ = i, w
		}
	}
	return best
}

// compressible reports whether a response of the media type ct is worth
// compressing.
func compressible(ct string) bool {
	mt, _, _ := strings.Cut(ct, ";")
	mt = strings.ToLower(strings.TrimSpace(mt))
	switch {
	case mt == "text/event-stream":
		return false
	case strings.HasPrefix(mt, "text/"),
		strings.HasSuffix(mt, "+xml"),
		strings.HasSuffix(mt, "+json"):
		return true
	}
	switch mt {
	case "application/javascript", "application/json", "application/xml",
		"application/wasm", "application/x-tar",
		"font/ttf", "font/otf", "application/font-sfnt":
		return true
	}
	return false
}

// A compressResponseWriter compresses the response written to it, if it
// is 200 OK, of a compressible type and the client accepts a coding of
// encoders. It decides when the first byte of the body is written, or the
// response is flushed, since the handler may set its headers up to then,
// and it then writes the header of the response.
type compressResponseWriter struct {
	http.ResponseWriter
	coding  int     // the index in encoders of the coding to use, or -1
	code    int     // the status passed to WriteHeader, or 0
	decided bool    // whether the header has been written
	enc     encoder // the body is written to enc, if not nil
}

func (w *compressResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compressResponseWriter) WriteHeader(code int) {
	switch {
	case w.decided:
		// Let the ResponseWriter report the superfluous call.
		w.ResponseWriter.WriteHeader(code)
	case w.code != 0:
		// The first call holds, as with the ResponseWriter.
	case code < 200:
		// Informational responses come ahead of the final one.
		w.ResponseWriter.WriteHeader(code)
	case code != http.StatusOK:
		w.code = code
		w.decide(nil)
	default:
		w.code = code
	}
}

func (w *compressResponseWriter) Write(p []byte) (int, error) {
	if !w.decided {
		if len(p) == 0 {
			return 0, nil
		}
		w.decide(p)
	}
	if w.enc != nil {
		return w.enc.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// ReadFrom lets an uncompressed response be sent the way the
// ResponseWriter sends files, such as with sendfile, as http.FileServer
// does for videos.
func (w *compressResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if _, ok := w.Header()["Content-Type"]; !w.decided && ok {
		w.decide(nil)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok && w.decided && w.enc == nil {
		return rf.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{w}, r)
}

func (w *compressResponseWriter) Flush() {
	w.FlushError()
}

func (w *compressResponseWriter) FlushError() error {
	if !w.decided {
		w.decide(nil)
	}
	if w.enc != nil {
		if err := w.enc.Flush(); err != nil {
			return err
		}
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// decide decides whether to compress the response and writes its header.
// p is the start of the body, if any, which the Content-Type is sniffed
// from if the handler has not set one, as the ResponseWriter would do
// itself.
func (w *compressResponseWriter) decide(p []byte) {
	w.decided = true
	h := w.Header()
	if cmp.Or(w.code, http.StatusOK) == http.StatusOK && h.Get("Content-Encoding") == "" {
		if _, ok := h["Content-Type"]; !ok && len(p) > 0 {
			h.Set("Content-Type", http.DetectContentType(p))
		}
		if compressible(h.Get("Content-Type")) {
			h.Add("Vary", "Accept-Encoding")
			n, err := strconv.ParseInt(h.Get("Content-Length"), 10, 64)
			if w.coding >= 0 && (err != nil || n >= compressMinSize) {
				h.Set("Content-Encoding", encoders[w.coding].coding)
				h.Del("Content-Length")
				// Ranges are of the uncompressed body.
				h.Del("Accept-Ranges")
				// A strong ETag names the uncompressed bytes.
				if etag := h.Get("ETag"); strings.HasPrefix(etag, `"`) {
					h.Set("ETag", "W/"+etag)
				}
				w.enc = encoders[w.coding].pool.Get().(encoder)
				w.enc.Reset(w.ResponseWriter)
			}
		}
	}
	if w.code != 0 {
		w.ResponseWriter.WriteHeader(w.code)
	}
}

// close finishes the response once the handler has returned.
func (w *compressResponseWriter) close() {
	if !w.decided {
		// Nothing was written: there is no body to compress.
		w.decided = true
		if w.code != 0 {
			w.ResponseWriter.WriteHeader(w.code)
		}
		return
	}
	if w.enc != nil {
		w.enc.Close()
		w.enc.Reset(io.Discard)
		encoders[w.coding].pool.Put(w.enc)
		w.enc = nil
	}
}
