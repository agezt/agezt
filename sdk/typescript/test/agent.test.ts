import { test } from "node:test";
import assert from "node:assert/strict";

import { DEFAULT_SOCKET_PATH, resolveSocketPath } from "../src/agent.js";

// SDK-001 (2026-08-12). The daemon binds the default path with Go's
// net.Listen("unix", "@agezt/agentgw.sock"), and Go maps a leading "@" to the
// Linux abstract namespace. Node does not — libuv copies the string straight
// into sun_path, so the untranslated default is a CWD-RELATIVE FILE PATH.
//
// That fails OPEN rather than closed: an agent subprocess whose CWD an attacker
// can write to gets "./@agezt/agentgw.sock" planted there, and every request
// then hands `Authorization: Bearer <capability token>` to whoever is listening.

const onLinux = process.platform === "linux";

test("the shipped default still uses the @ form the daemon binds", () => {
  assert.equal(DEFAULT_SOCKET_PATH, "@agezt/agentgw.sock");
});

test("@ becomes a NUL-prefixed abstract address on Linux, and only there", () => {
  const got = resolveSocketPath("@agezt/agentgw.sock");
  if (onLinux) {
    assert.equal(got, "\0agezt/agentgw.sock");
    // The literal relative path is the vulnerable form — it must not survive.
    assert.notEqual(got, "@agezt/agentgw.sock");
    assert.ok(!got.startsWith("@"), "a leading @ would be a relative file path");
  } else {
    // No abstract namespace off Linux, and Go binds the literal there too, so
    // translating would break agreement between the two ends.
    assert.equal(got, "@agezt/agentgw.sock");
  }
});

test("paths without a leading @ are never rewritten", () => {
  for (const p of ["/tmp/agentgw.sock", "./relative.sock", "agentgw.sock", ""]) {
    assert.equal(resolveSocketPath(p), p);
  }
});

test("only the first @ is consumed", () => {
  const got = resolveSocketPath("@a@b.sock");
  assert.equal(got, onLinux ? "\0a@b.sock" : "@a@b.sock");
});
