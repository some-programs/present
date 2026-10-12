"use strict";
// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This script runs slide presentations: navigation and syncing with the presenter notes window.
// It uses notes.js and source.js, which the templates load before it.
const SLIDE_CLASSES = ['far-past', 'past', 'current', 'next', 'far-next'];
const PM_TOUCH_SENSITIVITY = 15;
// The settings that the template defines as the globals notesEnabled and
// preloadEnabled, which a script cannot declare again.
const notesOn = !!window.notesEnabled;
const preloadOn = window.preloadEnabled !== false;
// sourceEnabled reports whether the page has the source panel of -doc. The
// presenter notes window does not show it.
let sourceEnabled = false;
let slideEls;
let curSlide = 0;
/* Slide movement */
function hideHelpText() {
    const help = document.getElementById('help');
    if (help)
        help.style.display = 'none';
}
function getSlideEl(no) {
    if (no < 0 || no >= slideEls.length) {
        return null;
    }
    return slideEls[no];
}
function updateSlideClass(slideNo, className) {
    const el = getSlideEl(slideNo);
    if (!el)
        return;
    if (className) {
        el.classList.add(className);
    }
    for (const c of SLIDE_CLASSES) {
        if (c !== className) {
            el.classList.remove(c);
        }
    }
}
function updateSlides() {
    for (let i = 0; i < slideEls.length; i++) {
        const offset = i - curSlide + 2;
        updateSlideClass(i, SLIDE_CLASSES[offset]);
    }
    triggerLeaveEvent(curSlide - 1);
    triggerEnterEvent(curSlide);
    window.setTimeout(() => {
        // Hide after the slide
        disableSlideFrames(curSlide - 2);
    }, 301);
    enableSlideFrames(curSlide - 1);
    enableSlideFrames(curSlide + 2);
    preloadSlideWindow();
    if (sourceEnabled)
        updateSource(curSlide);
    updateHash();
}
function storeSlide() {
    if (notesOn)
        localStorage.setItem(destSlideKey(), String(curSlide));
}
function prevSlide() {
    hideHelpText();
    if (curSlide > 0) {
        curSlide--;
        updateSlides();
    }
    storeSlide();
}
function nextSlide() {
    hideHelpText();
    if (curSlide < slideEls.length - 1) {
        curSlide++;
        updateSlides();
    }
    storeSlide();
}
/* Slide events */
function triggerSlideEvent(no, type, attr) {
    const el = getSlideEl(no);
    if (!el)
        return;
    const handler = el.getAttribute(attr);
    if (handler) {
        new Function(handler).call(el);
    }
    const evt = new Event(type, { bubbles: true, cancelable: true });
    evt.slideNumber = no + 1; // Make it readable
    el.dispatchEvent(evt);
}
function triggerEnterEvent(no) {
    triggerSlideEvent(no, 'slideenter', 'onslideenter');
}
function triggerLeaveEvent(no) {
    triggerSlideEvent(no, 'slideleave', 'onslideleave');
}
/* Touch events */
let touchStartX = 0;
let touchStartY = 0;
let touchDX = 0;
let touchDY = 0;
function handleTouchStart(event) {
    if (event.touches.length === 1) {
        touchDX = 0;
        touchDY = 0;
        touchStartX = event.touches[0].pageX;
        touchStartY = event.touches[0].pageY;
        document.body.addEventListener('touchmove', handleTouchMove, true);
        document.body.addEventListener('touchend', handleTouchEnd, true);
    }
}
function handleTouchMove(event) {
    if (event.touches.length > 1) {
        cancelTouch();
    }
    else {
        touchDX = event.touches[0].pageX - touchStartX;
        touchDY = event.touches[0].pageY - touchStartY;
        event.preventDefault();
    }
}
function handleTouchEnd() {
    const dx = Math.abs(touchDX);
    const dy = Math.abs(touchDY);
    if (dx > PM_TOUCH_SENSITIVITY && dy < (dx * 2) / 3) {
        if (touchDX > 0) {
            prevSlide();
        }
        else {
            nextSlide();
        }
    }
    cancelTouch();
}
function cancelTouch() {
    document.body.removeEventListener('touchmove', handleTouchMove, true);
    document.body.removeEventListener('touchend', handleTouchEnd, true);
}
/* Preloading media */
const preloadedSlides = new Set();
function preloadMediaForSlide(no) {
    if (preloadedSlides.has(no))
        return;
    const el = getSlideEl(no);
    if (!el)
        return;
    preloadedSlides.add(no);
    // Preload and decode slide images
    for (const img of el.querySelectorAll('img')) {
        if (img.complete) {
            img.decode?.().catch(() => { });
        }
        else {
            img.addEventListener('load', () => {
                img.decode?.().catch(() => { });
            });
        }
    }
    // Preload background images
    if (el.classList.contains('background')) {
        const bg = el.style.backgroundImage;
        const match = bg.match(/url\(["']?(.*?)["']?\)/);
        if (match && match[1]) {
            const bgImg = new Image();
            bgImg.src = match[1];
            bgImg.decode?.().catch(() => { });
        }
    }
    // Preload and initialize video frames
    for (const video of el.querySelectorAll('video')) {
        if (video.preload !== 'auto') {
            video.preload = 'auto';
        }
    }
}
function preloadSlideWindow() {
    if (!preloadOn)
        return;
    // Preload current slide as well as upcoming and previous slides in the window
    for (let offset = -1; offset <= 2; offset++) {
        preloadMediaForSlide(curSlide + offset);
    }
}
/* Preloading frames */
// The real src of each iframe, which is unloaded while its slide is far away.
const frameSrc = new WeakMap();
function slideFrames(no) {
    const el = getSlideEl(no);
    return el ? Array.from(el.getElementsByTagName('iframe')) : [];
}
function disableSlideFrames(no) {
    slideFrames(no).forEach(disableFrame);
}
function enableSlideFrames(no) {
    slideFrames(no).forEach(enableFrame);
}
function disableFrame(frame) {
    frame.src = 'about:blank';
}
function enableFrame(frame) {
    const src = frameSrc.get(frame);
    if (src && frame.src !== src && src !== 'about:blank') {
        frame.src = src;
    }
}
function setupFrames() {
    for (const frame of document.querySelectorAll('iframe')) {
        frameSrc.set(frame, frame.src);
        disableFrame(frame);
    }
    enableSlideFrames(curSlide);
    enableSlideFrames(curSlide + 1);
    enableSlideFrames(curSlide + 2);
}
function setupInteraction() {
    /* Clicking and tapping */
    const slides = document.querySelector('section.slides');
    for (const [id, onClick] of [
        ['prev-slide-area', prevSlide],
        ['next-slide-area', nextSlide],
    ]) {
        const el = document.createElement('div');
        el.className = 'slide-area';
        el.id = id;
        el.addEventListener('click', onClick, false);
        slides?.appendChild(el);
    }
    document.getElementById('help')?.addEventListener('click', hideHelpText);
    /* Swiping */
    document.body.addEventListener('touchstart', handleTouchStart, false);
}
/* Hash functions */
function getCurSlideFromHash() {
    const slideNo = parseInt(location.hash.slice(1), 10);
    curSlide = slideNo ? slideNo - 1 : 0;
}
function updateHash() {
    location.replace('#' + (curSlide + 1));
}
/* Event listeners */
function handleBodyKeyDown(event) {
    // If we're in a code element, only handle PgUp/PgDn.
    const target = event.target;
    const inCode = target instanceof Element && target.classList.contains('code');
    switch (event.key) {
        case 'n':
        case 'N':
            // Open the presenter notes window.
            if (!inCode && notesOn)
                toggleNotesWindow();
            break;
        case 's':
        case 'S':
            // Show the source of the slide.
            if (!inCode && sourceEnabled)
                toggleSource();
            break;
        case 'h':
        case 'H':
            // Hide the help text.
            if (!inCode)
                hideHelpText();
            break;
        case 'PageDown':
            nextSlide();
            event.preventDefault();
            break;
        case 'ArrowRight':
        case 'ArrowDown':
        case 'Enter':
        case ' ':
            if (inCode)
                break;
            nextSlide();
            event.preventDefault();
            break;
        case 'PageUp':
            prevSlide();
            event.preventDefault();
            break;
        case 'ArrowLeft':
        case 'ArrowUp':
        case 'Backspace':
            if (inCode)
                break;
            prevSlide();
            event.preventDefault();
            break;
    }
}
function addEventListeners() {
    document.addEventListener('keydown', handleBodyKeyDown, false);
}
/* Initialization */
// markPresenterView marks the copy of the slides in the presenter notes
// window, which shows the previous and next slide beside the current one.
function markPresenterView() {
    if (window.frameElement?.id === 'presenter-slides') {
        document.documentElement.classList.add('presenter-view');
    }
}
function handleDomLoaded() {
    slideEls = document.querySelectorAll('section.slides > article');
    setupFrames();
    markPresenterView();
    addEventListeners();
    sourceEnabled =
        !document.documentElement.classList.contains('presenter-view') &&
            setupSource();
    updateSlides();
    setupInteraction();
    const host = window.location.hostname;
    if (host === 'localhost' || host === '127.0.0.1' || host === '[::1]') {
        hideHelpText();
    }
    document.body.classList.add('loaded');
    setupNotesSync();
}
/* Synchronize windows when notes are enabled */
function setupNotesSync() {
    if (!notesOn)
        return;
    setupNotes();
    storeSlide();
    window.addEventListener('storage', updateOtherWindow, false);
}
// An update to local storage is caught only by the other window.
// The triggering window does not handle any sync actions.
function updateOtherWindow(e) {
    // Ignore remove storage events which are not meant to update the other window
    if (!e.newValue)
        return;
    const destSlide = storedSlide();
    while (destSlide > curSlide && curSlide < slideEls.length - 1) {
        nextSlide();
    }
    while (destSlide < curSlide && curSlide > 0) {
        prevSlide();
    }
    updatePlay(e);
    updateNotes();
}
getCurSlideFromHash();
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', handleDomLoaded, false);
}
else {
    handleDomLoaded();
}
