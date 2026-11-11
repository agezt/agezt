# W3.76ag — the run's own words move into the runs module

**Slice:** W3.76ag (the forty-first of the runtime's dissolution)
**Base:** `085d2b66` (the W3.76af merge)
**Date:** 2026-11-11

---

## 1. The family

A run is not silent. It publishes **two** things about itself:

- the **reading** it made of the user's utterance — `intent.interpreted`,
  with the hash, the canonical intent, the assumptions and the ambiguity,
  and never the raw words;
- the **analysis** of what its context selection left out — a
  `context.selection` manifest with the phase `failure_analysis`, published
  when the run fails.

Both were the **daemon's**: two public verbs over two private helpers.

```go
func (k *Kernel) PublishIntentInterpreted(corr, actor string, frame intentmodel.Frame) {
	k.publishIntentInterpreted(corr, actor, frame)
}

func (k *Kernel) PublishContextFailureAnalysis(corr, actor string, runErr error) {
	k.publishContextFailureAnalysis(corr, actor, runErr)
}
```

And the **run engine** — which lives *inside* the runs module — asked the
**host** for both of them through its `KernelAPI` port:

```go
r.k.PublishIntentInterpreted(corr, actor, intentFrame)
r.k.PublishContextFailureAnalysis(corr, actor, err)
```

**A port entry that is a round trip inside one module is a promise the host
never had to keep.** The module has the body; the module calls it.

The reading makes it plain: the run engine **interprets** the utterance
itself (`intentmodel.Interpret(intent)`), and then asked the host to publish
the reading it had just made.

## 2. What moved

A new package: **`kernel/modules/runs/publish`** — the runs module's publish
surface. It owns:

- `IntentInterpreted` — the run's reading, published without the raw
  utterance;
- `ContextFailureAnalysis` — the journal walk for the run's last
  context-selection manifest, the re-ranking of its rejected candidates and
  the manifest the analysis derives.

The dependencies are **narrow ports**, never the host's implementations:

| port | what it is |
|---|---|
| `Bus` | the bus the run's events travel on |
| `Journal` | the event log the failure analysis scans |
| `Selection` | the prompt module's manifest publisher — the module knows what a context selection manifest is |

The daemon keeps the bus and the journal it already had, and hands them to
the module. The prompt module's service is the `Selection` port: the
analysis asks the module to publish the manifest it derives, the way it
always did.

The daemon's two verbs and their two bodies are gone; the daemon's
`context_selection.go` file went with them (the analysis was the file's
whole content), and `intent.go` keeps only the daemon's **policy** events —
the intent confirmation, the session-scoped auto-approve, the
prompt-injection warning. Those are the daemon's policy path's events, not a
run's.

## 3. The port

The run engine's host port lost **two** entries — `PublishIntentInterpreted`
and `PublishContextFailureAnalysis` — and carries **30** now (it carried
**32**). What stayed is what the **host** owns.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 116 | **114** |
| the daemon's production lines | 5,417 | **5,372** |
| the daemon's production files | 55 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 32 entries | **30 entries** |
| the run engine's package | 970 | **997** |
| the runs module's **publish surface** | — | **new: 1 file / 131 lines** |
| the prompt module's package | 6 files / 756 lines | **6 files / 760 lines** |

**Two verbs off the daemon's surface and 45 lines with them**, and a file
gone with them: the daemon's `context_selection.go` held nothing else. The
run engine's package grew by 27 lines (the publisher's handle, the refusal,
and the comments that say where the calls go), and the module's new
publish package holds the 131 lines that moved.

## 5. The guard

The two verbs were in the control plane's **bus** family — the rule is by
name, and a kernel verb whose name carries "Publish" is the event bus
family's business. The family is derived from the kernel's own method set, so
the two names left it with the verbs; the family's comment says so out loud,
because a rule that claims a verb nobody has claims nothing.

## 6. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/publish_ratchet_test.go`, new): the
daemon holds **no** verb per run-publish operation, its production files
declare **none**, and the bodies are gone too — the reading's own shape (the
event the run publishes) and the analysis (the journal walk that derives the
manifest) do not come back under a new name.

**The module's side** (`kernel/modules/runs/runner/runner_publish_ratchet_test.go`,
new): the port carries **no** publish entry, the engine **holds** the
module's publisher, and a host that wired none leaves the engine quiet
instead of running silently.

The daemon's own surface ceiling followed the floor again: **116 → 114** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116 → 114), and the two retired names joined the
retired-verb list.

**The probe** put the daemon's two verbs back — with their real bodies, so
the world before this slice is the world that works — put the two entries
back into the engine's port and made the engine ask the host again: the
retired-verb ratchet, the surface ceiling (116 > 114), the daemon's three
publish ratchets, the module's port ratchet, the module's wiring ratchet
and the module's refusal ratchet all went red at once — **eight** ratchets
in one shot, and nothing else broke. The probe was reverted and the suites
went green again.

## 7. What the daemon keeps

The daemon's **policy** events stay where they are: `intent.confirmation_required`
(the policy hook's regret gate), `policy.auto_approved` (the session-scoped
operator grant) and `prompt_injection.warned` (the injection guard's warn
mode). They are the daemon's policy path's events, and the prompt-injection
MODE is the platform's rule (W3.74) — the daemon keeps the name.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the publish module's tests and
  the applications' tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The daemon holds **114** public verbs and **5,372** production lines; **forty**
application packages still name it; the run engine's host port carries **30**
entries.

What is left in the daemon is the run's **remaining machinery**: the plan
runner's registration (`RunPlan`), the resume handshake
(`ClaimResumeTicket`/`FinalizeResumeTicket`) and the two doors the run engine
still opens for the host's own state (`ResolveRunModel`, `BuildLoopConfig`).

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
