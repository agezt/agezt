import { test } from "node:test";
import assert from "node:assert/strict";
import * as http from "node:http";
import * as net from "node:net";

import { AgentClient, DEFAULT_SOCKET_PATH, resolveSocketPath } from "../src/agent.js";

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

// ---------------------------------------------------------------------------
// SDK-002. Everything above tests resolveSocketPath as a PURE FUNCTION, and
// that is exactly why this suite stayed green while EventbusHandle.subscribe()
// reached around the resolver with
// `(this.client as unknown as { socketPath: string }).socketPath` and connected
// to the raw "@agezt/agentgw.sock" — a CWD-relative file on Linux, where a
// planted listener collects the capability token. A helper test cannot see
// that. These assert the CALL SITE: what each connect path hands to
// http.request.
// ---------------------------------------------------------------------------

const TOKEN = "CAPABILITY-TOKEN";

interface Captured {
  /** The socketPath the SDK asked to connect to. */
  socketPath: string;
  /** The raw request head the gateway received. */
  head: string;
}

type ConnectionFactory = (options: net.NetConnectOpts, cb: () => void) => net.Socket;

/**
 * Run `use(client)` against a stand-in gateway and report exactly which address
 * the SDK asked for.
 *
 * Two seams, both local and both restored afterwards:
 *   - process.platform is forced to "linux", because off Linux
 *     resolveSocketPath is the identity and an assertion made there cannot tell
 *     the fix from the bug.
 *   - http.request() hands its whole options object (socketPath included) to
 *     the global agent's createConnection, so that is where the address is read
 *     back before the connection is diverted to a throwaway loopback listener.
 *     No unix socket or named pipe is created, and nothing leaves the machine.
 */
async function captureGatewayConnect(use: (client: AgentClient) => Promise<void>): Promise<Captured> {
  const realPlatform = process.platform;
  Object.defineProperty(process, "platform", { value: "linux", configurable: true });

  const captured: Captured = { socketPath: "", head: "" };

  const server = net.createServer((sock) => {
    sock.on("data", (chunk: Buffer) => {
      captured.head += chunk.toString();
      if (captured.head.includes("text/event-stream")) {
        sock.write(
          "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n" +
            'data: {"id":"e1","seq":1,"ts_unix_ms":0,"subject":"x","actor":"a","kind":"k"}\n\n'
        );
      } else {
        sock.write("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}");
      }
      sock.end();
    });
    sock.on("error", () => {});
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", () => resolve()));
  const { port } = server.address() as net.AddressInfo;

  const agent = http.globalAgent as unknown as { createConnection: ConnectionFactory };
  const realCreateConnection = agent.createConnection;
  agent.createConnection = function (options: net.NetConnectOpts, cb: () => void) {
    captured.socketPath = (options as { socketPath?: string }).socketPath ?? "";
    const diverted = { ...options, socketPath: undefined, path: undefined, host: "127.0.0.1", port };
    return realCreateConnection.call(this, diverted as unknown as net.NetConnectOpts, cb);
  };

  try {
    await use(new AgentClient({ token: TOKEN }));
  } finally {
    agent.createConnection = realCreateConnection;
    await new Promise<void>((resolve) => server.close(() => resolve()));
    Object.defineProperty(process, "platform", { value: realPlatform, configurable: true });
  }
  return captured;
}

test("subscribe() connects to the RESOLVED socket path, not the raw one (SDK-002)", async () => {
  const captured = await captureGatewayConnect(async (client) => {
    // The documented usage: iterating is what opens the connection.
    for await (const _ev of client.eventbus.subscribe("x.>")) break;
  });

  assert.equal(
    captured.socketPath,
    "\0agezt/agentgw.sock",
    "subscribe() must connect to the abstract address, which no planted file can shadow"
  );
  assert.notEqual(
    captured.socketPath,
    DEFAULT_SOCKET_PATH,
    "connected to the raw '@…' path — on Linux that is a CWD-relative FILE"
  );
  assert.ok(!captured.socketPath.startsWith("@"), "a leading @ would be a relative file path");
  // The assertion above only matters because this request carries the credential
  // the wrong address would have handed over.
  assert.match(captured.head, new RegExp(`Authorization: Bearer ${TOKEN}`));
});

test("unary requests connect to the RESOLVED socket path too", async () => {
  const captured = await captureGatewayConnect(async (client) => {
    await client.memory.search("x");
  });
  assert.equal(captured.socketPath, "\0agezt/agentgw.sock");
  assert.match(captured.head, new RegExp(`Authorization: Bearer ${TOKEN}`));
});

test("the address has one owner: an accessor, not a cast through private fields", () => {
  const client = new AgentClient({ token: TOKEN, socketPath: "/run/agezt/gw.sock" });
  // A cast reads whatever the field happens to hold; this reads what a connect
  // site may actually use. Off Linux the resolver is the identity, so this
  // assertion holds on every platform.
  assert.equal(client.resolvedSocketPath, resolveSocketPath("/run/agezt/gw.sock"));
  assert.equal(client.bearer, `Bearer ${TOKEN}`);
});
