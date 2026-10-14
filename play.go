// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"net/url"

	"github.com/some-programs/present/pkg/playground/socket"
)

// initPlayground registers the WebSocket handler that runs programs for
// static/js/play.js, if the playground is enabled.
func initPlayground(origin *url.URL) {
	if !*play {
		return
	}
	http.Handle("/socket", socket.NewHandler(origin))
}
