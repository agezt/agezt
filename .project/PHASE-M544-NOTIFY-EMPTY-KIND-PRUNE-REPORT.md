# M544 — Mutation testing notify: pin the empty-id channel-kind prune

## Context
`plugins/tools/notify` is how the agent proactively messages the operator. It can
only reach the operator's pre-configured allowlist (no arbitrary recipients), and
`Bind` drops any channel kind whose id list is empty so an unusable kind is never
advertised to the model. `GOMAXPROCS=3`. (coding/notify were the last two
un-mutation-tested tools; coding's truncate is rune-safety-tested and its `<= max`
edge is the same low-stakes cosmetic class documented in M543, so this milestone
targets notify, where a genuine survivor was found.)

## The genuine gap (closed)
```go
func (t *Tool) Bind(send Sender, targets map[string][]string) {
	pruned := map[string][]string{}
	for kind, ids := range targets {
		if len(ids) > 0 {            // ← drop kinds with no recipients
			pruned[kind] = append([]string(nil), ids...)
		}
	}
	...
}
```

`TestNotify_UnboundReportsNotConfigured` bound `{"slack": {}}` and asserted only
`res.IsError`. But the disabled-state outcome ("notify is not configured") and the
*wrong* outcome the mutant produces ("notify failed: " — proceeding to deliver to
zero recipients) are **both** `IsError`, so `len(ids) > 0 → >= 0` survived: an
empty-id kind would be kept, advertised as available in the tool definition, and
then silently deliver to nobody on every call.

## Fix
Strengthened the assertion to require the precise "not configured" message (proving
the empty kind was pruned away, not merely that the call errored) and that **no
send was attempted** (`len(cap.calls) == 0`).

## Negative control (manual, CPU-capped)
`len(ids) > 0 → >= 0`: FAIL — result is `notify failed:` against zero recipients
instead of "not configured". Restored byte-for-byte
(`git diff --ignore-all-space` on notify.go empty); passes again.

## Rest of notify — verified covered (no padding)
The other notify behaviors already have killing tests: partial-delivery failure
stays `IsError` and names the failed recipient (`TestNotify_PartialFailureIsError`
— asserts IsError + "FAILED" + the failed id), all-fail is `IsError`
(`TestNotify_AllSendsFailIsError`), empty text rejected with no send, channel
filter delivers only the named kind and rejects an unconfigured one, and `Bind`
copies its targets (`TestNotify_BindIsolatesTargets`). No further gap.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Plugins/tools mutation sweep — complete
file (M535), http (M536), shell (M537), mcpbridge (M538), peer (M541),
acpagent (M542), browser (M543), notify (M544); coding verified covered
(rune-safe truncate tested; `<= max` cosmetic edge honestly left unpinned).
Every tool under `plugins/tools/` has now been mutation-assessed.
