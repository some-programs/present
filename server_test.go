package main

import (
	"bytes"
	"net"
	"os"
	"strings"
	"testing"
)

func TestSourceFileName(t *testing.T) {
	s := &server{doc: true}
	for urlPath, want := range map[string]string{
		"/_examples/media.slide":   "examples/media.slide",
		"/talks/go/intro.slide":    "talks/go/intro.slide",
		"/_examplesx/a.slide":      "_examplesx/a.slide",
		"/_examples/../mine.slide": "mine.slide",
	} {
		if got := sourceFileName(s.file(urlPath)); got != want {
			t.Errorf("sourceFileName of %s = %q, want %q", urlPath, got, want)
		}
	}
}

func TestOriginURL(t *testing.T) {
	hostname, _ := os.Hostname()
	local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321}
	anyAddr := &net.TCPAddr{IP: net.IPv4zero, Port: 4321}
	for _, tt := range []struct {
		addr                 *net.TCPAddr
		httpAddr, originHost string
		want                 string
	}{
		{local, "127.0.0.1:4321", "", "http://127.0.0.1:4321"},
		{local, "localhost:4321", "", "http://localhost:4321"},
		{local, "127.0.0.1:0", "", "http://127.0.0.1:4321"},
		{anyAddr, "0.0.0.0:4321", "", "http://" + net.JoinHostPort(hostname, "4321")},
		{local, "127.0.0.1:4321", "example.com", "http://example.com:4321"},
		{local, "127.0.0.1:4321", "http://example.com", "http://example.com:4321"},
		{local, "127.0.0.1:4321", "https://example.com", "https://example.com:4321"},
	} {
		got, err := originURL(tt.addr, tt.httpAddr, tt.originHost)
		if err != nil || got.String() != tt.want {
			t.Errorf("originURL(%v, %q, %q) = %v, %v; want %s", tt.addr, tt.httpAddr, tt.originHost, got, err, tt.want)
		}
	}
}

func TestRenderDocSource(t *testing.T) {
	pr, err := newRenderer(embedFS, func(path string) string { return path }, pageSettings{})
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []bool{false, true} {
		s := &server{renderer: pr, doc: doc}
		var buf bytes.Buffer
		if err := s.renderDoc(&buf, examplesRoot, "intro.slide", "/_examples/intro.slide"); err != nil {
			t.Fatal(err)
		}
		page := buf.String()
		if got := strings.Contains(page, `<aside id="source"`); got != doc {
			t.Errorf("with -doc=%v, page has source panel: %v", doc, got)
		}
		if !doc {
			continue
		}
		for _, want := range []string{
			`<div class="src-name">examples/intro.slide</div>`,
			// Lines are numbered as in the presentation file.
			`<span class="src-n">131</span><span class="src-heading">## Links and HTML` + "\n" + `</span>`,
			`<span class="src-n">135</span><span class="src-command">.html html/badge.html` + "\n" + `</span>`,
			// The included HTML is shown as text, numbered from 1.
			`<pre class="src-lines"><span class="src-n">1</span><span>&lt;p style=`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("page does not contain %q", want)
			}
		}
	}
}
