import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { type AddressInfo } from "node:net";

import { APIError, Client, type Mail } from "../src/index.js";

// Mailbox surface (M937) against a stub answering the /api/v1/mailbox routes.

let server: Server;
let base: string;
let lastBody: Record<string, unknown> | null = null;
let lastUrl = "";

const MSG: Mail = {
  id: "m-1",
  topic: "dm",
  from: "myapp",
  to: "researcher",
  text: "deploy target?",
  ts_unix_ms: 1700000000000,
};

function json(res: ServerResponse, code: number, obj: unknown): void {
  res.writeHead(code, { "Content-Type": "application/json" });
  res.end(JSON.stringify(obj));
}

before(async () => {
  server = createServer((req: IncomingMessage, res: ServerResponse) => {
    const url = req.url ?? "";
    lastUrl = url;
    if (req.method === "GET") {
      if (url.startsWith("/api/v1/mailbox/inbox")) {
        json(res, 200, { name: "researcher", waiting: [MSG], count: 1 });
      } else if (url.startsWith("/api/v1/mailbox/messages/m-1/replies")) {
        json(res, 200, {
          id: "m-1",
          replies: [{ ...MSG, id: "m-2", from: "researcher", to: "myapp", reply_to: "m-1" }],
          count: 1,
        });
      } else if (url.startsWith("/api/v1/mailbox/messages")) {
        json(res, 200, { messages: [MSG], count: 1 });
      } else if (url === "/api/v1/mailbox/topics") {
        json(res, 200, { topics: { dm: 2, status: 1 } });
      } else {
        json(res, 404, { error: { type: "not_found", message: "nope" } });
      }
      return;
    }
    let raw = "";
    req.on("data", (c) => (raw += c));
    req.on("end", () => {
      lastBody = JSON.parse(raw || "{}") as Record<string, unknown>;
      if (url === "/api/v1/mailbox/messages") {
        json(res, 201, { message: { ...MSG, ...lastBody } });
      } else if (url === "/api/v1/mailbox/messages/m-1/ack") {
        json(res, 200, { acked: true, id: "m-1", by: lastBody.by });
      } else if (url.endsWith("/ack")) {
        json(res, 404, { error: { type: "not_found", message: "no message" } });
      } else {
        json(res, 404, { error: { type: "not_found", message: "nope" } });
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

after(() => server.close());

function client(): Client {
  return new Client(base, "testtoken", { timeoutMs: 5000 });
}

test("mailboxSend posts a DM and returns the message", async () => {
  const m = await client().mailboxSend({ from: "myapp", to: "researcher", text: "deploy target?" });
  assert.equal(m.id, "m-1");
  assert.deepEqual(lastBody, { text: "deploy target?", from: "myapp", to: "researcher" });
});

test("mailboxBroadcast targets every inbox; help flag is forwarded", async () => {
  await client().mailboxBroadcast("myapp", "heads-up");
  assert.equal(lastBody?.to, "*");
  await client().mailboxSend({ from: "w", text: "stuck", help: true });
  assert.equal(lastBody?.help, true);
});

test("mailboxInbox builds the query and maps the waiting list", async () => {
  const mails = await client().mailboxInbox("researcher", true, 5);
  assert.equal(mails.length, 1);
  assert.equal(mails[0].text, "deploy target?");
  assert.equal(lastUrl, "/api/v1/mailbox/inbox?name=researcher&all=true&limit=5");
});

test("mailboxAck posts by; unknown id throws APIError(404)", async () => {
  await client().mailboxAck("m-1", "researcher");
  assert.deepEqual(lastBody, { by: "researcher" });
  await assert.rejects(client().mailboxAck("nope", "researcher"), (e: unknown) => {
    assert.ok(e instanceof APIError);
    assert.equal(e.status, 404);
    return true;
  });
});

test("mailboxReplies / mailboxMessages / mailboxTopics map their shapes", async () => {
  const reps = await client().mailboxReplies("m-1");
  assert.equal(reps[0].reply_to, "m-1");
  const msgs = await client().mailboxMessages("dm", 3);
  assert.equal(msgs.length, 1);
  assert.equal(lastUrl, "/api/v1/mailbox/messages?topic=dm&limit=3");
  assert.deepEqual(await client().mailboxTopics(), { dm: 2, status: 1 });
});
