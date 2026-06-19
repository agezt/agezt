// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// These exercise the validation paths, which return before any daemon dial.
func TestProviderConnect_Validation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
		out  string // substring expected on stdout+stderr
	}{
		{"no args prints usage", nil, 2, "usage:"},
		{"-h prints usage ok", []string{"-h"}, 0, "provider connect"},
		{"missing url/model", []string{"deepseek"}, 2, "--url and --model are required"},
		{"key without env", []string{"x", "--url", "https://a", "--model", "m", "--key", "sk"}, 2, "--key needs --env"},
		{"unknown flag", []string{"x", "--bogus", "v"}, 2, "unknown flag"},
		{"flag missing value", []string{"x", "--url"}, 2, "needs a value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := cmdProviderConnect(c.args, &out, &errb)
			if code != c.want {
				t.Fatalf("code = %d, want %d", code, c.want)
			}
			if c.out != "" && !strings.Contains(out.String()+errb.String(), c.out) {
				t.Fatalf("output %q missing %q", out.String()+errb.String(), c.out)
			}
		})
	}
}
