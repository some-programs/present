// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// playgroundHandlers holds each playground's handlers, indexed by the
// playground's position on the page, so that mirrored clicks reach the
// right one.
export const playgroundHandlers = {
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
export function setupNotes() {
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
export function toggleNotesWindow() {
    if (!isParentWindow)
        return;
    if (notesWindow) {
        notesWindow.close();
        notesWindow = null;
        return;
    }
    openNotesWindow();
}
// destSlideKey returns the storage key holding the current slide, unique to
// this presentation so that different presentations don't mix.
// For golang.org/issue/24688.
export function destSlideKey() {
    let key = window.location.href;
    const slides = notesWindow?.document.getElementById('presenter-slides');
    if (slides instanceof HTMLIFrameElement) {
        key = slides.src;
    }
    return 'destSlide:' + key.split('#')[0];
}
// storedSlide returns the slide number stored under destSlideKey.
export function storedSlide() {
    return parseInt(localStorage.getItem(destSlideKey()) ?? '', 10);
}
function openNotesWindow() {
    const w = window.open('', '', 'width=1000,height=700');
    if (!w)
        return;
    notesWindow = w;
    // Hack to apply css. Requires existing html on notesWindow.
    w.document.write("<div style='display:none;'></div>");
    w.document.title = window.document.title;
    const slides = w.document.createElement('iframe');
    slides.id = 'presenter-slides';
    slides.src = window.location.href;
    w.document.body.appendChild(slides);
    // setTimeout needed for Firefox
    setTimeout(() => slides.focus(), 100);
    const notes = w.document.createElement('div');
    notes.id = 'presenter-notes';
    notes.innerHTML = notesFor(storedSlide());
    w.document.body.appendChild(notes);
    w.document.close();
    const style = w.document.createElement('link');
    style.rel = 'stylesheet';
    style.href = '/static/notes.css';
    w.document.head.appendChild(style);
    // Update the notes when the parent window changes slides.
    w.addEventListener('storage', updateNotes, false);
}
// notesFor returns the notes of the given slide as HTML. Slide 0 is the
// title slide.
function notesFor(slide) {
    const notes = slide === 0 ? window.titleNotes : window.sections?.[slide - 1]?.Notes;
    return (notes ?? []).map((n) => '<p>' + n + '</p>').join('');
}
export function updateNotes() {
    // When triggered from the parent window, notesWindow is null.
    // The storage event listener on notesWindow will update the notes.
    if (!notesWindow)
        return;
    const el = notesWindow.document.getElementById('presenter-notes');
    if (!el)
        return;
    el.innerHTML = notesFor(storedSlide());
}
/* Playground syncing */
// updatePlay applies a playground change made in the other window.
export function updatePlay(e) {
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
            if (out)
                out.style.cssText = localStorage.getItem('output-style') ?? '';
            return;
        }
    }
}
// updatePlayStorage records a playground button click so the other window
// can mirror it. Mirrored clicks reset the stored action so that successive
// clicks of the same button are synced too.
export function updatePlayStorage(action, index, e) {
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
