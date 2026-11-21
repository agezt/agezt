import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { PATHS } from "../src/index.js";
import type { Mail, RunResult } from "../src/index.js";

// The SDK parses what the DAEMON answered.
//
// The fixtures in sdk/fixtures/ are recorded from the daemon's own REST
// handlers (kernel/restapi's golden test), so these tests check the SDK's
// parsing against the server's bytes — not against a payload this test wrote
// itself. A change in the daemon's wire shape moves the fixture and fails
// here.
//
// Regenerate the fixtures with:
//   AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures

// The compiled test lives in sdk/typescript/dist/test, so the fixtures — which
// sit in sdk/fixtures — are three levels up.
const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "fixtures");

interface ManifestEntry {
  id: string;
  method: string;
  path: string;
  request: string;
  status: number;
  file: string;
  note?: string;
}

function manifest(): ManifestEntry[] {
  return (JSON.parse(readFileSync(join(FIXTURES, "manifest.json"), "utf8")) as {
    fixtures: ManifestEntry[];
  }).fixtures;
}

function load(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(FIXTURES, name), "utf8")) as Record<string, unknown>;
}

test("the SDK's generated paths and the server's fixtures are one set", () => {
  const serverPaths = new Set(manifest().map((entry) => entry.path));
  const sdkPaths = new Set<string>(Object.values(PATHS));
  assert.deepEqual(sdkPaths, serverPaths);
});

test("the daemon's health and models parse into the SDK's rows", () => {
  const health = load("health.json");
  assert.equal(health.status, "ok");
  assert.equal(health.version, "9.9.9");
  assert.equal(health.default_model, "model-a");
  assert.equal(health.model_count, 2);

  const models = load("models.json");
  assert.equal(models.default, "model-a");
  assert.deepEqual(models.models, ["model-a", "model-b"]);
});

test("the daemon's run answer parses into the SDK's row", () => {
  const result = load("run.json") as unknown as RunResult;
  assert.equal(result.correlation_id, "corr-golden");
  assert.equal(result.model, "model-a");
  assert.equal(result.status, "completed");
  assert.equal(result.answer, "golden answer");

  const arc = load("run_arc.json");
  assert.equal(arc.correlation_id, "corr-golden");
  assert.equal(arc.count, 1);
});

test("the daemon's mailbox answers parse into the SDK's mail row", () => {
  const sent = load("mailbox_send.json") as { message: Mail };
  assert.equal(sent.message.id, "m-3");
  assert.equal(sent.message.topic, "dm");
  assert.equal(sent.message.text, "golden message");
  assert.equal(sent.message.from, "agent-a");
  assert.equal(sent.message.to, "agent-b");
  assert.equal(sent.message.ts_unix_ms, 1700000000002);
  assert.equal(sent.message.help, undefined);

  const listed = load("mailbox_messages.json") as { messages: Mail[]; count: number };
  assert.equal(listed.count, 1);
  assert.equal(listed.messages[0].id, "m-1");
  assert.equal(listed.messages[0].topic, "dm");

  const inbox = load("mailbox_inbox.json") as { name: string; waiting: Mail[]; count: number };
  assert.equal(inbox.name, "agent-b");
  assert.equal(inbox.count, 2);
  assert.deepEqual(inbox.waiting.map((m) => m.id), ["m-2", "m-1"]);
  assert.equal(inbox.waiting[0].to, "*");

  const replies = load("mailbox_replies.json") as { id: string; replies: Mail[]; count: number };
  assert.equal(replies.id, "m-1");
  assert.equal(replies.count, 0);
  assert.deepEqual(replies.replies, []);

  const acked = load("mailbox_ack.json");
  assert.equal(acked.acked, true);
  assert.equal(acked.id, "m-1");
  assert.equal(acked.by, "agent-b");

  const topics = load("mailbox_topics.json") as { topics: Record<string, number> };
  assert.deepEqual(topics.topics, { dm: 1, status: 1 });
});
