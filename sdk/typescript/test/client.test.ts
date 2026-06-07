import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { type AddressInfo } from "node:net";

import { APIError, Client } from "../src/index.js";

let server: Server;
let base: string;
let lastBody: Record<string, unknown> | null = null;

function json(res: ServerResponse, code: number, obj: unknown): void {
  const body = JSON.stringify(obj);
  res.writeHead(code, { "Content-Type": "application/json" });
  res.end(body);
}

function authOK(req: IncomingMessage): boolean {
  return req.headers["authorization"] === "Bearer testtoken";
}

before(async () => {
  server = createServer((req, res) => {
    if (!authOK(req)) {
      json(res, 401, { error: { type: "unauthorized", message: "missing or invalid token" } });
      return;
    }
    const url = req.url ?? "";
    if (req.method === "GET" && url === "/api/v1/health") {
      json(res, 200, { status: "ok", version: "test", default_model: "m", model_count: 1 });
      return;
    }
    if (req.method === "GET" && url === "/api/v1/models") {
      json(res, 200, { default: "m", models: ["m", "n"] });
      return;
    }
    if (req.method === "GET" && url.startsWith("/api/v1/runs/")) {
      json(res, 200, { correlation_id: "c1", count: 2, events: [{ seq: 1 }, { seq: 2 }] });
      return;
    }
    if (req.method === "POST" && url === "/api/v1/runs") {
      let raw = "";
      req.on("data", (c) => (raw += c));
      req.on("end", () => {
        const body = JSON.parse(raw || "{}") as Record<string, unknown>;
        lastBody = body;
        if (body.intent === "boom") {
          json(res, 502, { correlation_id: "c2", model: "m", status: "failed", error: "provider exploded" });
          return;
        }
        if (body.stream) {
          res.writeHead(200, { "Content-Type": "text/event-stream" });
          res.write('event: start\ndata: {"correlation_id":"c3","model":"m"}\n\n');
          res.write('event: token\ndata: {"text":"hel"}\n\n');
          res.write(": heartbeat\n\n");
          res.write('event: token\ndata: {"text":"lo"}\n\n');
          res.write('event: done\ndata: {"correlation_id":"c3","status":"completed","answer":"hello"}\n\n');
          res.end();
          return;
        }
        json(res, 200, { correlation_id: "c4", model: body.model ?? "m", status: "completed", answer: "pong" });
      });
      return;
    }
    json(res, 404, { error: { type: "not_found", message: "nope" } });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as AddressInfo).port;
  base = `http://127.0.0.1:${port}`;
});

after(() => server.close());

function client(token = "testtoken"): Client {
  return new Client(base, token, { timeoutMs: 5000 });
}

test("health", async () => {
  const h = await client().health();
  assert.equal(h.status, "ok");
  assert.equal(h.version, "test");
});

test("models", async () => {
  const m = await client().models();
  assert.equal(m.default, "m");
  assert.ok(m.models.includes("n"));
});

test("run (sync) forwards the model and returns the answer", async () => {
  const r = await client().run("ping", "m");
  assert.equal(r.status, "completed");
  assert.equal(r.answer, "pong");
  assert.equal(r.correlation_id, "c4");
  assert.equal(lastBody?.model, "m");
});

test("a failed run throws APIError(502)", async () => {
  await assert.rejects(
    () => client().run("boom"),
    (e: unknown) => e instanceof APIError && e.status === 502 && e.detail.includes("provider exploded"),
  );
});

test("runStream yields start/token/token/done and reassembles tokens", async () => {
  const events = [];
  for await (const ev of client().runStream("hi")) events.push(ev);
  assert.deepEqual(events.map((e) => e.event), ["start", "token", "token", "done"]);
  const tokens = events.filter((e) => e.event === "token").map((e) => String(e.data.text ?? "")).join("");
  assert.equal(tokens, "hello");
  assert.equal(events.at(-1)?.data.answer, "hello");
});

test("getRun returns the event arc", async () => {
  const arc = await client().getRun("c1");
  assert.equal(arc.count, 2);
  assert.equal(arc.events.length, 2);
});

test("a bad token throws APIError(401)", async () => {
  await assert.rejects(
    () => client("WRONG").health(),
    (e: unknown) => e instanceof APIError && e.status === 401 && e.type === "unauthorized",
  );
});
