// SPDX-License-Identifier: MIT

package runtime

// The ratchet of W3.76ab: the daemon holds no verb over the RUN ENGINE's own
// operations.
//
// The run engine lives in the runs module (kernel/modules/runs/runner) and
// the daemon holds it — the host builds the run engine while opening itself.
// What the daemon must NOT do is ANSWER for the run engine a second time:
// the daemon used to carry six wrappers over the module's own body
// (DescribeImages, MaybeDistill, MaybeForge, MaybeShadowEval,
// VerifyCompletion and PublishHeuristicBypass), and the run engine asked the
// host for them through its port — a round trip.
//
// The daemon still NAMES the run engine where it needs it (the image
// admission asks the module for the vision sidecar; the workboard's proof
// asks the module for the verification). Naming the module is the host's
// business; answering for it twice is not.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the wrappers back has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunEnginesOwnOperations are the run engine's own operations — the ones
// the daemon must not answer for.
var theRunEnginesOwnOperations = []string{
	"DescribeImages",
	"MaybeDistill",
	"MaybeForge",
	"MaybeShadowEval",
	"VerifyCompletion",
	"PublishHeuristicBypass",
}

// TestTheDaemonHoldsNoRunEngineForwarder pins the daemon's HALF: it holds no
// verb per run engine operation.
func TestTheDaemonHoldsNoRunEngineForwarder(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range theRunEnginesOwnOperations {
		if methods[name] {
			t.Errorf("*Kernel has the %s verb again — the run engine is the runs module's, and one operation has one name (W3.76ab)", name)
		}
	}
}

// TestTheDaemonDoesNotWrapTheRunEngine pins the daemon's SHAPE: the daemon
// names the run engine where it needs it, but it does not wrap the module's
// own operations in its own file.
func TestTheDaemonDoesNotWrapTheRunEngine(t *testing.T) {
	src, err := os.ReadFile("runtime_lifecycle.go")
	if err != nil {
		t.Fatalf("read runtime_lifecycle.go: %v", err)
	}
	text := string(src)
	for _, name := range theRunEnginesOwnOperations {
		wrapper := regexp.MustCompile(`func \(k \*Kernel\) ` + name + `\(`)
		if wrapper.MatchString(text) {
			t.Errorf("the daemon grew the run engine's %s wrapper back; the run engine is the module's (W3.76ab)", name)
		}
	}
	if !strings.Contains(strings.ToLower(text), "run engine's own operations live in the runner package") {
		t.Error("the daemon's lifecycle file must say where the run engine's operations live")
	}
}
