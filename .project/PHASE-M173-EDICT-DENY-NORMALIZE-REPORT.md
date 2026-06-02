# M173 — Policy hard-deny floor matches the decoded action (security review)

## Why
An adversarial review of Edict — THE authorization gate the agent loop consults
before every tool call (`Decide(capability, input) → Allow/Deny/Ask`) — was run.
Edict enforces a non-overridable hard-deny floor (`rm -rf /`, `mkfs`, fork-bombs, …)
plus a per-capability trust ladder. The review confirmed the engine's core is
correct and found one Critical bypass in how the floor *matches*.

## Confirmed correct (left unchanged)
- **Deny-before-ladder:** the hard-deny loop runs first and returns immediately;
  no level or Ask policy can un-deny a floor hit.
- **Unknown capability → default-deny** (fail-safe); an unknown tool/op resolves to
  an unconfigured capability and is denied.
- **toolmap classification is conservative:** a write is never down-classified to a
  read; malformed JSON leaves the op empty → `file.` → default-deny; no panic.
- **`AskPrompt` fails closed** (`DecisionDeny` + `RequiresApproval`); a caller
  ignoring the flag still denies.
- **Parsers reject unknown trust-level/ask-policy strings** rather than defaulting
  lax; empty-substring deny rules are rejected (no deny-everything DoS).
- **Concurrency:** `Decide` reads under `RLock`; mutators take the write lock; the
  rule-slice swap is race-free.

## The bug (Critical)
`HardDenyRule.matches` did `strings.Contains(lower(input), lower(substring))` where
`input` was the **raw JSON tool argument** (`runtime.go` passes `string(tc.Input)`).
Because the model picks the command string, it could keep the decoded command
destructive while making the JSON text miss the floor substring:
- **Whitespace padding:** `{"command":"rm  -rf /"}` (two spaces) misses the
  `rm -rf /` rule; the shell still runs `rm -rf /` after JSON-decode.
- **JSON-escape evasion:** `{"command":"rm -rf /"}` (slash escaped) or
  `{"command":"rm -rf /"}` (the `m` escaped) — `json.Unmarshal` decodes both
  back to `rm -rf /` before execution, but Edict judged the encoded form and the
  floor never fired.

At `shell=L4` (or folded-Allow at the default L2) these executed. The "non-
overridable floor" was bypassable by the exact actor it constrains.

## Fix
`Decide` now matches each hard-deny rule against a set of **candidate** strings, not
just the raw input:
- `denyCandidates(input)` returns the raw input (so prior matches still fire — no
  regression) **plus**, when the input is JSON, every decoded string *value* with
  whitespace collapsed (`collectJSONStrings` + `collapseWhitespace`). Values are
  matched individually, so adjacent fields can't form a spurious match; non-JSON
  input contributes its own whitespace-collapsed form.
- Decoding defeats JSON-escape evasion (the parser turns `/` into `/`); the
  whitespace collapse defeats padding (`rm  -rf /` → `rm -rf /`).

Substring matching remains inherently best-effort against *semantic* rewrites
(flag reordering `rm -fr /`, equivalents like `find / -delete`); the review's
deeper recommendations — tokenized program/path matching, snapshot integrity
(signing the durable overlay), a stricter `runtime[<int>]` `IsRuntimeRule`, and a
fuller default floor list — are noted for follow-up milestones. This milestone
closes the encoding/whitespace evasion class, which is the part a model can trivially
trigger.

## Tests (+1, all passing)
`TestDecide_HardDeny_EvasionVariants` — at `shell=L4` (only the floor can stop
these): raw `rm -rf /` (no regression), double-space, and two JSON-escaped variants
(`/`, `m`, assembled from explicit bytes so the test input unambiguously
carries a real escape) are all hard-denied; a benign `ls -la /tmp` is still allowed
(no over-block).

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command, env var, or event kind.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1554 tests** (was 1553; +1), 61 packages.

## Result
The hard-deny floor can no longer be slipped by JSON-escaping or whitespace-padding
a banned command — the gate now judges the decoded, normalized action the tool will
actually execute.
