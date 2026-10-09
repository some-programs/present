// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socket implements a WebSocket-based playground backend.
// Clients connect to a websocket handler and send run/kill commands, and
// the server sends the output and exit status of the running processes.
// Multiple clients running multiple processes may be served concurrently.
// The wire format is JSON and is described by the Message type.
package socket // import "github.com/some-programs/present/pkg/playground/socket"

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/some-programs/present/pkg/txtar"
	"golang.org/x/net/websocket"
)

const (
	// The maximum number of messages to send per session (avoid flooding).
	msgLimit = 1000

	// Batch messages sent in this interval and send as a single message.
	msgDelay = 10 * time.Millisecond
)

// Message is the wire format for the websocket connection to the browser.
// It is used for both sending output messages and receiving commands, as
// distinguished by the Kind field.
type Message struct {
	Id      string // client-provided unique id for the process
	Kind    string // in: "run", "kill" out: "stdout", "stderr", "end"
	Body    string
	Options *Options `json:",omitempty"`
}

// Options specify additional message options.
type Options struct {
	Race bool   // use -race flag when building code (for "run" only)
	Ext  string // extension of the program's file, such as ".py"
}

// NewHandler returns a websocket server which checks the origin of requests.
func NewHandler(origin *url.URL) websocket.Server {
	return websocket.Server{
		Origin:    origin,
		Handshake: handshake,
		Handler:   websocket.Handler(socketHandler),
	}
}

// handshake checks the origin of a request during the websocket handshake.
func handshake(c *websocket.Config, req *http.Request) error {
	o, err := websocket.Origin(c, req)
	if err != nil {
		log.Println("bad websocket origin:", err)
		return websocket.ErrBadWebSocketOrigin
	}
	_, port, err := net.SplitHostPort(c.Origin.Host)
	if err != nil {
		log.Println("bad websocket origin:", err)
		return websocket.ErrBadWebSocketOrigin
	}
	ok := c.Origin.Scheme == o.Scheme && (c.Origin.Host == o.Host || c.Origin.Host == net.JoinHostPort(o.Host, port))
	if !ok {
		log.Println("bad websocket origin:", o)
		return websocket.ErrBadWebSocketOrigin
	}
	log.Println("accepting connection from:", req.RemoteAddr)
	return nil
}

// socketHandler handles the websocket connection for a given present session.
// It handles transcoding Messages to and from JSON format, and starting
// and killing processes.
func socketHandler(c *websocket.Conn) {
	in, out := make(chan *Message), make(chan *Message)
	errc := make(chan error, 1)

	// Decode messages from client and send to the in channel.
	go func() {
		dec := json.NewDecoder(c)
		for {
			var m Message
			if err := dec.Decode(&m); err != nil {
				errc <- err
				return
			}
			in <- &m
		}
	}()

	// Receive messages from the out channel and encode to the client.
	go func() {
		enc := json.NewEncoder(c)
		for m := range out {
			if err := enc.Encode(m); err != nil {
				errc <- err
				return
			}
		}
	}()
	defer close(out)

	// Start and kill processes and handle errors.
	proc := make(map[string]*process)
	for {
		select {
		case m := <-in:
			switch m.Kind {
			case "run":
				log.Println("running snippet from:", c.Request().RemoteAddr)
				proc[m.Id].Kill()
				proc[m.Id] = startProcess(m.Id, m.Body, out, m.Options)
			case "kill":
				proc[m.Id].Kill()
			}
		case err := <-errc:
			if !errors.Is(err, io.EOF) {
				// A encode or decode has failed; bail.
				log.Println(err)
			}
			// Shut down any running processes.
			for _, p := range proc {
				p.Kill()
			}
			return
		}
	}
}

// process represents a running process.
type process struct {
	out  chan<- *Message
	done chan struct{} // closed when wait completes
	run  *exec.Cmd
	path string
}

// startProcess builds and runs the given program, sending its output
// and end event as Messages on the provided channel.
func startProcess(id, body string, dest chan<- *Message, opt *Options) *process {
	var (
		done = make(chan struct{})
		out  = make(chan *Message)
		p    = &process{out: out, done: done}
	)
	go func() {
		defer close(done)
		for m := range buffer(limiter(out, p), time.After) {
			m.Id = id
			dest <- m
		}
	}()
	var err error
	if opt != nil && opt.Ext == ".ts" && hasTSC() {
		err = p.startTypeScript(body)
	} else if interp := interpreter(opt); interp != nil {
		err = p.startScript(interp, opt.Ext, body)
	} else if path, args := shebang(body); path != "" {
		err = p.startProcess(path, args, body)
	} else {
		err = p.start(body, opt)
	}
	if err != nil {
		p.end(err)
		return nil
	}
	go func() {
		p.end(p.run.Wait())
	}()
	return p
}

// end sends an "end" message to the client, containing the process id and the
// given error value. It also removes the binary, if present.
func (p *process) end(err error) {
	if p.path != "" {
		defer os.RemoveAll(p.path)
	}
	m := &Message{Kind: "end"}
	if err != nil {
		m.Body = err.Error()
	}
	p.out <- m
	close(p.out)
}

// A killer provides a mechanism to terminate a process.
// The Kill method returns only once the process has exited.
type killer interface {
	Kill()
}

// limiter returns a channel that wraps the given channel.
// It receives Messages from the given channel and sends them to the returned
// channel until it passes msgLimit messages, at which point it will kill the
// process and pass only the "end" message.
// When the given channel is closed, or when the "end" message is received,
// it closes the returned channel.
func limiter(in <-chan *Message, p killer) <-chan *Message {
	out := make(chan *Message)
	go func() {
		defer close(out)
		n := 0
		for m := range in {
			switch {
			case n < msgLimit || m.Kind == "end":
				out <- m
				if m.Kind == "end" {
					return
				}
			case n == msgLimit:
				// Kill in a goroutine as Kill will not return
				// until the process' output has been
				// processed, and we're doing that in this loop.
				go p.Kill()
			default:
				continue // don't increment
			}
			n++
		}
	}()
	return out
}

// buffer returns a channel that wraps the given channel. It receives messages
// from the given channel and sends them to the returned channel.
// Message bodies are gathered over the period msgDelay and coalesced into a
// single Message before they are passed on. Messages of the same kind are
// coalesced; when a message of a different kind is received, any buffered
// messages are flushed. When the given channel is closed, buffer flushes the
// remaining buffered messages and closes the returned channel.
// The timeAfter func should be time.After. It exists for testing.
func buffer(in <-chan *Message, timeAfter func(time.Duration) <-chan time.Time) <-chan *Message {
	out := make(chan *Message)
	go func() {
		defer close(out)
		var (
			tc    <-chan time.Time
			buf   []byte
			kind  string
			flush = func() {
				if len(buf) == 0 {
					return
				}
				out <- &Message{Kind: kind, Body: safeString(buf)}
				buf = buf[:0] // recycle buffer
				kind = ""
			}
		)
		for {
			select {
			case m, ok := <-in:
				if !ok {
					flush()
					return
				}
				if m.Kind == "end" {
					flush()
					out <- m
					return
				}
				if kind != m.Kind {
					flush()
					kind = m.Kind
					if tc == nil {
						tc = timeAfter(msgDelay)
					}
				}
				buf = append(buf, m.Body...)
			case <-tc:
				flush()
				tc = nil
			}
		}
	}()
	return out
}

// Kill stops the process if it is running and waits for it to exit.
func (p *process) Kill() {
	if p == nil || p.run == nil {
		return
	}
	p.run.Process.Kill()
	<-p.done // block until process exits
}

// shebang looks for a shebang ('#!') at the beginning of the passed string.
// If found, it returns the path and args after the shebang.
// args includes the command as args[0].
func shebang(body string) (path string, args []string) {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "#!") {
		return "", nil
	}
	body, _, _ = strings.Cut(body, "\n")
	fs := strings.Fields(body[2:])
	return fs[0], fs
}

// startProcess starts a given program given its path and passing the given body
// to the command standard input.
func (p *process) startProcess(path string, args []string, body string) error {
	cmd := &exec.Cmd{
		Path:   path,
		Args:   args,
		Stdin:  strings.NewReader(body),
		Stdout: &messageWriter{kind: "stdout", out: p.out},
		Stderr: &messageWriter{kind: "stderr", out: p.out},
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.run = cmd
	return nil
}

// interpreters maps the extension of a program's file to the command that
// runs it, given the file as its last argument.
var interpreters = map[string][]string{
	".py": {"python3", "-u"}, // unbuffered, so that output streams
	".js": {"node"},
	".ts": {"node"}, // Node strips the types itself; see startTypeScript
}

// interpreter returns the command that runs a program with the extension in
// opt, or nil if it is not run by an interpreter.
func interpreter(opt *Options) []string {
	if opt == nil {
		return nil
	}
	return interpreters[opt.Ext]
}

// startScript writes the given program to a file with extension ext and
// starts the interpreter interp on it.
func (p *process) startScript(interp []string, ext, body string) error {
	prog, err := p.writeProg("prog"+ext, body)
	if err != nil {
		return err
	}
	cmd := p.cmd("", append(slices.Clone(interp), prog)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.run = cmd
	return nil
}

// hasTSC reports whether the TypeScript compiler is installed.
func hasTSC() bool {
	_, err := exec.LookPath("tsc")
	return err == nil
}

// startTypeScript type checks the given TypeScript program and compiles it
// with tsc, then starts node on the result. A type error stops the program
// from running, like a Go compile error. The source map makes node report
// runtime errors at their lines in the TypeScript.
func (p *process) startTypeScript(body string) error {
	prog, err := p.writeProg("prog.ts", body)
	if err != nil {
		return err
	}
	cmd := p.cmd(p.path, "tsc", "--strict", "--target", "es2022",
		"--module", "esnext", "--moduleResolution", "bundler",
		"--sourceMap", "--noEmitOnError", "prog.ts")
	cmd.Stdout = cmd.Stderr // send compiler output to stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = p.cmd("", "node", "--enable-source-maps", strings.TrimSuffix(prog, ".ts")+".js")
	if err := cmd.Start(); err != nil {
		return err
	}
	p.run = cmd
	return nil
}

// writeProg writes body to the named file in a new temporary directory,
// which p.end removes, and returns the file's path.
func (p *process) writeProg(name, body string) (string, error) {
	path, err := os.MkdirTemp("", "present-")
	if err != nil {
		return "", err
	}
	p.path = path // to be removed by p.end

	prog := filepath.Join(path, name)
	if err := os.WriteFile(prog, []byte(body), 0o666); err != nil {
		return "", err
	}
	return prog, nil
}

// start builds and starts the given program, sending its output to p.out,
// and stores the running *exec.Cmd in the run field.
func (p *process) start(body string, opt *Options) error {
	// We "go build" and then exec the binary so that the
	// resultant *exec.Cmd is a handle to the user's program
	// (rather than the go tool process).
	// This makes Kill work.

	path, err := os.MkdirTemp("", "present-")
	if err != nil {
		return err
	}
	p.path = path // to be removed by p.end

	out := "prog"
	if runtime.GOOS == "windows" {
		out = "prog.exe"
	}
	bin := filepath.Join(path, out)

	// write body to x.go files
	// The browser sends the code with a leading newline, so an archive
	// that starts with a file header has a blank comment; it is not a
	// program of its own.
	a := txtar.Parse([]byte(body))
	if len(bytes.TrimSpace(a.Comment)) != 0 {
		a.Files = append(a.Files, txtar.File{Name: "prog.go", Data: a.Comment})
		a.Comment = nil
	}
	fsys, err := txtar.FS(a)
	if err != nil {
		return err
	}
	if err := os.CopyFS(path, fsys); err != nil {
		return err
	}

	// A snippet without its own go.mod becomes a module of its own, at
	// the language version of the installed go command.
	if !slices.ContainsFunc(a.Files, func(f txtar.File) bool { return f.Name == "go.mod" }) {
		cmd := p.cmd(path, "go", "mod", "init", "prog")
		cmd.Stdout, cmd.Stderr = nil, nil
		if out, err := cmd.CombinedOutput(); err != nil {
			if len(out) == 0 {
				return err
			}
			return errors.New(strings.TrimSpace(string(out)))
		}
	}

	// build x.go, creating x
	args := []string{"go", "build", "-tags", "OMIT"}
	if opt != nil && opt.Race {
		p.out <- &Message{
			Kind: "stderr",
			Body: "Running with race detector.\n",
		}
		args = append(args, "-race")
	}
	args = append(args, "-o", bin)
	cmd := p.cmd(path, args...)
	cmd.Stdout = cmd.Stderr // send compiler output to stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// run x
	cmd = p.cmd("", bin)
	if opt != nil && opt.Race {
		cmd.Env = append(cmd.Env, "GOMAXPROCS=2")
	}
	if err := cmd.Start(); err != nil {
		// If we failed to exec, that might be because they built
		// a non-main package instead of an executable.
		// Check and report that.
		if name, err := packageName(body); err == nil && name != "main" {
			return errors.New(`executable programs must use "package main"`)
		}
		return err
	}
	p.run = cmd
	return nil
}

// cmd builds an *exec.Cmd that writes its standard output and error to the
// process' output channel.
func (p *process) cmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdout = &messageWriter{kind: "stdout", out: p.out}
	cmd.Stderr = &messageWriter{kind: "stderr", out: p.out}
	return cmd
}

func packageName(body string) (string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "prog.go",
		strings.NewReader(body), parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	return f.Name.String(), nil
}

// messageWriter is an io.Writer that converts all writes to Message sends on
// the out channel with the specified id and kind.
type messageWriter struct {
	kind string
	out  chan<- *Message
}

func (w *messageWriter) Write(b []byte) (n int, err error) {
	w.out <- &Message{Kind: w.kind, Body: safeString(b)}
	return len(b), nil
}

// safeString returns b as a valid UTF-8 string.
func safeString(b []byte) string {
	return strings.ToValidUTF8(string(b), string(utf8.RuneError))
}
