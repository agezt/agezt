//! The blocking [`Client`] for a running Agezt daemon's REST API.

use std::io::BufRead;
use std::time::Duration;

use crate::errors::{Error, Result};
use crate::http::{self, BodyReader, Target};
use crate::json::Value;

/// Daemon health summary (`GET /api/v1/health`).
#[derive(Debug, Clone)]
pub struct Health {
    pub status: String,
    pub version: String,
    pub default_model: String,
    pub model_count: i64,
}

/// Available models (`GET /api/v1/models`).
#[derive(Debug, Clone)]
pub struct Models {
    pub default: String,
    pub models: Vec<String>,
}

/// A completed (non-streaming) run (`POST /api/v1/runs`).
#[derive(Debug, Clone)]
pub struct RunResult {
    pub correlation_id: String,
    pub model: String,
    pub status: String,
    pub answer: String,
}

/// One Server-Sent Event from a streaming run. `event` is one of `start`,
/// `token`, `done`, `error`; `data` is the decoded JSON payload (e.g.
/// `{"text": "…"}` for a `token`).
#[derive(Debug, Clone)]
pub struct StreamEvent {
    pub event: String,
    pub data: Value,
}

/// The journaled event arc of a past run (`GET /api/v1/runs/{id}`).
#[derive(Debug, Clone)]
pub struct RunArc {
    pub correlation_id: String,
    pub count: i64,
    pub events: Vec<Value>,
}

/// A client for a running Agezt daemon's REST API.
///
/// ```no_run
/// use agezt::Client;
/// # fn main() -> agezt::Result<()> {
/// let c = Client::new("http://127.0.0.1:8800", "<token>");
/// println!("{}", c.health()?.version);
/// let r = c.run("summarise the latest commits", None)?;
/// println!("{}", r.answer);
/// for ev in c.run_stream("write a haiku about Go", None)? {
///     let ev = ev?;
///     if ev.event == "token" {
///         print!("{}", ev.data.str("text").unwrap_or(""));
///     }
/// }
/// # Ok(())
/// # }
/// ```
#[derive(Debug, Clone)]
pub struct Client {
    base_url: String,
    token: String,
    timeout: Duration,
    tenant: Option<String>,
}

impl Client {
    /// Create a client for the daemon at `base_url` (e.g.
    /// `http://127.0.0.1:8800`) authenticating with `token` (the daemon's admin
    /// token or a tenant token). Defaults to a 30s per-request timeout.
    pub fn new(base_url: impl Into<String>, token: impl Into<String>) -> Client {
        Client {
            base_url: base_url.into().trim_end_matches('/').to_string(),
            token: token.into(),
            timeout: Duration::from_secs(30),
            tenant: None,
        }
    }

    /// Set the per-request timeout (connect + read).
    pub fn with_timeout(mut self, timeout: Duration) -> Client {
        self.timeout = timeout;
        self
    }

    /// Target an isolated tenant on a multi-tenant daemon (sent as the
    /// `X-Agezt-Tenant` header).
    pub fn with_tenant(mut self, tenant: impl Into<String>) -> Client {
        self.tenant = Some(tenant.into());
        self
    }

    // --- public API -------------------------------------------------------

    /// The daemon's liveness + version summary.
    pub fn health(&self) -> Result<Health> {
        let v = self.get_json("/api/v1/health")?;
        Ok(Health {
            status: str_field(&v, "status"),
            version: str_field(&v, "version"),
            default_model: str_field(&v, "default_model"),
            model_count: v.get("model_count").and_then(Value::as_i64).unwrap_or(0),
        })
    }

    /// The default model id and the full set of available model ids.
    pub fn models(&self) -> Result<Models> {
        let v = self.get_json("/api/v1/models")?;
        let models = v
            .get("models")
            .and_then(Value::as_array)
            .map(|a| {
                a.iter()
                    .filter_map(|e| e.as_str().map(String::from))
                    .collect()
            })
            .unwrap_or_default();
        Ok(Models {
            default: str_field(&v, "default"),
            models,
        })
    }

    /// Execute an intent and return the final answer (blocking). `model`
    /// optionally selects the model (and thereby its provider).
    ///
    /// Returns [`Error::Api`] if the run fails or the request is rejected.
    pub fn run(&self, intent: &str, model: Option<&str>) -> Result<RunResult> {
        let body = run_body(intent, model, false);
        let v = self.post_json("/api/v1/runs", &body)?;
        Ok(RunResult {
            correlation_id: str_field(&v, "correlation_id"),
            model: str_field(&v, "model"),
            status: str_field(&v, "status"),
            answer: str_field(&v, "answer"),
        })
    }

    /// Execute an intent, returning an iterator of [`StreamEvent`]s
    /// (`start`/`token`/`done`/`error`) as the agent produces them. Each item
    /// is a [`Result`] so a mid-stream transport failure surfaces in the loop.
    pub fn run_stream(&self, intent: &str, model: Option<&str>) -> Result<RunStream> {
        let body = run_body(intent, model, true);
        let resp = self.send(
            "POST",
            "/api/v1/runs",
            Some(body.as_bytes()),
            "text/event-stream",
        )?;
        if !(200..300).contains(&resp.status) {
            return Err(api_error(resp.status, &resp.read_text()?));
        }
        Ok(RunStream {
            lines: resp.body.into_lines(),
            done: false,
        })
    }

    /// The journaled event arc of a past run.
    pub fn get_run(&self, correlation_id: &str) -> Result<RunArc> {
        let path = format!("/api/v1/runs/{}", percent_encode(correlation_id));
        let v = self.get_json(&path)?;
        let events = v
            .get("events")
            .and_then(Value::as_array)
            .map(<[Value]>::to_vec)
            .unwrap_or_default();
        Ok(RunArc {
            correlation_id: str_field(&v, "correlation_id"),
            count: v.get("count").and_then(Value::as_i64).unwrap_or(0),
            events,
        })
    }

    // --- internals --------------------------------------------------------

    fn get_json(&self, path: &str) -> Result<Value> {
        let resp = self.send("GET", path, None, "application/json")?;
        self.read_json(resp)
    }

    fn post_json(&self, path: &str, body: &str) -> Result<Value> {
        let resp = self.send("POST", path, Some(body.as_bytes()), "application/json")?;
        self.read_json(resp)
    }

    fn read_json(&self, resp: http::Response) -> Result<Value> {
        let status = resp.status;
        let text = resp.read_text()?;
        if !(200..300).contains(&status) {
            return Err(api_error(status, &text));
        }
        if text.trim().is_empty() {
            return Ok(Value::Object(Default::default()));
        }
        Value::parse(&text).map_err(|e| Error::Transport(format!("invalid JSON response: {e}")))
    }

    fn send(
        &self,
        method: &str,
        path: &str,
        body: Option<&[u8]>,
        accept: &str,
    ) -> Result<http::Response> {
        let target = Target::parse(&self.base_url)?;
        let auth = format!("Bearer {}", self.token);
        let mut headers: Vec<(&str, &str)> = vec![("Authorization", &auth), ("Accept", accept)];
        if body.is_some() {
            headers.push(("Content-Type", "application/json"));
        }
        if let Some(t) = &self.tenant {
            headers.push(("X-Agezt-Tenant", t));
        }
        http::request(&target, method, path, &headers, body, self.timeout).map_err(Error::from)
    }
}

/// The streaming-run iterator returned by [`Client::run_stream`].
pub struct RunStream {
    lines: std::io::BufReader<BodyReader>,
    done: bool,
}

impl Iterator for RunStream {
    type Item = Result<StreamEvent>;

    fn next(&mut self) -> Option<Result<StreamEvent>> {
        if self.done {
            return None;
        }
        let mut event = String::from("message");
        let mut data_lines: Vec<String> = Vec::new();
        loop {
            let mut raw = String::new();
            match self.lines.read_line(&mut raw) {
                Ok(0) => {
                    // EOF: flush a trailing frame with no terminating blank line.
                    self.done = true;
                    if data_lines.is_empty() {
                        return None;
                    }
                    return Some(Ok(make_event(event, &data_lines)));
                }
                Ok(_) => {}
                Err(e) => {
                    self.done = true;
                    return Some(Err(Error::Transport(e.to_string())));
                }
            }
            let line = raw.trim_end_matches(['\r', '\n']);
            if line.is_empty() {
                if data_lines.is_empty() {
                    // A blank line with no preceding data: keep scanning.
                    continue;
                }
                return Some(Ok(make_event(event, &data_lines)));
            }
            if line.starts_with(':') {
                continue; // SSE comment / heartbeat
            }
            if let Some(rest) = line.strip_prefix("event:") {
                event = rest.trim().to_string();
            } else if let Some(rest) = line.strip_prefix("data:") {
                data_lines.push(rest.strip_prefix(' ').unwrap_or(rest).to_string());
            }
        }
    }
}

fn make_event(event: String, data_lines: &[String]) -> StreamEvent {
    let joined = data_lines.join("\n");
    let data = Value::parse(&joined).unwrap_or_else(|_| {
        let mut m = std::collections::BTreeMap::new();
        m.insert("raw".to_string(), Value::Str(joined));
        Value::Object(m)
    });
    StreamEvent { event, data }
}

fn run_body(intent: &str, model: Option<&str>, stream: bool) -> String {
    let mut m = std::collections::BTreeMap::new();
    m.insert("intent".to_string(), Value::Str(intent.to_string()));
    if let Some(model) = model {
        m.insert("model".to_string(), Value::Str(model.to_string()));
    }
    if stream {
        m.insert("stream".to_string(), Value::Bool(true));
    }
    Value::Object(m).to_json()
}

fn str_field(v: &Value, key: &str) -> String {
    v.str(key).unwrap_or("").to_string()
}

/// Map a non-2xx response body to an [`Error::Api`]. Understands both
/// `{"error": {"type", "message"}}` and the failed-run `{"status", "error"}`.
fn api_error(status: u16, body: &str) -> Error {
    let mut kind = String::new();
    let mut message = String::new();
    if let Ok(v) = Value::parse(body) {
        match v.get("error") {
            Some(Value::Object(_)) => {
                let err = v.get("error").unwrap();
                kind = err.str("type").unwrap_or("").to_string();
                message = err.str("message").unwrap_or("").to_string();
            }
            Some(Value::Str(s)) => {
                // failed-run body: {"status": "failed", "error": "…"}
                kind = v.str("status").unwrap_or("").to_string();
                message = s.clone();
            }
            _ => {}
        }
    }
    Error::Api {
        status,
        kind,
        message,
    }
}

/// Percent-encode a path segment, escaping anything outside the unreserved set.
fn percent_encode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for &b in s.as_bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(b as char)
            }
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builds_run_body_with_optional_fields() {
        assert_eq!(run_body("hi", None, false), r#"{"intent":"hi"}"#);
        assert_eq!(
            run_body("hi", Some("m"), true),
            r#"{"intent":"hi","model":"m","stream":true}"#
        );
    }

    #[test]
    fn maps_structured_and_failed_run_errors() {
        let e = api_error(
            401,
            r#"{"error":{"type":"unauthorized","message":"no token"}}"#,
        );
        match e {
            Error::Api {
                status,
                kind,
                message,
            } => {
                assert_eq!(status, 401);
                assert_eq!(kind, "unauthorized");
                assert_eq!(message, "no token");
            }
            _ => panic!("wrong variant"),
        }

        let e = api_error(502, r#"{"status":"failed","error":"provider exploded"}"#);
        match e {
            Error::Api { kind, message, .. } => {
                assert_eq!(kind, "failed");
                assert_eq!(message, "provider exploded");
            }
            _ => panic!("wrong variant"),
        }

        // Non-JSON body still yields an Api error (empty kind/message).
        assert!(matches!(
            api_error(500, "boom"),
            Error::Api { status: 500, .. }
        ));
    }

    #[test]
    fn percent_encodes_reserved() {
        assert_eq!(percent_encode("01HABC-xyz_.~"), "01HABC-xyz_.~");
        assert_eq!(percent_encode("a/b c"), "a%2Fb%20c");
    }
}
