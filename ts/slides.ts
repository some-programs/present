// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This module runs slide presentations: navigation, scaling to the window,
// and syncing with the presenter notes window.

import {
  destSlideKey,
  setupNotes,
  storedSlide,
  toggleNotesWindow,
  updateNotes,
  updatePlay,
} from './notes.js';

const SLIDE_CLASSES = ['far-past', 'past', 'current', 'next', 'far-next'];

const PM_TOUCH_SENSITIVITY = 15;

// The size of the slide canvas, in CSS pixels; see styles.css.
const SLIDE_WIDTH = 1250;
const SLIDE_HEIGHT = 750;

// SlideEvent is dispatched on a slide as 'slideenter' or 'slideleave'.
interface SlideEvent extends Event {
  slideNumber: number;
}

const notesEnabled = !!window.notesEnabled;

let slideEls: NodeListOf<HTMLElement>;
let curSlide = 0;

/* Slide movement */

function hideHelpText() {
  const help = document.getElementById('help');
  if (help) help.style.display = 'none';
}

function getSlideEl(no: number): HTMLElement | null {
  if (no < 0 || no >= slideEls.length) {
    return null;
  }
  return slideEls[no];
}

function updateSlideClass(slideNo: number, className?: string) {
  const el = getSlideEl(slideNo);
  if (!el) return;

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

  updateHash();
}

function storeSlide() {
  if (notesEnabled) localStorage.setItem(destSlideKey(), String(curSlide));
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

function triggerSlideEvent(no: number, type: string, attr: string) {
  const el = getSlideEl(no);
  if (!el) return;

  const handler = el.getAttribute(attr);
  if (handler) {
    new Function(handler).call(el);
  }

  const evt = new Event(type, { bubbles: true, cancelable: true });
  (evt as SlideEvent).slideNumber = no + 1; // Make it readable
  el.dispatchEvent(evt);
}

function triggerEnterEvent(no: number) {
  triggerSlideEvent(no, 'slideenter', 'onslideenter');
}

function triggerLeaveEvent(no: number) {
  triggerSlideEvent(no, 'slideleave', 'onslideleave');
}

/* Touch events */

let touchStartX = 0;
let touchStartY = 0;
let touchDX = 0;
let touchDY = 0;

function handleTouchStart(event: TouchEvent) {
  if (event.touches.length === 1) {
    touchDX = 0;
    touchDY = 0;

    touchStartX = event.touches[0].pageX;
    touchStartY = event.touches[0].pageY;

    document.body.addEventListener('touchmove', handleTouchMove, true);
    document.body.addEventListener('touchend', handleTouchEnd, true);
  }
}

function handleTouchMove(event: TouchEvent) {
  if (event.touches.length > 1) {
    cancelTouch();
  } else {
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
    } else {
      nextSlide();
    }
  }

  cancelTouch();
}

function cancelTouch() {
  document.body.removeEventListener('touchmove', handleTouchMove, true);
  document.body.removeEventListener('touchend', handleTouchEnd, true);
}

/* Preloading frames */

// The real src of each iframe, which is unloaded while its slide is far away.
const frameSrc = new WeakMap<HTMLIFrameElement, string>();

function slideFrames(no: number): HTMLIFrameElement[] {
  const el = getSlideEl(no);
  return el ? Array.from(el.getElementsByTagName('iframe')) : [];
}

function disableSlideFrames(no: number) {
  slideFrames(no).forEach(disableFrame);
}

function enableSlideFrames(no: number) {
  slideFrames(no).forEach(enableFrame);
}

function disableFrame(frame: HTMLIFrameElement) {
  frame.src = 'about:blank';
}

function enableFrame(frame: HTMLIFrameElement) {
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
  ] as const) {
    const el = document.createElement('div');
    el.className = 'slide-area';
    el.id = id;
    el.addEventListener('click', onClick, false);
    slides?.appendChild(el);
  }

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

function handleBodyKeyDown(event: KeyboardEvent) {
  // If we're in a code element, only handle PgUp/PgDn.
  const target = event.target;
  const inCode = target instanceof Element && target.classList.contains('code');

  switch (event.key) {
    case 'n':
    case 'N':
      // Open the presenter notes window.
      if (!inCode && notesEnabled) toggleNotesWindow();
      break;
    case 'h':
    case 'H':
    case 'Escape':
      // Hide the help text.
      if (!inCode) hideHelpText();
      break;

    case 'PageDown':
      nextSlide();
      event.preventDefault();
      break;
    case 'ArrowRight':
    case 'ArrowDown':
    case 'Enter':
    case ' ':
      if (inCode) break;
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
      if (inCode) break;
      prevSlide();
      event.preventDefault();
      break;
  }
}

function slidesSection(): HTMLElement | null {
  return document.querySelector<HTMLElement>('section.slides');
}

function scaleSmallViewports() {
  const el = slidesSection();
  if (!el) return;
  const sAspectRatio = SLIDE_WIDTH / SLIDE_HEIGHT;
  const wAspectRatio = window.innerWidth / window.innerHeight;

  let transform = '';
  if (wAspectRatio <= sAspectRatio && window.innerWidth < SLIDE_WIDTH) {
    transform = 'scale(' + window.innerWidth / SLIDE_WIDTH + ')';
  } else if (window.innerHeight < SLIDE_HEIGHT) {
    transform = 'scale(' + window.innerHeight / SLIDE_HEIGHT + ')';
  }
  el.style.transform = transform;
}

function addEventListeners() {
  document.addEventListener('keydown', handleBodyKeyDown, false);

  let resizeTimeout: number | undefined;
  window.addEventListener('resize', () => {
    // throttle resize events
    window.clearTimeout(resizeTimeout);
    resizeTimeout = window.setTimeout(() => {
      resizeTimeout = undefined;
      scaleSmallViewports();
    }, 50);
  });

  // Reset the scaling of the slides when printing.
  const beforePrint = () => {
    const el = slidesSection();
    if (el) el.style.transform = '';
  };
  window.addEventListener('beforeprint', beforePrint);
  window.matchMedia('print').addEventListener('change', (mql) => {
    if (mql.matches) beforePrint();
  });
}

/* Initialization */

function addGeneralStyle() {
  const style = document.createElement('link');
  style.rel = 'stylesheet';
  style.href = '/static/styles.css';
  document.body.appendChild(style);

  const viewport = document.createElement('meta');
  viewport.name = 'viewport';
  viewport.content = 'width=device-width,height=device-height,initial-scale=1';
  document.head.appendChild(viewport);

  const appleCapable = document.createElement('meta');
  appleCapable.name = 'apple-mobile-web-app-capable';
  appleCapable.content = 'yes';
  document.head.appendChild(appleCapable);

  scaleSmallViewports();
}

function handleDomLoaded() {
  slideEls = document.querySelectorAll<HTMLElement>(
    'section.slides > article'
  );

  setupFrames();

  addGeneralStyle();
  addEventListeners();

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
  if (!notesEnabled) return;
  setupNotes();
  storeSlide();
  window.addEventListener('storage', updateOtherWindow, false);
}

// An update to local storage is caught only by the other window.
// The triggering window does not handle any sync actions.
function updateOtherWindow(e: StorageEvent) {
  // Ignore remove storage events which are not meant to update the other window
  if (!e.newValue) return;

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
} else {
  handleDomLoaded();
}
