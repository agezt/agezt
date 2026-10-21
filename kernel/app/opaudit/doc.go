// SPDX-License-Identifier: MIT

// Package opaudit owns the operation audit's policy: what may be journaled
// about an operation's arguments and errors. The journal payload itself —
// the op.invoked/op.completed/op.failed events — is the host's to publish;
// the rules that decide what goes into those payloads belong here.
package opaudit
