//! A minimal, dependency-free HTTP/1.1 client over [`std::net::TcpStream`].
//!
//! Just enough to talk to a local Agezt daemon: one request per connection
//! (`Connection: close`), `Content-Length` or `chunked` or read-to-EOF response
//! bodies, and a body reader that streams (for Server-Sent Events) without
//! buffering the whole response. **Plain `http://` only** — the std library has
//! no TLS; front the daemon with a reverse proxy if you need `https`. This
//! mirrors the daemon's documented loopback deployment and keeps the SDK at
//! zero runtime dependencies.

use std::io::{self, BufRead, BufReader, Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::time::Duration;

/// A parsed request target.
pub(crate) struct Target {
    pub host: String,
    pub port: u16,
    pub path_prefix: String,
}

impl Target {
    /// Split a base URL like `http://127.0.0.1:8800` (optionally with a path)
    /// into host, port and any path prefix. Rejects non-`http` schemes.
    pub fn parse(base_url: &str) -> io::Result<Target> {
        let rest = match base_url.strip_prefix("http://") {
            Some(r) => r,
            None => {
                if base_url.starts_with("https://") {
                    return Err(invalid(
                        "https is not supported (the std-only client has no TLS); \
                         use http:// behind a TLS-terminating proxy",
                    ));
                }
                return Err(invalid("base_url must start with http://"));
            }
        };
        let (authority, path_prefix) = match rest.find('/') {
            Some(i) => (&rest[..i], rest[i..].trim_end_matches('/').to_string()),
            None => (rest, String::new()),
        };
        if authority.is_empty() {
            return Err(invalid("base_url has no host"));
        }
        let (host, port) = match authority.rsplit_once(':') {
            Some((h, p)) => {
                let port = p
                    .parse::<u16>()
                    .map_err(|_| invalid("base_url has an invalid port"))?;
                (h.to_string(), port)
            }
            None => (authority.to_string(), 80),
        };
        if host.is_empty() {
            return Err(invalid("base_url has no host"));
        }
        Ok(Target {
            host,
            port,
            path_prefix,
        })
    }
}

/// A response: status code plus a streaming body reader.
pub(crate) struct Response {
    pub status: u16,
    pub body: BodyReader,
}

impl Response {
    /// Read the whole body into a `String` (for unary JSON responses).
    pub fn read_text(mut self) -> io::Result<String> {
        let mut s = String::new();
        self.body.read_to_string(&mut s)?;
        Ok(s)
    }
}

/// Perform one HTTP request and return the response. `headers` are extra header
/// lines (without CRLF); `body` is an optional request entity.
pub(crate) fn request(
    target: &Target,
    method: &str,
    path: &str,
    headers: &[(&str, &str)],
    body: Option<&[u8]>,
    timeout: Duration,
) -> io::Result<Response> {
    let addr = (target.host.as_str(), target.port)
        .to_socket_addrs()?
        .next()
        .ok_or_else(|| invalid("could not resolve host"))?;
    let stream = TcpStream::connect_timeout(&addr, timeout)?;
    stream.set_read_timeout(Some(timeout))?;
    stream.set_write_timeout(Some(timeout))?;

    let full_path = format!("{}{}", target.path_prefix, path);
    let host_header = if target.port == 80 {
        target.host.clone()
    } else {
        format!("{}:{}", target.host, target.port)
    };

    let mut req = Vec::with_capacity(256);
    write!(req, "{method} {full_path} HTTP/1.1\r\n")?;
    write!(req, "Host: {host_header}\r\n")?;
    write!(req, "Connection: close\r\n")?;
    for (k, v) in headers {
        write!(req, "{k}: {v}\r\n")?;
    }
    if let Some(b) = body {
        write!(req, "Content-Length: {}\r\n", b.len())?;
    }
    req.extend_from_slice(b"\r\n");
    if let Some(b) = body {
        req.extend_from_slice(b);
    }

    let mut stream = stream;
    stream.write_all(&req)?;
    stream.flush()?;

    let mut reader = BufReader::new(stream);

    // Status line: HTTP/1.1 <code> <reason>
    let mut status_line = String::new();
    reader.read_line(&mut status_line)?;
    let status = parse_status(&status_line)?;

    // Headers until a blank line.
    let mut content_length: Option<u64> = None;
    let mut chunked = false;
    loop {
        let mut line = String::new();
        let n = reader.read_line(&mut line)?;
        if n == 0 {
            break; // connection closed before headers ended
        }
        let line = line.trim_end_matches(['\r', '\n']);
        if line.is_empty() {
            break;
        }
        if let Some((k, v)) = line.split_once(':') {
            let key = k.trim().to_ascii_lowercase();
            let val = v.trim();
            if key == "content-length" {
                content_length = val.parse::<u64>().ok();
            } else if key == "transfer-encoding" && val.eq_ignore_ascii_case("chunked") {
                chunked = true;
            }
        }
    }

    let mode = if chunked {
        BodyMode::Chunked {
            remaining: 0,
            done: false,
        }
    } else if let Some(len) = content_length {
        BodyMode::Length { remaining: len }
    } else {
        BodyMode::Eof
    };

    Ok(Response {
        status,
        body: BodyReader { reader, mode },
    })
}

fn parse_status(line: &str) -> io::Result<u16> {
    // e.g. "HTTP/1.1 200 OK"
    let mut parts = line.split_whitespace();
    let _http = parts.next();
    let code = parts
        .next()
        .and_then(|c| c.parse::<u16>().ok())
        .ok_or_else(|| invalid("malformed status line"))?;
    Ok(code)
}

/// How the response body is framed.
enum BodyMode {
    /// `Content-Length: n` — exactly `remaining` more bytes.
    Length { remaining: u64 },
    /// `Transfer-Encoding: chunked`.
    Chunked { remaining: u64, done: bool },
    /// No length and not chunked — read until the peer closes.
    Eof,
}

/// A [`Read`] over the response body that transparently decodes the framing.
/// Used both for whole-body reads and for streaming SSE line by line.
pub(crate) struct BodyReader {
    reader: BufReader<TcpStream>,
    mode: BodyMode,
}

impl Read for BodyReader {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        match &mut self.mode {
            BodyMode::Eof => self.reader.read(buf),
            BodyMode::Length { remaining } => {
                if *remaining == 0 {
                    return Ok(0);
                }
                let want = (*remaining).min(buf.len() as u64) as usize;
                let n = self.reader.read(&mut buf[..want])?;
                *remaining -= n as u64;
                Ok(n)
            }
            BodyMode::Chunked { remaining, done } => {
                if *done {
                    return Ok(0);
                }
                if *remaining == 0 {
                    // Read the next chunk-size line (hex, optional ;ext).
                    let mut size_line = String::new();
                    self.reader.read_line(&mut size_line)?;
                    let hex = size_line.trim_end_matches(['\r', '\n']);
                    let hex = hex.split(';').next().unwrap_or("").trim();
                    let size = u64::from_str_radix(hex, 16)
                        .map_err(|_| invalid("malformed chunk size"))?;
                    if size == 0 {
                        // Consume the trailing CRLF (and any trailers) and stop.
                        *done = true;
                        let mut tail = String::new();
                        // Best-effort: read until blank line / EOF.
                        loop {
                            tail.clear();
                            let n = self.reader.read_line(&mut tail)?;
                            if n == 0 || tail.trim_end_matches(['\r', '\n']).is_empty() {
                                break;
                            }
                        }
                        return Ok(0);
                    }
                    *remaining = size;
                }
                let want = (*remaining).min(buf.len() as u64) as usize;
                let n = self.reader.read(&mut buf[..want])?;
                *remaining -= n as u64;
                if *remaining == 0 {
                    // Each chunk's data is followed by a CRLF; discard it.
                    let mut crlf = [0u8; 2];
                    let _ = self.reader.read_exact(&mut crlf);
                }
                Ok(n)
            }
        }
    }
}

impl BodyReader {
    /// Wrap the body in a buffered line reader for SSE parsing.
    pub fn into_lines(self) -> BufReader<BodyReader> {
        BufReader::new(self)
    }
}

fn invalid(msg: &str) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, msg.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_targets() {
        let t = Target::parse("http://127.0.0.1:8800").unwrap();
        assert_eq!(t.host, "127.0.0.1");
        assert_eq!(t.port, 8800);
        assert_eq!(t.path_prefix, "");

        let t = Target::parse("http://example.com/base/").unwrap();
        assert_eq!(t.host, "example.com");
        assert_eq!(t.port, 80);
        assert_eq!(t.path_prefix, "/base");
    }

    #[test]
    fn rejects_https_and_garbage() {
        assert!(Target::parse("https://x").is_err());
        assert!(Target::parse("ftp://x").is_err());
        assert!(Target::parse("http://:99").is_err()); // empty host
    }

    #[test]
    fn rejects_bad_port_and_empty_host() {
        assert!(Target::parse("http://host:notaport").is_err());
        assert!(Target::parse("http://").is_err());
    }

    #[test]
    fn parses_status_line() {
        assert_eq!(parse_status("HTTP/1.1 200 OK").unwrap(), 200);
        assert_eq!(parse_status("HTTP/1.1 404 Not Found").unwrap(), 404);
        assert!(parse_status("garbage").is_err());
    }
}
