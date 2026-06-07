//! Official Rust client for the [Agezt](https://github.com/agezt/agezt) agentic OS.
//!
//! Talks to a running Agezt daemon's native REST API (`/api/v1`) — the same
//! governed kernel loop `agt run` uses (Edict policy, the journal, cost
//! governance) — over plain HTTP with a bearer token.
//!
//! **Zero runtime dependencies** — standard library only (a tiny built-in
//! HTTP/1.1 client and JSON codec), matching the Python (`urllib`+`json`) and
//! TypeScript (platform `fetch`) SDKs and the project's stdlib-first ethos.
//!
//! ```no_run
//! use agezt::Client;
//!
//! fn main() -> agezt::Result<()> {
//!     let c = Client::new("http://127.0.0.1:8800", "<token>");
//!
//!     println!("{}", c.health()?.version);
//!     println!("default model: {}", c.models()?.default);
//!
//!     // Blocking run — returns the final answer.
//!     let r = c.run("summarise the latest commits", None)?;
//!     println!("{}", r.answer);
//!
//!     // Streaming run — tokens as the agent produces them.
//!     for ev in c.run_stream("write a haiku about Go", None)? {
//!         let ev = ev?;
//!         if ev.event == "token" {
//!             print!("{}", ev.data.str("text").unwrap_or(""));
//!         }
//!     }
//!
//!     // The journaled event arc of a past run.
//!     let arc = c.get_run(&r.correlation_id)?;
//!     println!("{} events", arc.count);
//!     Ok(())
//! }
//! ```
//!
//! ## Scope
//!
//! The client speaks **plain `http://`** (the std library ships no TLS). That
//! matches the daemon's documented loopback deployment
//! (`AGEZT_REST_ADDR=127.0.0.1:8800`); to reach it over `https`, front the
//! daemon with a TLS-terminating reverse proxy and point the client at that.

#![forbid(unsafe_code)]

mod client;
mod errors;
mod http;
mod json;

pub use client::{Client, Health, Models, RunArc, RunResult, RunStream, StreamEvent};
pub use errors::{Error, Result};
pub use json::Value;
