//! A tiny, dependency-free JSON implementation.
//!
//! The Agezt REST payloads are small and well-shaped, so this module keeps a
//! minimal recursive-descent parser and a serializer rather than pulling in
//! `serde` — matching the SDK's zero-runtime-dependency promise. [`Value`] is
//! part of the public API ([`crate::StreamEvent::data`] and the elements of
//! [`crate::RunArc::events`] are `Value`s), so it carries ergonomic accessors.

use std::collections::BTreeMap;
use std::fmt::{self, Write as _};

/// A decoded JSON value.
///
/// Objects preserve their keys in a [`BTreeMap`] (sorted), which is all the
/// daemon's responses need; use [`Value::get`] and the `as_*` accessors to read
/// fields ergonomically:
///
/// ```
/// # use agezt::Value;
/// let v = Value::parse(r#"{"text":"hi","n":3}"#).unwrap();
/// assert_eq!(v.get("text").and_then(Value::as_str), Some("hi"));
/// assert_eq!(v.get("n").and_then(Value::as_i64), Some(3));
/// ```
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    /// JSON `null`.
    Null,
    /// `true` / `false`.
    Bool(bool),
    /// A number that parsed as an integer.
    Int(i64),
    /// A number with a fraction or exponent (or one too large for `i64`).
    Float(f64),
    /// A string.
    Str(String),
    /// An array.
    Array(Vec<Value>),
    /// An object. Keys are kept sorted.
    Object(BTreeMap<String, Value>),
}

impl Value {
    /// Parse a JSON document. Returns an error string describing the first
    /// problem; trailing non-whitespace is rejected.
    pub fn parse(s: &str) -> Result<Value, String> {
        let mut p = Parser {
            bytes: s.as_bytes(),
            pos: 0,
        };
        p.skip_ws();
        let v = p.parse_value()?;
        p.skip_ws();
        if p.pos != p.bytes.len() {
            return Err(format!("trailing data at byte {}", p.pos));
        }
        Ok(v)
    }

    /// For an [`Value::Object`], the value under `key` (else `None`).
    pub fn get(&self, key: &str) -> Option<&Value> {
        match self {
            Value::Object(m) => m.get(key),
            _ => None,
        }
    }

    /// The string, if this is a [`Value::Str`].
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Value::Str(s) => Some(s),
            _ => None,
        }
    }

    /// The integer, if this is a [`Value::Int`] (or an integral [`Value::Float`]).
    pub fn as_i64(&self) -> Option<i64> {
        match self {
            Value::Int(n) => Some(*n),
            Value::Float(f) if f.fract() == 0.0 => Some(*f as i64),
            _ => None,
        }
    }

    /// The number as `f64`, for either numeric variant.
    pub fn as_f64(&self) -> Option<f64> {
        match self {
            Value::Int(n) => Some(*n as f64),
            Value::Float(f) => Some(*f),
            _ => None,
        }
    }

    /// The boolean, if this is a [`Value::Bool`].
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            Value::Bool(b) => Some(*b),
            _ => None,
        }
    }

    /// The elements, if this is a [`Value::Array`].
    pub fn as_array(&self) -> Option<&[Value]> {
        match self {
            Value::Array(a) => Some(a),
            _ => None,
        }
    }

    /// Convenience: `get(key)` then `as_str()`, the common case for reading a
    /// stream event payload (`ev.data.str("text")`).
    pub fn str(&self, key: &str) -> Option<&str> {
        self.get(key).and_then(Value::as_str)
    }

    /// Serialize back to a compact JSON string.
    pub fn to_json(&self) -> String {
        let mut out = String::new();
        // write! to a String is infallible.
        let _ = self.write_json(&mut out);
        out
    }

    fn write_json(&self, out: &mut String) -> fmt::Result {
        match self {
            Value::Null => out.write_str("null"),
            Value::Bool(true) => out.write_str("true"),
            Value::Bool(false) => out.write_str("false"),
            Value::Int(n) => write!(out, "{n}"),
            Value::Float(f) => write!(out, "{f}"),
            Value::Str(s) => write_json_string(s, out),
            Value::Array(a) => {
                out.push('[');
                for (i, v) in a.iter().enumerate() {
                    if i > 0 {
                        out.push(',');
                    }
                    v.write_json(out)?;
                }
                out.push(']');
                Ok(())
            }
            Value::Object(m) => {
                out.push('{');
                for (i, (k, v)) in m.iter().enumerate() {
                    if i > 0 {
                        out.push(',');
                    }
                    write_json_string(k, out)?;
                    out.push(':');
                    v.write_json(out)?;
                }
                out.push('}');
                Ok(())
            }
        }
    }
}

fn write_json_string(s: &str, out: &mut String) -> fmt::Result {
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\u{08}' => out.push_str("\\b"),
            '\u{0c}' => out.push_str("\\f"),
            c if (c as u32) < 0x20 => {
                write!(out, "\\u{:04x}", c as u32)?;
            }
            c => out.push(c),
        }
    }
    out.push('"');
    Ok(())
}

struct Parser<'a> {
    bytes: &'a [u8],
    pos: usize,
}

impl<'a> Parser<'a> {
    fn skip_ws(&mut self) {
        while self.pos < self.bytes.len() {
            match self.bytes[self.pos] {
                b' ' | b'\t' | b'\n' | b'\r' => self.pos += 1,
                _ => break,
            }
        }
    }

    fn peek(&self) -> Option<u8> {
        self.bytes.get(self.pos).copied()
    }

    fn parse_value(&mut self) -> Result<Value, String> {
        self.skip_ws();
        match self.peek() {
            Some(b'{') => self.parse_object(),
            Some(b'[') => self.parse_array(),
            Some(b'"') => Ok(Value::Str(self.parse_string()?)),
            Some(b't') | Some(b'f') => self.parse_bool(),
            Some(b'n') => self.parse_null(),
            Some(c) if c == b'-' || c.is_ascii_digit() => self.parse_number(),
            Some(c) => Err(format!("unexpected byte {:?} at {}", c as char, self.pos)),
            None => Err("unexpected end of input".to_string()),
        }
    }

    fn expect(&mut self, b: u8) -> Result<(), String> {
        if self.peek() == Some(b) {
            self.pos += 1;
            Ok(())
        } else {
            Err(format!("expected {:?} at {}", b as char, self.pos))
        }
    }

    fn parse_object(&mut self) -> Result<Value, String> {
        self.expect(b'{')?;
        let mut map = BTreeMap::new();
        self.skip_ws();
        if self.peek() == Some(b'}') {
            self.pos += 1;
            return Ok(Value::Object(map));
        }
        loop {
            self.skip_ws();
            let key = self.parse_string()?;
            self.skip_ws();
            self.expect(b':')?;
            let val = self.parse_value()?;
            map.insert(key, val);
            self.skip_ws();
            match self.peek() {
                Some(b',') => {
                    self.pos += 1;
                }
                Some(b'}') => {
                    self.pos += 1;
                    break;
                }
                _ => return Err(format!("expected ',' or '}}' at {}", self.pos)),
            }
        }
        Ok(Value::Object(map))
    }

    fn parse_array(&mut self) -> Result<Value, String> {
        self.expect(b'[')?;
        let mut arr = Vec::new();
        self.skip_ws();
        if self.peek() == Some(b']') {
            self.pos += 1;
            return Ok(Value::Array(arr));
        }
        loop {
            let val = self.parse_value()?;
            arr.push(val);
            self.skip_ws();
            match self.peek() {
                Some(b',') => {
                    self.pos += 1;
                }
                Some(b']') => {
                    self.pos += 1;
                    break;
                }
                _ => return Err(format!("expected ',' or ']' at {}", self.pos)),
            }
        }
        Ok(Value::Array(arr))
    }

    fn parse_string(&mut self) -> Result<String, String> {
        self.expect(b'"')?;
        let mut s = String::new();
        loop {
            let c = match self.peek() {
                Some(c) => c,
                None => return Err("unterminated string".to_string()),
            };
            self.pos += 1;
            match c {
                b'"' => break,
                b'\\' => {
                    let esc = self.peek().ok_or("unterminated escape")?;
                    self.pos += 1;
                    match esc {
                        b'"' => s.push('"'),
                        b'\\' => s.push('\\'),
                        b'/' => s.push('/'),
                        b'n' => s.push('\n'),
                        b'r' => s.push('\r'),
                        b't' => s.push('\t'),
                        b'b' => s.push('\u{08}'),
                        b'f' => s.push('\u{0c}'),
                        b'u' => s.push(self.parse_unicode_escape()?),
                        other => return Err(format!("invalid escape \\{:?}", other as char)),
                    }
                }
                // A multi-byte UTF-8 sequence: copy its bytes verbatim.
                0x80..=0xFF => {
                    let start = self.pos - 1;
                    while let Some(b) = self.peek() {
                        if b >= 0x80 {
                            self.pos += 1;
                        } else {
                            break;
                        }
                    }
                    match std::str::from_utf8(&self.bytes[start..self.pos]) {
                        Ok(frag) => s.push_str(frag),
                        Err(_) => return Err("invalid UTF-8 in string".to_string()),
                    }
                }
                _ => s.push(c as char),
            }
        }
        Ok(s)
    }

    fn parse_unicode_escape(&mut self) -> Result<char, String> {
        let hi = self.read_hex4()?;
        // Surrogate pair: a high surrogate must be followed by \uXXXX low.
        if (0xD800..=0xDBFF).contains(&hi) {
            if self.peek() == Some(b'\\') {
                self.pos += 1;
                self.expect(b'u')?;
                let lo = self.read_hex4()?;
                if (0xDC00..=0xDFFF).contains(&lo) {
                    let c = 0x10000 + ((hi - 0xD800) << 10) + (lo - 0xDC00);
                    return char::from_u32(c).ok_or_else(|| "invalid code point".to_string());
                }
                return Err("invalid low surrogate".to_string());
            }
            return Err("unpaired high surrogate".to_string());
        }
        char::from_u32(hi).ok_or_else(|| "invalid code point".to_string())
    }

    fn read_hex4(&mut self) -> Result<u32, String> {
        if self.pos + 4 > self.bytes.len() {
            return Err("truncated \\u escape".to_string());
        }
        let slice = &self.bytes[self.pos..self.pos + 4];
        let hex = std::str::from_utf8(slice).map_err(|_| "bad \\u escape")?;
        let n = u32::from_str_radix(hex, 16).map_err(|_| "bad \\u hex")?;
        self.pos += 4;
        Ok(n)
    }

    fn parse_bool(&mut self) -> Result<Value, String> {
        if self.bytes[self.pos..].starts_with(b"true") {
            self.pos += 4;
            Ok(Value::Bool(true))
        } else if self.bytes[self.pos..].starts_with(b"false") {
            self.pos += 5;
            Ok(Value::Bool(false))
        } else {
            Err(format!("invalid literal at {}", self.pos))
        }
    }

    fn parse_null(&mut self) -> Result<Value, String> {
        if self.bytes[self.pos..].starts_with(b"null") {
            self.pos += 4;
            Ok(Value::Null)
        } else {
            Err(format!("invalid literal at {}", self.pos))
        }
    }

    fn parse_number(&mut self) -> Result<Value, String> {
        let start = self.pos;
        let mut is_float = false;
        if self.peek() == Some(b'-') {
            self.pos += 1;
        }
        while let Some(c) = self.peek() {
            match c {
                b'0'..=b'9' => self.pos += 1,
                b'.' | b'e' | b'E' | b'+' | b'-' => {
                    is_float = true;
                    self.pos += 1;
                }
                _ => break,
            }
        }
        let text =
            std::str::from_utf8(&self.bytes[start..self.pos]).map_err(|_| "bad number bytes")?;
        if !is_float {
            if let Ok(n) = text.parse::<i64>() {
                return Ok(Value::Int(n));
            }
        }
        text.parse::<f64>()
            .map(Value::Float)
            .map_err(|_| format!("invalid number {text:?}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_scalars() {
        assert_eq!(Value::parse("null").unwrap(), Value::Null);
        assert_eq!(Value::parse("true").unwrap(), Value::Bool(true));
        assert_eq!(Value::parse("false").unwrap(), Value::Bool(false));
        assert_eq!(Value::parse("-42").unwrap(), Value::Int(-42));
        assert_eq!(Value::parse("3.5").unwrap(), Value::Float(3.5));
        assert_eq!(Value::parse("1e3").unwrap(), Value::Float(1000.0));
        assert_eq!(Value::parse(r#""hi""#).unwrap(), Value::Str("hi".into()));
    }

    #[test]
    fn parses_string_escapes_and_unicode() {
        let v = Value::parse(r#""a\n\t\"\\\/Aé""#).unwrap();
        assert_eq!(v.as_str(), Some("a\n\t\"\\/Aé"));
        // Surrogate pair → 😀 (U+1F600).
        let v = Value::parse(r#""😀""#).unwrap();
        assert_eq!(v.as_str(), Some("😀"));
        // Raw multi-byte UTF-8 passes through untouched.
        let v = Value::parse(r#""café → ☕""#).unwrap();
        assert_eq!(v.as_str(), Some("café → ☕"));
    }

    #[test]
    fn parses_nested_and_accesses() {
        let v = Value::parse(r#"{"a":[1,2,{"b":"x"}],"n":true,"z":null}"#).unwrap();
        assert_eq!(v.get("n").and_then(Value::as_bool), Some(true));
        assert_eq!(v.get("z"), Some(&Value::Null));
        let a = v.get("a").and_then(Value::as_array).unwrap();
        assert_eq!(a.len(), 3);
        assert_eq!(a[0].as_i64(), Some(1));
        assert_eq!(a[2].str("b"), Some("x"));
    }

    #[test]
    fn round_trips_through_serializer() {
        let src = r#"{"answer":"he\"llo\n","k":12,"ok":true,"xs":[1,2.5]}"#;
        let v = Value::parse(src).unwrap();
        let reparsed = Value::parse(&v.to_json()).unwrap();
        assert_eq!(v, reparsed);
        assert_eq!(v.str("answer"), Some("he\"llo\n"));
    }

    #[test]
    fn rejects_trailing_and_truncated() {
        assert!(Value::parse("{} junk").is_err());
        assert!(Value::parse(r#"{"a":"#).is_err());
        assert!(Value::parse(r#""unterminated"#).is_err());
        assert!(Value::parse("[1,2,]").is_err());
    }

    #[test]
    fn empty_object_and_array() {
        assert_eq!(Value::parse("{}").unwrap(), Value::Object(BTreeMap::new()));
        assert_eq!(Value::parse("[]").unwrap(), Value::Array(vec![]));
        assert_eq!(
            Value::parse("  {\n}\t").unwrap(),
            Value::Object(BTreeMap::new())
        );
    }
}
