// This script shows the source of the current slide beside it, with -doc.
// The server renders the source of every slide into the #source panel, a
// block for each, and a last one for slides without a source; this script
// shows the block of the current slide. S opens and closes the panel, and
// ?source=true or ?source=false in the URL opens or closes it on loading.

const OPEN_CLASS = 'source-open';
// Remembers an open panel across reloads, such as with -hot.
const OPEN_KEY = 'present-source-open';

let panel: HTMLElement;
let blocks: HTMLElement[] = [];
let shown = -1;

function isOpen(): boolean {
  return document.documentElement.classList.contains(OPEN_CLASS);
}

function setOpen(open: boolean) {
  document.documentElement.classList.toggle(OPEN_CLASS, open);
  try {
    if (open) sessionStorage.setItem(OPEN_KEY, '1');
    else sessionStorage.removeItem(OPEN_KEY);
  } catch {
    // Storage may be unavailable; the panel then opens closed.
  }
}

function toggleSource() {
  setOpen(!isOpen());
}

function closeSource() {
  if (isOpen()) setOpen(false);
}

// updateSource shows the source of slide no, counted from 0.
function updateSource(no: number) {
  const i = Math.min(no, blocks.length - 1);
  if (i === shown) return;
  blocks.forEach((b, j) => (b.hidden = j !== i));
  panel.scrollTop = 0;
  shown = i;
}

// setupSource sets up the source panel, and reports whether the page has
// one.
function setupSource(): boolean {
  const el = document.getElementById('source');
  if (!el) return false;
  panel = el;
  blocks = Array.from(
    panel.querySelectorAll<HTMLElement>(':scope > .src-slide')
  );
  let open = false;
  try {
    open = sessionStorage.getItem(OPEN_KEY) === '1';
  } catch {
    // As if never opened.
  }
  // The source parameter decides when the page is opened, but not when it
  // is reloaded, such as with -hot, which keeps the panel as it was left.
  const param = sourceParam();
  if (param !== null && !isReload()) open = param;
  setOpen(open);
  return true;
}

// sourceParam reports whether the URL asks for the panel to be open, with
// ?source=true, or closed, with ?source=false, or returns null if it does
// not ask.
function sourceParam(): boolean | null {
  switch (new URLSearchParams(location.search).get('source')) {
    case 'true':
      return true;
    case 'false':
      return false;
  }
  return null;
}

// isReload reports whether the page was reloaded, rather than opened.
function isReload(): boolean {
  const [nav] = performance.getEntriesByType(
    'navigation'
  ) as PerformanceNavigationTiming[];
  return nav?.type === 'reload';
}
