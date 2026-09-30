// SPDX-License-Identifier: MIT

// Package nostr is a two-way Nostr channel. It connects to a set of relays over
// WebSocket, subscribes to kind-1 notes that mention the agent's pubkey, and
// replies with signed, threaded kind-1 events (NIP-01 / NIP-10). Outbound-only
// use (Pulse briefs, `agt send`) publishes a standalone note.
//
// Nostr is the one channel that can't ride AGEZT's stdlib-only convention: it
// needs a WebSocket transport (github.com/coder/websocket) and BIP340 schnorr
// signing over secp256k1 (github.com/btcsuite/btcd/btcec) — both added
// deliberately for this channel.
//
// Security (SPEC-04 §1.7): inbound events are data, never kernel instructions.
// Every inbound event's schnorr signature is verified locally against its id and
// author pubkey BEFORE it is trusted — a malicious relay cannot forge an event
// attributed to an allowlisted author. An Allowlist of author pubkeys gates who
// may drive the agent (empty = fail-closed for driving; still journaled), and
// event ids are de-duplicated across relays. The private key signs outbound
// events and never leaves the process.
package nostr
