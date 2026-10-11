// Values that templates/slides.tmpl defines as globals before the scripts
// run. Declaring them here has tsc report a script that declares them again,
// which browsers reject.

// notesEnabled reports whether present runs with -notes.
declare var notesEnabled: boolean | undefined;
// preloadEnabled reports whether present preloads slide media.
declare var preloadEnabled: boolean | undefined;
