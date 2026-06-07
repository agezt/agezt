//! Integration tests against an in-process mock daemon built on the standard
//! library's `TcpListener` — no third-party HTTP or test framework, mirroring
//! the TypeScript SDK's `node:http` mock. The mock speaks real HTTP/1.1
//! (Content-Length for JSON, chunked transfer-encoding for the SSE stream), so
//! these exercise the client's actual transport path end to end.

use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::thread;
use std::time::Duration;

use agezt::{Client, Error};

/// Spawn the mock daemon on an ephemeral port; return its base URL.
fn start_mock() -> String {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let addr = listener.local_addr().unwrap();
    thread::spawn(move || {
        for stream in listener.incoming() {
            if let Ok(stream) = stream {
                thread::spawn(move || handle(stream));
            }
        }
    });
    format!("http://127.0.0.1:{}", addr.port())
}

struct Req {
    method: String,
    path: String,
    authorized: bool,
    tenant: Option<String>,
    body: String,
}

fn read_request(stream: &TcpStream) -> Option<Req> {
    let mut reader = BufReader::new(stream.try_clone().ok()?);
    let mut line = String::new();
    reader.read_line(&mut line).ok()?;
    let mut parts = line.split_whitespace();
    let method = parts.next()?.to_string();
    let path = parts.next()?.to_string();

    let mut content_length = 0usize;
    let mut authorized = false;
    let mut tenant = None;
    loop {
        let mut h = String::new();
        let n = reader.read_line(&mut h).ok()?;
        if n == 0 {
            break;
        }
        let h = h.trim_end_matches(['\r', '\n']);
        if h.is_empty() {
            break;
        }
        if let Some((k, v)) = h.split_once(':') {
            let key = k.trim().to_ascii_lowercase();
            let val = v.trim();
            if key == "content-length" {
                content_length = val.parse().unwrap_or(0);
            } else if key == "authorization" && val == "Bearer testtoken" {
                authorized = true;
            } else if key == "x-agezt-tenant" {
                tenant = Some(val.to_string());
            }
        }
    }

    let mut body = vec![0u8; content_length];
    if content_length > 0 {
        reader.read_exact(&mut body).ok()?;
    }
    Some(Req {
        method,
        path,
        authorized,
        tenant,
        body: String::from_utf8_lossy(&body).into_owned(),
    })
}

fn write_json(stream: &mut TcpStream, code: u16, body: &str) {
    let resp = format!(
        "HTTP/1.1 {code} X\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    let _ = stream.write_all(resp.as_bytes());
}

fn write_sse(stream: &mut TcpStream, frames: &[&str]) {
    let head = "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n";
    let _ = stream.write_all(head.as_bytes());
    for frame in frames {
        // Each frame is its own HTTP chunk, so the client must reassemble SSE
        // frames across chunk boundaries.
        let _ = stream.write_all(format!("{:x}\r\n", frame.len()).as_bytes());
        let _ = stream.write_all(frame.as_bytes());
        let _ = stream.write_all(b"\r\n");
    }
    let _ = stream.write_all(b"0\r\n\r\n");
}

/// Pull the value of a `"key":"…"` string field out of a JSON object body.
fn extract_str(body: &str, key: &str) -> Option<String> {
    let needle = format!("\"{key}\":\"");
    let start = body.find(&needle)? + needle.len();
    let rest = &body[start..];
    let end = rest.find('"')?;
    Some(rest[..end].to_string())
}

fn handle(mut stream: TcpStream) {
    let req = match read_request(&stream) {
        Some(r) => r,
        None => return,
    };
    if !req.authorized {
        write_json(
            &mut stream,
            401,
            r#"{"error":{"type":"unauthorized","message":"missing or invalid token"}}"#,
        );
        return;
    }
    match (req.method.as_str(), req.path.as_str()) {
        ("GET", "/api/v1/health") => {
            // Reflect the tenant header (when present) into `version` so a test
            // can prove X-Agezt-Tenant was transmitted.
            let version = req.tenant.as_deref().unwrap_or("test");
            let body = format!(
                r#"{{"status":"ok","version":"{version}","default_model":"m","model_count":1}}"#
            );
            write_json(&mut stream, 200, &body);
        }
        ("GET", "/api/v1/models") => {
            write_json(&mut stream, 200, r#"{"default":"m","models":["m","n"]}"#)
        }
        ("GET", p) if p.starts_with("/api/v1/runs/") => write_json(
            &mut stream,
            200,
            r#"{"correlation_id":"c1","count":2,"events":[{"seq":1},{"seq":2}]}"#,
        ),
        ("POST", "/api/v1/runs") => {
            if extract_str(&req.body, "intent").as_deref() == Some("boom") {
                write_json(
                    &mut stream,
                    502,
                    r#"{"correlation_id":"c2","model":"m","status":"failed","error":"provider exploded"}"#,
                );
            } else if req.body.contains("\"stream\":true") {
                write_sse(
                    &mut stream,
                    &[
                        "event: start\ndata: {\"correlation_id\":\"c3\",\"model\":\"m\"}\n\n",
                        "event: token\ndata: {\"text\":\"hel\"}\n\n",
                        ": heartbeat\n\n",
                        "event: token\ndata: {\"text\":\"lo\"}\n\n",
                        "event: done\ndata: {\"correlation_id\":\"c3\",\"status\":\"completed\",\"answer\":\"hello\"}\n\n",
                    ],
                );
            } else {
                let model = extract_str(&req.body, "model").unwrap_or_else(|| "m".to_string());
                let body = format!(
                    r#"{{"correlation_id":"c4","model":"{model}","status":"completed","answer":"pong"}}"#
                );
                write_json(&mut stream, 200, &body);
            }
        }
        _ => write_json(
            &mut stream,
            404,
            r#"{"error":{"type":"not_found","message":"nope"}}"#,
        ),
    }
}

fn client(base: &str, token: &str) -> Client {
    Client::new(base, token).with_timeout(Duration::from_secs(5))
}

#[test]
fn health() {
    let base = start_mock();
    let h = client(&base, "testtoken").health().unwrap();
    assert_eq!(h.status, "ok");
    assert_eq!(h.version, "test");
    assert_eq!(h.default_model, "m");
    assert_eq!(h.model_count, 1);
}

#[test]
fn models() {
    let base = start_mock();
    let m = client(&base, "testtoken").models().unwrap();
    assert_eq!(m.default, "m");
    assert_eq!(m.models, vec!["m".to_string(), "n".to_string()]);
}

#[test]
fn run_forwards_model_and_returns_answer() {
    let base = start_mock();
    // A model distinct from the server default proves it was forwarded: the
    // mock echoes back whatever `model` it parsed from the request body.
    let r = client(&base, "testtoken")
        .run("ping", Some("special"))
        .unwrap();
    assert_eq!(r.status, "completed");
    assert_eq!(r.answer, "pong");
    assert_eq!(r.correlation_id, "c4");
    assert_eq!(r.model, "special");
}

#[test]
fn failed_run_is_api_error_502() {
    let base = start_mock();
    let err = client(&base, "testtoken").run("boom", None).unwrap_err();
    match err {
        Error::Api {
            status,
            kind,
            message,
        } => {
            assert_eq!(status, 502);
            assert_eq!(kind, "failed");
            assert!(message.contains("provider exploded"), "got {message:?}");
        }
        other => panic!("expected Api error, got {other:?}"),
    }
}

#[test]
fn run_stream_yields_frames_and_reassembles_tokens() {
    let base = start_mock();
    let c = client(&base, "testtoken");
    let events: Vec<_> = c
        .run_stream("hi", None)
        .unwrap()
        .map(|e| e.unwrap())
        .collect();

    let names: Vec<&str> = events.iter().map(|e| e.event.as_str()).collect();
    assert_eq!(names, vec!["start", "token", "token", "done"]);

    let tokens: String = events
        .iter()
        .filter(|e| e.event == "token")
        .filter_map(|e| e.data.str("text").map(String::from))
        .collect();
    assert_eq!(tokens, "hello");

    let last = events.last().unwrap();
    assert_eq!(last.data.str("answer"), Some("hello"));
    assert_eq!(last.data.str("correlation_id"), Some("c3"));
}

#[test]
fn get_run_returns_event_arc() {
    let base = start_mock();
    let arc = client(&base, "testtoken").get_run("c1").unwrap();
    assert_eq!(arc.correlation_id, "c1");
    assert_eq!(arc.count, 2);
    assert_eq!(arc.events.len(), 2);
    assert_eq!(
        arc.events[0].get("seq").and_then(agezt::Value::as_i64),
        Some(1)
    );
}

#[test]
fn bad_token_is_api_error_401() {
    let base = start_mock();
    let err = client(&base, "WRONG").health().unwrap_err();
    match err {
        Error::Api { status, kind, .. } => {
            assert_eq!(status, 401);
            assert_eq!(kind, "unauthorized");
        }
        other => panic!("expected Api error, got {other:?}"),
    }
}

#[test]
fn tenant_header_is_transmitted() {
    let base = start_mock();
    // The mock reflects the X-Agezt-Tenant header into `version`, so a non-default
    // value proves the header was actually sent on the wire.
    let c = Client::new(&base, "testtoken")
        .with_tenant("acme")
        .with_timeout(Duration::from_secs(5));
    assert_eq!(c.health().unwrap().version, "acme");

    // Without a tenant, the mock falls back to "test".
    assert_eq!(client(&base, "testtoken").health().unwrap().version, "test");
}
