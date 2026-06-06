# M556 — Durable, repeatable E2E smoke artifact (`scripts/e2e-smoke.sh` + `make e2e`)

## Context
Criterion 7 was verified manually across M550–M555. To make the zero-defect
"running app" property **reproducible proof** rather than a one-time check —
the goal's "çalıştırılabilir kanıt" — the e2e sweep is codified into a committed,
runnable script and a `make e2e` target.

## What it does (one invocation, real daemon, keyless echo mock)
1. control-plane run loop (`agt run` → echo)
2. `doctor` + `journal verify` (BLAKE3 chain `ok:true`)
3. OpenAI `/v1/chat/completions` non-streaming
4. OpenAI streaming — asserts the **content delta is present** (the M550 regression
   guard: a non-streaming provider's answer must not be dropped from the stream)
5. native REST `/api/v1/runs` → completed
6. error paths: bad auth → 401, malformed JSON → 400, valid 17 MB body → 413
7. 10 concurrent chat requests → all 200
8. halt → run refused → resume → run works
9. graceful shutdown + panic scan (fails on any `panic`/runtime error in the log)

Exit 0 = PASS. Runnable as `make e2e` (builds binaries first) or
`scripts/e2e-smoke.sh <agezt> <agt>` against prebuilt binaries. CPU-capped
(`GOMAXPROCS=3`).

## Two script bugs found and fixed while making it pass (NOT product bugs)
- A bare `wait` (step 7) blocked on the backgrounded **daemon** process forever —
  fixed to `wait` only the curl PIDs.
- Under `set -o pipefail`, `agt run | grep -q 'halted'` returned the refused-run's
  non-zero exit and masked a successful grep match — fixed to capture-then-grep.
- Cleanup made tolerant of Windows open-file locks on the daemon's journal (sleep
  before `rm`, ignore errors on the throwaway temp dir).
The product behaved correctly throughout; these were harness-only fixes.

## Result
`make e2e` / `scripts/e2e-smoke.sh /tmp/agezt /tmp/agt` → all 10 steps `ok`,
`E2E SMOKE: PASS`, exit 0. The runtime/E2E criterion is now re-runnable on demand
(and wireable into CI later). No production code change (Makefile + script only;
`go vet ./...` clean, go.mod/go.sum unchanged).

## Reliability flows also verified this session (M555 + here)
Provider failover (`DEMO_FAIL_PRIMARY` → 2 `provider.fallback` events, run still
completes), multi-agent delegation, loop-guard (graceful `max iterations
exceeded`), SSRF egress refusal, prompt cache, vision attachment path — all 0
panics.
