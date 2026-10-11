// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Write is one piece of program output passed to an OutputFunc.
//
// The first write has Kind 'start' and no body. It is followed by writes of
// Kind 'stdout' or 'stderr' with a Body, and a final write of Kind 'end'
// whose optional Body describes a failure, such as "killed".
interface Write {
  Kind: 'start' | 'stdout' | 'stderr' | 'end';
  Body?: string;
}

type OutputFunc = (write: Write) => void;

interface RunOptions {
  Race: boolean;
  Ext: string; // extension of the program's file, such as ".py"
}

interface Running {
  Kill(): void;
}

// Transport compiles and runs programs, reporting their output.
interface Transport {
  Run(body: string, output: OutputFunc, options: RunOptions): Running;
}

// Message is the wire format of the /socket WebSocket.
// See Message in pkg/playground/socket.
interface Message {
  Id: string;
  Kind: string;
  Body?: string;
  Options?: RunOptions;
}

// SocketTransport runs programs on the present server over a WebSocket.
class SocketTransport implements Transport {
  private nextID = 0;
  private readonly outputs = new Map<string, OutputFunc>();
  private readonly started = new Set<string>();
  private readonly websocket: WebSocket;

  constructor() {
    const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.websocket = new WebSocket(
      scheme + '//' + window.location.host + '/socket'
    );
    this.websocket.onclose = () => {
      console.log('websocket connection closed');
    };
    this.websocket.onmessage = (e: MessageEvent<string>) => {
      this.receive(JSON.parse(e.data) as Message);
    };
  }

  Run(body: string, output: OutputFunc, options: RunOptions): Running {
    const id = String(this.nextID++);
    this.outputs.set(id, output);
    this.send({ Id: id, Kind: 'run', Body: body, Options: options });
    return {
      Kill: () => this.send({ Id: id, Kind: 'kill' }),
    };
  }

  private receive(m: Message) {
    const output = this.outputs.get(m.Id);
    if (!output) return;
    if (!this.started.has(m.Id)) {
      output({ Kind: 'start' });
      this.started.add(m.Id);
    }
    output({ Kind: m.Kind as Write['Kind'], Body: m.Body });
  }

  // send sends m, waiting for the connection to open if necessary.
  private send(m: Message) {
    const data = JSON.stringify(m);
    if (this.websocket.readyState === WebSocket.CONNECTING) {
      this.websocket.addEventListener(
        'open',
        () => this.websocket.send(data),
        { once: true }
      );
      return;
    }
    this.websocket.send(data);
  }
}

// playgroundOutput returns an OutputFunc that appends program output to el.
function playgroundOutput(el: HTMLElement): OutputFunc {
  return (write: Write) => {
    if (write.Kind === 'start') {
      el.textContent = '';
      return;
    }

    let cl = 'system';
    if (write.Kind === 'stdout' || write.Kind === 'stderr') cl = write.Kind;

    let m = write.Body ?? '';
    if (write.Kind === 'end') {
      m = '\nProgram exited' + (m ? ': ' + m : '.');
    }

    if (m.startsWith('IMAGE:')) {
      // TODO(adg): buffer all writes before creating image
      const img = document.createElement('img');
      img.src = 'data:image/png;base64,' + m.slice(6);
      el.appendChild(img);
      return;
    }

    // ^L clears the screen.
    const s = m.split('\x0c');
    if (s.length > 1) {
      el.textContent = '';
      m = s[s.length - 1];
    }

    const needScroll = el.scrollTop + el.offsetHeight === el.scrollHeight;

    const span = document.createElement('span');
    span.className = cl;
    span.textContent = m;
    el.appendChild(span);

    if (needScroll) el.scrollTop = el.scrollHeight - el.offsetHeight;
  };
}
