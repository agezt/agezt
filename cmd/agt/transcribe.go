// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/stt"
)

// sttClientFromEnv builds an STT client from the AGEZT_STT_* environment, falling
// back to OPENAI_API_KEY for the key. modelOverride (a --model flag) wins over
// AGEZT_STT_MODEL.
func sttClientFromEnv(modelOverride string) *stt.Client {
	key := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	model := modelOverride
	if model == "" {
		model = strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_MODEL"))
	}
	return stt.New(stt.Config{
		APIURL: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_URL")),
		APIKey: key,
		Model:  model,
	})
}

// cmdTranscribe implements `agt transcribe <file> [--model m] [--run] [--json]`:
// turn an audio file into text via the configured STT endpoint, and optionally
// feed that text straight to the agent as an intent (`--run`).
func cmdTranscribe(args []string, stdout, stderr io.Writer) int {
	file := ""
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
		case a == "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s transcribe: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s transcribe <audiofile> [--model m] [--run] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "transcribe an audio file to text via the configured STT endpoint\n")
			fmt.Fprintf(stdout, "  --run     feed the transcript to the agent as an intent\n")
			fmt.Fprintf(stdout, "  --model   transcription model (default AGEZT_STT_MODEL or whisper-1)\n")
			fmt.Fprintf(stdout, "env: AGEZT_STT_API_URL (default OpenAI), AGEZT_STT_API_KEY (or OPENAI_API_KEY)\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s transcribe: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			if file != "" {
				fmt.Fprintf(stderr, "%s transcribe: unexpected extra argument %q\n", brand.CLI, a)
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintf(stderr, "%s transcribe: an audio file is required\n", brand.CLI)
		return 2
	}
	return transcribeFile(file, model, run, asJSON, stdout, stderr)
}

// transcribeFile reads an audio file, transcribes it, prints the text (or JSON),
// and optionally runs it. Shared by `agt transcribe` and `agt listen`.
func transcribeFile(file, model string, run, asJSON bool, stdout, stderr io.Writer) int {
	audio, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "%s transcribe: read %s: %v\n", brand.CLI, file, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	text, err := sttClientFromEnv(model).Transcribe(ctx, filepath.Base(file), audio)
	if err != nil {
		fmt.Fprintf(stderr, "%s transcribe: %v\n", brand.CLI, err)
		return 1
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintf(stderr, "%s transcribe: empty transcript\n", brand.CLI)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]string{"text": text})
	} else {
		fmt.Fprintln(stdout, text)
	}

	if run {
		// Drive the agent with the transcript as the intent (full governed loop).
		return cmdRun([]string{text}, stdout, stderr)
	}
	return 0
}
