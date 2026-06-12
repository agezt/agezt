# Agezt Rust SDK

The official Rust client for the [Agezt](https://github.com/agezt/agezt)
agentic OS. It talks to a running Agezt daemon's native REST API (`/api/v1`) —
the same governed kernel loop `agt run` uses (Edict policy, the journal, cost
governance) — over plain HTTP with a bearer token.

**Zero runtime dependencies — standard library only.** A tiny built-in HTTP/1.1
client and JSON codec stand in for `reqwest`/`serde`, matching the Python
(`urllib`+`json`) and TypeScript (platform `fetch`) SDKs and the project's
stdlib-first ethos.

## Install

```toml
# Cargo.toml — from crates.io (when published):
[dependencies]
agezt = "1.0"
```

Or point at this repo directly:

```toml
[dependencies]
agezt = { git = "https://github.com/agezt/agezt", branch = "main" }
```

## Quick start

Start a daemon with the REST API enabled and note the token it prints:

```bash
AGEZT_REST_ADDR=127.0.0.1:8800 agezt
#   rest api : http://127.0.0.1:8800/api/v1  (Authorization: Bearer <token>)
```

```rust
use agezt::Client;

fn main() -> agezt::Result<()> {
    let c = Client::new("http://127.0.0.1:8800", "<token>");

    println!("{}", c.health()?.version);        // "1.0.0"
    println!("{}", c.models()?.default);

    // Blocking run — returns the final answer.
    let r = c.run("summarise the latest commits", None)?;
    println!("{}", r.answer);

    // Streaming run — tokens as the agent produces them.
    for ev in c.run_stream("write a haiku about Go", None)? {
        let ev = ev?;
        if ev.event == "token" {
            print!("{}", ev.data.str("text").unwrap_or(""));
        }
    }

    // The journaled event arc of a past run.
    let arc = c.get_run(&r.correlation_id)?;
    println!("{} events", arc.count);
    Ok(())
}
```

## API

| Method | REST endpoint | Returns |
|---|---|---|
| `health()` | `GET /api/v1/health` | `Health` (status, version, default_model, model_count) |
| `models()` | `GET /api/v1/models` | `Models` (`default`, `models`) |
| `run(intent, model)` | `POST /api/v1/runs` | `RunResult` (correlation_id, model, status, answer) |
| `run_stream(intent, model)` | `POST /api/v1/runs` (SSE) | `Iterator<Item = Result<StreamEvent>>` (`start`/`token`/`done`/`error`) |
| `get_run(correlation_id)` | `GET /api/v1/runs/{id}` | `RunArc` (correlation_id, count, events) |
| `mailbox_send(&MailDraft)` | `POST /api/v1/mailbox/messages` | `Mail` — DM by name, broadcast (`to: "*"`), post, reply, or help |
| `mailbox_broadcast(from, text)` | `POST /api/v1/mailbox/messages` | `Mail` (lands in every inbox) |
| `mailbox_inbox(name, include_read, limit)` | `GET /api/v1/mailbox/inbox` | `Vec<Mail>` waiting for `name` |
| `mailbox_ack(id, by)` | `POST /api/v1/mailbox/messages/{id}/ack` | marks it read for `by` |
| `mailbox_replies(id, limit)` | `GET /api/v1/mailbox/messages/{id}/replies` | `Vec<Mail>`, oldest first |
| `mailbox_messages(topic, limit)` | `GET /api/v1/mailbox/messages` | `Vec<Mail>`, newest first |
| `mailbox_topics()` | `GET /api/v1/mailbox/topics` | `BTreeMap<String, i64>` topic → count |
| `mailbox_watch(name, topic)` | `GET /api/v1/mailbox/watch` (SSE) | `Iterator<Item = Result<Mail>>` as messages land (push, no polling) |

`Client::new(base_url, token)` defaults to a 30-second per-request timeout and
no tenant. Chain `.with_timeout(Duration)` and `.with_tenant("<id>")` to target
an isolated tenant on a multi-tenant daemon (sent as the `X-Agezt-Tenant`
header). The `model` argument is an `Option<&str>` — pass `Some("sonnet")` to
select a model (and thereby its provider), or `None` for the daemon default.

A `StreamEvent`'s `data` is a [`Value`](src/json.rs) (the SDK's small JSON type);
read fields with `data.str("text")`, `data.get("k")`, and the `as_*` accessors.

Non-2xx responses become `Error::Api { status, kind, message }`; transport/IO
failures become `Error::Transport`.

## Scope: plain HTTP

The client speaks **`http://` only** — the Rust standard library ships no TLS,
and the SDK takes no dependencies. That matches the daemon's documented loopback
deployment (`AGEZT_REST_ADDR=127.0.0.1:8800`). To reach a daemon over `https`,
front it with a TLS-terminating reverse proxy and point the client at that.

## Tests

Standard-library only — the integration tests run a mock daemon on a
`std::net::TcpListener` (no third-party HTTP server or test framework):

```bash
cd sdk/rust
cargo test
```

This realizes the Rust quarter of decision A4's "SDKs in Go/TS/Python/Rust".
