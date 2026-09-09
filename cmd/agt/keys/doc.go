// SPDX-License-Identifier: MIT

// Package keys is the home of `agt provider keys <subcommand>` —
// the per-provider API-key ring (M700). It is the first
// subcommand-family-as-package: the main `provider` dispatcher
// still lives in cmd/agt/provider.go (it is the entry point for
// eleven other provider subcommands), but the keys subcommand
// family — list / add / activate / rm — moved here as a single
// Go package. The benefit is the same as moving any other
// command-as-package: the keys subcommand and its helpers
// (`providerKeysProviderFlag`, `providerKeyRequest`,
// `providerKeyTargetLabel`, `providerKeyProviderFlag`) now
// have a clear home, can be unit-tested without compiling
// every other provider file, and the dispatcher in main is
// exactly one line of glue (`case "keys": return keys.Run(...)`).
//
// The 4 handlers + 4 helpers + dispatch + tests total 270+
// lines. The package boundary matches the dispatch table: the
// `cmdProvider` switch in cmd/agt/provider.go has twelve cases
// (creds, keys, connect, chatgpt, check, log, stats, rejections,
// reload, setup, import, cost). Each is a candidate for the
// same subcommand-family-as-package move. The keys family was
// the first because it is self-contained: it does not depend
// on any cross-file helper in main, only on the package
// aliases `dial.New` (extracted in Day 5) and the brand
// constants.
package keys
