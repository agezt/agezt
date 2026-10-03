// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/fetch"
	httptool "github.com/agezt/agezt/plugins/tools/http"
)

type nopIndex struct{}

func (nopIndex) PutEntry(e artifact.Entry, _ []byte, _ int64) (artifact.Entry, error) { return e, nil }

// fetch is governed as http.get — the same capability as the http tool's GET —
// so AGEZT_HTTP_ALLOWED_HOSTS must restrict it too. It used to be ignored: with
// http pinned to one host, fetch still downloaded from anywhere.
func TestFetchHonoursTheHTTPAllowlist(t *testing.T) {
	deps := func(env map[string]string) toolreg.BuildDeps {
		return toolreg.BuildDeps{Stderr: &bytes.Buffer{}, Get: func(k string) string { return env[k] }}
	}
	invoke := func(env map[string]string, url string) string {
		t.Helper()
		built, err := specFetch().Build(deps(env))
		if err != nil {
			t.Fatal(err)
		}
		fe := built.Tool.(*fetch.Tool)
		fe.SetIndex(nopIndex{})
		in, _ := json.Marshal(map[string]string{"url": url})
		res, err := fe.Invoke(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		return res.Output
	}

	pinned := map[string]string{"AGEZT_HTTP_ALLOWED_HOSTS": "api.example.com"}
	if out := invoke(pinned, "https://elsewhere.invalid/file.bin"); !strings.Contains(out, "host not in allowlist") {
		t.Fatalf("fetch reached a host outside AGEZT_HTTP_ALLOWED_HOSTS: %s", out)
	}
	// Unpinned stays default-allow: the refusal (if any) is the network, not the allowlist.
	if out := invoke(map[string]string{}, "https://elsewhere.invalid/file.bin"); strings.Contains(out, "host not in allowlist") {
		t.Fatalf("default posture refused a public host: %s", out)
	}

	// Both tools get the same posture from the same settings.
	hb, err := buildHTTP(deps(pinned))
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := specFetch().Build(deps(pinned))
	ht, fe := hb.Tool.(*httptool.Tool), fb.Tool.(*fetch.Tool)
	if ht.AllowAll != fe.AllowAll || strings.Join(ht.AllowedHosts, ",") != strings.Join(fe.AllowedHosts, ",") ||
		ht.AllowLoopback != fe.AllowLoopback || ht.AllowPrivate != fe.AllowPrivate {
		t.Fatalf("http and fetch postures differ: http=%+v fetch=%+v", ht, fe)
	}
}
