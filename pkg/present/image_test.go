package present

import "testing"

func TestParseImage(t *testing.T) {
	tests := []struct {
		cmd  string
		want Image
	}{
		{".image a.png", Image{URL: "a.png"}},
		{".image a.png 0.5", Image{URL: "a.png", WidthFrac: 0.5, HeightFrac: 0.5}},
		{".image a.png _ 0.5", Image{URL: "a.png", WidthFrac: 0.5}},
		{".image a.png 0.25 _", Image{URL: "a.png", HeightFrac: 0.25}},
		{".image a.png _ 900", Image{URL: "a.png", Width: 900}},
		{".image a.png 400 1.0", Image{URL: "a.png", Height: 400, WidthFrac: 1}},
		{".image g/a.dot 0.5", Image{URL: "g/a.svg", Source: "g/a.dot", WidthFrac: 0.5, HeightFrac: 0.5}},
		{".image https://example.com/a.dot", Image{URL: "https://example.com/a.dot"}},
		{".image a.mmd", Image{URL: "a.svg", Source: "a.mmd"}},
		{".image g.v1/a.mermaid _ 0.5", Image{URL: "g.v1/a.svg", Source: "g.v1/a.mermaid", WidthFrac: 0.5}},
	}
	for _, tt := range tests {
		elem, err := parseImage(nil, "x.slide", 1, tt.cmd)
		if err != nil {
			t.Errorf("%s: %v", tt.cmd, err)
			continue
		}
		tt.want.Cmd = tt.cmd
		if got := elem.(Image); got != tt.want {
			t.Errorf("%s:\ngot  %+v\nwant %+v", tt.cmd, got, tt.want)
		}
	}

	for _, cmd := range []string{
		".image",
		".image a.png 500",
		".image a.png 1.5",
		".image a.png 0",
		".image a.png 0.0 _",
		".image a.png _ -3",
		".image a.png _ x",
		".image a.png 1 2 3",
	} {
		if _, err := parseImage(nil, "x.slide", 1, cmd); err == nil {
			t.Errorf("%s: no error", cmd)
		}
	}
}
