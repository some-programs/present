package main

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestFontsTar(t *testing.T) {
	srv := httptest.NewServer(fontsTarHandler(embedFS))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got, want := resp.Header.Get("Content-Type"), "application/x-tar"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	var names []string
	tr := tar.NewReader(resp.Body)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(data)) != hdr.Size {
			t.Errorf("%s: read %d bytes, header says %d", hdr.Name, len(data), hdr.Size)
		}
	}

	for _, want := range []string{
		"fonts/",
		"fonts/OFL-Roboto.txt",
		"fonts/OFL-JetBrainsMono.txt",
		"fonts/Roboto/",
		"fonts/Roboto/Roboto-Regular.ttf",
		"fonts/JetBrainsMono/",
		"fonts/JetBrainsMono/JetBrainsMono-Regular.ttf",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("archive lacks %s", want)
		}
	}
	ttfs, err := fs.Glob(embedFS, fontsDir+"/*/*.ttf")
	if err != nil {
		t.Fatal(err)
	}
	// The directories are fonts/ and one for each family.
	if got, want := len(names), len(fontLicenses)+len(ttfs)+3; got != want {
		t.Errorf("archive has %d entries, want %d: %q", got, want, names)
	}
}
