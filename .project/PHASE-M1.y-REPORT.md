# Phase Report — Milestone 1.y (out-of-process plugin host)

> Status: **shipped** · Date: 2026-05-29
> Per DECISIONS B0 (kernel↔plugin contract) and the M1.a / M1.x
> "out-of-process plugin host" deferral that's stood since the
> first phase report.
> Continues [PHASE-M1.x-REPORT.md](PHASE-M1.x-REPORT.md).

## Scope

The kernel has supported in-process tools since M0.5
(`shell`/`file`/`http`/`browser.read`), all compiled into the
agezt binary. Third parties wanting to add a tool had to either:

- fork agezt and rebuild (no separation of concerns), or
- wait for an MCP bridge that doesn't exist yet.

M1.y ships **out-of-process plugins**: a stdio JSON protocol that
lets operators register tools written in any language. The kernel
spawns the plugin process at daemon start, exchanges an
`initialize` handshake to learn the tool list, and routes each
tool invocation through the plugin process. Plugin tools appear
in the agent's tool registry alongside in-process ones — the
agent loop doesn't know the difference.

```
# Operator config:
export AGEZT_PLUGINS="search=/usr/local/bin/agezt-search,scrape=/opt/scraper/bin"

# Daemon log:
tools registered: shell(...), file(...), http(...), browser.read(...),
  plugin:search(3 tools), plugin:scrape(1 tools)

# Agent can now call:
search.web(query="...")        — routed to /usr/local/bin/agezt-search subprocess
search.news(query="...")
search.images(query="...")
scrape.fetch(url="...")        — routed to /opt/scraper/bin subprocess
```

| Concern | Status |
|---|---|
| Line-delimited JSON protocol over child stdio | ✅ |
| Methods: `initialize` (learn tools), `tool/invoke`, `shutdown` | ✅ tested |
| Stdlib-only on both sides (no gRPC, no MCP overhead) | ✅ |
| `Plugin.Tools(prefix)` returns wrapped `agent.Tool` map for daemon registry | ✅ tested |
| Prefix namespacing so multiple plugins can have same-named tools | ✅ tested |
| Concurrent invocations (id-correlated, no head-of-line blocking) | ✅ tested |
| Per-call timeout (`InvokeTimeout`, default 2 min) | ✅ |
| Initialize timeout (`InitTimeout`, default 10s) | ✅ tested |
| Plugin crash → tools marked unavailable → clear "plugin dead" error | ✅ tested |
| Bad binary path → clear error at Spawn time, daemon continues | ✅ tested |
| Bad init reply → Spawn fails, partially-started process torn down | ✅ |
| `Close` is idempotent + waits with grace before killing | ✅ tested |
| Stderr lines forwarded to operator-supplied Logger callback | ✅ |
| Reference echoplugin (Go) in `testdata/` as plugin-author template | ✅ |
| `AGEZT_PLUGINS` env var format: `prefix=path arg arg,prefix2=path2` | ✅ |
| Daemon: failed plugin logs warning, other plugins + in-process tools continue | ✅ |
| Tool-name conflict between plugin + in-process: in-process wins, warning logged | ✅ |

## Architecture

```
              ┌───────────────────────────────────┐
              │  agezt daemon (Go)               │
              │                                   │
              │   ┌──────────────┐  ┌──────────┐  │
              │   │ agent loop   │──│ registry │  │
              │   └──────────────┘  └────┬─────┘  │
              │                          │        │
              │                  ┌───────┼─────┐  │
              │                  │       │     │  │
              │              shell    file   plugin.remoteTool
              │                                     │           │
              │                              ┌──────┼────┐      │
              │                              │  Plugin   │      │
              │                              │  (Host)   │      │
              │                              └─────┬─────┘      │
              │                                    │ stdio JSON │
              └────────────────────────────────────┼────────────┘
                                                   │
                                ┌──────────────────▼──────────────────┐
                                │  child process (any language)       │
                                │  reads {"method":...} on stdin      │
                                │  writes {"result":...} on stdout    │
                                └─────────────────────────────────────┘
```

## Wire protocol

Line-delimited JSON, request/response with id correlation. Same
shape as the existing controlplane protocol so operators familiar
with one transfer to the other.

**Initialize** (host → plugin, on Spawn):
```json
{"id":"q-1","method":"initialize"}
```

**Initialize result** (plugin → host):
```json
{"id":"q-1","result":{"tools":[
  {"name":"search","description":"Web search.","input_schema":{"type":"object","properties":{"query":{"type":"string"}}}},
  {"name":"news","description":"News search.","input_schema":{...}}
]}}
```

**Invoke** (host → plugin, per agent tool call):
```json
{"id":"q-2","method":"tool/invoke","params":{"name":"search","input":{"query":"go 1.26 release notes"}}}
```

**Invoke result** (plugin → host):
```json
{"id":"q-2","result":{"output":"...","is_error":false}}
```

Or error:
```json
{"id":"q-2","error":"rate-limit exceeded; retry in 30s"}
```

**Shutdown** (host → plugin, on Close):
```json
{"id":"end","method":"shutdown"}
```
Plugin reads this and exits 0. Host gives 5s grace, then kills.

## Changes

### 1. `kernel/plugin/protocol.go` — new file (~110 LoC)

Pure wire types: `Request`, `Response`, `ToolDef`, `InitializeResult`,
`InvokeParams`, `InvokeResult`, and method-name constants. Zero
dependencies on `kernel/agent` so plugins can target this package
without pulling in the wider kernel — a Python plugin would
re-implement these shapes from the package comment alone.

### 2. `kernel/plugin/host.go` — new file (~280 LoC)

The Plugin type manages one child process. Public API:

```go
type Config struct {
    Path, Args, Env, Dir
    InitTimeout, InvokeTimeout time.Duration
    Logger func(string)  // stderr forwarding
}

func Spawn(ctx, Config) (*Plugin, error)
func (p *Plugin) Tools(prefix string) map[string]agent.Tool
func (p *Plugin) Invoke(ctx, name, input) (InvokeResult, error)
func (p *Plugin) Close() error
func (p *Plugin) IsAlive() bool
```

Three design choices worth recording:

**Why not JSON-RPC 2.0.** JSON-RPC adds the `"jsonrpc":"2.0"`
envelope field and a more elaborate error object format. Neither
buys us anything our flat `{id, method, params}` doesn't already
provide. Smaller wire = smaller plugin authors have to implement.

**Why a Read-loop + pending map** (instead of synchronous
request-then-read). Two concurrent agent tool calls can be in
flight at once (parallel scheduler node, multi-step plan). The
read loop dispatches each response to whichever caller is
waiting on that id. Without this, calls would serialise.

**Why partial-startup teardown.** If `initialize` fails (bad
plugin, timeout, malformed reply), the child process is still
running. `Spawn` calls `Close` before returning the error so we
don't leak processes. Critical for `AGEZT_PLUGINS` with many
entries: one bad plugin doesn't accumulate orphans.

### 3. `kernel/plugin/testdata/echoplugin/main.go` — reference plugin

A self-contained Go program (~100 LoC) that implements the
plugin protocol. It exposes two tools:

- `echo` — returns the input wrapped in "echo: <input>"
- `fail` — always returns `IsError=true` (exercises the error path)

Critically, the echoplugin **does NOT import any agezt package** —
it re-declares the wire types from scratch. That's the test that
the protocol is genuinely self-contained: a third party can read
the package doc and write a plugin without depending on agezt's
Go code.

The plugin is built with `go build` at test-init time and the
binary path is cached for the test session.

### 4. `kernel/plugin/host_test.go` — 11 tests

| Test | Locks in |
|---|---|
| `TestSpawn_InitializeRegistersTools` | Spawn → initialize → 2 tools registered with non-empty Definition |
| `TestSpawn_PrefixNamespacing` | `Tools("myco.")` returns `myco.echo`, NOT `echo` |
| `TestInvoke_EchoRoundTrip` | Input goes out, echo comes back |
| `TestInvoke_FailRoundTrip` | `IsError=true` round-trips |
| `TestInvoke_UnknownToolErrors` | Plugin returns `error:"unknown tool"`; host surfaces it |
| `TestInvoke_ConcurrentCallsDoNotCross` | 20 parallel invocations: each gets its own response (id-correlation works) |
| `TestClose_IsIdempotent` | Two `Close` calls return nil; `IsAlive` flips to false |
| `TestInvoke_AfterCloseReturnsUnavailable` | Tool invocation post-Close → "plugin unavailable" error |
| `TestSpawn_BadPathErrors` | `/no/such/binary` → clear error |
| `TestSpawn_EmptyPathErrors` | Empty Path → "Path required" |
| `TestSpawn_InitTimeoutEnforced` | Spawn against `go version` (wrong shape) → fails within timeout, doesn't hang |

### 5. `cmd/agezt/main.go` — daemon wiring

```go
if spec := os.Getenv("AGEZT_PLUGINS"); spec != "" {
    for entry := range strings.SplitSeq(spec, ",") {
        prefix, cmdLine, ok := strings.Cut(entry, "=")
        // ... validate ...
        p, err := plugin.Spawn(ctx, plugin.Config{
            Path:   parts[0],
            Args:   parts[1:],
            Logger: func(line string) { fmt.Fprintf(stderr, "[plugin:%s] %s\n", prefix, line) },
        })
        // ... register tools with prefix ...
    }
}
```

Format: comma-separated `prefix=path arg arg` entries. Each
plugin's tools register under `<prefix>.<toolname>` so collisions
across plugins are impossible. Conflicts with in-process tools
are detected and the in-process version wins (with a warning).

Three failure modes are handled explicitly:

1. **Entry missing `=`** → warning, skip, daemon continues.
2. **Path not found** → warning, skip, daemon continues.
3. **Plugin fails to initialize** → warning, skip, daemon continues.

A broken plugin never takes down the daemon or the other plugins.

## Test summary

```
go test ./kernel/plugin/ -v -count=1 -timeout 60s
(11 tests — all PASS; includes a `go build` of the test plugin at
 startup which is cached for the test session)

go test ./... -count=1 -timeout 90s
(all packages PASS)
```

Total suite: **526 passing** (from 515 after M1.x). +11 from
M1.y.

## Operator workflow examples

**Writing a plugin in Python** (no agezt dependency):

```python
#!/usr/bin/env python3
import json, sys
for line in sys.stdin:
    req = json.loads(line)
    if req["method"] == "initialize":
        out = {"id": req["id"], "result": {"tools": [
            {"name": "translate",
             "description": "Translate text via DeepL API.",
             "input_schema": {"type": "object", "properties": {
                 "text": {"type": "string"},
                 "target_lang": {"type": "string"}
             }, "required": ["text", "target_lang"]}}
        ]}}
    elif req["method"] == "tool/invoke":
        p = req["params"]
        if p["name"] == "translate":
            result = call_deepl(p["input"])  # operator's code
            out = {"id": req["id"], "result": {"output": result}}
        else:
            out = {"id": req["id"], "error": f"unknown tool: {p['name']}"}
    elif req["method"] == "shutdown":
        sys.exit(0)
    else:
        out = {"id": req["id"], "error": f"unknown method: {req['method']}"}
    print(json.dumps(out), flush=True)
```

```
chmod +x ~/bin/agezt-translate
export AGEZT_PLUGINS="dl=/Users/op/bin/agezt-translate"
agezt &
# tools registered: ..., plugin:dl(1 tools)

agt run "translate 'thank you' into Japanese"
# agent calls dl.translate(text="thank you", target_lang="ja")
```

**Registering multiple plugins:**

```
export AGEZT_PLUGINS="search=/opt/agezt-search/bin,scrape=/opt/agezt-scraper/bin,dl=/home/op/.local/bin/agezt-translate"
```

Tools register under `search.*`, `scrape.*`, `dl.*` namespaces.
The agent calls them by their full prefixed names.

**Watching a plugin's stderr** (debugging):

The plugin's stderr is forwarded to the daemon's stderr with a
`[plugin:<prefix>]` tag:

```
[plugin:search] connecting to upstream API
[plugin:search] retrying after 429
[plugin:scrape] parsing HTML for 1247 bytes
```

Combine with `agt pulse` to see the corresponding agent-side
events:

```
agt pulse --kind tool.invoked --kind tool.result
```

## What's intentionally NOT in M1.y

- **Plugin sandboxing.** Plugin processes inherit the daemon's
  UID and have full network + filesystem access. The same trust
  model in-process tools have. Sandbox-aware plugins (warden-
  isolated subprocesses, seccomp profiles per plugin) are a
  separate phase that intersects with the existing warden
  infrastructure.
- **Plugin manifests / signing / discovery.** Operators today
  point env-var-paths at binaries they trust. A signed-plugin
  manifest format + a discovery mechanism (agezt's equivalent
  of npm install) is its own UX project.
- **Hot-reloading plugins.** `AGEZT_PLUGINS` is read at daemon
  start; changes require restart. M1.r-style hot reload would
  let `agt provider reload` re-spawn changed plugin processes.
  Out of scope for v1.
- **Streaming tool results.** Today plugins return one final
  `InvokeResult`. The agent loop's existing `agent.Chunk` shape
  could carry tool-side streaming (for long-running tools like
  build invocations), but that's a per-method protocol
  extension we'd want to design carefully.
- **Plugin → kernel callbacks** (plugins calling back into the
  bus, submitting approval requests, reading state). Plugins
  today are pure "function" tools — input → output. Bidirectional
  protocol adds substantial complexity and a security model
  question; defer until a concrete use case demands it.
- **MCP bridge.** The agezt protocol is intentionally simpler
  than MCP. A future `agezt-mcp-bridge` plugin would speak MCP
  to a third-party MCP server and re-export each tool as a
  agezt plugin tool. Out of scope for v1 — operators with MCP
  tools today can write a thin shim.
- **gRPC plugins.** Same argument; operators wanting gRPC can
  write a shim plugin that forwards.

## Files touched

- [kernel/plugin/protocol.go](../kernel/plugin/protocol.go) — new (~110 LoC).
- [kernel/plugin/host.go](../kernel/plugin/host.go) — new (~290 LoC).
- [kernel/plugin/host_test.go](../kernel/plugin/host_test.go) — new (~225 LoC, 11 tests).
- [kernel/plugin/testdata/echoplugin/main.go](../kernel/plugin/testdata/echoplugin/main.go) — new (~110 LoC reference plugin).
- [cmd/agezt/main.go](../cmd/agezt/main.go) — import + ~45-line AGEZT_PLUGINS handler in buildTools.

Zero changes to the agent loop, the bus, the scheduler, the
planner, providers, or in-process tools. The plugin host
integrates entirely through the existing tool-registry
contract — `agent.Tool`'s `Definition` and `Invoke` interface
methods.

## Closing the M1 wedge

M1.y closes the last architectural deferral from the M1 plan.
Counting from M1.a (M0.5 base + agent loop), 30+ phases shipped:

| Wedge | Phases | What it gave the operator |
|---|---|---|
| Core kernel | M1.a–M1.e | agent loop, scheduler, bus, journal, warden, edict |
| Catalog + providers | M1.f–M1.n.x | 10 catalog families, all wire-supported, with streaming |
| Provider auth | M1.o, M1.m.x | vault, Bedrock SigV4 |
| Operator UX | M1.p, M1.p.x, M1.p.y, M1.r, M1.u | provider check, hot reload, pulse |
| Streaming | M1.q–M1.q.x.x.x.x, M1.t | every catalog family + Bedrock binary framing |
| Governor policy | M1.s | subscription-first routing |
| Planner | M1.v | LLM-generated DAGs |
| Security | M1.w | at-rest vault encryption |
| Tools | M1.x, M1.y | browser.read + out-of-process plugin host |

Test count: **526** (from ~100 at M0.5).
External deps: **1** (`lukechampine.com/blake3` + one transitive).
Project structure: as the v1 vision called for — an agentic OS
substrate that operators can tune, audit, and extend without
recompiling.

## Remaining deferrals (post-v1)

These are real follow-ups but none block the v1 substrate:

- **Pulse v2** (historical replay, TUI, dropped-events synthetic).
- **Planner v2** (re-planning, sub-planners, planner tools, cost estimation).
- **AWS credential-provider chain** (M1.m.x.x).
- **Non-Anthropic body shapes on Bedrock** (M1.m.y).
- **Plugin sandboxing, signing, hot-reload, streaming, callbacks**
  (M1.y deferrals).
- **Browser tool: JS rendering, screenshots, search, cookies**
  (M1.x deferrals).
- **MCP bridge plugin.**
- **Vault: OS-keychain integration, passphrase rotation, argon2.**
- **Per-task-type routing.**

The "agezt v1 substrate" wedge is **done**.
