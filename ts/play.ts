// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This module adds Run, Kill and Close buttons to every div.playground on
// the page and runs the code through the present server.

import {
  playgroundHandlers,
  updatePlayStorage,
  type PlayHandler,
} from './notes.js';
import {
  playgroundOutput,
  SocketTransport,
  type Running,
  type Transport,
} from './playground.js';

// Size limits for the output panel while resizing, in CSS pixels.
const minWidth = 135;
const maxWidth = 990;
const minHeight = 27;
const maxHeight = 608;

// text returns the program text in node, skipping buttons and line numbers.
function text(node: Node): string {
  let s = '';
  for (const n of node.childNodes) {
    if (n instanceof Element) {
      if (n.tagName === 'BUTTON') continue;
      if (n.tagName === 'SPAN' && n.className === 'number') continue;
      if (n.tagName === 'DIV' || n.tagName === 'BR' || n.tagName === 'PRE') {
        s += '\n';
      }
      s += text(n);
      continue;
    }
    if (n.nodeType === Node.TEXT_NODE) {
      s += n.nodeValue;
    }
  }
  return s.replace(/ /g, ' '); // replace non-breaking spaces
}

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max);
}

// makeResizable lets el be resized by dragging its top edge, left edge or
// top-left corner, keeping its bottom-right corner in place. It calls
// onResize after each change.
function makeResizable(el: HTMLElement, onResize: () => void) {
  const handles: [string, boolean, boolean][] = [
    ['n', false, true],
    ['w', true, false],
    ['nw', true, true],
  ];
  for (const [dir, horizontal, vertical] of handles) {
    const handle = document.createElement('div');
    handle.className = 'resize-handle resize-' + dir;
    handle.addEventListener('pointerdown', (down: PointerEvent) => {
      down.preventDefault();
      handle.setPointerCapture(down.pointerId);

      const style = getComputedStyle(el);
      const left = el.offsetLeft;
      const top = el.offsetTop;
      const width = parseFloat(style.width);
      const height = parseFloat(style.height);
      // The slides may be scaled to fit the window, so convert pointer
      // movement into layout pixels.
      const scale = el.getBoundingClientRect().width / el.offsetWidth || 1;

      const move = (e: PointerEvent) => {
        if (horizontal) {
          const dx = (e.clientX - down.clientX) / scale;
          const w = clamp(width - dx, minWidth, maxWidth);
          el.style.left = left + width - w + 'px';
          el.style.width = w + 'px';
        }
        if (vertical) {
          const dy = (e.clientY - down.clientY) / scale;
          const h = clamp(height - dy, minHeight, maxHeight);
          el.style.top = top + height - h + 'px';
          el.style.height = h + 'px';
        }
        onResize();
      };
      const up = () => {
        handle.removeEventListener('pointermove', move);
        handle.removeEventListener('pointerup', up);
        handle.removeEventListener('pointercancel', up);
      };
      handle.addEventListener('pointermove', move);
      handle.addEventListener('pointerup', up);
      handle.addEventListener('pointercancel', up);
    });
    el.appendChild(handle);
  }
}

function button(className: string, label: string, onClick: PlayHandler) {
  const b = document.createElement('button');
  b.className = className;
  b.textContent = label;
  b.addEventListener('click', onClick, false);
  return b;
}

// init sets up the playground code. index identifies it when presenter
// notes are enabled, so that it can be synced with the other window.
function init(transport: Transport, code: HTMLElement, index: number) {
  const output = document.createElement('div');
  const outpre = document.createElement('pre');
  const notesEnabled = !!window.notesEnabled;
  let running: Running | undefined;

  const onKill: PlayHandler = () => {
    running?.Kill();
    if (notesEnabled) updatePlayStorage('onKill', index);
  };

  const onRun: PlayHandler = (e) => {
    const race =
      !!e?.shiftKey || localStorage.getItem('play-shiftKey') === 'true';
    running?.Kill();
    output.style.display = 'block';
    outpre.textContent = '';
    run1.style.display = 'none';
    running = transport.Run(text(code), playgroundOutput(outpre), {
      Race: race,
    });
    if (notesEnabled) updatePlayStorage('onRun', index, e);
  };

  const onClose: PlayHandler = () => {
    running?.Kill();
    output.style.display = 'none';
    run1.style.display = 'inline-block';
    if (notesEnabled) updatePlayStorage('onClose', index);
  };

  const run1 = button('run', 'Run', onRun);

  const button1 = document.createElement('div');
  button1.classList.add('buttons');
  button1.appendChild(run1);
  code.after(button1);

  const buttons = document.createElement('div');
  buttons.classList.add('buttons');
  buttons.append(
    button('run', 'Run', onRun),
    button('kill', 'Kill', onKill),
    button('close', 'Close', onClose)
  );

  output.classList.add('output');
  output.append(buttons, outpre);
  output.style.display = 'none';
  button1.after(output);

  // On slides the output panel floats over the slide and can be resized.
  const onSlide = code.closest('section.slides') !== null;
  if (onSlide) {
    makeResizable(output, () => {
      if (!notesEnabled) return;
      localStorage.setItem('play-index', String(index));
      localStorage.setItem('output-style', output.style.cssText);
    });
  }

  if (notesEnabled) {
    playgroundHandlers.onRun.push(onRun);
    playgroundHandlers.onClose.push(onClose);
    playgroundHandlers.onKill.push(onKill);

    code.addEventListener('input', () => {
      localStorage.setItem('play-index', String(index));
      localStorage.setItem('play-code', code.innerHTML);
    });
  }
}

const transport = new SocketTransport();
document
  .querySelectorAll<HTMLElement>('div.playground')
  .forEach((code, i) => init(transport, code, i));
