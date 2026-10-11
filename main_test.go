package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommand(t *testing.T) {
	for _, tt := range []struct {
		args       []string
		code       int
		out, inErr string // text the output and the error output must contain
	}{
		{[]string{"-h"}, 0, "serve ", ""},
		{[]string{"help"}, 0, "new-slide", ""},
		{[]string{"-notes"}, 2, "", "present serve -notes"},
		{[]string{"serve", "-h"}, 0, "", "-notes"},
		{[]string{"serve", "talks"}, 2, "", `unexpected argument "talks"`},
		{[]string{"export"}, 2, "", "usage: present export"},
		{[]string{"bogus"}, 2, "", `unknown command "bogus"`},
	} {
		var stdout, stderr bytes.Buffer
		code := runCommand(tt.args[0], tt.args[1:], &stdout, &stderr)
		if code != tt.code || !strings.Contains(stdout.String(), tt.out) || !strings.Contains(stderr.String(), tt.inErr) {
			t.Errorf("present %s: exit code %d, output %q, error output %q; want %d, %q and %q",
				strings.Join(tt.args, " "), code, stdout.String(), stderr.String(), tt.code, tt.out, tt.inErr)
		}
	}
}
