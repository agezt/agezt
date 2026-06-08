// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// recordFunc captures `seconds` of audio to the file `out` by running cmdline.
// Injectable so tests exercise `agt listen` without a real microphone.
var recordFunc = execRecord

// cmdListen implements `agt listen [--seconds N] [--model m] [--run] [--json]`:
// record from the microphone via an operator-configured command, transcribe the
// clip, and print it (or drive the agent with `--run`).
//
// Microphone capture has no portable Go path without a CGO audio dependency, so —
// like the tunnel — Agezt drives an operator-chosen recorder via AGEZT_VOICE_RECORD_CMD,
// substituting `{seconds}` and `{out}` (the temp WAV to write). Examples:
//
//	# Linux (ALSA):   arecord -d {seconds} -f cd -t wav {out}
//	# macOS/Windows (ffmpeg): ffmpeg -f avfoundation -i :0 -t {seconds} -y {out}
//	#                          ffmpeg -f dshow -i audio="Microphone" -t {seconds} -y {out}
func cmdListen(args []string, stdout, stderr io.Writer) int {
	seconds := 5
	model := ""
	run := false
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--run":
			run = true
		case a == "--json":
			asJSON = true
		case a == "--seconds":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s listen: --seconds needs a value\n", brand.CLI)
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s listen: --seconds must be a positive integer\n", brand.CLI)
				return 2
			}
			seconds = n
		case a == "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s listen: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s listen [--seconds N] [--model m] [--run] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "record from the microphone, transcribe it, and print (or --run) the text\n")
			fmt.Fprintf(stdout, "  --seconds N  how long to record (default 5)\n")
			fmt.Fprintf(stdout, "  --run        feed the transcript to the agent as an intent\n")
			fmt.Fprintf(stdout, "set AGEZT_VOICE_RECORD_CMD with {seconds} and {out} placeholders, e.g.\n")
			fmt.Fprintf(stdout, "  arecord -d {seconds} -f cd -t wav {out}\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s listen: unexpected argument %q\n", brand.CLI, a)
			return 2
		}
	}

	tmpl := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "VOICE_RECORD_CMD"))
	if tmpl == "" {
		fmt.Fprintf(stderr, "%s listen: set AGEZT_VOICE_RECORD_CMD to your recorder, e.g.\n", brand.CLI)
		fmt.Fprintf(stderr, "  AGEZT_VOICE_RECORD_CMD='arecord -d {seconds} -f cd -t wav {out}'   # Linux\n")
		fmt.Fprintf(stderr, "  AGEZT_VOICE_RECORD_CMD='ffmpeg -f dshow -i audio=\"Microphone\" -t {seconds} -y {out}'  # Windows\n")
		return 2
	}

	out := filepath.Join(os.TempDir(), "agezt-listen-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".wav")
	defer os.Remove(out)

	cmdline := substituteRecord(tmpl, seconds, out)
	fmt.Fprintf(stderr, "recording %ds…\n", seconds)
	if err := recordFunc(context.Background(), cmdline, out, stderr); err != nil {
		fmt.Fprintf(stderr, "%s listen: record failed: %v\n", brand.CLI, err)
		return 1
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		fmt.Fprintf(stderr, "%s listen: recorder produced no audio (check AGEZT_VOICE_RECORD_CMD)\n", brand.CLI)
		return 1
	}

	return transcribeFile(out, model, run, asJSON, stdout, stderr)
}

// substituteRecord splits the template into argv and replaces {seconds} / {out}.
func substituteRecord(tmpl string, seconds int, out string) []string {
	fields := strings.Fields(tmpl)
	for i, f := range fields {
		f = strings.ReplaceAll(f, "{seconds}", strconv.Itoa(seconds))
		f = strings.ReplaceAll(f, "{out}", out)
		fields[i] = f
	}
	return fields
}

// execRecord runs the recorder command, streaming its stderr (ffmpeg/arecord
// progress) to the operator.
func execRecord(ctx context.Context, cmdline []string, _ string, stderr io.Writer) error {
	if len(cmdline) == 0 {
		return fmt.Errorf("empty record command")
	}
	cmd := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
	cmd.Stderr = stderr
	return cmd.Run()
}
