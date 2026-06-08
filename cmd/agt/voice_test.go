// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockSTT serves a fixed transcript at /audio/transcriptions.
func mockSTT(t *testing.T, transcript string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, `{"text":"`+transcript+`"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTranscribe_FileToText(t *testing.T) {
	t.Setenv("AGEZT_STT_API_URL", mockSTT(t, "  hello from a file  "))
	t.Setenv("AGEZT_STT_API_KEY", "sk-test")

	dir := t.TempDir()
	audio := filepath.Join(dir, "clip.wav")
	if err := os.WriteFile(audio, []byte("RIFFfake-wav"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := cmdTranscribe([]string{audio}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "hello from a file" {
		t.Errorf("transcript = %q", out.String())
	}
}

func TestTranscribe_JSON(t *testing.T) {
	t.Setenv("AGEZT_STT_API_URL", mockSTT(t, "json please"))
	dir := t.TempDir()
	audio := filepath.Join(dir, "a.wav")
	_ = os.WriteFile(audio, []byte("x"), 0o644)
	var out, errb bytes.Buffer
	if code := cmdTranscribe([]string{audio, "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"text"`) || !strings.Contains(out.String(), "json please") {
		t.Errorf("json output = %s", out.String())
	}
}

func TestTranscribe_MissingFileAndNoArg(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdTranscribe(nil, &out, &errb); code != 2 {
		t.Errorf("no-arg exit = %d, want 2", code)
	}
	out.Reset()
	errb.Reset()
	if code := cmdTranscribe([]string{filepath.Join(t.TempDir(), "nope.wav")}, &out, &errb); code != 1 {
		t.Errorf("missing-file exit = %d, want 1", code)
	}
}

func TestListen_RecordsThenTranscribes(t *testing.T) {
	t.Setenv("AGEZT_STT_API_URL", mockSTT(t, "spoken words"))
	t.Setenv("AGEZT_VOICE_RECORD_CMD", "fake-recorder -d {seconds} -o {out}")

	// Inject a recorder that writes fake audio to the requested output path.
	orig := recordFunc
	defer func() { recordFunc = orig }()
	var gotCmdline []string
	recordFunc = func(_ context.Context, cmdline []string, out string, _ io.Writer) error {
		gotCmdline = cmdline
		return os.WriteFile(out, []byte("RIFFfake"), 0o644)
	}

	var out, errb bytes.Buffer
	code := cmdListen([]string{"--seconds", "3"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "spoken words" {
		t.Errorf("transcript = %q", out.String())
	}
	// {seconds}/{out} were substituted in the recorder argv.
	joined := strings.Join(gotCmdline, " ")
	if !strings.Contains(joined, "-d 3") || strings.Contains(joined, "{out}") {
		t.Errorf("recorder argv not substituted: %v", gotCmdline)
	}
}

func TestListen_NoRecordCmdErrors(t *testing.T) {
	t.Setenv("AGEZT_VOICE_RECORD_CMD", "")
	var out, errb bytes.Buffer
	if code := cmdListen(nil, &out, &errb); code != 2 {
		t.Errorf("exit = %d, want 2 (no recorder configured)", code)
	}
	if !strings.Contains(errb.String(), "AGEZT_VOICE_RECORD_CMD") {
		t.Errorf("expected guidance about AGEZT_VOICE_RECORD_CMD, got %s", errb.String())
	}
}

func TestSubstituteRecord(t *testing.T) {
	got := substituteRecord("arecord -d {seconds} -t wav {out}", 7, "/tmp/x.wav")
	want := []string{"arecord", "-d", "7", "-t", "wav", "/tmp/x.wav"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}
