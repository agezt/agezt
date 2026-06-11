# Phase M831 — `fetch` tool: download a URL → browsable artifact

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "sistemde sıfır
hata olması için ne gerekiyorsa yap, daha fazla tool ekle bize gerekebilen şu
anki toollar yetersiz gibi" — add more built-in tools; the current set is thin.

## Gap

The agent could read a page's **text** (`http`/`browser.read`) but had no way to
**keep a file from the web**. Binary content — an image, PDF, archive, dataset —
was unreachable: `http` returns text into context (and is capped), and nothing
landed it in the artifact store / Files view (M822–M823) where the operator can
preview and download it. So "save this picture", "grab that PDF" had no path.

## What shipped (`plugins/tools/fetch/`)

A new in-process `fetch` tool:

- **`fetch {url, name?}`** → GETs the URL through a **netguard-protected** client
  (same SSRF guard as `http`/`web_search`; loopback/private only when
  `AGEZT_ALLOW_ALL`/the private-net flags are set), reads up to **50 MiB**,
  detects the mime (Content-Type, else sniffed), and **`PutEntry`s the bytes into
  the artifact index** with `Source:"fetch"` and `Kind` = `image` (image/*) or
  `download`. Returns `{id, ref, name, mime, size, saved}` so the model can
  reference the saved file by id.
- Decoupled from the kernel via an `Indexer` interface; the daemon injects the
  real `*artifact.Index` with `SetIndex(k.ArtifactIndex())` after Open (mirrors
  the M827 indexer wiring). Without an index it fails soft ("artifact store
  unavailable").
- Fail-soft on every operator-reachable error (bad URL, HTTP ≥400, empty body,
  over-limit) — returns an error *result*, never a hard tool failure that aborts
  the run.

## Wiring

- `cmd/agezt/main.go` `buildTools`: register `fetch` (`fetch(url→artifact)` in the
  startup tools line), relax egress under allow-all, inject the index post-Open.
- `kernel/edict/toolmap.go`: `case "fetch": return CapHTTPGet` — a download is a
  network GET, so it reuses the existing capability (no new grant, no new env).

## Verification

- **Unit** (`fetch_test.go`): stub server + fake index — asserts the image is
  stored with the right mime/kind/name/source/bytes/time and the result reports
  the id; rejects non-http/empty URLs and missing index; HTTP 404 is a soft error.
- **Live** (isolated `AGEZT_HOME`, real MiniMax agent): `agt run "fetch
  https://go.dev/images/go-logo-blue.svg and save it"` → agent invoked `fetch`,
  got `art-01KTT…`, and the index sidecar shows
  `{kind:image, source:fetch, mime:image/svg+xml, name:go-logo-blue.svg,
  size:1472}` — i.e. it lands in the Files gallery automatically (no frontend
  change: M823 already renders `image`/`download` artifacts).

## Gate

fetch + edict + artifact + catalog + runtime + webui + controlplane tests green;
vet + staticcheck + linux cross-build clean; gofmt swept. go.mod unchanged
(net/http + existing netguard/artifact only). No new env var → no `configEnvVars`
change. SSRF guard intact; default-allow posture preserved (fetch is on by
default, restriction only via the existing egress/policy opt-outs).
