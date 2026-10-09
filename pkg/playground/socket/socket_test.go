// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"os/exec"
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
