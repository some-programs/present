# Fonts

Roboto and JetBrains Mono as variable WOFF2 files, mirrored from the Google
Fonts css2 API (Roboto v51 on 2026-10-09, JetBrains Mono v24 on 2026-10-11).
Each file holds one script subset (latin, latin-ext, cyrillic, greek,
vietnamese, ...); `../fonts.css` maps them with `unicode-range`, so browsers
fetch only the subsets a page uses.

Files are named `<family>-<style>-<subset>.woff2`.

Both families are licensed under the SIL Open Font License 1.1:
`OFL-Roboto.txt` (github.com/googlefonts/roboto-classic) and
`OFL-JetBrainsMono.txt` (github.com/JetBrains/JetBrainsMono).

`fonts.css` uses `font-display: block` so slides never flash a fallback
font, and `templates/slides.tmpl` preloads `roboto-normal-latin.woff2` and
`jetbrainsmono-normal-latin.woff2`.

`ttf/` holds the same families as TrueType files for installing on a
computer, which present serves as `/download/fonts.tar`: Roboto v3.016 (the
`hinted/static` directory of the github.com/googlefonts/roboto-classic
release) and JetBrains Mono v2.304 (`fonts/ttf` of the
github.com/JetBrains/JetBrainsMono release, without the JetBrainsMonoNL
files). They are static fonts, one file per weight and style, rather than
the variable fonts of the releases, because every program supports them,
including those that render diagrams. The file names are the upstream ones.

To refresh the WOFF2 files, fetch this URL with a current browser's User-Agent:

    https://fonts.googleapis.com/css2?family=Roboto:ital,wdth,wght@0,75..100,100..900;1,75..100,100..900&family=JetBrains+Mono:ital,wght@0,100..800;1,100..800&display=swap

Download every `src` URL it lists, point `fonts.css` at the local copies and
change `font-display: swap` to `font-display: block`. Keep the preloaded file
names in `templates/slides.tmpl` in step.
