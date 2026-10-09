// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func init() {
	Register("fontsize", parseFontSize)
}

// FontSize sets the font size of the elements that follow it in a section,
// up to the next FontSize or the end of the section. The elems template
// function applies it.
type FontSize struct {
	Cmd string // original command from present source

	// Size is the CSS font-size. Sizes relative to the default, and the
	// default itself, use the CSS variable --font-size-default, so that
	// they do not depend on a FontSize of an enclosing section.
	Size string
}

func (f FontSize) TemplateName() string { return "fontsize" }

// fontSizeRE matches the sizes .fontsize accepts: a factor of the default
// size, a percentage of it, or a size in px or pt.
var fontSizeRE = regexp.MustCompile(`^(\d+(?:\.\d+)?|\.\d+)(%|px|pt)?$`)

func parseFontSize(_ *Context, fileName string, lineno int, text string) (Elem, error) {
	args := strings.Fields(text)
	switch len(args) {
	case 1:
		return FontSize{Cmd: text, Size: "var(--font-size-default)"}, nil
	case 2:
		m := fontSizeRE.FindStringSubmatch(args[1])
		if m == nil {
			return nil, fmt.Errorf("%s:%d: invalid .fontsize size %q: want a factor such as 0.8, a percentage, or a size in px or pt", fileName, lineno, args[1])
		}
		size := args[1]
		if unit := m[2]; unit == "" || unit == "%" {
			factor, _ := strconv.ParseFloat(m[1], 64)
			if unit == "%" {
				factor /= 100
			}
			size = fmt.Sprintf("calc(%s * var(--font-size-default))", strconv.FormatFloat(factor, 'g', -1, 64))
		}
		return FontSize{Cmd: text, Size: size}, nil
	}
	return nil, fmt.Errorf("%s:%d: .fontsize takes at most one size", fileName, lineno)
}
