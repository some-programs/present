"use strict";
// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// playgroundHandlers holds each playground's handlers, indexed by the
// playground's position on the page, so that mirrored clicks reach the
// right one.
const playgroundHandlers = {
    onRun: [],
    onKill: [],
    onClose: [],
};
// Keys shared by all presentations for syncing playground state with the
// notes window. destSlideKey() adds a per-presentation key.
const playStorageKeys = [
    'play-index',
    'play-action',
    'play-code',
    'output-style',
    'play-shiftKey',
];
// The child window that displays the slides with notes.
let notesWindow = null;
const isParentWindow = window.parent === window;
// setupNotes prepares the presenter notes. Call it only when notes are
// enabled.
function setupNotes() {
    if (!isParentWindow)
        return;
    // When the parent window closes, remove our storage keys and close the
    // child window.
    window.addEventListener('beforeunload', () => {
        localStorage.removeItem(destSlideKey());
        for (const key of playStorageKeys) {
            localStorage.removeItem(key);
        }
        notesWindow?.close();
    });
}
function toggleNotesWindow() {
    if (!isParentWindow)
        return;
    if (notesWindow) {
        notesWindow.close();
        notesWindow = null;
        stopTimer();
        return;
    }
    openNotesWindow();
}
/* Timer */
// The notes window shows how long the current slide has been showing and
// how long the notes window has been open.
let timerInterval;
let timerSlide = NaN;
let slideStart = 0;
let notesStart = 0;
// formatDuration formats ms as m:ss, or h:mm:ss from one hour on.
function formatDuration(ms) {
    const s = Math.floor(ms / 1000);
    const h = Math.floor(s / 3600);
    const m = Math.floor(s / 60) % 60;
    const ss = String(s % 60).padStart(2, '0');
    if (h > 0)
        return h + ':' + String(m).padStart(2, '0') + ':' + ss;
    return m + ':' + ss;
}
function startTimer() {
    notesStart = slideStart = Date.now();
    timerSlide = storedSlide();
    window.clearInterval(timerInterval);
    timerInterval = window.setInterval(updateTimer, 500);
    updateTimer();
}
function stopTimer() {
    window.clearInterval(timerInterval);
    timerInterval = undefined;
}
function updateTimer() {
    if (!notesWindow || notesWindow.closed) {
        // The presenter closed the notes window.
        notesWindow = null;
        stopTimer();
        return;
    }
    const now = Date.now();
    const slide = storedSlide();
    if (!Object.is(slide, timerSlide)) {
        timerSlide = slide;
        slideStart = now;
    }
    const el = notesWindow.document.getElementById('timer');
    if (!el)
        return;
    el.textContent =
        'slide ' +
            formatDuration(now - slideStart) +
            ' · total ' +
            formatDuration(now - notesStart);
}
// destSlideKey returns the storage key holding the current slide, unique to
// this presentation so that different presentations don't mix.
// For golang.org/issue/24688.
function destSlideKey() {
    let key = window.location.href;
    const slides = notesWindow?.document.getElementById('presenter-slides');
    if (slides instanceof HTMLIFrameElement) {
        key = slides.src;
    }
    return 'destSlide:' + key.split('#')[0];
}
// storedSlide returns the slide number stored under destSlideKey.
function storedSlide() {
    return parseInt(localStorage.getItem(destSlideKey()) ?? '', 10);
}
// openNotesWindow opens the notes window, which the server renders from
// the "notes" template in templates/slides.tmpl, and fills it in once it
// has loaded.
function openNotesWindow() {
    const w = window.open(window.location.pathname + '?notes', '', 'width=1000,height=700');
    if (!w)
        return;
    notesWindow = w;
    w.addEventListener('DOMContentLoaded', () => {
        // instanceof would test against this window's HTMLIFrameElement.
        const slides = w.document.querySelector('#presenter-slides');
        if (!slides)
            return;
        slides.src = window.location.href;
        // setTimeout needed for Firefox
        setTimeout(() => slides.focus(), 100);
        updateNotes();
        // Update the notes when the parent window changes slides.
        w.addEventListener('storage', updateNotes, false);
    });
    startTimer();
}
// updateNotes shows the notes of the current slide. The server renders the
// notes of every slide into the notes window, a block for each, and a last
// one for slides without notes, such as the one -thankyou adds.
function updateNotes() {
    // When triggered from the parent window, notesWindow is null.
    // The storage event listener on notesWindow will update the notes.
    if (!notesWindow)
        return;
    const blocks = notesWindow.document.querySelectorAll('#presenter-notes > .slide-notes');
    const i = Math.min(storedSlide(), blocks.length - 1);
    blocks.forEach((b, j) => (b.hidden = j !== i));
}
/* Playground syncing */
// updatePlay applies a playground change made in the other window.
function updatePlay(e) {
    const i = parseInt(localStorage.getItem('play-index') ?? '', 10);
    switch (e.key) {
        case 'play-action': {
            // Sync 'run', 'kill', 'close' actions
            const action = localStorage.getItem('play-action');
            if (action)
                playgroundHandlers[action]?.[i]?.();
            return;
        }
        case 'play-code': {
            // Sync code editing
            const play = document.querySelectorAll('div.playground')[i];
            if (play)
                play.innerHTML = localStorage.getItem('play-code') ?? '';
            return;
        }
        case 'output-style': {
            // Sync resizing of playground output
            const out = document.querySelectorAll('.output')[i];
            if (out) {
                out.style.cssText = localStorage.getItem('output-style') ?? '';
                out.dataset.resized = ''; // so that it keeps its size, see play.ts
            }
            return;
        }
    }
}
// updatePlayStorage records a playground button click so the other window
// can mirror it. Mirrored clicks reset the stored action so that successive
// clicks of the same button are synced too.
function updatePlayStorage(action, index, e) {
    localStorage.setItem('play-index', String(index));
    if (localStorage.getItem('play-action') === action) {
        // We're the receiving window, and the message has been received
        localStorage.removeItem('play-action');
    }
    else {
        // We're the triggering window, send the message
        localStorage.setItem('play-action', action);
    }
    if (action === 'onRun') {
        if (localStorage.getItem('play-shiftKey') === 'true') {
            localStorage.removeItem('play-shiftKey');
        }
        else if (e?.shiftKey) {
            localStorage.setItem('play-shiftKey', 'true');
        }
    }
}
