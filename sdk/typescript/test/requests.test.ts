// The SDK's request rows are the daemon's contract.
//
// The request bodies in sdk/fixtures/requests.json are rendered by the daemon's
// own REST contract (kernel/contract/restapiapi's RowBody), so this test checks
// that the SDK's request rows serialize to the very bytes the contract
// declares — not to a payload this test wrote itself. A change in the
// contract's request vocabulary moves the fixture and fails here.
//
// The rows are built FROM the fixture: the local names come from the
// generated catalog, so this test spells no field of its own.
//
// Regenerate the fixture with:
//   AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import {
  MAIL_ACK_NAMES,
  MAIL_DRAFT_NAMES,
  RUN_REQUEST_NAMES,
  mailAckToWire,
  mailDraftToWire,
  runRequestToWire,
} from "../src/index.js";
import type { MailAck, MailDraft, RunRequest } from "../src/index.js";

// The compiled test lives in sdk/typescript/dist/test, so the fixtures — which
// sit in sdk/fixtures — are three levels up.
const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "fixtures");

function rows(): Record<string, Record<string, unknown>> {
  const parsed = JSON.parse(readFileSync(join(FIXTURES, "requests.json"), "utf8")) as {
    rows: Record<string, Record<string, unknown>>;
  };
  return parsed.rows;
}

/** The row the contract's body describes, by its local names. */
function rowOf(body: Record<string, unknown>, names: Record<string, string>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [wire, value] of Object.entries(body)) {
    out[names[wire]] = value;
  }
  return out;
}

test("a run request serializes to the contract's body", () => {
  const body = rows().run_request;
  const row = rowOf(body, RUN_REQUEST_NAMES) as unknown as RunRequest;
  assert.deepEqual(runRequestToWire(row), body);
});

test("a message to leave serializes to the contract's body", () => {
  const body = rows().mail_draft;
  const row = rowOf(body, MAIL_DRAFT_NAMES) as unknown as MailDraft;
  assert.deepEqual(mailDraftToWire(row), body);
});

test("an ack serializes to the contract's body", () => {
  const body = rows().mail_ack;
  const row = rowOf(body, MAIL_ACK_NAMES) as unknown as MailAck;
  assert.deepEqual(mailAckToWire(row), body);
});

test("an unset optional field is not sent", () => {
  // The daemon reads an absent optional field and an empty one as the same
  // thing, but a request that says `"help": false` is not the request that
  // says nothing about help.
  assert.deepEqual(mailDraftToWire({ text: "hello" }), { text: "hello" });
});
