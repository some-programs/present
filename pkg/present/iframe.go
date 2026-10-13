// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"strings"
)

func init() {
	Register("iframe", parseIframe)
}

type Iframe struct {
	Cmd    string // original command from present source
	URL    string
	Width  int
	Height int
}

func (i Iframe) TemplateName() string { return "iframe" }

func parseIframe(ctx *Context, fileName string, lineno int, text string) (Elem, error) {
	args := strings.Fields(text)
	if len(args) < 2 {
		return nil, fmt.Errorf("%s:%d: incorrect iframe invocation: %q: want a URL, and optionally a height and a width", fileName, lineno, text)
	}
	i := Iframe{Cmd: text, URL: args[1]}
	switch len(args) {
	case 2:
		// no size parameters
	case 4:
		var err error
		if i.Height, i.Width, err = parseDims(args[2], args[3]); err != nil {
			return nil, fmt.Errorf("%s:%d: incorrect iframe invocation: %q: %v", fileName, lineno, text, err)
		}
	default:
		return nil, fmt.Errorf("%s:%d: incorrect iframe invocation: %q: want a URL, and optionally a height and a width", fileName, lineno, text)
	}
	return i, nil
}
