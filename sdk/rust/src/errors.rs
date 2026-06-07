//! Error type for the Agezt client.

use std::fmt;

/// Anything that can go wrong talking to the daemon.
///
/// Mirrors the Python/TypeScript SDKs: a non-2xx response becomes
/// [`Error::Api`] (carrying the HTTP status and, when the body provides them,
/// the machine-readable `type` and human-readable `message`); anything that
/// fails before a complete response — DNS, connect, I/O, a malformed
/// response — becomes [`Error::Transport`].
#[derive(Debug)]
pub enum Error {
    /// The daemon returned a non-2xx response.
    Api {
        /// The HTTP status code.
        status: u16,
        /// The machine-readable error type from the body (`error.type`, or the
        /// failed-run `status`), or empty when absent.
        kind: String,
        /// The human-readable message from the body, or the raw body.
        message: String,
    },
    /// A transport, I/O, or protocol failure before a complete response.
    Transport(String),
}

impl Error {
    /// The HTTP status code for an [`Error::Api`] (else `None`).
    pub fn status(&self) -> Option<u16> {
        match self {
            Error::Api { status, .. } => Some(*status),
            Error::Transport(_) => None,
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::Api {
                status,
                kind,
                message,
            } => {
                let detail = if !message.is_empty() {
                    message.as_str()
                } else if !kind.is_empty() {
                    kind.as_str()
                } else {
                    "request failed"
                };
                write!(f, "agezt: HTTP {status}: {detail}")
            }
            Error::Transport(msg) => write!(f, "agezt: transport error: {msg}"),
        }
    }
}

impl std::error::Error for Error {}

impl From<std::io::Error> for Error {
    fn from(e: std::io::Error) -> Self {
        Error::Transport(e.to_string())
    }
}

/// Shorthand for results returned by this crate.
pub type Result<T> = std::result::Result<T, Error>;
