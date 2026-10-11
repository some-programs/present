// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func init() {
	Register("image", parseImage)
}

// An Image is an image from a .image command:
//
//	.image [-background-color color] file [size | height width]
//
// Each size is a number of pixels, a fraction from 0.0 to 1.0 of the
// slide's content area, or _ for none. A single size is a fraction of both
// the width and the height. Slides fit each image into the space left for
// it, keeping its aspect ratio; with no size it fills that space.
//
// A diagram file, Graphviz (.dot) or Mermaid (.mmd, .mermaid), is shown as
// the SVG file next to it with the same name, which the program serving the
// slides keeps up to date. The -background-color flag sets the background
// of a Mermaid diagram, which is white otherwise; a Graphviz diagram sets
// its own with the bgcolor attribute.
type Image struct {
	Cmd    string // original command from present source
	URL    string
	Width  int
	Height int

	WidthFrac  float64
	HeightFrac float64

	// Source is the diagram file that URL is made from, if any.
	Source string
	// BackgroundColor is the background of the rendering of Source, a
	// CSS color, if not the default.
	BackgroundColor string

	// Error says why URL could not be made from Source. Missing reports
	// that there is no earlier URL to show instead.
	Error   string
	Missing bool
}

// diagramExts are the extensions of the diagram files that an image can be
// made from.
var diagramExts = []string{".dot", ".mmd", ".mermaid"}

// mermaidExts are the extensions of the diagram files rendered by Mermaid.
var mermaidExts = []string{".mmd", ".mermaid"}

// cssColor matches the CSS colors that -background-color accepts, such as
// transparent, #f8f8f8 and rgb(0,0,0,0.5), written without spaces.
var cssColor = regexp.MustCompile(`^(#[0-9A-Fa-f]+|[A-Za-z]+(\([0-9.,%]+\))?)$`)

func (i Image) TemplateName() string { return "image" }

func parseImage(_ *Context, fileName string, lineno int, text string) (Elem, error) {
	args := strings.Fields(text)
	img := Image{Cmd: text}
	// Flags come before the file name.
	for len(args) > 1 && strings.HasPrefix(args[1], "-") {
		switch args[1] {
		case "-background-color":
			if len(args) < 3 || !cssColor.MatchString(args[2]) {
				return nil, fmt.Errorf("%s:%d: incorrect image invocation: %q: -background-color needs a CSS color, such as transparent", fileName, lineno, text)
			}
			img.BackgroundColor = args[2]
			args = slices.Delete(args, 1, 3)
		default:
			return nil, fmt.Errorf("%s:%d: incorrect image invocation: %q: unknown flag %s", fileName, lineno, text, args[1])
		}
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("incorrect image invocation: %q", text)
	}
	img.URL = args[1]
	ext := path.Ext(img.URL)
	if slices.Contains(diagramExts, ext) && !strings.Contains(img.URL, "://") {
		img.Source = img.URL
		img.URL = strings.TrimSuffix(img.URL, ext) + ".svg"
	}
	if img.BackgroundColor != "" && (img.Source == "" || !slices.Contains(mermaidExts, ext)) {
		return nil, fmt.Errorf("%s:%d: incorrect image invocation: %q: -background-color is only for Mermaid diagrams (.mmd, .mermaid)", fileName, lineno, text)
	}
	var err error
	switch len(args) {
	case 2:
		// no size parameters
	case 3:
		img.HeightFrac, err = parseFrac(args[2])
		img.WidthFrac = img.HeightFrac
	case 4:
		// If a parameter is empty (underscore), leave the field set to
		// zero. The "image" action template will then omit that img tag
		// attribute and the browser will calculate the value to preserve
		// the aspect ratio.
		img.Height, img.HeightFrac, err = parseSize(args[2])
		if err == nil {
			img.Width, img.WidthFrac, err = parseSize(args[3])
		}
	default:
		err = fmt.Errorf("too many arguments")
	}
	if err != nil {
		return nil, fmt.Errorf("%s:%d: incorrect image invocation: %q: %v", fileName, lineno, text, err)
	}
	return img, nil
}

// parseSize parses an image size: a number of pixels, a fraction, or _.
func parseSize(s string) (pixels int, frac float64, err error) {
	if s == "_" {
		return 0, 0, nil
	}
	if strings.Contains(s, ".") {
		frac, err = parseFrac(s)
		return 0, frac, err
	}
	pixels, err = strconv.Atoi(s)
	if err != nil || pixels <= 0 {
		return 0, 0, fmt.Errorf("bad size %q", s)
	}
	return pixels, 0, nil
}

// parseFrac parses a fraction greater than 0 and at most 1.
func parseFrac(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || !strings.Contains(s, ".") || f <= 0 || f > 1 {
		return 0, fmt.Errorf("bad fraction %q: want a number from 0.0 to 1.0", s)
	}
	return f, nil
}
