// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// invokeLive runs one program through the real warden (actually executes the
// interpreter) and returns the rendered result. Skips nothing — callers gate on
// runtime availability.
func invokeLive(t *testing.T, tool *Tool, lang, code string) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"language": lang, "code": code})
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("%s Invoke hard error: %v", lang, err)
	}
	return res.Output, res.IsError
}

// bodyAfterHeader strips the one-line "[code_exec] language=… isolation=…" header
// render() prepends, leaving just the program's own output.
func bodyAfterHeader(out string) string {
	_, body, _ := strings.Cut(out, "\n")
	return body
}

// TestCodeExec_LiveRuntimeParity actually runs each DETECTED runtime end-to-end
// and asserts the two properties that must hold identically across all of them:
// (1) clean output — the program's printed line is exactly what comes back, no
// interpreter chatter (the regression guard for the Windows py-launcher noise,
// M685, now enforced for every language); (2) the secret scrub holds — a
// secret-shaped env var in the daemon's environment never reaches the code.
// Languages whose interpreter isn't installed are skipped, so this is safe in CI.
func TestCodeExec_LiveRuntimeParity(t *testing.T) {
	rt := DetectRuntimes()
	if len(rt) == 0 {
		t.Skip("no code_exec runtimes installed")
	}
	// A secret in the parent (daemon) environment that must NOT leak into code.
	t.Setenv("AGEZT_SECRET_PROBE", "leakme-do-not-show")

	type prog struct{ marker, probe string }
	progs := map[string]prog{
		LangPython: {
			marker: `print("PARITY-OK")`,
			probe:  `import os; print(os.environ.get("AGEZT_SECRET_PROBE", "SCRUBBED"))`,
		},
		LangNode: {
			marker: `console.log("PARITY-OK")`,
			probe:  `console.log(process.env.AGEZT_SECRET_PROBE || "SCRUBBED")`,
		},
		LangDeno: {
			marker: `console.log("PARITY-OK")`,
			probe:  `console.log(Deno.env.get("AGEZT_SECRET_PROBE") ?? "SCRUBBED")`,
		},
	}

	for lang := range rt {
		p, ok := progs[lang]
		if !ok {
			continue
		}

		t.Run(lang+"/clean-output", func(t *testing.T) {
			tool := New(t.TempDir(), rt)
			out, isErr := invokeLive(t, tool, lang, p.marker)
			if isErr {
				t.Fatalf("%s errored:\n%s", lang, out)
			}
			// The body must be EXACTLY the printed line — no install/compile chatter.
			if body := strings.TrimSpace(bodyAfterHeader(out)); body != "PARITY-OK" {
				t.Errorf("%s output not clean: want \"PARITY-OK\", got %q\nfull:\n%s", lang, body, out)
			}
		})

		t.Run(lang+"/secret-scrub", func(t *testing.T) {
			tool := New(t.TempDir(), rt)
			out, isErr := invokeLive(t, tool, lang, p.probe)
			if isErr {
				t.Fatalf("%s errored:\n%s", lang, out)
			}
			if strings.Contains(out, "leakme-do-not-show") {
				t.Errorf("%s LEAKED the secret env var into code:\n%s", lang, out)
			}
			if !strings.Contains(out, "SCRUBBED") {
				t.Errorf("%s should print SCRUBBED (the var must be absent), got:\n%s", lang, out)
			}
		})
	}
}
