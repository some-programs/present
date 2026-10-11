// Values that templates/slides.tmpl defines as globals before the modules run.

interface Window {
  // notesEnabled reports whether present runs with -notes.
  notesEnabled?: boolean;
  // preloadEnabled reports whether present preloads slide media.
  preloadEnabled?: boolean;
}
