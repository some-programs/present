// This module shows the source of the current slide beside it, with -doc.
// The server renders the source of every slide into the #source panel, a
// block for each, and a last one for slides without a source; this module
// shows the block of the current slide. S opens and closes the panel.
const OPEN_CLASS = 'source-open';
// Remembers an open panel across reloads, such as with -hot.
const OPEN_KEY = 'present-source-open';
let panel;
let blocks = [];
let shown = -1;
function isOpen() {
    return document.documentElement.classList.contains(OPEN_CLASS);
}
function setOpen(open) {
    document.documentElement.classList.toggle(OPEN_CLASS, open);
    try {
        if (open)
            sessionStorage.setItem(OPEN_KEY, '1');
        else
            sessionStorage.removeItem(OPEN_KEY);
    }
    catch {
        // Storage may be unavailable; the panel then opens closed.
    }
}
export function toggleSource() {
    setOpen(!isOpen());
}
export function closeSource() {
    if (isOpen())
        setOpen(false);
}
// updateSource shows the source of slide no, counted from 0.
export function updateSource(no) {
    const i = Math.min(no, blocks.length - 1);
    if (i === shown)
        return;
    blocks.forEach((b, j) => (b.hidden = j !== i));
    panel.scrollTop = 0;
    shown = i;
}
// setupSource sets up the source panel, and reports whether the page has
// one.
export function setupSource() {
    const el = document.getElementById('source');
    if (!el)
        return false;
    panel = el;
    blocks = Array.from(panel.querySelectorAll(':scope > .src-slide'));
    let open = false;
    try {
        open = sessionStorage.getItem(OPEN_KEY) === '1';
    }
    catch {
        // As if never opened.
    }
    setOpen(open);
    return true;
}
