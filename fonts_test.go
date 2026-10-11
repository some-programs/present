package main

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestLoadFontFaces(t *testing.T) {
	faces, err := loadFontFaces(embedFS)
	if err != nil {
		t.Fatal(err)
	}
	woff2s, err := fs.Glob(embedFS, "static/fonts/*.woff2")
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(woff2s) {
		t.Errorf("got %d faces, want one for each of the %d WOFF2 files", len(faces), len(woff2s))
	}
	for _, f := range faces {
		if f.family != "Roboto" && f.family != "JetBrains Mono" {
			t.Errorf("face of family %q", f.family)
		}
		if f.ranges == nil {
			t.Errorf("%s face without unicode-range", f.family)
		}
		if !strings.Contains(f.rule, "src:url(data:font/woff2;base64,") || strings.Contains(f.rule, "fonts/") {
			t.Errorf("%s rule does not embed its font file: %.200s", f.family, f.rule)
		}
	}
}

func TestParseUnicodeRange(t *testing.T) {
	got, err := parseUnicodeRange("U+0000-00FF, U+0131, u+02??")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]rune{{0, 0xff}, {0x131, 0x131}, {0x200, 0x2ff}}
	if !slices.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
	for _, bad := range []string{"0131", "U+", "U+00FF-0000", "U+XYZ"} {
		if _, err := parseUnicodeRange(bad); err == nil {
			t.Errorf("parseUnicodeRange(%q) succeeded", bad)
		}
	}
}

func TestEmbedSVGFonts(t *testing.T) {
	faces := []fontFace{
		{family: "Roboto", ranges: [][2]rune{{0, 0xff}}, rule: "[roboto latin]"},
		{family: "Roboto", ranges: [][2]rune{{0x400, 0x4ff}}, rule: "[roboto cyrillic]"},
		{family: "Roboto", italic: true, ranges: [][2]rune{{0, 0xff}}, rule: "[roboto italic latin]"},
		{family: "Roboto", ranges: [][2]rune{{1, 0x1f}}, rule: "[roboto controls]"},
		{family: "JetBrains Mono", rule: "[mono]"},
	}
	for _, tt := range []struct {
		svg, want string
	}{
		{
			`<svg font-family="Roboto"><text>a</text></svg>`,
			`<svg font-family="Roboto"><style>[roboto latin]</style><text>a</text></svg>`,
		},
		{
			"<?xml version=\"1.0\"?>\n<!DOCTYPE svg>\n<!-- a > b -->\n<svg style=\"font-family: Roboto\">\n<text>Привет</text></svg>",
			"<?xml version=\"1.0\"?>\n<!DOCTYPE svg>\n<!-- a > b -->\n<svg style=\"font-family: Roboto\"><style>[roboto latin][roboto cyrillic]</style>\n<text>Привет</text></svg>",
		},
		{
			`<svg><text font-family="Roboto" font-style="italic">a</text></svg>`,
			`<svg><style>[roboto latin][roboto italic latin]</style><text font-family="Roboto" font-style="italic">a</text></svg>`,
		},
		{
			`<svg><style>.c{font-family:JetBrains Mono}</style><text><em>a</em></text></svg>`,
			`<svg><style>[mono]</style><style>.c{font-family:JetBrains Mono}</style><text><em>a</em></text></svg>`,
		},
		// No embedded family.
		{`<svg><text font-family="sans-serif">a</text></svg>`, ""},
		// Not an SVG document.
		{`<html><p style="font-family: Roboto">a</p></html>`, ""},
		{`<svg:svg xmlns:svg="http://www.w3.org/2000/svg" font-family="Roboto"/>`, ""},
		{`<svg font-family="Roboto"/>`, ""},
		{`Roboto`, ""},
	} {
		want := tt.want
		if want == "" {
			want = tt.svg
		}
		if got := string(embedSVGFonts([]byte(tt.svg), faces)); got != want {
			t.Errorf("embedSVGFonts(%q)\n got %q\nwant %q", tt.svg, got, want)
		}
	}
}

func TestServeSVG(t *testing.T) {
	faces, err := loadFontFaces(embedFS)
	if err != nil {
		t.Fatal(err)
	}
	defer func(old []fontFace) { svgFontFaces = old }(svgFontFaces)
	svgFontFaces = faces

	dir := t.TempDir()
	name := filepath.Join(dir, "g.svg")
	if err := os.WriteFile(name, []byte(`<svg font-family="Roboto"><text>a</text></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if !serveSVG(w, httptest.NewRequest("GET", "/g.svg", nil), name) {
		t.Fatal("serveSVG reported that it did not serve g.svg")
	}
	if got, want := w.Header().Get("Content-Type"), "image/svg+xml"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got := w.Body.String(); !strings.HasPrefix(got, `<svg font-family="Roboto"><style>@font-face{font-family:'Roboto';`) {
		t.Errorf("body does not start with an embedded Roboto: %.200s", got)
	}

	for _, name := range []string{filepath.Join(dir, "missing.svg"), dir} {
		w := httptest.NewRecorder()
		if serveSVG(w, httptest.NewRequest("GET", "/x.svg", nil), name) || w.Body.Len() > 0 {
			t.Errorf("serveSVG of %s served it", name)
		}
	}
}
