// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/some-programs/present/pkg/playground/socket"
	"github.com/some-programs/present/pkg/present"
)

var scripts = []string{"jquery.js", "jquery-ui.js", "playground.js", "play.js"}

// playScript registers an HTTP handler at /play.js that serves all the
// scripts specified by the variable above, and appends a line that
// initializes the playground with the WebSocket transport.
func playScript(fsys fs.FS) {
	modTime := time.Now()
	var buf bytes.Buffer
	for _, p := range scripts {
		b, err := fs.ReadFile(fsys, "static/"+p)
		if err != nil {
			panic(err)
		}
		buf.Write(b)
	}
	buf.WriteString("\ninitPlayground(new SocketTransport());\n")
	b := buf.Bytes()
	http.HandleFunc("/play.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/javascript")
		http.ServeContent(w, r, "", modTime, bytes.NewReader(b))
	})
}

func initPlayground(fsys fs.FS, origin *url.URL) {
	if !present.PlayEnabled {
		return
	}
	playScript(fsys)
	http.Handle("/socket", socket.NewHandler(origin))
}

func playable(c present.Code) bool {
	return present.PlayEnabled && c.Play
}
