# Agent SDK Architecture — AGEZT

## Problem Statement

When an AI agent writes Python, Node.js, or Deno code, that code runs in a **subprocess with no connection to AGEZT**. The agent's LLM context knows how to emit events, query memory, and send channel messages—but the code it generates does not.

```
┌─────────────────────────────────────────────────────────────┐
│                     AI Agent (LLM)                           │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  I can emit events, read memory, query agents...    │    │
│  └─────────────────────────────────────────────────────┘    │
│                           │                                   │
│                           │ writes code                       │
│                           ▼                                   │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  Python/Node subprocess  ←  NO CONNECTION TO AGEZT  │    │
│  │  print("hello")           can't access capabilities │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

This architecture bridges that gap: agents issue **scoped, temporary tokens** to code they spawn, giving that code controlled access to AGEZT capabilities.

---

## Design Principles

1. **Agent-centric**: The agent controls what capabilities its code can access. Tokens are scoped to the agent's run context.
2. **Defense in depth**: Multiple layers—token validation, capability checks, rate limiting, and audit logging.
3. **Minimal footprint**: Code written by agents shouldn't need a full AGEZT SDK import to get basic capabilities.
4. **Polyglot**: First-class support for Python, TypeScript/Node, Deno, Go, and Rust.
5. **Audit everything**: Every capability access is journaled with the agent's correlation ID.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AGEZT Daemon                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     Agent Gateway (Internal)                           │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │   │
│  │  │  REST API    │  │  WS Events   │  │   Auth       │               │   │
│  │  │  /agent/*    │  │  /agent/ws   │  │   Layer      │               │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │   │
│  │         │                  │                  │                        │   │
│  │  ┌──────▼──────────────────▼──────────────────▼───────┐              │   │
│  │  │              Capability Router                      │              │   │
│  │  │  • Token validation                                 │              │   │
│  │  │  • Capability enforcement                           │              │   │
│  │  │  • Rate limiting                                   │              │   │
│  │  │  • Audit logging                                   │              │   │
│  │  └──────┬───────┬──────┬──────┬──────┬──────┬───────┘              │   │
│  └─────────┼───────┼──────┼──────┼──────┼──────┼───────────────────────┘   │
│            │       │      │      │      │      │                              │
│  ┌─────────▼───────▼──────▼──────▼──────▼──────▼───────────────────────┐   │
│  │                        AGEZT Kernel                                  │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  │   │
│  │  │  Bus    │  │ Channel │  │ Memory  │  │ Roster  │  │ Journal │  │   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘  │   │
│  └────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      ▲
                                      │ HTTPS + Token
                                      │
┌─────────────────────────────────────┴─────────────────────────────────────┐
│                        AI Agent Subprocess                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                       │
│  │ Python SDK  │  │  TS/JS SDK  │  │  Go SDK     │  ←  Rust SDK         │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────────┘     │
│         │                 │                 │                              │
│         └─────────────────┴────────┬────────┘                              │
│                                    │                                       │
│                         ┌──────────▼──────────┐                           │
│                         │   Agent SDK Client   │                           │
│                         │  • agezt.connect()  │                           │
│                         │  • Scoped token    │                           │
│                         │  • All capabilities │                           │
│                         └─────────────────────┘                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Token System

### Token Types

| Token Type | Created By | Lifetime | Scope |
|------------|------------|----------|-------|
| **Run Token** | Agent run start | Run duration + 5min | Full agent capabilities |
| **Subprocess Token** | Agent via SDK | Agent-defined (max 24h) | Agent-specified capabilities only |
| **Tool Token** | Code execution | Per-call | Single capability |

### Token Format (JWT)

```json
{
  "sub": "subprocess_abc123",
  "iss": "agezt",
  "aud": "agezt-agent",
  "exp": 1718900000,
  "iat": 1718896400,
  "run_id": "01HXYZ...",
  "capabilities": ["eventbus:publish", "memory:read", "memory:write"],
  "rate_limit": {
    "requests_per_minute": 60,
    "burst": 10
  },
  "parent": "01HXYZ...",
  "pid": 12345
}
```

### Capability Namespaces

| Namespace | Capabilities |
|-----------|-------------|
| `eventbus` | `publish`, `subscribe`, `publish_streaming` |
| `channel` | `send`, `read`, `list` |
| `memory` | `read`, `write`, `delete`, `search`, `list` |
| `log` | `read`, `write`, `append` |
| `agent` | `list`, `query`, `delegate` |
| `db` | `query`, `read`, `write` |
| `run` | `create`, `status`, `cancel`, `stream` |

---

## REST API Specification

> **Accuracy note.** The endpoint catalogue below is the *intended full design*.
> The agent gateway as currently implemented (`kernel/agentgw`) exposes a
> **subset** of these, and under a different prefix — **`/v1/...`**, not
> `/api/v1/agent/...`. The handler registrations in `kernel/agentgw/gateway.go`
> are the source of truth. The routes live today are:
>
> ```
> POST   /v1/eventbus/publish      GET    /v1/eventbus/subscribe
> POST   /v1/memory/write          GET    /v1/memory/search    DELETE /v1/memory/delete
> POST   /v1/log/write             GET    /v1/log/read
> GET    /v1/config                POST   /v1/config           GET /v1/config/audit  GET /v1/config/search
> GET    /v1/agent/list            GET    /v1/agent/query
> POST   /v1/token/create
> ```
>
> Channels, the data lake, run management, and the richer discovery/token
> verbs below are roadmap, not yet served. Treat the rest of this section as
> the design target until a route appears in `gateway.go`.

### Base URL
```
/api/v1/agent
```

### Authentication
All requests require:
```
Authorization: Bearer <token>
X-Run-Correlation: <correlation_id>
```

### Endpoints

#### Eventbus

```
POST   /api/v1/agent/eventbus/publish
Body:  { "subject": "agent.custom", "payload": {...}, "tags": {...} }
Returns: { "event_id": "01H...", "seq": 12345 }

POST   /api/v1/agent/eventbus/subscribe
Body:  { "pattern": "agent.>", "buffer_size": 256 }
Returns: { "subscription_id": "sub_xyz" }

DELETE /api/v1/agent/eventbus/subscribe/:id
Returns: { "ok": true }

GET    /api/v1/agent/eventbus/events/:id
Returns: { "event": {...} }
```

#### Channels

```
POST   /api/v1/agent/channel/send
Body:  { "channel_kind": "telegram", "channel_id": "123", "text": "..." }
Returns: { "sent": true, "message_id": "msg_xyz" }

GET    /api/v1/agent/channel/inbox
Query: ?limit=20&channel=telegram
Returns: { "threads": [...], "count": 5 }

GET    /api/v1/agent/channel/channels
Returns: { "channels": [{ "kind": "telegram", "name": "My Bot" }] }
```

#### Memory

```
POST   /api/v1/agent/memory/write
Body:  { "subject": "project-x", "content": "...", "type": "FACT" }
Returns: { "id": "mem_abc", "created": true }

GET    /api/v1/agent/memory/read/:id
Returns: { "record": {...} }

GET    /api/v1/agent/memory/search
Query: ?q=project+x&limit=10
Returns: { "results": [...], "count": 3 }

DELETE /api/v1/agent/memory/:id
Returns: { "forgotten": true }

GET    /api/v1/agent/memory/list
Returns: { "records": [...], "count": 42 }
```

#### Logs

```
POST   /api/v1/agent/log/write
Body:  { "level": "info", "message": "...", "context": {...} }
Returns: { "logged": true }

GET    /api/v1/agent/log/read
Query: ?level=error&since=1718890000&limit=100
Returns: { "entries": [...], "count": 5 }

GET    /api/v1/agent/log/stats
Returns: { "counts": { "info": 100, "warn": 10, "error": 2 } }
```

#### Agent Discovery

```
GET    /api/v1/agent/agents
Returns: { "agents": [{ "id": "...", "slug": "researcher", "status": "active" }] }

GET    /api/v1/agent/agents/:slug
Returns: { "profile": {...}, "recent_activity": [...] }

GET    /api/v1/agent/agents/:slug/status
Returns: { "online": true, "last_seen": 1718890000 }
```

#### Database (Data Lake)

```
GET    /api/v1/agent/db/collections
Returns: { "collections": [...] }

GET    /api/v1/agent/db/collections/:name/records
Query: ?search=...&limit=20&offset=0
Returns: { "records": [...], "count": 50 }

POST   /api/v1/agent/db/collections/:name/records
Body:  { "field": "value" }
Returns: { "record": { "id": "...", ... } }

DELETE /api/v1/agent/db/collections/:name/records/:id
Returns: { "deleted": true }
```

#### Run Management

```
POST   /api/v1/agent/run
Body:  { "intent": "...", "model": "claude-3-5-sonnet" }
Returns: { "correlation_id": "01H...", "status": "running" }

GET    /api/v1/agent/run/:correlation_id
Returns: { "status": "completed", "answer": "...", "cost_usd": 0.04 }

GET    /api/v1/agent/run/:correlation_id/events
Returns: { "events": [...], "count": 25 }

DELETE /api/v1/agent/run/:correlation_id
Returns: { "cancelled": true }
```

#### Token Management

```
POST   /api/v1/agent/token/create
Body:  { 
  "capabilities": ["eventbus:publish", "memory:read"],
  "lifetime_seconds": 3600,
  "rate_limit": { "rpm": 30 }
}
Returns: { "token": "eyJ...", "expires_at": 1718900000 }

POST   /api/v1/agent/token/introspect
Body:  { "token": "eyJ..." }
Returns: { "valid": true, "capabilities": [...], "expires_at": 1718900000 }

DELETE /api/v1/agent/token/revoke
Body:  { "token": "eyJ..." }
Returns: { "revoked": true }
```

---

## WebSocket API

For streaming subscriptions (eventbus, live run updates):

```
GET /api/v1/agent/ws?token=<token>&subscribe=agent.>&run=<correlation_id>

Frames (JSON):
{ "type": "event", "event": {...} }
{ "type": "run_update", "update": { "status": "streaming", "delta": "..." } }
{ "type": "heartbeat" }
{ "type": "error", "code": "RATE_LIMITED", "message": "..." }
```

---

## SDK Design

### Python SDK (`agezt` package)

```python
import agezt

# Agent creates a scoped token for its subprocess
token = agezt.token_create(
    capabilities=["eventbus:publish", "memory:read", "memory:write"],
    lifetime_seconds=3600
)
# Pass `token` to subprocess as env var or argument

# Subprocess connects with the token
client = agezt.connect(token=os.environ["AGEZT_TOKEN"])

# Eventbus
client.eventbus.publish("agent.file_saved", {"path": "/tmp/x.txt"})
sub = client.eventbus.subscribe("agent.>")
for event in sub:
    print(event.subject, event.payload)

# Memory
mem_id = client.memory.write("project-x", "Completed feature Y", type="OBSERVATION")
record = client.memory.read(mem_id)
results = client.memory.search("project-x")

# Channels
client.channel.send("telegram", "123", "Build complete!")

# Logs
client.log.write("info", "Build succeeded", {"duration_ms": 5000})

# Agent discovery
agents = client.agents.list()
researcher = client.agents.get("researcher")

# Database
collections = client.db.collections()
records = client.db.query("builds", search="project-x")
```

### TypeScript/Node SDK (`@agezt/sdk`)

```typescript
import { AgentClient } from '@agezt/sdk';

const client = new AgentClient({
  token: process.env.AGEZT_TOKEN!,
  baseUrl: 'http://localhost:8800'
});

// Eventbus
await client.eventbus.publish('agent.file_saved', { path: '/tmp/x.txt' });
const sub = client.eventbus.subscribe('agent.>');
for await (const event of sub) {
  console.log(event.subject, event.payload);
}

// Memory
const memId = await client.memory.write({
  subject: 'project-x',
  content: 'Completed feature Y',
  type: 'OBSERVATION'
});
const record = await client.memory.read(memId);
const results = await client.memory.search({ q: 'project-x', limit: 10 });

// Channels
await client.channel.send({ kind: 'telegram', channelId: '123', text: 'Done!' });

// Logs
await client.log.write({ level: 'info', message: 'Build succeeded', context: { duration: 5000 } });

// Agent discovery
const agents = await client.agents.list();
const researcher = await client.agents.get('researcher');

// Database
const collections = await client.db.collections();
const records = await client.db.query({ collection: 'builds', search: 'project-x' });
```

### Go SDK

```go
package main

import (
    "context"
    "log"
    "os"
    "github.com/agezt/agezt/sdk/agent"
)

func main() {
    client, err := agent.Dial(os.Getenv("AGEZT_TOKEN"))
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()

    // Eventbus
    evt, err := client.Eventbus.Publish(ctx, "agent.file_saved", map[string]any{"path": "/tmp/x.txt"})
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Published event %s", evt.ID)

    sub, err := client.Eventbus.Subscribe(ctx, "agent.>")
    if err != nil {
        log.Fatal(err)
    }
    defer sub.Cancel()

    // Memory
    memID, err := client.Memory.Write(ctx, &agent.MemoryRecord{
        Subject:  "project-x",
        Content:  "Completed feature Y",
        Type:     agent.MemoryTypeObservation,
    })
    
    // Agent discovery
    agents, err := client.Agents.List(ctx)
}
```

---

## Security Model

### Defense Layers

1. **Token Validation**: JWT signature verification, expiration check, issuer validation
2. **Capability Enforcement**: Each endpoint checks `token.capabilities` before processing
3. **Rate Limiting**: Per-token RPM limits, configurable burst
4. **Audit Logging**: Every request logged with correlation ID, token ID, capability accessed
5. **Output Sanitization**: Secrets redacted from eventbus payloads before journaling

### Capability Inheritance

```
Agent Run Token (full capabilities)
    │
    ├──creates──► Subprocess Token (scoped)
    │                    │
    │                    ├──creates──► Tool Token (single capability)
    │                    │
    │                    └── Restricts ──► Can only call allowed capabilities
    │
    └── Audit trail: all operations tagged with run_id
```

### Rate Limiting

| Token Type | Default RPM | Burst | Scope |
|------------|-------------|-------|-------|
| Subprocess | 60 | 10 | Per token |
| Memory write | 30 | 5 | Per token |
| Eventbus publish | 120 | 20 | Per token |
| Agent query | 20 | 5 | Per token |

---

## Implementation Plan

### Phase 1: Core Gateway
- [ ] Agent Gateway HTTP handler in kernel
- [ ] JWT token validation middleware
- [ ] Capability checking middleware
- [ ] Basic REST endpoints (eventbus, memory, agents)
- [ ] Rate limiting middleware
- [ ] Audit logging

### Phase 2: Extended Capabilities  
- [ ] WebSocket subscription support
- [ ] Channel endpoints
- [ ] Log endpoints
- [ ] Database endpoints
- [ ] Run management endpoints

### Phase 3: SDKs
- [ ] Python SDK (update existing)
- [ ] TypeScript/Node SDK (update existing)
- [ ] Go SDK (update existing)
- [ ] Rust SDK (new)

### Phase 4: Advanced Features
- [ ] Token introspection API
- [ ] Token revocation
- [ ] Progressive capability expansion
- [ ] Cross-agent delegation tokens

---

## File Structure

```
kernel/
├── agentgw/                    # NEW: Agent Gateway
│   ├── gateway.go             # Main gateway handler
│   ├── auth.go                # Token validation
│   ├── capabilities.go         # Capability enforcement
│   ├── ratelimit.go           # Rate limiting
│   ├── audit.go               # Audit logging
│   ├── handlers/
│   │   ├── eventbus.go
│   │   ├── channel.go
│   │   ├── memory.go
│   │   ├── log.go
│   │   ├── agent.go
│   │   ├── db.go
│   │   ├── run.go
│   │   └── token.go
│   └── ws/
│       └── websocket.go       # WebSocket handler

sdk/
├── python/
│   └── agezt/
│       ├── client.py          # Existing → extend
│       ├── agent.py           # NEW: AgentClient
│       ├── token.py           # NEW: Token utilities
│       └── capabilities.py    # NEW: Type-safe capabilities
├── typescript/
│   └── src/
│       ├── client.ts          # Existing → extend
│       ├── agent-client.ts    # NEW: AgentClient
│       ├── token.ts           # NEW: Token utilities
│       └── capabilities.ts    # NEW: Type-safe capabilities
├── go/
│   └── sdk/
│       ├── agent.go           # NEW: Agent client
│       └── token.go           # NEW: Token utilities
└── rust/                      # NEW
    └── src/
        ├── lib.rs
        ├── client.rs
        └── token.rs
```

---

## Key Design Decisions

### 1. Why JWT for tokens?
- Stateless validation (no server-side session store)
- Self-contained: expiry, capabilities, issuer all in one
- Cryptographically signed, tamper-proof
- Standard library support across all languages

### 2. Why not use AGEZT's existing controlplane protocol?
- Controlplane is synchronous, request/response
- Agent code needs async subscriptions (eventbus)
- Different security model: controlplane assumes trusted operator
- Agent gateway provides capability-level granularity

### 3. Why a separate gateway rather than extending the existing REST API?
- Clear separation between "operator" and "agent code" access
- Different security policies (agents get scoped tokens, operators get admin tokens)
- Prevents agent code from accessing admin-only commands
- Cleaner audit trail for "what did agent code do vs operator"

### 4. Why WebSocket for subscriptions?
- Eventbus subscriptions need push-based delivery
- Long-lived connections for real-time updates
- Multiplexing multiple subscriptions on one connection
- Heartbeat mechanism for connection health

---

## Error Responses

All errors follow a consistent format:

```json
{
  "error": {
    "code": "CAPABILITY_DENIED",
    "message": "Token does not have eventbus:publish capability",
    "details": {
      "required": "eventbus:publish",
      "token_caps": ["memory:read", "memory:write"]
    }
  }
}
```

| Code | HTTP | Meaning |
|------|------|---------|
| `INVALID_TOKEN` | 401 | Token missing, malformed, or expired |
| `CAPABILITY_DENIED` | 403 | Token lacks required capability |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `NOT_FOUND` | 404 | Resource doesn't exist |
| `VALIDATION_ERROR` | 400 | Invalid request parameters |
| `INTERNAL_ERROR` | 500 | Server-side failure |

---

## Open Questions

1. **Token renewal**: Should subprocess tokens be renewable, or must the agent create new ones?
2. **Capability escalation**: Should agents be able to request broader capabilities mid-run?
3. **Cross-tenant delegation**: In multi-tenant mode, can agents delegate to agents in other tenants?
4. **Token rotation**: Should long-running subprocesses periodically rotate their tokens?
5. **Quota vs Rate**: Should we track quota (total calls) separately from rate (calls/minute)?

---

## Related Documentation

- [AGEZT Architecture Report](./ARCHITECTURAL-REPORT.md)
- [Controlplane Protocol](./kernel/controlplane/protocol.go)
- [Event Bus Design](./kernel/bus/bus.go)
- [Agent Roster](./kernel/roster/roster.go)
