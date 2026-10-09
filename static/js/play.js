// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This module adds Run, Kill and Close buttons to every div.playground on
// the page and runs the code through the present server.
import { playgroundHandlers, updatePlayStorage, } from './notes.js';
import { playgroundOutput, SocketTransport, } from './playground.js';
// Size limits for the output panel while resizing, in CSS pixels.
const minWidth = 135;
const maxWidth = 990;
const minHeight = 27;
const maxHeight = 608;
// How long the output panel of a run waits for the program's first output
// before it shows, in milliseconds. Output within that time sizes the panel
// before it is seen.
const showDelay = 300;
// text returns the program text in node, skipping buttons and line numbers.
// A DIV or PRE, such as a line added while editing, starts a new line unless
// the text is already at the start of one; a BR is always a line break.
function text(node) {
    let s = '';
    const walk = (node) => {
        for (const n of node.childNodes) {
            if (n instanceof Element) {
                if (n.tagName === 'BUTTON')
                    continue;
                if (n.tagName === 'SPAN' && n.className === 'number')
                    continue;
                if (n.tagName === 'BR') {
                    s += '\n';
                }
                else if ((n.tagName === 'DIV' || n.tagName === 'PRE') &&
                    s !== '' &&
                    !s.endsWith('\n')) {
                    s += '\n';
                }
                walk(n);
                continue;
            }
            if (n.nodeType === Node.TEXT_NODE) {
                s += n.nodeValue;
            }
        }
    };
    walk(node);
    return s.replace(/ /g, ' '); // replace non-breaking spaces
}
function clamp(v, min, max) {
    return Math.min(Math.max(v, min), max);
}
// makeResizable lets el be resized by dragging its top edge, left edge or
// top-left corner, keeping its bottom-right corner in place. It calls
// onResize after each change.
function makeResizable(el, onResize) {
    const handles = [
        ['n', false, true],
        ['w', true, false],
        ['nw', true, true],
    ];
    for (const [dir, horizontal, vertical] of handles) {
        const handle = document.createElement('div');
        handle.className = 'resize-handle resize-' + dir;
        handle.addEventListener('pointerdown', (down) => {
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
            const move = (e) => {
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
// fit grows the output panel el, keeping its bottom-right corner in place,
// until the output in pre fits, the panel is as large as it may be, or it
// reaches the padding of its slide. Lines that are still too long wrap.
function fit(el, pre) {
    const style = getComputedStyle(el);
    const width = parseFloat(style.width);
    const height = parseFloat(style.height);
    // The slides may be scaled to fit the window, so convert what is measured
    // on the screen into layout pixels.
    const r = el.getBoundingClientRect();
    const scale = r.width / el.offsetWidth || 1;
    // Measure the output without wrapping, so that long lines count in full.
    // scrollWidth is rounded, and a line a fraction of a pixel too long
    // wraps, so measure the text itself.
    pre.style.whiteSpace = 'pre';
    const text = document.createRange();
    text.selectNodeContents(pre);
    const textWidth = Math.ceil(text.getBoundingClientRect().width / scale);
    pre.style.whiteSpace = '';
    const wantWidth = width + textWidth - pre.clientWidth;
    let roomWidth = maxWidth;
    let roomHeight = maxHeight;
    const slide = el.closest('article');
    if (slide) {
        const s = slide.getBoundingClientRect();
        const ss = getComputedStyle(slide);
        let left = s.left + parseFloat(ss.paddingLeft) * scale;
        // Keep the resize handles, which stick out of the panel, clear of the
        // area that goes to the previous slide when clicked.
        const prev = document.getElementById('prev-slide-area');
        if (prev) {
            left = Math.max(left, prev.getBoundingClientRect().right + 10 * scale);
        }
        roomWidth = (r.right - left) / scale;
        roomHeight = (r.bottom - s.top) / scale - parseFloat(ss.paddingTop);
        // r includes the panel's padding and border; width and height do not.
        roomWidth -= el.offsetWidth - width;
        roomHeight -= el.offsetHeight - height;
    }
    const w = Math.max(width, Math.min(wantWidth, maxWidth, roomWidth));
    if (w > width) {
        el.style.left = el.offsetLeft + width - w + 'px';
        el.style.width = w + 'px';
    }
    // Wrapping at the new width decides how tall the output is.
    const wantHeight = height + pre.scrollHeight - pre.clientHeight;
    const h = Math.max(height, Math.min(wantHeight, maxHeight, roomHeight));
    if (h > height) {
        el.style.top = el.offsetTop + height - h + 'px';
        el.style.height = h + 'px';
    }
}
// unfit puts the output panel el back to its size from the style sheet.
function unfit(el) {
    for (const p of ['left', 'top', 'width', 'height']) {
        el.style.removeProperty(p);
    }
}
// closeOpen closes the playground whose output panel is open, if any, so
// that running one playground closes the output of another.
let closeOpen;
function button(className, label, onClick) {
    const b = document.createElement('button');
    b.className = className;
    b.textContent = label;
    b.addEventListener('click', onClick, false);
    return b;
}
// init sets up the playground code. index identifies it when presenter
// notes are enabled, so that it can be synced with the other window.
function init(transport, code, index) {
    const output = document.createElement('div');
    const outpre = document.createElement('pre');
    const notesEnabled = !!window.notesEnabled;
    // On slides the output panel floats over the slide, grows to fit the
    // output and can be resized. Once it has been resized, it keeps its size.
    const onSlide = code.closest('section.slides') !== null;
    const autoFit = () => onSlide && output.dataset.resized === undefined;
    let running;
    let showTimer;
    const onKill = () => {
        running?.Kill();
        if (notesEnabled)
            updatePlayStorage('onKill', index);
    };
    // close kills the program and hides the output panel. Unlike onClose, it
    // is not mirrored to the notes window: closing another playground's
    // panel happens in each window when the run is mirrored.
    const close = () => {
        running?.Kill();
        clearTimeout(showTimer);
        output.style.visibility = '';
        output.style.display = 'none';
        run1.style.display = 'inline-block';
        if (closeOpen === close)
            closeOpen = undefined;
    };
    const onRun = (e) => {
        const race = !!e?.shiftKey || localStorage.getItem('play-shiftKey') === 'true';
        if (closeOpen !== close)
            closeOpen?.();
        closeOpen = close;
        running?.Kill();
        clearTimeout(showTimer);
        // The panel is fitted to each run's output. A closed panel stays hidden
        // until the program's first output, or for showDelay, so that it is
        // first seen at its fitted size. An open one keeps its size and the
        // previous output until then; the output is cleared on the 'start'
        // write that comes before the first output.
        const open = output.style.display === 'block';
        let unfitted = false;
        if (autoFit() && !open) {
            unfit(output);
            unfitted = true;
            output.style.visibility = 'hidden';
            showTimer = setTimeout(() => (output.style.visibility = ''), showDelay);
        }
        output.style.display = 'block';
        if (!open)
            outpre.textContent = '';
        run1.style.display = 'none';
        const write = playgroundOutput(outpre);
        const output1 = (w) => {
            clearTimeout(showTimer);
            output.style.visibility = '';
            if (autoFit() && !unfitted)
                unfit(output);
            unfitted = true;
            write(w);
            if (autoFit())
                fit(output, outpre);
        };
        running = transport.Run(text(code), output1, {
            Race: race,
            Ext: code.dataset.ext ?? '',
        });
        if (notesEnabled)
            updatePlayStorage('onRun', index, e);
    };
    const onClose = () => {
        close();
        if (notesEnabled)
            updatePlayStorage('onClose', index);
    };
    const run1 = button('run', 'Run', onRun);
    const button1 = document.createElement('div');
    button1.classList.add('buttons');
    button1.appendChild(run1);
    code.after(button1);
    const buttons = document.createElement('div');
    buttons.classList.add('buttons');
    buttons.append(button('run', 'Run', onRun), button('kill', 'Kill', onKill), button('close', 'Close', onClose));
    output.classList.add('output');
    output.append(buttons, outpre);
    output.style.display = 'none';
    button1.after(output);
    if (onSlide) {
        makeResizable(output, () => {
            output.dataset.resized = '';
            if (!notesEnabled)
                return;
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
    .querySelectorAll('div.playground')
    .forEach((code, i) => init(transport, code, i));
