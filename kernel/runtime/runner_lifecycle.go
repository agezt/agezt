package runtime

// Provenance: SPDX-License-Identifier: MIT Runtime runner lifecycle methods:
//             CompleteAgentLifecycle + ErrNoVisionModel sentinel. Split from
//             runexec_helpers.go during Day 211 god-file refactor (#34).
//             Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/modules/runs/runner"
)

// verifyCompletion body moved to runner.Runner.VerifyCompletion on
// Day 33. The public wrapper (runtime.go:VerifyCompletion) delegates
// to the Runner.

// visionDescribeMaxTokens / assureVerifyMaxTokens / shadowEvalLimit
// moved to runner/helpers.go on Day 33.

// ErrNoVisionModel is returned by DescribeImages when no
// vision-capable model is available (checked by the daemon's
// channel handler). It is runner's sentinel re-exported — the
// same value, so errors.Is matches either name without a
// translation step.
var ErrNoVisionModel = runner.ErrNoVisionModel

// DescribeImages body moved to runner.Runner.DescribeImages on
// Day 33. Callers in this package reach it through *Kernel.DescribeImages
// (see compose.go / research.go for the production callers).

// The RUN ENGINE's operations are the runs module's (W3.76ac): the daemon
// hands over the module's face (RunnerService) and the callers above ask the
// module. What stays in the daemon is the HOST's own: the correlation id a
// run carries.
//
// The run's LIVE STATE is the module's too (W3.76ae): the steering registry
// and the per-run registration live in kernel/modules/runs/state, and the
// run engine asks the module — not the host — for them.

// publishHeuristicBypass body moved to runner.Runner.PublishHeuristicBypass
// on Day 33. RunWith calls it through k.runner.PublishHeuristicBypass.

// deterministicHeuristicBypass moved to runner/helpers.go on Day 34
// so the Runner can use it through the package boundary.

// maybeDistill / maybeForge / maybeShadowEval moved to the runner
// sub-package on Day 32 — Runner.MaybeDistill / MaybeForge /
// MaybeShadowEval hold the bodies. The *Kernel wrappers in
// runtime.go delegate to the Runner.
