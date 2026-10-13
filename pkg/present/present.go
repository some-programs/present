// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"
)

var (
	parsers = make(map[string]ParseFunc)
	funcs   = template.FuncMap{}
)

// Template returns an empty template with the action functions in its FuncMap.
func Template() *template.Template {
	return template.New("").Funcs(funcs)
}

// Render renders the doc to the given writer using the provided template.
func (d *Doc) Render(w io.Writer, t *template.Template) error {
	data := struct {
		*Doc
		Template       *template.Template
		PlayEnabled    bool
		NotesEnabled   bool
		PreloadEnabled bool
	}{d, t, PlayEnabled, NotesEnabled, PreloadEnabled}
	return t.ExecuteTemplate(w, "root", data)
}

// Render renders the section to the given writer using the provided template.
func (s *Section) Render(w io.Writer, t *template.Template) error {
	data := struct {
		*Section
		Template    *template.Template
		PlayEnabled bool
	}{s, t, PlayEnabled}
	return t.ExecuteTemplate(w, "section", data)
}

type ParseFunc func(ctx *Context, fileName string, lineNumber int, inputLine string) (Elem, error)

// Register binds the named action, which does not begin with a period, to the
// specified parser to be invoked when the name, with a period, appears in the
// present input text.
func Register(name string, parser ParseFunc) {
	if len(name) == 0 || name[0] == ';' {
		panic("bad name in Register: " + name)
	}
	parsers["."+name] = parser
}

// Doc represents an entire document.
type Doc struct {
	Title      string
	Subtitle   string
	Summary    string
	Time       time.Time
	Authors    []Author
	TitleNotes []string // the notes of the title slide; see Section.Notes
	Sections   []Section
	Tags       []string
}

// Author represents the person who wrote and/or is presenting the document.
type Author struct {
	Elem []Elem
}

// TextElem returns the first text elements of the author details.
// This is used to display the author' name, job title, and company
// without the contact details.
func (p *Author) TextElem() (elems []Elem) {
	for _, el := range p.Elem {
		if _, ok := el.(Text); !ok {
			break
		}
		elems = append(elems, el)
	}
	return
}

// Section represents a section of a document (such as a presentation slide)
// comprising a title and a list of elements.
type Section struct {
	Number  []int
	Title   string
	ID      string // HTML anchor ID
	Elem    []Elem
	Notes   []string // Markdown, each a line of notes or a block of them
	Classes []string
	Styles  []string

	// Background is the image of a .background command, if any.
	Background string

	// Line is the line number of the section's heading in the file.
	Line int
}

// HTMLAttributes for the section
func (s Section) HTMLAttributes() template.HTMLAttr {
	if len(s.Classes) == 0 && len(s.Styles) == 0 {
		return ""
	}

	var class string
	if len(s.Classes) > 0 {
		class = fmt.Sprintf(`class=%q`, strings.Join(s.Classes, " "))
	}
	var style string
	if len(s.Styles) > 0 {
		style = fmt.Sprintf(`style=%q`, strings.Join(s.Styles, " "))
	}
	return template.HTMLAttr(strings.Join([]string{class, style}, " "))
}

// Sections contained within the section.
func (s Section) Sections() (sections []Section) {
	for _, e := range s.Elem {
		if section, ok := e.(Section); ok {
			sections = append(sections, section)
		}
	}
	return
}

// Level returns the level of the given section.
// The document title is level 1, main section 2, etc.
func (s Section) Level() int {
	return len(s.Number) + 1
}

// FormattedNumber returns a string containing the concatenation of the
// numbers identifying a Section.
func (s Section) FormattedNumber() string {
	b := &bytes.Buffer{}
	for _, n := range s.Number {
		fmt.Fprintf(b, "%v.", n)
	}
	return b.String()
}

func (s Section) TemplateName() string { return "section" }

// Elem defines the interface for a present element. That is, something that
// can provide the name of the template used to render the element.
type Elem interface {
	TemplateName() string
}

// renderElem implements the elem template function, used to render
// sub-templates.
func renderElem(t *template.Template, e Elem) (template.HTML, error) {
	var data any = e
	if s, ok := e.(Section); ok {
		data = struct {
			Section
			Template *template.Template
		}{s, t}
	}
	return execTemplate(t, e.TemplateName(), data)
}

// renderElems implements the elems template function, which renders a list
// of elements. Each run of elements after a FontSize is wrapped in a div of
// class "fontsize" that sets its size.
func renderElems(t *template.Template, elems []Elem) (template.HTML, error) {
	var b strings.Builder
	open := false
	for _, e := range elems {
		if f, ok := e.(FontSize); ok {
			if open {
				b.WriteString("</div>")
			}
			// f.Size is safe in the attribute: parseFontSize made it.
			fmt.Fprintf(&b, `<div class="fontsize" style="font-size: %s">`, f.Size)
			open = true
			continue
		}
		html, err := renderElem(t, e)
		if err != nil {
			return "", err
		}
		b.WriteString(string(html))
	}
	if open {
		b.WriteString("</div>")
	}
	return template.HTML(b.String()), nil
}

// pageNum derives a page number from a section.
func pageNum(s Section, offset int) int {
	if len(s.Number) == 0 {
		return offset
	}
	return s.Number[0] + offset
}

func init() {
	funcs["elem"] = renderElem
	funcs["elems"] = renderElems
	funcs["pagenum"] = pageNum
	funcs["markdown"] = renderInlineMarkdown
	funcs["markdownBlock"] = func(s string) (template.HTML, error) { return renderMarkdown([]byte(s)) }
}

// execTemplate is a helper to execute a template and return the output as a
// template.HTML value.
func execTemplate(t *template.Template, name string, data any) (template.HTML, error) {
	b := new(bytes.Buffer)
	err := t.ExecuteTemplate(b, name, data)
	if err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// Text represents a paragraph of plain text.
type Text struct {
	Lines []string
}

func (t Text) TemplateName() string { return "text" }
