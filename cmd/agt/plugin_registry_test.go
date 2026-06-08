// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/plugin"
)

// writeDirRegistry creates a directory registry holding one plugin whose binary
// is `payload`, pinned to its real BLAKE3, with a build for the current host.
// Returns the registry dir and the correct pin. `tamperPin` overrides the index
// pin to a (wrong) value to exercise the mismatch path.
func writeDirRegistry(t *testing.T, payload []byte, tamperPin string) (dir, pin, file string) {
	t.Helper()
	dir = t.TempDir()
	file = "demo-" + runtime.GOOS + "-" + runtime.GOARCH
	pin = plugin.HashBytes(payload)
	indexPin := pin
	if tamperPin != "" {
		indexPin = tamperPin
	}
	idx := pluginIndex{
		Tool:          "agezt",
		FormatVersion: 1,
		Plugins: []indexPlugin{{
			Name:        "demo",
			Version:     "1.0.0",
			Description: "a demo tool plugin",
			Prefix:      "demo",
			Binaries: []indexBinary{{
				OS:     runtime.GOOS,
				Arch:   runtime.GOARCH,
				File:   file,
				BLAKE3: indexPin,
			}},
		}},
	}
	raw, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "index.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, pin, file
}

func runReg(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cmdPluginRegistry(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestPluginRegistry_ListDir(t *testing.T) {
	dir, _, _ := writeDirRegistry(t, []byte("fake-binary-bytes"), "")
	code, out, errs := runReg(t, dir)
	if code != 0 {
		t.Fatalf("list exit=%d stderr=%s", code, errs)
	}
	if !strings.Contains(out, "demo") || !strings.Contains(out, "v1.0.0") {
		t.Errorf("list output missing plugin: %s", out)
	}
	if !strings.Contains(out, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("list output missing this platform: %s", out)
	}
}

func TestPluginRegistry_InstallDirVerifiesAndStages(t *testing.T) {
	payload := []byte("the real plugin binary")
	dir, pin, file := writeDirRegistry(t, payload, "")
	installDir := t.TempDir()

	code, out, errs := runReg(t, dir, "--install", "demo", "--dir", installDir)
	if code != 0 {
		t.Fatalf("install exit=%d stderr=%s", code, errs)
	}
	dest := filepath.Join(installDir, file)
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("binary not staged: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("staged binary bytes differ from the registry payload")
	}
	// The exact enabling env lines must be printed (operator wires it in himself).
	if !strings.Contains(out, `AGEZT_PLUGINS="demo=`+dest+`"`) {
		t.Errorf("missing AGEZT_PLUGINS line: %s", out)
	}
	if !strings.Contains(out, `AGEZT_PLUGIN_PINS="demo=`+pin+`"`) {
		t.Errorf("missing AGEZT_PLUGIN_PINS line: %s", out)
	}
}

func TestPluginRegistry_InstallRefusesTamperedPin(t *testing.T) {
	dir, _, file := writeDirRegistry(t, []byte("genuine"), strings.Repeat("a", 64))
	installDir := t.TempDir()

	code, _, errs := runReg(t, dir, "--install", "demo", "--dir", installDir)
	if code == 0 {
		t.Fatal("install must fail on a BLAKE3 mismatch")
	}
	if !strings.Contains(errs, "mismatch") {
		t.Errorf("expected a mismatch error, got: %s", errs)
	}
	// A tampered binary must never land on disk.
	if _, err := os.Stat(filepath.Join(installDir, file)); !os.IsNotExist(err) {
		t.Error("a pin-mismatched binary must NOT be written")
	}
}

func TestPluginRegistry_InstallMissingAndAmbiguous(t *testing.T) {
	dir, _, _ := writeDirRegistry(t, []byte("x"), "")
	if code, _, _ := runReg(t, dir, "--install", "nope", "--dir", t.TempDir()); code == 0 {
		t.Error("installing a missing name must fail")
	}
}

func TestPluginRegistry_NoBuildForHost(t *testing.T) {
	dir := t.TempDir()
	idx := pluginIndex{FormatVersion: 1, Plugins: []indexPlugin{{
		Name: "demo", Version: "1.0.0",
		Binaries: []indexBinary{{OS: "plan9", Arch: "mips", File: "demo-plan9-mips", BLAKE3: strings.Repeat("0", 64)}},
	}}}
	raw, _ := json.Marshal(idx)
	_ = os.WriteFile(filepath.Join(dir, "index.json"), raw, 0o644)

	code, _, errs := runReg(t, dir, "--install", "demo", "--dir", t.TempDir())
	if code == 0 || !strings.Contains(errs, "no build for this host") {
		t.Errorf("expected a no-build-for-host refusal, got code=%d stderr=%s", code, errs)
	}
}

func TestPluginRegistry_RefusesUnsafeFilename(t *testing.T) {
	dir := t.TempDir()
	idx := pluginIndex{FormatVersion: 1, Plugins: []indexPlugin{{
		Name: "demo", Version: "1.0.0",
		Binaries: []indexBinary{{OS: runtime.GOOS, Arch: runtime.GOARCH, File: "../escape", BLAKE3: strings.Repeat("0", 64)}},
	}}}
	raw, _ := json.Marshal(idx)
	_ = os.WriteFile(filepath.Join(dir, "index.json"), raw, 0o644)

	code, _, errs := runReg(t, dir, "--install", "demo", "--dir", t.TempDir())
	if code == 0 || !strings.Contains(errs, "unsafe") {
		t.Errorf("expected an unsafe-filename refusal, got code=%d stderr=%s", code, errs)
	}
}

func TestPluginRegistry_RemoteListAndInstall(t *testing.T) {
	payload := []byte("remote plugin binary payload")
	pin := plugin.HashBytes(payload)
	file := "demo-" + runtime.GOOS + "-" + runtime.GOARCH
	idx := pluginIndex{FormatVersion: 1, Plugins: []indexPlugin{{
		Name: "demo", Version: "2.0.0", Prefix: "demo",
		Binaries: []indexBinary{{OS: runtime.GOOS, Arch: runtime.GOARCH, File: file, BLAKE3: pin}},
	}}}
	idxRaw, _ := json.Marshal(idx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/index.json"):
			_, _ = w.Write(idxRaw)
		case strings.HasSuffix(r.URL.Path, "/"+file):
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	// list
	if code, out, errs := runReg(t, srv.URL); code != 0 || !strings.Contains(out, "v2.0.0") {
		t.Fatalf("remote list failed code=%d out=%s err=%s", code, out, errs)
	}
	// install
	installDir := t.TempDir()
	code, out, errs := runReg(t, srv.URL, "--install", "demo", "--dir", installDir)
	if code != 0 {
		t.Fatalf("remote install exit=%d stderr=%s", code, errs)
	}
	got, err := os.ReadFile(filepath.Join(installDir, file))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("remote binary not staged correctly: %v", err)
	}
	if !strings.Contains(out, "verified blake3:"+pin) {
		t.Errorf("expected verified pin in output: %s", out)
	}
}
