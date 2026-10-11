"use strict";
// This module reloads the page when present runs with -hot and the
// presentation, or a file it uses, changes. The reloaded slides open at the
// slide that changed (see changedSlide). If the presentation no longer
// renders, the page stays as it is and shows the error instead. Errors
// rendering diagrams are shown the same way.
//
// The templates load this module before slides.js, so that it sees the
// slides as the server sent them, before the other modules change them.
const slidesSelector = 'section.slides > article';
const sentSlides = Array.from(document.querySelectorAll(slidesSelector), (el) => el.outerHTML);
// changedSlide returns the index of the slide to open in doc, or null to
// stay on the current one. It returns the first slide that differs from the
// slides on the page, except that it keeps the current slide if that is one
// of them, as when a file used on several slides changes. That holds only
// while the number of slides stays the same: adding or removing a slide
// moves the ones after it, so then the first difference is the slide that
// was edited.
function changedSlide(doc) {
    const slides = Array.from(doc.querySelectorAll(slidesSelector), (el) => el.outerHTML);
    if (slides.length === 0)
        return null;
    // As in slides.js, #1 is the title slide.
    const cur = (parseInt(location.hash.slice(1), 10) || 1) - 1;
    if (slides.length === sentSlides.length &&
        sentSlides[cur] !== slides[cur]) {
        return cur;
    }
    const n = Math.max(sentSlides.length, slides.length);
    for (let i = 0; i < n; i++) {
        if (sentSlides[i] !== slides[i])
            return Math.min(i, slides.length - 1);
    }
    return null;
}
function showError(text) {
    let el = document.getElementById('hot-error');
    if (!el) {
        el = document.createElement('pre');
        el.id = 'hot-error';
        el.style.cssText =
            'position: fixed; top: 0; left: 0; right: 0; z-index: 1000; margin: 0;' +
                'padding: 12px 20px; max-height: 40vh; overflow: auto;' +
                'white-space: pre-wrap; font: 14px/1.4 "Roboto Mono", monospace;' +
                'color: #fff; background: #b00020;';
        document.body.appendChild(el);
    }
    el.textContent = text;
}
// Show why diagrams could not be rendered, since without -notes there is no
// presenter notes window to show it when an earlier rendering is shown
// instead.
const imageErrors = new Set(Array.from(document.querySelectorAll('.image-error'), (el) => el.textContent));
if (imageErrors.size > 0)
    showError(Array.from(imageErrors).join('\n\n'));
async function reload() {
    let res;
    let text;
    try {
        res = await fetch(location.pathname, { cache: 'no-store' });
        text = await res.text();
    }
    catch {
        return;
    }
    if (!res.ok) {
        showError(text);
        return;
    }
    const slide = changedSlide(new DOMParser().parseFromString(text, 'text/html'));
    if (slide !== null)
        history.replaceState(null, '', '#' + (slide + 1));
    location.reload();
}
const events = new EventSource('/_hot?doc=' + encodeURIComponent(decodeURIComponent(location.pathname)));
events.addEventListener('change', reload);
// After the server restarts, reload in case anything changed while it was
// down.
let lost = false;
events.addEventListener('error', () => {
    lost = true;
});
events.addEventListener('open', () => {
    if (lost)
        reload();
});
