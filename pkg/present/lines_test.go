// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"slices"
	"strings"
	"testing"
)

func TestScanLines(t *testing.T) {
	var input []string
	var want []LineKind
	for _, tt := range []struct {
		line string
		kind LineKind
	}{
		{"# Title", TitleLine},
		{"Subtitle", HeaderLine},
		{": Title note.", NoteLine},
		{"9 Oct 2026", HeaderLine},
		{"", BlankLine},
		{"Ada", AuthorLine},
		{"// Comment.", CommentLine},
		{"   ", BlankLine},
		{"Bob", AuthorLine},
		{"", BlankLine},
		{"## Slide", HeadingLine},
		{"", BlankLine},
		{"Text", TextLine},
		{"// Comment.", CommentLine},
		{"#hashtag", TextLine},
		{".image x.png", CommandLine},
		{".bogus", CommandLine},
		{": Note.", NoteLine},
		{"", BlankLine},
		{"```", TextLine},
		{"# code", TextLine},
		{"", TextLine},
		{"// code", TextLine},
		{"```", TextLine},
		{"::: notes", NoteLine},
		{"// kept", NoteLine},
		{":::", NoteLine},
		{"### Subsection", HeadingLine},
		{"    indented", TextLine},
		{"", TextLine},
		{"    code", TextLine},
	} {
		input = append(input, tt.line)
		want = append(want, tt.kind)
	}
	lines, err := ScanLines(strings.NewReader(strings.Join(input, "\n")), "x.slide")
	if err != nil {
		t.Fatal(err)
	}
	var got []LineKind
	for i, l := range lines {
		if l.Text != input[i] {
			t.Errorf("line %d: text %q, want %q", i+1, l.Text, input[i])
		}
		got = append(got, l.Kind)
	}
	if !slices.Equal(got, want) {
		for i := range max(len(got), len(want)) {
			if i >= len(got) || i >= len(want) || got[i] != want[i] {
				t.Errorf("line %d %q: kind %v, want %v", i+1, input[i], got[i], want[i])
			}
		}
	}
}

func TestScanLinesError(t *testing.T) {
	_, err := ScanLines(strings.NewReader("# Title\n\n## One\n\n#### Deep\n"), "x.slide")
	if err == nil || !strings.HasPrefix(err.Error(), "x.slide:5: badly nested section") {
		t.Errorf("got error %v, want x.slide:5: badly nested section", err)
	}
}
