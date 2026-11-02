// SPDX-License-Identifier: MIT

package reasoningapi

import "errors"

// ErrNoService is answered when the daemon holds no reasoning module at all.
//
// The reasoning capability IS the module (W3.76v): the Council of Elders, the
// Conductor and the deep-research harness live in kernel/modules/reasoning, and
// the daemon holds the module's FACE — nothing more. A daemon that was given
// no module holds no capability, and the layers above ask the MODULE, not the
// daemon.
//
// So the refusal is one row, raised where the absence is felt: the
// application that asks the module raises it, and a caller compares with
// errors.Is. The daemon no longer raises it itself — it holds no reasoning
// verbs to raise it with.
var ErrNoService = errors.New("reasoning: the reasoning capability is not available on this daemon")
