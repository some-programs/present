package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// fontsDir holds the TrueType fonts that fontsTarHandler serves.
const fontsDir = "static/fonts/ttf"

// fontLicenses are the license files of the fonts in fontsDir.
var fontLicenses = []string{
	"static/fonts/OFL-Roboto.txt",
	"static/fonts/OFL-JetBrainsMono.txt",
}

// fontsTarHandler serves a tar archive of the TrueType fonts in fsys and
// their licenses, for installing the fonts that presentations use, such
// as for diagrams to render with them. The archive holds a single
// directory, fonts.
func fontsTarHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", `attachment; filename="fonts.tar"`)
		if err := writeFontsTar(w, fsys); err != nil {
			log.Printf("fonts.tar: %v", err)
			// Abort the response so that the client does not take a
			// truncated archive for a complete one.
			panic(http.ErrAbortHandler)
		}
	})
}

// writeFontsTar writes the tar archive that fontsTarHandler serves to w.
func writeFontsTar(w io.Writer, fsys fs.FS) error {
	tw := tar.NewWriter(w)
	// The embedded files are read-only and carry no modification time,
	// so the archive sets the modes and time they are extracted with.
	// Fontconfig rescans a font directory only when its time changes.
	now := time.Now()
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "fonts/",
		Mode:     0o755,
		ModTime:  now,
	}); err != nil {
		return err
	}
	addFile := func(name, file string) error {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(data)),
			ModTime:  now,
		}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	}
	for _, file := range fontLicenses {
		if err := addFile("fonts/"+path.Base(file), file); err != nil {
			return err
		}
	}
	err := fs.WalkDir(fsys, fontsDir, func(file string, d fs.DirEntry, err error) error {
		if err != nil || file == fontsDir {
			return err
		}
		name := "fonts/" + strings.TrimPrefix(file, fontsDir+"/")
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     name + "/",
				Mode:     0o755,
				ModTime:  now,
			})
		}
		return addFile(name, file)
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// fontsCSS is the style sheet that declares the fonts of the slides.
const fontsCSS = "static/fonts.css"

// A fontFace is an @font-face rule of fontsCSS.
type fontFace struct {
	family string
	italic bool
	// ranges are the characters the face covers, from its unicode-range,
	// or nil for all characters.
	ranges [][2]rune
	// rule is the rule, with its font file embedded as a data URL.
	rule string
}

var (
	fontFaceRE = regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	fontURLRE  = regexp.MustCompile(`url\(\s*['"]?([^'")]*)['"]?\s*\)`)
)

// loadFontFaces returns the @font-face rules of fontsCSS in fsys, each with
// its font file embedded, so that the rules work in documents that may not
// load other files.
func loadFontFaces(fsys fs.FS) ([]fontFace, error) {
	css, err := fs.ReadFile(fsys, fontsCSS)
	if err != nil {
		return nil, err
	}
	var faces []fontFace
	for _, m := range fontFaceRE.FindAllSubmatch(css, -1) {
		var face fontFace
		var descs []string
		for d := range strings.SplitSeq(string(m[1]), ";") {
			name, value, ok := strings.Cut(d, ":")
			if !ok {
				continue
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			switch name {
			case "font-family":
				face.family = strings.Trim(value, `'"`)
			case "font-style":
				face.italic = value == "italic" || strings.HasPrefix(value, "oblique")
			case "unicode-range":
				face.ranges, err = parseUnicodeRange(value)
				if err != nil {
					return nil, fmt.Errorf("%s: %v", fontsCSS, err)
				}
			case "src":
				value, err = embedFontURLs(fsys, value)
				if err != nil {
					return nil, err
				}
			}
			descs = append(descs, name+":"+value)
		}
		face.rule = "@font-face{" + strings.Join(descs, ";") + "}"
		faces = append(faces, face)
	}
	return faces, nil
}

// embedFontURLs replaces the URLs of font files relative to fontsCSS in
// src, the value of a src descriptor, with data URLs of the files in fsys.
func embedFontURLs(fsys fs.FS, src string) (string, error) {
	var err error
	src = fontURLRE.ReplaceAllStringFunc(src, func(u string) string {
		ref := fontURLRE.FindStringSubmatch(u)[1]
		if err != nil || strings.Contains(ref, ":") {
			return u
		}
		var data []byte
		data, err = fs.ReadFile(fsys, path.Join(path.Dir(fontsCSS), ref))
		mime := "font/" + strings.TrimPrefix(path.Ext(ref), ".")
		return "url(data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data) + ")"
	})
	return src, err
}

// parseUnicodeRange parses the value of a unicode-range descriptor, such
// as "U+0000-00FF, U+0131, U+02??".
func parseUnicodeRange(s string) ([][2]rune, error) {
	var ranges [][2]rune
	for r := range strings.SplitSeq(s, ",") {
		r = strings.TrimSpace(r)
		hex, ok := strings.CutPrefix(strings.ToUpper(r), "U+")
		if !ok {
			return nil, fmt.Errorf("bad unicode-range %q", r)
		}
		lo, hi, ok := strings.Cut(hex, "-")
		if !ok {
			// A ? stands for any hex digit.
			lo, hi = strings.ReplaceAll(hex, "?", "0"), strings.ReplaceAll(hex, "?", "F")
		}
		l, err1 := strconv.ParseUint(lo, 16, 32)
		h, err2 := strconv.ParseUint(hi, 16, 32)
		if err1 != nil || err2 != nil || l > h {
			return nil, fmt.Errorf("bad unicode-range %q", r)
		}
		ranges = append(ranges, [2]rune{rune(l), rune(h)})
	}
	return ranges, nil
}

// covers reports whether the face covers one of the characters in chars.
func (f fontFace) covers(chars map[rune]bool) bool {
	if f.ranges == nil {
		return true
	}
	for c := range chars {
		for _, r := range f.ranges {
			if r[0] <= c && c <= r[1] {
				return true
			}
		}
	}
	return false
}

// embedSVGFonts returns svg with a style element for the faces it may use,
// added to the start of its root element. An SVG shown as an image, by an
// img element or a CSS background, cannot use the fonts of the page or load
// font files, but it can use fonts embedded in it.
//
// A face is embedded when svg names its family, covers one of the
// characters in svg other than control characters, and is upright, or italic and svg has italic or
// emphasized text. svg is returned unchanged if no face is embedded, or if
// its root is not an svg element.
func embedSVGFonts(svg []byte, faces []fontFace) []byte {
	d := xml.NewDecoder(bytes.NewReader(svg))
	var root xml.StartElement
	for {
		tok, err := d.RawToken()
		if err != nil {
			return svg
		}
		if se, ok := tok.(xml.StartElement); ok {
			root = se
			break
		}
	}
	end := int(d.InputOffset())
	if root.Name.Space != "" || root.Name.Local != "svg" || bytes.HasSuffix(svg[:end], []byte("/>")) {
		return svg
	}

	// Control characters such as newlines are not drawn, but some faces,
	// such as Roboto's symbols, list them in their unicode-range.
	chars := map[rune]bool{}
	for _, c := range string(svg) {
		if !unicode.IsControl(c) {
			chars[c] = true
		}
	}
	italic := bytes.Contains(svg, []byte("italic")) || bytes.Contains(svg, []byte("<em>")) || bytes.Contains(svg, []byte("<i>"))
	var rules []string
	for _, f := range faces {
		if (!f.italic || italic) && bytes.Contains(svg, []byte(f.family)) && f.covers(chars) {
			rules = append(rules, f.rule)
		}
	}
	if rules == nil {
		return svg
	}
	style := "<style>" + strings.Join(rules, "") + "</style>"
	return slices.Concat(svg[:end], []byte(style), svg[end:])
}

// svgFontFaces are the faces that serveSVG embeds in SVG files.
var svgFontFaces []fontFace

// serveSVG serves the SVG file name with the fonts it uses embedded, as
// embedSVGFonts does. It reports false, having written nothing, if name
// cannot be read as a file.
func serveSVG(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	svg, err := io.ReadAll(f)
	if err != nil {
		return false
	}
	http.ServeContent(w, r, name, fi.ModTime(), bytes.NewReader(embedSVGFonts(svg, svgFontFaces)))
	return true
}
