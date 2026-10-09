// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
In the absence of any formal way to specify interfaces in JavaScript,
here's a skeleton implementation of a playground transport.

        function Transport() {
                // Set up any transport state (eg, make a websocket connection).
                return {
                        Run: function(body, output, options) {
                                // Compile and run the program 'body' with 'options'.
				// Call the 'output' callback to display program output.
                                return {
                                        Kill: function() {
                                                // Kill the running program.
                                        }
                                };
                        }
                };
        }

	// The output callback is called multiple times, and each time it is
	// passed an object of this form.
        var write = {
                Kind: 'string', // 'start', 'stdout', 'stderr', 'end'
                Body: 'string'  // content of write or end status message
        }

	// The first call must be of Kind 'start' with no body.
	// Subsequent calls may be of Kind 'stdout' or 'stderr'
	// and must have a non-null Body string.
	// The final call should be of Kind 'end' with an optional
	// Body string, signifying a failure ("killed", for example).

	// The output callback must be of this form.
	// See PlaygroundOutput (below) for an implementation.
        function outputCallback(write) {
        }
*/

function SocketTransport() {
  'use strict';

  var id = 0;
  var outputs = {};
  var started = {};
  var websocket;
  if (window.location.protocol == 'http:') {
    websocket = new WebSocket('ws://' + window.location.host + '/socket');
  } else if (window.location.protocol == 'https:') {
    websocket = new WebSocket('wss://' + window.location.host + '/socket');
  }

  websocket.onclose = function() {
    console.log('websocket connection closed');
  };

  websocket.onmessage = function(e) {
    var m = JSON.parse(e.data);
    var output = outputs[m.Id];
    if (output === null) return;
    if (!started[m.Id]) {
      output({ Kind: 'start' });
      started[m.Id] = true;
    }
    output({ Kind: m.Kind, Body: m.Body });
  };

  function send(m) {
    websocket.send(JSON.stringify(m));
  }

  return {
    Run: function(body, output, options) {
      var thisID = id + '';
      id++;
      outputs[thisID] = output;
      send({ Id: thisID, Kind: 'run', Body: body, Options: options });
      return {
        Kill: function() {
          send({ Id: thisID, Kind: 'kill' });
        },
      };
    },
  };
}

function PlaygroundOutput(el) {
  'use strict';

  return function(write) {
    if (write.Kind == 'start') {
      el.innerHTML = '';
      return;
    }

    var cl = 'system';
    if (write.Kind == 'stdout' || write.Kind == 'stderr') cl = write.Kind;

    var m = write.Body;
    if (write.Kind == 'end') {
      m = '\nProgram exited' + (m ? ': ' + m : '.');
    }

    if (m.indexOf('IMAGE:') === 0) {
      // TODO(adg): buffer all writes before creating image
      var url = 'data:image/png;base64,' + m.substr(6);
      var img = document.createElement('img');
      img.src = url;
      el.appendChild(img);
      return;
    }

    // ^L clears the screen.
    var s = m.split('\x0c');
    if (s.length > 1) {
      el.innerHTML = '';
      m = s.pop();
    }

    m = m.replace(/&/g, '&amp;');
    m = m.replace(/</g, '&lt;');
    m = m.replace(/>/g, '&gt;');

    var needScroll = el.scrollTop + el.offsetHeight == el.scrollHeight;

    var span = document.createElement('span');
    span.className = cl;
    span.innerHTML = m;
    el.appendChild(span);

    if (needScroll) el.scrollTop = el.scrollHeight - el.offsetHeight;
  };
}
