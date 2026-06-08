# PHASE M587 — Voice input (speech-to-text)

**Status:** DONE — local, gated (unit + end-to-end CLI smoke green), ready for
branch/PR. **Owner chose: OpenAI Whisper-compatible HTTP STT + both file and
microphone sources** (via AskUserQuestion).

## What shipped

Talk to the agent. Audio → text via an OpenAI-compatible STT endpoint, then
optionally straight into the governed loop.

- **`kernel/stt`** — a minimal client for `POST <base>/audio/transcriptions`
  (OpenAI/Groq/local whisper.cpp all speak this). `net/http` + `mime/multipart`
  only, no dependency. `Transcribe(ctx, filename, audio) → text`: multipart
  `file` + `model` fields, Bearer auth, bounded JSON `{text}` parse, key scrubbed
  from errors. Defaults to OpenAI + `whisper-1`.
- **`agt transcribe <file> [--model m] [--run] [--json]`** — transcribe a recorded
  file; `--run` feeds the transcript to the agent as an intent (delegates to the
  full `cmdRun` governed loop).
- **`agt listen [--seconds N] [--model m] [--run] [--json]`** — record the mic,
  then transcribe + (optionally) run. Microphone capture has no portable Go path
  without a CGO audio dep, so — like the tunnel — Agezt drives an operator-chosen
  recorder via `AGEZT_VOICE_RECORD_CMD` with `{seconds}` / `{out}` placeholders
  (e.g. `arecord -d {seconds} -f cd -t wav {out}` on Linux, `ffmpeg … {out}` on
  macOS/Windows). Clear error + examples if it's unset.

## Wiring

- STT env (read by both commands, shared `sttClientFromEnv`): `AGEZT_STT_API_URL`
  (default OpenAI), `AGEZT_STT_API_KEY` (falls back to `OPENAI_API_KEY`),
  `AGEZT_STT_MODEL` (default `whisper-1`); plus `AGEZT_VOICE_RECORD_CMD`. All four
  registered in `kernel/controlplane/config.go` (alphabetical).
- `transcribe` / `listen` dispatch cases + `agt help` lines. Both are pure CLI
  (the STT call is a direct HTTP request; `--run` dials the daemon like `agt run`).

## Tests + smoke (all green)

- **5 unit tests** for `kernel/stt`: defaults/overrides; multipart POST carries
  audio + filename + model + bearer; empty-audio guard; HTTP-error + body-error
  surfaced.
- **6 CLI tests** (`voice_test.go`): file→text (mock STT via `t.Setenv`), `--json`,
  missing-file / no-arg exits; `listen` records (injected `recordFunc`) →
  transcribes, `{seconds}`/`{out}` substitution verified; no-recorder guidance;
  `substituteRecord`.
- **End-to-end CLI smoke**: real `agt` against a mock STT server + a mock recorder
  binary — `agt transcribe clip.wav` → "the quick brown fox"; `agt listen
  --seconds 2` → recorded (mock) → "the quick brown fox" with a "recording 2s…"
  progress line. Both sources proven.
- gofmt clean (staged blobs); staticcheck + vet clean; full Go suite green; 6×
  cross-build OK; `go.mod` unchanged (no new dependency).

## Scope / follow-ups (documented)

- **HTTP upload source** (`POST /v1/audio/transcriptions` on the daemon's
  OpenAI-compatible API, so any OpenAI audio client uploads to Agezt directly) is
  the natural next increment; it touches the `api` server package. The file +
  microphone CLI surfaces cover the primary "speak to the agent" paths today.
- The real microphone/STT path is env-bound (a recorder binary + an STT
  endpoint), like the channels' real-service smoke; the record→STT→run pipeline is
  fully covered offline with a mock recorder + mock STT.
- "Always-listening" ambient mode (VAD/wake-word, continuous chunking) is a future
  step on top of `agt listen`.

## Backlog after M587

All planned features that are autonomously buildable are now shipped. Remaining is
owner-gated: actually publishing the SDKs (secrets — workflow ready), cutting the
release (owner chose to stay 1.0.0), and restoring CI Actions minutes.
