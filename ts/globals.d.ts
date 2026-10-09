// Values that templates/slides.tmpl defines as globals before the modules run.

interface PresentSection {
  Notes?: string[] | null;
}

interface Window {
  // notesEnabled reports whether present runs with -notes.
  notesEnabled?: boolean;
  // sections holds the slides' sections; set only when notes are enabled.
  sections?: PresentSection[] | null;
  // titleNotes holds the notes for the title slide; set only when notes are enabled.
  titleNotes?: string[] | null;
}
