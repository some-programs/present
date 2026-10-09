// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"strings"
)

func init() {
	Register("video", parseVideo)
}

type Video struct {
	Cmd        string // original command from present source
	URL        string
	SourceType string
	Width      int
	Height     int
}

func (v Video) TemplateName() string { return "video" }

func parseVideo(ctx *Context, fileName string, lineno int, text string) (Elem, error) {
	args := strings.Fields(text)
	if len(args) < 3 {
		return nil, fmt.Errorf("incorrect video invocation: %q", text)
	}
	vid := Video{Cmd: text, URL: args[1], SourceType: args[2]}
	switch len(args) {
	case 3:
		// no size parameters
	case 5:
		var err error
		if vid.Height, vid.Width, err = parseDims(args[3], args[4]); err != nil {
			return nil, fmt.Errorf("%s:%d: incorrect video invocation: %q: %v", fileName, lineno, text, err)
		}
	default:
		return nil, fmt.Errorf("incorrect video invocation: %q", text)
	}
	return vid, nil
}
