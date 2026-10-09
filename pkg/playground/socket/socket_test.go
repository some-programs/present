// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuffer(t *testing.T) {
	afterChan := make(chan time.Time)
	ch := make(chan *Message)
	go func() {
		ch <- &Message{Kind: "err", Body: "a"}
		ch <- &Message{Kind: "err", Body: "b"}
		ch <- &Message{Kind: "out", Body: "1"}
		ch <- &Message{Kind: "out", Body: "2"}
		afterChan <- time.Time{} // value itself doesn't matter
		ch <- &Message{Kind: "out", Body: "3"}
		ch <- &Message{Kind: "out", Body: "4"}
		close(ch)
	}()

	var ms []*Message
	timeAfter := func(d time.Duration) <-chan time.Time {
		return afterChan
	}
	for m := range buffer(ch, timeAfter) {
		ms = append(ms, m)
	}
	if len(ms) != 3 {
		t.Fatalf("got %v messages, want 3", len(ms))
	}
	if g, w := ms[0].Body, "ab"; g != w {
		t.Errorf("message 0 body = %q, want %q", g, w)
	}
	if g, w := ms[1].Body, "12"; g != w {
		t.Errorf("message 1 body = %q, want %q", g, w)
	}
	if g, w := ms[2].Body, "34"; g != w {
		t.Errorf("message 2 body = %q, want %q", g, w)
	}
}

type killRecorder chan struct{}

func (k killRecorder) Kill() { close(k) }

func TestLimiter(t *testing.T) {
	ch := make(chan *Message)
	go func() {
		var m Message
		for range msgLimit + 10 {
			ch <- &m
		}
		ch <- &Message{Kind: "end"}
	}()

	kr := make(killRecorder)
	n := 0
	for m := range limiter(ch, kr) {
		n++
		if n > msgLimit && m.Kind != "end" {
			t.Errorf("received non-end message after limit")
		}
	}
	if n != msgLimit+1 {
		t.Errorf("received %v messages, want %v", n, msgLimit+1)
	}
	<-kr
}

func TestStartProcess(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not found")
	}
	for _, tt := range []struct {
		name, body, want string
	}{{
		name: "plain",
		body: `package main

import "fmt"

func main() {
	for i := range 3 {
		fmt.Print(i)
	}
}
`,
		want: "012",
	}, {
		name: "txtar",
		body: `package main

import "fmt"

func main() { fmt.Print(msg) }
-- msg.go --
package main

const msg = "hello"
`,
		want: "hello",
	}, {
		name: "go.mod",
		body: `package main

import "fmt"

func main() { fmt.Print("mod") }
-- go.mod --
module example.com/m
`,
		want: "mod",
	}, {
		name: "txtar without comment",
		body: `
-- go.mod --
module example.com/m
-- main.go --
package main

import (
	"fmt"

	"example.com/m/msg"
)

func main() { fmt.Print(msg.Hello) }
-- msg/msg.go --
package msg

const Hello = "hello"
`,
		want: "hello",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			dest := make(chan *Message, 100)
			startProcess("id", tt.body, dest, nil)
			var stdout, stderr strings.Builder
			for m := range dest {
				switch m.Kind {
				case "stdout":
					stdout.WriteString(m.Body)
				case "stderr":
					stderr.WriteString(m.Body)
				}
				if m.Kind == "end" {
					if m.Body != "" {
						t.Fatalf("end: %s\nstderr: %s", m.Body, stderr.String())
					}
					break
				}
			}
			if got := stdout.String(); got != tt.want {
				t.Errorf("stdout = %q, want %q (stderr: %q)", got, tt.want, stderr.String())
			}
		})
	}
}

func TestStartScript(t *testing.T) {
	for _, tt := range []struct {
		name, ext, body, want, wantErr string
		noTSC                          bool // run without tsc in PATH
	}{{
		name:    "python",
		ext:     ".py",
		body:    "import sys\nprint('hello')\nsys.exit(1 / 0)\n",
		want:    "hello\n",
		wantErr: "line 3",
	}, {
		name:    "javascript",
		ext:     ".js",
		body:    "import { EOL } from 'node:os';\nprocess.stdout.write('hello' + EOL);\nthrow new Error('boom');\n",
		want:    "hello\n",
		wantErr: "prog.js:3",
	}, {
		name:    "typescript",
		ext:     ".ts",
		body:    "enum E { A, B }\nconsole.log('hello', E.B);\nthrow new Error('boom');\n",
		want:    "hello 1\n",
		wantErr: "prog.ts:3",
	}, {
		name:    "typescript type error",
		ext:     ".ts",
		body:    "console.log('hello');\nconst s: string = 1;\n",
		wantErr: "prog.ts(2,7): error TS2322",
	}, {
		name:    "typescript without tsc",
		ext:     ".ts",
		body:    "const s: string = 'hello';\nconsole.log(s);\nthrow new Error('boom');\n",
		want:    "hello\n",
		wantErr: "prog.ts:3",
		noTSC:   true,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			opt := &Options{Ext: tt.ext}
			interp, err := exec.LookPath(interpreter(opt)[0])
			if err != nil {
				t.Skip(err)
			}
			switch {
			case tt.noTSC:
				// Leave only the interpreter in PATH.
				dir := t.TempDir()
				if err := os.Symlink(interp, filepath.Join(dir, filepath.Base(interp))); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
			case tt.ext == ".ts" && !hasTSC():
				t.Skip("tsc not found")
			}
			dest := make(chan *Message, 100)
			startProcess("id", tt.body, dest, opt)
			var stdout, stderr strings.Builder
			var end string
			for m := range dest {
				switch m.Kind {
				case "stdout":
					stdout.WriteString(m.Body)
				case "stderr":
					stderr.WriteString(m.Body)
				}
				if m.Kind == "end" {
					end = m.Body
					break
				}
			}
			if got := stdout.String(); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
			if end == "" {
				t.Errorf("end: no error, want one")
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}
