// SPDX-License-Identifier: MIT

package builtintools

// Late-bound specs (Phase 2.2 PR 5): the tools whose wiring needs things
// built AFTER the kernel — the live channels (notify / send_media) and
// the shared board store (board). Each spec's Late hook runs in
// Set.ConfigureLate at the exact point cmd/agezt/main.go used to call
// the hand-wired .Bind()s, closing over the concrete instance its OWN
// Build produced. notify / send_media keep their historical gate: they
// register only when the operator configured at least one notify-capable
// channel with a non-empty allowlist (BuildDeps.NotifyTargets carries
// the daemon's derivation of that; empty/nil ⇒ the specs build nothing,
// exactly like the old `if len(notifyTargets) > 0` block). The board
// tool is always registered and simply stays unbound when the store
// failed to open (LateDeps.Board nil).

import (
	"fmt"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	boardtool "github.com/agezt/agezt/plugins/tools/boardtool"
	"github.com/agezt/agezt/plugins/tools/notify"
	"github.com/agezt/agezt/plugins/tools/sendmedia"
)

// specNotify — proactive messaging (`notify`, M143): the agent pings the
// operator on a configured chat channel. Destinations stay pinned to each
// channel's operator allowlist (the agent supplies only text); the sender is
// wired Late, once the live channels exist.
func specNotify() toolregapi.Spec {
	var nt *notify.Tool
	var targets map[string][]string
	return toolregapi.Spec{
		Name: "notify",
		Build: func(d toolregapi.BuildDeps) (toolregapi.Built, error) {
			if len(d.NotifyTargets) == 0 {
				return toolregapi.Built{}, nil // no configured+allowlisted channel ⇒ not registered
			}
			targets = d.NotifyTargets
			nt = notify.New() // unbound; Late wires the sender once channels exist
			return toolregapi.Built{Tool: nt}, nil
		},
		Late: func(_ toolapi.Tool, d toolregapi.LateDeps) error {
			if d.ChannelSend != nil {
				nt.Bind(d.ChannelSend, targets)
			}
			return nil
		},
	}
}

// specSendMedia — the attachment-carrying sibling of notify (same allowlist
// pinning), so the agent can push an image/voice/file artifact to the
// operator. The artifact resolver goes through the registry's row — a plain
// closure the daemon builds over the live blob store (W3.39g) — resolved
// lazily per call.
func specSendMedia() toolregapi.Spec {
	var smt *sendmedia.Tool
	var targets map[string][]string
	return toolregapi.Spec{
		Name: "send_media",
		Build: func(d toolregapi.BuildDeps) (toolregapi.Built, error) {
			if len(d.NotifyTargets) == 0 {
				return toolregapi.Built{}, nil
			}
			targets = d.NotifyTargets
			smt = sendmedia.New()
			return toolregapi.Built{Tool: smt}, nil
		},
		Late: func(_ toolapi.Tool, d toolregapi.LateDeps) error {
			if d.ChannelSendMedia == nil {
				return nil
			}
			resolve := d.ArtifactBytes
			smt.Bind(d.ChannelSendMedia, targets, func(ref string) ([]byte, error) {
				if resolve == nil {
					return nil, fmt.Errorf("artifact store unavailable")
				}
				return resolve(ref)
			})
			return nil
		},
	}
}

// specBoard — the shared, persistent message board every agent can post to and
// read from (`board`, M647), so they can coordinate and talk to each other.
// Always registered; Late binds the ONE shared board.Store instance the
// control plane and REST mailbox also write through (M937 — a second instance
// would silently clobber the last write) plus the board.posted notifier
// (M656) so a standing order can trigger on a topic. When the store failed to
// open the daemon passes a nil Board and the tool simply stays unbound,
// exactly like the old `if boardErr != nil` skip.
func specBoard() toolregapi.Spec {
	var bt *boardtool.Tool
	return toolregapi.Spec{
		Name: "board",
		Build: func(toolregapi.BuildDeps) (toolregapi.Built, error) {
			bt = boardtool.New()
			return toolregapi.Built{Tool: bt}, nil
		},
		Late: func(_ toolapi.Tool, d toolregapi.LateDeps) error {
			if d.Board != nil {
				bt.BindStore(d.Board)
				bt.OnPost(d.BoardNotify)
			}
			return nil
		},
	}
}
