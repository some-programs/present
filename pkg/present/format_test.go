// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	for _, tt := range []struct {
		name, in, want string
	}{
		{
			"spaces and line endings",
			"\n\n# Title  \r\nSubtitle \r\n\r\nAda  \r\n\r\n\r\nBob\n\n\n##   Slide   {#id}  \r\nText.  \r\n.code x.go\n\n\n\n: Note.  \n\n\n",
			"# Title\nSubtitle\n\nAda\n\nBob\n\n## Slide {#id}\n\nText.\n\n.code x.go\n\n: Note.\n",
		},
		{
			"apart and together",
			"# T\n\n## Slide\nText.\n.image a.png\n.caption A.\n: One.\n: Two.\nMore text.\n### Sub\nText.\n// About the image.\n.image b.png\n## Next\n",
			"# T\n\n## Slide\n\nText.\n\n.image a.png\n.caption A.\n\n: One.\n: Two.\n\nMore text.\n\n### Sub\n\nText.\n// About the image.\n.image b.png\n\n## Next\n",
		},
		{
			"hard line breaks",
			"# T\n\n## Slide\n\nOne  \nTwo\\\nThree\\  \nFour  \n",
			"# T\n\n## Slide\n\nOne\\\nTwo\\\nThree\\  \nFour\n",
		},
		{
			"code keeps its spaces",
			"# T\n\n## Slide\n\nText:\n\n    code  \n\n\n    more  \n\n```  \nfenced  \n\n\n```\n\n<div>  \n</div>\n",
			"# T\n\n## Slide\n\nText:\n\n    code  \n\n\n    more  \n\n```\nfenced  \n\n\n```\n\n<div>  \n</div>\n",
		},
		{
			"block of notes",
			"# T\n\n## Slide\n\nText.\n  :::notes  \n  a  \n\n\n- b  \n  c\n:::  \nMore.\n",
			"# T\n\n## Slide\n\nText.\n\n::: notes\n  a\n\n- b\\\n  c\n:::\n\nMore.\n",
		},
		{
			"already formatted",
			"# T\nSub\n9 Oct 2026\n\nAda\n\n## Slide\n\nText.\n",
			"# T\nSub\n9 Oct 2026\n\nAda\n\n## Slide\n\nText.\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format("x.slide", []byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Format:\n%s\nwant:\n%s", got, tt.want)
			}
			again, err := Format("x.slide", got)
			if err != nil || string(again) != string(got) {
				t.Errorf("Format is not idempotent: %q, %v", again, err)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	_, err := Format("x.slide", []byte("# T\n\n## Slide\n\n```\nnot closed\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "x.slide:5: fenced code block is not closed") {
		t.Errorf("got error %v, want x.slide:5: fenced code block is not closed", err)
	}
}

// FuzzFormat checks that Format never fails its own check that it does
// not change how a file presents, and that formatting is idempotent.
func FuzzFormat(f *testing.F) {
	for _, pattern := range []string{"testdata/*.md", "../../examples/*.slide", "../../examples/*.article"} {
		files, err := filepath.Glob(pattern)
		if err != nil {
			f.Fatal(err)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				f.Fatal(err)
			}
			data, _, _ = bytes.Cut(data, []byte("\n---\n"))
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		got, err := Format("x.slide", src)
		if err != nil {
			if strings.Contains(err.Error(), "bug in present") {
				t.Fatalf("Format(%q): %v", src, err)
			}
			return
		}
		again, err := Format("x.slide", got)
		if err != nil || !bytes.Equal(again, got) {
			t.Fatalf("Format(%q) = %q, but formatting that gives %q, %v", src, got, again, err)
		}
	})
}
