# 12 — SDKs and the Cross-Language Contract

**Scope:** `sdk/` (Go SDK), `sdk/python`, `sdk/typescript`, `sdk/rust`, `contract/` (`fixtures/`, `gen/`), the generators/gates
`tools/jsonschemagen` + `tools/sdkparity`, `examples/agezt-run`, and the policy docs `docs/SDK-PARITY.md`, `docs/API-STABILITY.md`,
`docs/EVENT-SCHEMA.md` (claims verified against the code on disk, 2026-10-02).

Sibling docs: server side of the REST / control-plane / agent-gateway surfaces is in [03-control-plane-and-http.md](03-control-plane-and-http.md);
event kinds and the journal in [06-data-memory-state.md](06-data-memory-state.md); the *plugin-author* SDK (`plugins/sdk`) and providers in
[08-providers.md](08-providers.md); CI/Makefile wiring in [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md); frontend fixture consumers in
[11-frontend-console.md](11-frontend-console.md).

---

## 1. Responsibilities at a glance

| Area | What it is | Talks to | Auth |
|---|---|---|---|
| `sdk/` (Go, package `sdk`) | Embeddable Go client for a **local** daemon. Runs intents, streams raw journal events, lists runs, resolves approvals, uses the mailbox. | **Control plane** (newline-delimited JSON over TCP, address from `<AGEZT_HOME>/runtime/control.addr`) — *not* REST. | Token from `runtime/control.token`, overridable by `AGEZT_TOKEN`; sent in every request body (`"token"` field). |
| `sdk/python` (`agezt` 1.1.0) | Two clients: `Client`/`AsyncClient` (REST app client) and `AgentClient` (subprocess "Agent SDK"). Stdlib only. | REST `/api/v1/*` (HTTP + SSE) / agent gateway (`/v1/*` over a unix socket or TCP). | `Authorization: Bearer <rest.token or tenant token>`, optional `X-Agezt-Tenant`; agent side: `Bearer <agentgw capability token>`. |
| `sdk/typescript` (`@agezt/sdk` 1.1.0) | Same two clients as Python: `Client` (platform `fetch`) and `AgentClient` (`node:http` over unix socket). Zero runtime deps. | Same as Python. | Same as Python. |
| `sdk/rust` (`agezt` 1.0.0) | Blocking REST client only, std-only (own HTTP/1.1 client and JSON codec). **No agent-gateway client.** | REST `/api/v1/*`, `http://` only. | Bearer + optional `X-Agezt-Tenant`. |
| `contract/gen` | `types.gen.go`, Go types generated from `.project/agezt-contract.jsonc` (the **plugin JSON-RPC** contract). Git-ignored, zero importers. | n/a | n/a |
| `contract/fixtures` | 8 hand-written JSON event-payload fixtures + a Go shape-sanity test; 2 of them also read by frontend Vitest tests. | n/a | n/a |
| `tools/jsonschemagen` | JSONC → Go struct generator (stdlib only). `make gen`. | reads `.project/agezt-contract.jsonc` | — |
| `tools/sdkparity` | Static route-string coverage report `docs/SDK-PARITY.md`; `-check` fails CI if the doc is stale. | reads `kernel/restapi/restapi*.go`, SDK source trees | — |
| `examples/agezt-run` | Minimal Go SDK program: stream a run, print cost, list last 5 runs. | Go SDK | — |

Import graph (from `internal-deps.txt`): `sdk -> internal/paths kernel/controlplane kernel/event`; `examples/agezt-run -> sdk`;
`contract/fixtures ->` (nothing); `contract/gen ->` (nothing). **Nothing in the Go tree imports `contract/gen`.** The only Go importers of
`github.com/agezt/agezt/sdk` are `examples/agezt-run/main.go` and `sdk/example_test.go`.

---

## 2. The three wire surfaces SDKs target

```
                         +--------------------------- agezt daemon ---------------------------+
 Go SDK (sdk/)  -------> | control plane: TCP, NDJSON Request/Response  (kernel/controlplane)  |  token = runtime/control.token
                         |   addr: <base>/runtime/control.addr                                  |        (or $AGEZT_TOKEN)
 Py/TS/Rust Client ----> | REST /api/v1 (kernel/restapi)  [only if AGEZT_REST_ADDR is set]     |  token = <base>/rest.token
                         |   HTTP JSON + SSE (runs stream, mailbox watch)                       |        (minted fresh each boot)
 Py/TS AgentClient ----> | agent gateway /v1/* (kernel/agentgw), HTTP/1.1 over unix socket      |  token = HMAC "JWT-like"
                         |   default listen: @agezt/agentgw-<8 hex>.sock (random per boot!)     |        capability token
                         +-------------------------------------------------------------------+
```

### 2.1 Control plane (Go SDK)

- Request (one JSON line): `{"id":"q-<hhmmss.mmm>","cmd":"<name>","token":"<tok>","args":{...}}` (`controlplane.Request`, `kernel/controlplane/protocol.go`).
- Response lines: `{"id","type":"event"|"result"|"error","event"?,"result"?,"error"?}` (`controlplane.Response`).
- One TCP connection per call (`Client.dial`, 5 s dial timeout, 10 s write deadline). Read deadline derived from the ctx deadline; i/o timeouts
  are mapped back to `context.DeadlineExceeded`.
- Commands used by the Go SDK: `run` (streaming, `Stream`), `runs_list`, `approvals`, `decide`, `board_send`, `board_inbox`, `board_ack`,
  `board_replies`, `board_read`, `board_get`, `pulse_subscribe` (open-ended, `StreamUntilCancel`).
- `docs/API-STABILITY.md` labels this protocol **internal** while labelling the Go SDK built on it **beta** — the doc itself calls the Go SDK
  "transitional" (Known gaps #1).

### 2.2 REST `/api/v1` (Python / TypeScript / Rust `Client`)

Registered in `kernel/restapi/restapi_routes.go` (`Server.Handler`). Off unless `AGEZT_REST_ADDR` is set
(`cmd/agezt/httpsurfaces_apis.go: buildRESTAPI`), which mints a random 32-byte hex token per boot, writes it to `<base>/rest.token` (0600) and
prints only a prefix in the banner.

| Route | Methods | Tier | SDK use | Response shape |
|---|---|---|---|---|
| `/api/v1/health` | GET | user | `health()` | `{status, version, default_model, model_count}` |
| `/api/v1/models` | GET | user | `models()` | `{default, models[]}` |
| `/api/v1/runs` | POST | user | `run`, `run_stream` | body `{intent, model?, stream?}` only. Sync: 200 `{correlation_id, model, status:"completed", answer}` or **502** `{correlation_id, model, status:"failed", error:"…"}`. Stream (body `stream:true` **or** `Accept: text/event-stream`): SSE `start {correlation_id, model}` → `token {text}`* → `done {…}` / `error {…}` |
| `/api/v1/runs/{id}` | GET | user | `get_run` | `{correlation_id, count, events[]}` |
| `/api/v1/artifacts`, `/api/v1/artifacts/` | GET | user | **none** | — |
| `/api/v1/mailbox/messages` | GET, POST | **admin** | `mailbox_messages`, `mailbox_send` | GET `{messages, count}`; POST 201 `{message}` |
| `/api/v1/mailbox/messages/{id}/ack`, `/replies` | GET, POST | **admin** | `mailbox_ack`, `mailbox_replies` | `{acked,id,by}`, `{id, replies, count}` |
| `/api/v1/mailbox/inbox` | GET `?name=&all=&limit=` | **admin** | `mailbox_inbox` | `{name, waiting, count}` |
| `/api/v1/mailbox/watch` | GET `?name=&topic=` (SSE) | **admin** | `mailbox_watch` | `ready` frame, then `mail` frames; `: keepalive` comment every 25 s |
| `/api/v1/mailbox/topics` | GET | **admin** | `mailbox_topics` | `{topics:{name:count}}` |
| `/api/v1/update`, `/update/apply` | GET / POST | admin | intentionally none | — |

Errors: `writeErr` emits `{"error":{"type","message"}}` (`kernel/restapi/restapi_prom.go`). All three REST SDKs parse both that shape and the
failed-run shape `{"status":"failed","error":"…"}`. Note `docs/API-STABILITY.md` says `{error:{code,message,details?}}` — the key is actually
`type`, not `code`.

The mailbox routes are admin-tier because the board is daemon-global with no tenant partition (V-011 comment). SDK docstrings saying the
client works with "the daemon's admin token or a tenant token" are only true for health/models/runs; a tenant token gets 401 on mailbox calls.

### 2.3 Agent gateway (Python / TypeScript `AgentClient`)

Server: `kernel/agentgw/gateway.go: Gateway.Listen`. Routes: `GET /v1/eventbus/subscribe`, `POST /v1/eventbus/publish`,
`POST /v1/memory/write`, `DELETE /v1/memory/delete`, `GET /v1/memory/search`, `GET /v1/log/read`, `POST /v1/log/write`, `GET /v1/agent/list`,
`GET /v1/agent/query`, `POST /v1/token/create`, `GET /v1/config/{key}`, `GET /v1/config`, `GET /v1/config/search`, `POST /v1/config`,
`GET /v1/config/audit`, `GET /health` (no auth). Token = HMAC-SHA256 JWT-like (`iss`/`aud` = `agezt-agentgw`), secret from
`AGEZT_AGENTGW_TOKEN_SECRET` or a per-install file (`agentgw.ResolveTokenSecret`); minted by `agt token` (`cmd/agt/token.go`).

**Reachability gap (verified):** both SDKs default to `"@agezt/agentgw.sock"`, but `agentgw.DefaultGatewayConfig` listens on
`uniqueSocketPath()` = `"@agezt/agentgw-<random 8 hex>.sock"` and `kernel/runtime/compose.go` only overrides it from `AGEZT_AGENTGW_SOCKET`.
The chosen path is never written to disk or exported to child processes, and nothing in Go sets `AGEZT_AGENT_TOKEN`. So the Agent SDK's
default constructor cannot reach a production daemon unless the operator sets `AGEZT_AGENTGW_SOCKET=@agezt/agentgw.sock` (or passes a
matching `socket_path`). The TS test `"the shipped default still uses the @ form the daemon binds"` pins this stale assumption.

---

## 3. Method-by-method parity matrix

Legend: name = present; **—** = absent; *(REST)* = goes through `/api/v1`; *(CP)* = control plane.

### 3.1 App client

| Capability | Go `sdk.Client` (CP) | Python `Client` / `AsyncClient` | TypeScript `Client` | Rust `Client` |
|---|---|---|---|---|
| Construct | `Dial(baseDir)` (`""` → `$AGEZT_HOME`/`~/.agezt`); `DefaultBaseDir()` | `Client(base_url, token, timeout=30.0, tenant=None)`; rejects non-http(s) scheme (`ValueError`) | `new Client(baseUrl, token, {timeoutMs=30000, tenant})`; no scheme check | `Client::new(base, token)`, `.with_timeout(Duration)`, `.with_tenant(t)`; `https://` rejected at request time (no TLS) |
| Health | — | `health() -> dict` | `health(): Promise<Health>` | `health() -> Result<Health>` |
| Models | — | `models() -> dict` | `models(): Promise<Models>` | `models() -> Result<Models>` |
| Blocking run | `Run(ctx, intent, opts...) (*Result, error)` | `run(intent, model=None) -> RunResult` | `run(intent, model?)` | `run(intent, Option<&str>)` |
| Run result fields | `Answer, CorrelationID, Model, Iterations, CostUSD` | `correlation_id, model, status, answer` | same as Py | same as Py |
| Streaming run | `RunStream(ctx, intent, onEvent func(*Event), opts...)` — callback gets **raw journal events** (`event.Event`) | `run_stream(...) -> Iterator[StreamEvent]` (`start/token/done/error`) | `runStream(...)`: `AsyncGenerator<StreamEvent>` | `run_stream(...) -> Result<RunStream>` (`Iterator<Item=Result<StreamEvent>>`) |
| Per-run options | `WithModel`, `WithTenant`, `WithSystem`, `WithTimeout`, `WithTools(names...)` (empty = explicit no-tools), `WithImages(dataURLs...)`, `WithMaxCostUSD` | model only (tenant is per-client header) | model only | model only |
| Event helpers | `TokenText(ev)`, `ToolCall(ev)`, `IsTerminal(ev)` | — (inspect `ev.event`/`ev.data`) | — | — (`Value::str` etc.) |
| Get run event arc | — | `get_run(id) -> dict` | `getRun(id): Promise<RunArc>` | `get_run(id) -> RunArc` |
| List runs | `Runs(ctx, limit) ([]RunInfo, error)` | — | — | — |
| Pending approvals | `PendingApprovals(ctx) ([]Approval, error)` | — | — | — |
| Approve / deny | `Approve(ctx,id,reason)`, `Deny(ctx,id,reason)` | — | — | — |
| Mailbox send | `SendMail(ctx, MailDraft) (Mail, error)` | `mailbox_send(text, *, from_, to, topic, reply_to, help) -> Mail` | `mailboxSend(MailDraft)` | `mailbox_send(&MailDraft)` |
| Broadcast | `Broadcast(ctx, from, text)` | `mailbox_broadcast(from_, text)` | `mailboxBroadcast(from, text)` | `mailbox_broadcast(from, text)` |
| Inbox | `Inbox(ctx, name, includeRead, limit)` | `mailbox_inbox(name, include_read=False, limit=0)` | `mailboxInbox(name, includeRead=false, limit=0)` | `mailbox_inbox(name, include_read, limit: u32)` |
| Ack | `AckMail(ctx, id, by)` | `mailbox_ack(message_id, by)` | `mailboxAck(messageId, by)` | `mailbox_ack(message_id, by)` |
| Replies | `MailReplies(ctx, id, limit)` | `mailbox_replies(message_id, limit=0)` | `mailboxReplies(messageId, limit=0)` | `mailbox_replies(message_id, limit)` |
| Messages | `MailMessages(ctx, topic, limit)` | `mailbox_messages(topic="", limit=0)` | `mailboxMessages(topic="", limit=0)` | `mailbox_messages(topic, limit)` |
| Topics | — | `mailbox_topics() -> Dict[str,int]` | `mailboxTopics()` | `mailbox_topics() -> BTreeMap<String,i64>` |
| Watch (push) | `WatchMail(ctx, name, fn func(Mail)) error` — **name filter only**, no topic; blocks until ctx cancel | `mailbox_watch(name="", topic="") -> Iterator[Mail]` | `mailboxWatch(name="", topic="")`: `AsyncGenerator<Mail>` | `mailbox_watch(name, topic) -> Result<MailWatch>` |
| Mail time field | `At time.Time` (from `ts_unix_ms`) | `ts_unix_ms: int`; sender is `from_` | `ts_unix_ms?: number`; most fields optional | `ts_unix_ms: i64` |
| Async variant | ctx-based, sync | `AsyncClient` (same surface, `run_in_executor` + `asyncio.Queue` bridge; `aclose`, `async with`) | natively async | blocking only |
| Artifacts | — | — | — | — |
| Errors | `*controlplane.ErrServerError{Msg}`, wrapped net/ctx errors | `AgeztError` ⊃ `APIError(status, type, message)`, `ConfigAccessError`; transport errors are raw `urllib`/`OSError`/`TimeoutError` | `AgeztError` ⊃ `APIError(status, type, detail)`, `ConfigAccessError`; transport = fetch `TypeError`/`AbortError` | `Error::Api{status, kind, message}`, `Error::Transport(String)`; `Error::status()` |

### 3.2 Agent-subprocess client ("Agent SDK")

| Handle.method | Gateway route | Python `AgentClient` | TypeScript `AgentClient` | Go | Rust |
|---|---|---|---|---|---|
| construct | — | `AgentClient(token, socket_path=DEFAULT_SOCKET_PATH, timeout=30.0)` | `new AgentClient({token, socketPath?, timeoutMs?})` | — | — |
| `eventbus.publish(event, payload?)` | `POST /v1/eventbus/publish` | yes | yes | — | — |
| `eventbus.subscribe(pattern=">")` | `GET /v1/eventbus/subscribe` (SSE) | `Iterator[dict]` (pattern **not URL-encoded**) | `AsyncGenerator<BusEvent>` (encoded) | — | — |
| `memory.write(type, subject, content, tags?)` | `POST /v1/memory/write` | returns `record` dict | `Promise<MemoryRecord>` | — | — |
| `memory.search(query, limit=20)` | `GET /v1/memory/search` | `q` **not URL-encoded** | encoded | — | — |
| `memory.delete(id)` | `DELETE /v1/memory/delete` | id not encoded | encoded | — | — |
| `log.write(message, level="info", meta?)` | `POST /v1/log/write` | yes | `write(message, {level, meta})` | — | — |
| `agent.list()` / `agent.query(id)` | `GET /v1/agent/list` / `query` | yes (id not encoded) | yes | — | — |
| `config.get(key, reason?)` | `GET /v1/config/{key}` | key not encoded | key encoded | — | — |
| `config.list_keys()` / `listKeys()` | `GET /v1/config` | yes | yes | — | — |
| `config.search(query)` | `GET /v1/config/search` | yes (encoded) | yes | — | — |
| `Capability` constants | — | 11 (no `CONFIG_*`) | 14 (+ `CONFIG_ACCESS/LIST/SEARCH`) | — | — |
| Error type | — | `AgentError(code, message, status_code)` | `AgentError(code, message, statusCode)` | — | — |
| `GET /v1/log/read`, `POST /v1/token/create`, `POST /v1/config`, `GET /v1/config/audit` | — | — | — | — | — |

The gateway also defines capabilities with no SDK constant (`channel.send/read/list`, `db.query/read/write`, `config.write`;
`kernel/agentgw/types.go`).

---

## 4. Go SDK — `sdk/` (package `sdk`)

**Purpose.** Public Go embedding API over the local control plane. Mirrors `agt run` argument encoding "byte-for-byte" (`buildRunArgs`).
**Depends on:** `internal/paths` (`BaseDir`), `kernel/controlplane` (`Client`, `Cmd*`), `kernel/event` (`Event`, `Kind*`).
**Exported:** `Client`, `Dial`, `DefaultBaseDir`, `Event` (= `event.Event` alias — kernel type leaks into the public API), `Result`, `Option`,
`With*` options, `RunInfo`, `Approval`, `Mail`, `MailDraft`, `TokenText`, `ToolCall`, `IsTerminal`.
**Persistence:** none written; reads `<base>/runtime/control.addr` and `<base>/runtime/control.token`.
**Env:** `AGEZT_HOME` (via `paths.BaseDir`), `AGEZT_TOKEN` (via `controlplane.NewClient`, overrides on-disk token — tenant auth path).
**Concurrency:** `Client` is a value holding `*controlplane.Client{addr, token}` — stateless, safe for concurrent use (new TCP conn per call).
`RunStream`/`WatchMail` callbacks run on the stream-reading goroutine (the caller's), so they must not block. `StreamUntilCancel` spawns one
watcher goroutine that closes the conn on ctx cancel.
**Events consumed:** `llm.token`, `tool.invoked`, `task.completed`, `task.failed` (helpers); `board.posted` (watch, via `pulse_subscribe` with
`pattern:"board.>"`, `kinds:["board.posted"]`).

| File | What it does |
|---|---|
| `sdk.go` | Package doc; `Client`, `Dial` (resolves base dir, builds `controlplane.Client`; no network until first call), `Result`, `runConfig` + `With*` options (`WithMaxCostUSD` converts $ → microcents ×1e9, ignores ≤0; `WithTools()` with zero names = explicit empty allow-list), `Run`/`RunStream` (`CmdRun` via `cp.Stream`), `buildRunArgs` (timeout as Go duration string, tools/images as `[]any`, `max_cost` as float64 microcents), `parseResult` (`answer`, `correlation_id`, `model`, `iters`, `spent_mc`/1e9), `intFromAny`. |
| `runs.go` | `RunInfo` + `Runs(ctx, limit)` → `CmdRunsList`; `parseRuns` maps `started_unix_ms`/`duration_ms` to `time.Time`/`time.Duration`, `parent_correlation`, `reason`. |
| `events.go` | Pure helpers over `*Event`: `TokenText` (payload `text` of `llm.token`), `ToolCall` (payload `tool` of `tool.invoked`), `IsTerminal`. |
| `approvals.go` | `Approval`, `PendingApprovals` (`CmdApprovals`, reads `pending[]`: `id, capability, tool_name, reason, actor, input, timeout_unix`), `Approve`/`Deny` → `CmdDecide{id, decision:"grant"|"deny", reason}`; `anyToString` re-encodes structured `input` as compact JSON. |
| `mailbox.go` | `Mail`, `MailDraft`, `SendMail` (`CmdBoardSend`, result `sent`), `Broadcast` (`to:"*"`), `Inbox` (`CmdBoardInbox{to, all, limit}` → `waiting`), `AckMail`, `MailReplies`, `MailMessages`, `WatchMail` (subscribes `pulse_subscribe`, filters with `mailForName`, then fetches each body with `CmdBoardGet`, falling back to metadata if evicted), `parseMail(s)`. |

Tests (`*_test.go`, 51 tests): `client_call_test.go` (26) stands up `fakeCP`, a real TCP listener speaking the NDJSON protocol, writes
runtime files into a temp base dir and drives every method end-to-end (clears `AGEZT_TOKEN`); `sdk_test.go` arg building/`Dial`; `runs_test.go`,
`approvals_test.go`, `mailbox_test.go`, `events*_test.go`, `parse_edge_test.go`, `intfromany_test.go` parser edge cases;
`example_test.go` = godoc examples (`ExampleClient_Run`, `_RunStream`, `_Runs`, `_PendingApprovals`; no `// Output:` so not executed).

`examples/agezt-run/main.go` — CLI flags `-model`, `-timeout` (default 5m); streams with `TokenText`/`ToolCall`, prints correlation, model,
iterations, cost, then `Runs(ctx, 5)`.

---

## 5. Python SDK — `sdk/python` (`agezt` 1.1.0, `requires-python >=3.9`, `dependencies = []`)

**Concurrency:** `Client` is stateless apart from a per-instance `urllib` opener; one HTTP connection per call. `AsyncClient` runs every
blocking call in the default thread-pool executor; streams use a producer thread → `loop.call_soon_threadsafe(queue.put_nowait, …)` with a
unique `done` sentinel; exceptions are forwarded through the queue and re-raised in the consumer. Abandoning an `async for` early does **not**
stop the producer thread (it runs until the HTTP stream ends). `AgentClient`'s `_SocketClient` serializes unary requests with a
`threading.Lock`; `subscribe` opens its own socket outside the lock.

| File | What it does |
|---|---|
| `agezt/__init__.py` | Module docstring/quick-start; re-exports `Client`, `AsyncClient`, `Mail`, `AgentClient`, `Capability`, `AgentError`, `RunResult`, `StreamEvent`, `AgeztError`, `APIError`, `ConfigAccessError`; `__version__ = "1.1.0"`. |
| `agezt/client.py` | REST `Client`: dataclasses `RunResult`, `Mail` (`from_` field), `StreamEvent`; `_SameOriginRedirectHandler` refuses cross-origin redirects (PY-002) and the token is attached with `add_unredirected_header`; scheme allow-list `http`/`https` (PY-006); path ids quoted with `safe=""` (PY-005); `_api_error` maps both error shapes; `_parse_sse` (strips exactly one leading space after `data:`, `:` comments skipped, non-JSON data → `{"raw": …}`, flushes trailing frame). |
| `agezt/aio.py` | `AsyncClient` wrapping a private sync `Client`; same methods as coroutines / async generators; `aclose()` no-op; `async with`. |
| `agezt/agent.py` | Agent SDK: `DEFAULT_SOCKET_PATH = "@agezt/agentgw.sock"`; `_resolve_socket_path` (leading `@` → `b"\0…"` abstract address on Linux only, SDK-001); `Capability` constants; `AgentError`; `_SocketClient` (hand-rolled HTTP/1.1 over AF_UNIX, or TCP for `host:port`/`tcp://`; falls back to AF_INET when AF_UNIX is unavailable; reads to EOF; decodes chunked); handles `_EventbusHandle`, `_MemoryHandle`, `_LogHandle`, `_AgentHandle`, `_ConfigHandle`; `_AgentClient` base with `socket_path` property owned by the transport (SDK-002: single connect path); `AgentClient(_AgentClient)`. |
| `agezt/errors.py` | `AgeztError`, `APIError(status, type, message)`, `ConfigAccessError(key, code, message, status=403)`. |
| `examples/agent_example.py` | Agent-SDK demo reading `AGEZT_AGENT_TOKEN`: memory write/search, eventbus publish, log write, agent list. |
| `pyproject.toml` | setuptools build, package discovery `agezt*`, no deps. |
| `README.md` | Install, quick start, method → endpoint table (REST client only; Agent SDK undocumented here). |

Tests (`tests/`, stdlib `unittest`, run by CI job `python-sdk`): `test_client.py` (in-process `HTTPServer` mock `_Handler`; health/models/run/
stream/get_run/401/tenant header; `ParseSSEFieldTest` pins the single-space rule shared with Rust/TS), `test_aio.py` (reuses `_Handler`),
`test_mailbox.py`, `test_client_security.py` (redirect token-leak, scheme validation, path-segment encoding), `test_agent_security.py`
(fake socket module; every connect site uses the resolved abstract address).

---

## 6. TypeScript SDK — `sdk/typescript` (`@agezt/sdk` 1.1.0, ESM, Node ≥18)

Build: `tsc -p tsconfig.json` (`target ES2022`, `module NodeNext`, `strict`, `outDir dist`, `rootDir .`, includes `src`, `test`, `examples`).
`dist/` is git-ignored (`.gitignore:55`) and built on test/publish. devDeps only (`typescript ^7.0.2`, `@types/node`). `package.json` `files`
lists `dist/examples` and tsconfig includes `examples`, but no `examples/` directory exists. A local `pnpm-lock.yaml` exists untracked next to
the tracked `package-lock.json` (CI uses npm).

**Concurrency:** single-threaded event loop. REST `Client.fetch` arms an `AbortController` timer and clears it in `.finally` on the fetch
promise — which settles when **headers** arrive, so the timeout bounds time-to-headers only, not SSE body duration (the `mailboxWatch` doc
comment implies otherwise). `parseSSE` calls `reader.cancel()` in `finally` so early `break` aborts the socket (tested).

| File | What it does |
|---|---|
| `src/index.ts` | Public barrel: `Client` + types (`ClientOptions, Health, Mail, MailDraft, Models, RunResult, RunArc, StreamEvent`), errors, `AgentClient`, `Capability`, agent types (`AgentClientOptions, MemoryRecord, SearchResult, AgentProfile, BusEvent`), `AgentError`. `DEFAULT_SOCKET_PATH`/`resolveSocketPath` and handle classes are exported from `agent.ts` but not re-exported. |
| `src/client.ts` | REST `Client`: `health`, `models`, `run`, `runStream`, `getRun` (`encodeURIComponent`), mailbox methods; private `getJSON`, `fetch` (Bearer, Accept, Content-Type when body, `X-Agezt-Tenant`); `apiError` (both error shapes); `parseSSE` / `indexOfFrameEnd` (`\n\n` or `\r\n\r\n`) / `parseFrame` (strips one leading space). No scheme validation, no redirect guard (relies on platform `fetch` redirect semantics). |
| `src/agent.ts` | Agent SDK over `node:http` with `socketPath`: `resolveSocketPath` (`@` → `\0` on Linux), `Capability` const object + union type, `AgentError`, `AgentClient` (`resolvedSocketPath` and `bearer` accessors are the single connect path, SDK-002; `request` maps 401/403/429 to fixed `AgentError`s, other ≥400 to `error.code/message`), `MemoryHandle`, `EventbusHandle` (SSE via `ReadableStream`, splits on lines, JSON-parses `data:`), `LogHandle`, `AgentHandle`, `ConfigHandle`. |
| `src/errors.ts` | `AgeztError`, `APIError(status, type, detail)`, `ConfigAccessError(key, code, detail, status=403)`. |
| `package.json`, `tsconfig.json`, `package-lock.json` | Manifest (`exports["."]` → `dist/src/index.{js,d.ts}`), compiler config, lockfile. |
| `README.md` | REST client usage + method table. |

Tests (`test/*.test.ts`, `node --test` on compiled JS, CI job `typescript-sdk`): `client.test.ts` (mock `http.Server`; run/stream/arc/401/
tenant; early-break cancels SSE for both `runStream` and `mailboxWatch`), `mailbox.test.ts`, `agent.test.ts` (socket-path resolution and
single-owner accessor).

---

## 7. Rust SDK — `sdk/rust` (`agezt` 1.0.0, edition 2021, MSRV 1.70, `[dependencies]` empty, `#![forbid(unsafe_code)]`)

**Concurrency:** fully blocking; `Client` is `Clone` + plain data; one `TcpStream` per request with `Connection: close`; read/write timeouts
= client timeout (default 30 s). `RunStream`/`MailWatch` own the socket and parse lazily.

| File | What it does |
|---|---|
| `src/lib.rs` | Crate docs (doc-test `no_run` example), module wiring, re-exports `Client, Health, Mail, MailDraft, MailWatch, Models, RunArc, RunResult, RunStream, StreamEvent`, `Error, Result`, `Value`. |
| `src/client.rs` | Typed structs and `Client` methods (§3.1); `RunStream` (SSE iterator: `event:`/`data:`, one leading space stripped, `:` comments skipped, trailing frame flushed at EOF); `MailWatch` (yields only `mail` frames); helpers `run_body` (BTreeMap → sorted keys), `str_field`, `mail_from(s)`, `api_error`, `percent_encode` (RFC 3986 unreserved set). Unit tests inline. |
| `src/http.rs` | Minimal HTTP/1.1: `Target::parse` (http only; `https://` → explicit "no TLS" error; optional path prefix), `request` (`connect_timeout`, headers, `Content-Length`), `parse_status`, `BodyReader` with `BodyMode::{Length, Chunked, Eof}` usable as streaming `Read`. Does not follow redirects (3xx surfaces as `Error::Api`). Uses only the first resolved address. |
| `src/json.rs` | Dependency-free recursive-descent JSON `Value` (`Null, Bool, Int(i64), Float(f64), Str, Array, Object(BTreeMap)`), accessors `get/as_str/as_i64/as_f64/as_bool/as_array/str`, `parse` (rejects trailing data, validates surrogate pairs), `to_json`. No nesting-depth limit. |
| `src/errors.rs` | `Error::{Api{status, kind, message}, Transport(String)}`, `Display`, `From<io::Error>`, `Result<T>`. |
| `Cargo.toml`, `Cargo.lock`, `README.md` | Manifest, lock (single package), docs (`agezt = "1.0"`). |

Tests: `tests/client.rs` — a `TcpListener` mock (`start_mock`, `handle`) covering health/models/run/502/stream/arc/401/tenant/all mailbox
methods; CI job `rust-sdk` runs `cargo fmt --check` + `cargo test`.

---

## 8. Contract pipeline

### 8.1 `contract/gen` + `tools/jsonschemagen`

```
.project/agezt-contract.jsonc  (tracked; "Contract v1 (JSON-RPC 2.0 over stdio)" — plugin protocol, top level = bare "$schemas": {...})
        │  make gen  ==  go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
        ▼
stripJSONCComments (// only, string-aware) → wrap in {} → json.Unmarshal → for each schema name (sorted):
   object+properties → struct (fields sorted, snake_case→Exported w/ initialisms, non-required ⇒ ",omitempty")
   object+additionalProperties → map[string]T ; bare object → map[string]json.RawMessage
   string/integer/number/boolean → named scalar ; array → []T ; top-level $ref → type alias ; else → json.RawMessage + TODO
   enum-only / untyped properties → json.RawMessage
→ go/format.Source (on failure writes <out>.unformatted) → contract/gen/types.gen.go
```

Generated types today: `Attachment, Capability, ChatMessage, CompletionChunk, CompletionOptions, CompletionRequest, Contribution, Event,
EventEmit, HealthResult, Limits, ModelInfo, PluginError, ProviderChunk (= CompletionChunk), ProviderCompletionRequest, RegisterParams,
RegisterResult, ToolCall, ToolDef, ToolEvent, ToolInvocation, ToolSchema, UnifiedMessage, Usage`.

Facts that contradict the generator's own header ("CI verifies in sync"):
- `.gitignore:24` ignores `/contract/gen/*.gen.go`, so the file is untracked. The CI job (`ci.yml` ~L194) regenerates it and runs
  `git diff --exit-code -- contract/gen/`; `git diff` never reports untracked/ignored files, so **this drift gate cannot fail** except when
  the generator itself errors. (The 2026-09 audit records the ignore as "intentional"; the gate is still vacuous.)
- No Go package imports `contract/gen` (import graph + grep). The `multi-arch` CI job regenerates it before cross-build only so `./...`-style
  builds don't miss it. `docs/REFACTORING-SCAN-2026-08.md` already flags it as dead architecture (7-kind `Capability` enum, `RegisterParams`
  never implemented by `kernel/plugin`).
- It is **not** the REST SDK contract; none of the four app SDKs is generated from it. REST SDK types are hand-written per language.

Tests: `tools/jsonschemagen/main_test.go` (`TestStripJSONCComments`, `TestExportedName`, `TestRefTypeName`, `TestEndToEndContract`) and
`coverage_test.go` (every emit branch, error paths via `exitProcess` seam).

### 8.2 `contract/fixtures`

All fixtures are **hand-written** payload examples (not generated, not recorded from a daemon). Only `fixtures_test.go` (Go, package
`fixtures`) validates them — it checks each file's *own* shape invariants, not that any emitter produces it.

| Fixture | Event kind it models | Go test | Other consumers |
|---|---|---|---|
| `context_compacted_skill_rescue.json` | `context.compacted` (+ `skill_rescued_count/chars`) | `TestContextCompactedSkillRescueFixture` | `frontend/src/lib/chat.test.ts` ("folds the shared context.compacted skill rescue fixture") |
| `subagent_completed_async.json` | `subagent.completed` (async) | `TestSubAgentCompletedAsyncFixture` | `frontend/src/features/runs/lib/rundetail.test.ts`, `frontend/src/lib/replay.test.ts` |
| `subagent_spawned.json` | `subagent.spawned` | `TestSubAgentSpawnedFixture` | — |
| `skill_activated_explicit.json` | `skill.activated` | `TestSkillActivatedExplicitFixture` | — |
| `delegate_await_tool_result.json` | `tool.result` for `delegate_await` | `TestDelegateAwaitToolResultFixture` | — |
| `tool_search_result.json` | `tool_search` tool output | `TestToolSearchResultFixture` | — |
| `config_reload_boundaries.json` | live vs restart env apply classes | `TestConfigReloadBoundariesFixture` | — |
| `policy_decision.json` | `policy.decision` | **none** | **none** (orphan) |

Frontend tests read them via `readFileSync("../contract/fixtures/<name>")` (relative to `frontend/`). No Python/TS/Rust SDK test and no
kernel emitter test reads any fixture.

### 8.3 `tools/sdkparity` → `docs/SDK-PARITY.md`

1. `extractRoutes("kernel/restapi/restapi*.go")` concatenates matching files and applies regex `\.Handle(?:Func)?\("(/api/v1/[^"]*)"`
   (tied to call shape, not receiver name, after a 17-day silent-miss when `mux.HandleFunc` became `router.Handle`); empty glob = hard error.
   Trailing `/` ⇒ "prefix" route.
2. SDK trees: Go `sdk/*.go` (marked `NativeOnly` → `n/a`), Python `sdk/python/agezt/*.py`, TS `sdk/typescript/src/*.ts`, Rust `sdk/rust/src/*.rs`.
3. `sdkCovers`: substring search of the route path in any SDK file; for prefix routes also the path **without** the trailing slash.
4. `render` writes the table, per-SDK `covered/total` (excluding `/api/v1/update`, `/api/v1/update/apply` — `intentionallyUnsupportedInSDK`),
   and fixed prose listing which behavioral test files exist.
5. `-check <path>` compares CRLF-normalized output to the file and exits 1 if stale; `-out` rewrites. Run by `make sdk-parity` and CI job
   `deps-check` (`ci.yml` ~L478).

Verified 2026-10-02: `go run ./tools/sdkparity -check docs/SDK-PARITY.md` passes; Python/TS/Rust each 9/11 (both misses are `/api/v1/artifacts*`).

What it does **not** enforce: it never fails because an SDK lacks a route (only because the report is stale); method-level parity, request/
response field parity, the Go SDK, and the Agent SDK (`agent.py`/`agent.ts` routes are `/v1/*`, invisible to the `/api/v1` regex) are out of
scope. Prefix-route coverage is weak: `/api/v1/runs/` is "covered" by any file containing `/api/v1/runs` (the POST route string).

---

## 9. Key flows

**Go streaming run**
1. `sdk.Dial("")` → `paths.BaseDir()` → `controlplane.NewClient` reads `runtime/control.addr` + token (env `AGEZT_TOKEN` wins).
2. `RunStream` folds options → `buildRunArgs` → `cp.Stream(ctx, "run", args, cb)`.
3. One TCP conn; request line written; for each `{"type":"event"}` line, `onEvent(*event.Event)` (full journal event incl. `Seq`, `Hash`);
   `{"type":"result"}` → `parseResult`; `{"type":"error"}` → `*ErrServerError`.

**REST streaming run (Py/TS/Rust)**
1. `POST /api/v1/runs` with `{"intent","model"?, "stream":true}` and `Accept: text/event-stream`.
2. Server subscribes before starting (no lost tokens), emits `start`, `token{text}`…, then `done{correlation_id,status,answer?}` or `error`.
3. SDK parses frames into `StreamEvent{event, data}`; consumers concatenate `token.data.text`.

**Go mailbox watch**
1. `cp.StreamUntilCancel("pulse_subscribe", {pattern:"board.>", kinds:["board.posted"]})`.
2. Per `board.posted` event: decode metadata, `mailForName` filter (directed to name case-insensitively, or `to=="*"` not sent by name).
3. `board_get{id}` to fetch the body (metadata-only fallback) → `fn(Mail)`. One extra control-plane round trip per message.

**REST mailbox watch**: `GET /api/v1/mailbox/watch?name=&topic=` → `ready` frame (subscription live), `mail` frames with full message view,
`: keepalive` every 25 s (keeps Python/Rust 30 s socket read timeouts alive).

---

## 10. API stability tiers (from `docs/API-STABILITY.md`, checked against code)

| Surface | Declared tier | Code reality |
|---|---|---|
| REST `/api/v1/*` | beta | Matches; additive. Error key is `type` (doc says `code`). Mailbox is admin-only. |
| Web UI `/api/*` | internal | — (see 03/11) |
| OpenAI-compatible `/v1/...` | compatibility target | — (see 03) |
| Control-plane protocol | internal | Yet the **beta** Go SDK is built directly on it and aliases `kernel/event.Event` in its public API. |
| Plugin protocol | beta | `contract/gen` types for it are unimported/dead. |
| SDK packages | beta, semver per package | Python 1.1.0, TS 1.1.0, Rust 1.0.0, Go module-coupled. Versions are not feature-aligned: Rust has the same REST surface as the 1.1.0 SDKs but no Agent SDK. |
| Event/journal subjects | beta | `docs/EVENT-SCHEMA.md` rules match `kernel/event/event.go` (`Event` field order `id, seq, ts_unix_ms, prev_hash, hash, subject, actor, kind, correlation_id, causation_id, payload, tags`) and the append-only `kinds.go` comments. |

`docs/EVENT-SCHEMA.md` is a policy document (no global schema version); its consumer rules (match on `kind`, tolerate unknown fields) are what
the SDKs do: Go helpers decode only the field they need, REST SDKs keep `data` as an untyped map/`Value`.

Publishing: `.github/workflows/publish-sdks.yml` builds/dry-run-packages each SDK on release or manual dispatch and publishes only when
`PYPI_API_TOKEN` / `NPM_TOKEN` / `CARGO_REGISTRY_TOKEN` secrets exist.

---

## 11. Extension point — adding a new SDK method across all four languages

Decide first which surface the feature lives on. REST SDKs can only expose what `/api/v1` serves; the Go SDK can only expose what the control
plane serves.

1. **Server.**
   - REST: add the handler and `router.Handle("/api/v1/…", <tier RouteOpts>, h)` in `kernel/restapi/restapi_routes*.go` (keep the literal-path
     call shape — `sdkparity`'s regex depends on it). Use `writeErr(type, message)` for errors; choose user vs admin tier deliberately
     (admin for anything not tenant-partitioned).
   - Control plane (for Go): add a `Cmd*` constant + handler in `kernel/controlplane` (see 03), args read via the package's typed accessors.
2. **Go** (`sdk/<area>.go`): method on `*Client` calling `c.cp.Call` / `Stream` / `StreamUntilCancel`; a `parseX(map[string]any)` that uses
   comma-ok assertions and `intFromAny` so missing fields are zero (tests in `parse_edge_test.go` style); add a `fakeCP` handler case in
   `client_call_test.go`; optionally an `Example…` in `example_test.go`.
3. **Python** (`agezt/client.py`): method using `_get`/`_post_json`/`_request`; quote every path segment with `urllib.parse.quote(x, safe="")`,
   build queries with `urlencode`; add a dataclass with `_from` if typed. Mirror it in `aio.py` via `_in_thread` (or the queue bridge for
   streams). Export from `__init__.py`. Test with the `HTTPServer` mock in `tests/`.
4. **TypeScript** (`src/client.ts`): method using `getJSON`/`fetch` + `apiError`; `encodeURIComponent` for segments, `URLSearchParams` for
   queries; add the interface and export it from `src/index.ts`; add a `node:test` case and, if new, append the compiled test file to the
   `test` script in `package.json` (the list is explicit).
5. **Rust** (`src/client.rs`): typed `pub struct`, method using `get_json`/`post_json`/`send`, `percent_encode` for segments; re-export from
   `lib.rs`; add a branch to the mock `handle` in `tests/client.rs`; run `cargo fmt`.
6. **Parity report:** `go run ./tools/sdkparity -out docs/SDK-PARITY.md` (and extend `noteForRoute` / the behavioral-coverage prose in
   `render` if appropriate); `-check` must pass in CI.
7. **Docs/versions:** update each SDK README method table, bump the minor version (`pyproject.toml` + `__init__.__version__`,
   `package.json`, `Cargo.toml`), and the "Current SDK versions" table in `docs/API-STABILITY.md`.
8. **Event payloads consumed by SDKs/UI:** if the feature introduces a payload others parse, add a fixture in `contract/fixtures/` plus a
   `fixtures_test.go` shape test, and read it from the consumer's test (frontend pattern: `readFileSync("../contract/fixtures/…")`).

For an Agent-SDK capability: add the gateway route + `AgentCapability` in `kernel/agentgw`, then a handle method in both `agent.py` and
`agent.ts` (plus a `Capability` constant in both — they have already drifted). There is no Go or Rust agent client to update.

---

## 12. Gotchas / invariants

**Invariants enforced by tests**
- SSE `data:` lines strip exactly **one** leading space in all three REST SDKs (`ParseSSEFieldTest.test_matches_the_other_sdks_single_space_rule`).
- Python never sends the bearer token across origins or through urllib's redirect header copying (`test_client_security.py`); all requests go
  through the guarded opener (`test_every_urlopen_site_goes_through_the_guarded_opener`).
- Path ids stay inside one segment (Python `safe=""`, TS `encodeURIComponent`, Rust `percent_encode`).
- Agent SDK: every connect site uses the resolved (abstract on Linux) address — SDK-001/SDK-002 in both languages.
- TS stream consumers that `break` early cancel the HTTP body (no socket leak).
- Go `WithTools()` with no args ≠ omitting it (`TestWithTools_ExplicitEmptyVsOmitted`); `WithMaxCostUSD(≤0)` is a no-op.
- `sdkparity` must find live routes (`TestExtractRoutes_FindsLiveRoutes`), so moving registrations to a non-`restapi*.go` file or changing the
  call shape fails loudly.

**Verified defects / drift (not fixed here)**
1. **Agent SDK default socket is unreachable** — daemon binds `@agezt/agentgw-<random>.sock` unless `AGEZT_AGENTGW_SOCKET` is set; SDKs
   default to `@agezt/agentgw.sock`; the path is not advertised to subprocesses (§2.3).
2. **Python `_SocketClient` ignores HTTP status:** `_parse_http_response` parses the status code but never raises on 4xx/5xx, so a 401/403
   returns the error JSON as a "result" (e.g. `config.get` on a denied key returns `""`, `memory.write` returns `{}`). It also never sends
   `Connection: close` yet reads until EOF, so each unary call waits for the Go server's keep-alive idle close (`IdleTimeout` unset ⇒ falls back
   to `ReadTimeout` 30 s) or hits the 30 s socket timeout (`AgentError("TIMEOUT")`).
3. **Python `agent.py` `__all__` names a non-existent `AsyncAgentClient`** — `from agezt.agent import *` raises `AttributeError` (reproduced).
4. **Python agent handles don't URL-encode** `memory.search` `q`, `memory.delete` `id`, `agent.query` `id`, `eventbus.subscribe` `pattern`,
   `config.get` key (query/path injection); TS encodes all of them.
5. **`ConfigAccessError` is never produced from a real denial** in either SDK: Python cannot see the status (item 2); TS's `request()` turns
   403 into `AgentError("FORBIDDEN")`, which `ConfigHandle.get` re-wraps as `ConfigAccessError(code "INTERNAL_ERROR", status 500)`.
6. **Default 30 s timeout caps blocking runs** in Python/TS/Rust: `POST /api/v1/runs` sends no headers until the run finishes, so any run longer
   than 30 s fails client-side (Python `TimeoutError`/`URLError`, TS `AbortError`, Rust `Error::Transport`) while the daemon keeps running it.
   Streaming runs have no server keepalive either, so a >30 s silent gap (long tool call) kills Python/Rust streams; TS streams are unaffected
   because its timer only covers time-to-headers.
7. **`contract/gen` drift gate is vacuous** (ignored file + `git diff`) and the generated package is unimported (§8.1).
8. **`policy_decision.json` is an orphan fixture**; fixtures validate themselves, not emitters — a renamed payload field in the kernel would
   not fail any fixture test.
9. **Feature skew vs. `docs/API-STABILITY.md`'s "SDK-complete = all four support it or say why":** runs-list/approvals exist only in Go;
   health/models/get-run/topics only in REST SDKs; REST runs accept only `intent/model/stream` (no system/tools/timeout/images/max-cost, no
   iterations/cost in the result); `/api/v1/artifacts*` has no SDK; Rust has no Agent SDK; Go `WatchMail` has no topic filter.
10. **README drift:** Python/Rust READMEs say "note the token it prints" — the daemon prints only a prefix; the full REST token is in
    `<AGEZT_HOME>/rest.token` and changes on every restart. SDK docstrings claim tenant tokens work, but mailbox routes are admin-only.
11. **Layering:** the public Go SDK imports `kernel/controlplane` and `kernel/event` (a kernel type is part of its API) — acceptable inside one
    module but contrary to API-STABILITY's "control plane is internal".
12. Rust `json.rs` has no recursion-depth limit (deeply nested hostile JSON can overflow the stack); Rust uses only the first resolved socket
    address (no IPv4/IPv6 fallback) and never follows redirects.
13. TS `package.json` publishes `dist/examples` and tsconfig includes `examples`, but the directory does not exist.
