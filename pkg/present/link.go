// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register("link", parseLink)
}

type Link struct {
	Cmd   string // original command from present source
	URL   *url.URL
	Label string
}

func (l Link) TemplateName() string { return "link" }

func parseLink(ctx *Context, fileName string, lineno int, text string) (Elem, error) {
	args := strings.Fields(text)
	if len(args) < 2 {
		return nil, fmt.Errorf("%s:%d: .link expects a URL", fileName, lineno)
	}
	url, err := url.Parse(args[1])
	if err != nil {
		return nil, fmt.Errorf("%s:%d: %v", fileName, lineno, err)
	}
	label := ""
	if len(args) > 2 {
		label = strings.Join(args[2:], " ")
	} else {
		scheme := url.Scheme + "://"
		if url.Scheme == "mailto" {
			scheme = "mailto:"
		}
		label = strings.Replace(url.String(), scheme, "", 1)
	}
	return Link{text, url, label}, nil
}
