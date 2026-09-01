# AGEZT — Python + Rust Deep Scan (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · commit `e0041337` · branch `main`
**Scope:** `sdk/python/` (~3.6k LOC), `sdk/rust/` (~2.0k LOC), plus all stray Python
(`scripts/dev/`, `plugins/builtinskills/*/scripts/`).
**Skills applied:** `sc-lang-python`, `sc-lang-rust`.

**Threat model used.** Both targets are *published client SDKs* (PyPI `agezt` 1.1.0,
crates.io `agezt` 1.0.0). The code runs on a **consumer's** machine, holding a bearer
token that grants agent-level control of the daemon (`POST /api/v1/runs` → the full
governed tool loop: shell, file, code_exec). So the two adversaries that matter are:

1. **A malicious or compromised daemon** (or anything MITM'ing the plaintext link) —
   what can the *server* do to the client process?
2. **A malicious argument** — the SDK's whole purpose is to carry LLM-derived and
   inter-agent strings, so caller-supplied arguments must be assumed tainted.

Every finding below cites a line I read. Empirical proofs-of-concept were run in the
scratchpad against **copies of the logic** or against the crate as a library — no
daemon was started and no SDK call was made against any live endpoint.

---

## Summary

| Language | Critical | High | Medium | Low |
|---|---:|---:|---:|---:|
| Python | 1 | 2 | 3 | 2 |
| Rust | 0 | 1 | 2 | 3 |

**Most important finding: `PY-001`** — five call sites in `sdk/python/agezt/agent.py`
interpolate caller-supplied strings straight into a hand-built HTTP request line with no
percent-encoding, giving CRLF request smuggling against the Agent Gateway. The smuggled
request inherits the SDK's own genuine `Authorization` header.

---

# PYTHON

## PY-001 — CRLF injection / HTTP request smuggling in the Agent Gateway client

- **Severity:** CRITICAL · **Confidence:** 92
- **CWE:** CWE-93 (CRLF Injection), CWE-444 (HTTP Request Smuggling), CWE-113
- **File:** `sdk/python/agezt/agent.py:173`, `:191` (the sink) fed by `:296`, `:344`, `:356`, `:410`, `:469`

`_SocketClient._request` hand-builds the HTTP request line by f-string:

```python
# sdk/python/agezt/agent.py:173
req_lines = [f"{method} {path} HTTP/1.1"]
...
# sdk/python/agezt/agent.py:191
sock.sendall("\r\n".join(req_lines).encode("utf-8"))
```

`path` reaches it from five callers, **none of which percent-encode**:

| Line | Code | Tainted input |
|---|---|---|
| `agent.py:296` | `"/v1/eventbus/subscribe?pattern=" + pattern` | `pattern` |
| `agent.py:344` | `f"/v1/memory/search?q={query}&limit={limit}"` | `query` |
| `agent.py:356` | `f"/v1/memory/delete?id={id}"` | `id` |
| `agent.py:410` | `f"/v1/agent/query?id={id}"` | `id` |
| `agent.py:469` | `f"/v1/config/{key}"` | `key` |

Note `_ConfigHandle.get` is inconsistent with itself: it carefully `urlencode`s the
`reason` parameter at `agent.py:474` while leaving `key` raw at `agent.py:469`.

**Exploitation path.** This client is documented as the transport for *AI agent
subprocess code* (`agent.py:1-5`). A subprocess that calls
`client.memory.search(<string derived from LLM output, a board message, or a fetched
document>)` lets that string carry `\r\n`. Verified byte construction:

```
GET /v1/memory/search?q=x HTTP/1.1
Host: localhost
                                     <- attacker's blank line ends request #1
POST /v1/memory/delete?id=victim HTTP/1.1
X-Ignore: &limit=20 HTTP/1.1
Host: localhost
Accept: application/json
Authorization: Bearer REALTOKEN      <- the SDK's own header completes request #2
```

Two syntactically well-formed request lines leave `sendall` from one API call. The
attacker does **not** need to know the token: by omitting their own `Host`/`Authorization`
lines they let the SDK's genuine trailing headers become the smuggled request's header
block. The smuggled request therefore executes with full token authority against any
gateway route — `POST /v1/token/create` (`kernel/agentgw/gateway.go:154`, the
token-minting endpoint), `/v1/memory/delete`, `/v1/config/*`. That converts a *read*
capability into arbitrary gateway authority, defeating the scoped-capability model the
module's own docstring advertises (`agent.py:3-5`).

**Why not a false positive.** There is no encoding, validation, or rejection of `\r`/`\n`
anywhere between the five call sites and `sendall`. I reconstructed the exact wire bytes
by replaying `agent.py:173-191` and `agent.py:344` verbatim; both request lines parse as
well-formed. (I did not fire it at a daemon — per scope. Whether a *specific* payload is
accepted depends on Go's duplicate-header handling, which the attacker controls by
omitting their own `Host`/`Authorization` lines.)

**Remediation.** Percent-encode every interpolated segment and reject control characters
at the transport boundary:

```python
from urllib.parse import quote, urlencode

# callers
path = "/v1/memory/search?" + urlencode({"q": query, "limit": limit})
path = "/v1/config/" + quote(key, safe="")

# and belt-and-braces in _request, before building req_lines:
if any(c in path for c in "\r\n"):
    raise AgentError("INVALID_PATH", "control characters in request path", 400)
for k, v in (headers or {}).items():
    if any(c in f"{k}{v}" for c in "\r\n"):
        raise AgentError("INVALID_HEADER", "control characters in header", 400)
```

---

## PY-002 — Bearer token is forwarded to a redirect target on a different host

- **Severity:** HIGH · **Confidence:** 95
- **CWE:** CWE-522 (Insufficiently Protected Credentials), CWE-200
- **File:** `sdk/python/agezt/client.py:270`; reached via `:138`, `:252`, `:287`

```python
# sdk/python/agezt/client.py:269-271
req = urllib.request.Request(self.base_url + path, data=data, method=method)
req.add_header("Authorization", "Bearer " + self.token)
```

`add_header` (as opposed to `add_unredirected_header`) stores the header in
`Request.headers`, and CPython's `HTTPRedirectHandler.redirect_request` copies **every**
header except `content-length`/`content-type` onto the redirected request — with no
same-origin check.

**Verified** against the installed CPython 3.14.6 by calling `redirect_request` directly
(no network):

```
original req.headers      : {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
after cross-host 302 ->   : https://evil.example.com/collect
forwarded headers         : {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
TOKEN LEAKED TO evil host : True
```

and by reading the stdlib source, which filters on content headers only:

```python
CONTENT_HEADERS = ("content-length", "content-type")
newheaders = {k: v for k, v in req.headers.items() if k.lower() not in CONTENT_HEADERS}
```

**Exploitation path.** A compromised daemon — or, because the SDK accepts plain `http://`
with no scheme check (PY-006), any network attacker on the path — answers any of the 13
API calls with `302 Location: https://attacker/`. `urlopen` follows it automatically and
hands over the admin/tenant bearer token. That token is full agent-level control of the
daemon (`POST /api/v1/runs` → shell/file/code_exec). All three `urlopen` sites are
affected: `client.py:138` (`run_stream`), `:252` (`mailbox_watch`), `:287` (`_do`, i.e.
every unary call).

**Why not a false positive.** No `HTTPRedirectHandler` subclass, no custom opener, and no
`add_unredirected_header` anywhere in `sdk/python/`; the default opener with default
redirect handling is what `urlopen` uses. Confirmed empirically on the exact interpreter
in this environment.

**Remediation.** Use the stdlib API that exists precisely for this, and pin the origin:

```python
req.add_unredirected_header("Authorization", "Bearer " + self.token)
```

Better, install an opener with a redirect handler that refuses (or strips credentials on)
any redirect whose scheme/host/port differs from `base_url`.

---

## PY-003 — SSE path bypasses the abstract-socket fix, reintroducing SDK-001's credential leak

- **Severity:** HIGH · **Confidence:** 88
- **CWE:** CWE-522, CWE-426 (Untrusted Search Path), CWE-668
- **File:** `sdk/python/agezt/agent.py:570-572`, `:580` — versus the fix at `:49-69`

`_resolve_socket_path` exists specifically to stop a credential leak, and says so:

```python
# sdk/python/agezt/agent.py:59-61
#  2. It fails OPEN into a credential leak — an agent subprocess whose CWD is
#     attacker-writable can have "./@agezt/agentgw.sock" planted there, and
#     every request then hands the capability token to whoever is listening.
```

`_SocketClient._connect` applies it (`agent.py:156`). **`_subscribe` does not:**

```python
# sdk/python/agezt/agent.py:570-572
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.settimeout(self.timeout)
sock.connect(self.socket_path)          # <- raw, unresolved
...
# sdk/python/agezt/agent.py:580
f"Authorization: Bearer {self.token}\r\n"
```

With the module default `DEFAULT_SOCKET_PATH = "@agezt/agentgw.sock"` (`agent.py:46`),
CPython copies that string verbatim into `sun_path`, making it the CWD-relative file path
`./@agezt/agentgw.sock` — exactly the scenario the docstring describes. Any process that
can create a file in the agent subprocess's working directory plants a listener there and
receives the capability token on line 580.

This is the repo's documented failure mode: **a fix whose guarantee the code does not
implement on every path.** `_request` is fixed; `_subscribe` is not.

**Secondary defects on the same three lines.** `_subscribe` also bypasses
`_SocketClient._detect_tcp`/`_create_socket` (`agent.py:122-144`), so it (a) raises
`AttributeError` on Windows, where `socket.AF_UNIX` does not exist — the fallback at
`agent.py:141` is not reached; and (b) cannot use a `tcp://host:port` or `host:port`
socket path at all, despite the class docstring at `agent.py:109-113` advertising both.

**Why not a false positive.** `_subscribe` is a method of `_AgentClient`, which holds its
own `self.socket_path` (`agent.py:531`) separate from the `_SocketClient` at `:533`; it
never calls into the fixed helper. `_EventbusHandle.subscribe` (`agent.py:296`) is a
public, documented entry point (`agent.py:27-29`), so this path ships reachable.

**Remediation.** Route `_subscribe` through the same transport helpers:

```python
sock = self._sock._create_socket()
sock.settimeout(self.timeout)
self._sock._connect(sock)
```

Then add a regression test asserting that `_subscribe` on `"@agezt/agentgw.sock"` never
issues a `connect()` to a relative filesystem path.

---

## PY-004 — Unbounded response reads let a malicious daemon exhaust the consumer's memory

- **Severity:** MEDIUM · **Confidence:** 85
- **CWE:** CWE-400 (Uncontrolled Resource Consumption), CWE-789
- **File:** `sdk/python/agezt/client.py:288`, `:299`; `sdk/python/agezt/agent.py:196-201`

```python
# sdk/python/agezt/client.py:288
raw = resp.read()                       # no length argument, no cap
# sdk/python/agezt/client.py:299
body = json.loads(e.read().decode("utf-8"))   # error bodies too
```

```python
# sdk/python/agezt/agent.py:196-201
response = b""
while True:
    chunk = sock.recv(4096)
    if not chunk:
        break
    response += chunk               # unbounded, and O(n²) bytes concatenation
```

No `MaxBytes`-style cap anywhere in `sdk/python/`. A hostile daemon streams an
arbitrarily large body (or a large 500 error body, hitting the `_api_error` path at
`client.py:299`) and the client process grows until the OS kills it. `_parse_sse`
(`client.py:318-319`) iterates the stream line-by-line with no per-line cap either, so a
single unterminated line has the same effect.

**Why not a false positive.** Timeouts bound *latency*, not *volume* — a steady
high-throughput stream never trips the 30 s read timeout because data keeps arriving.

**Remediation.** Cap every read and fail closed:

```python
MAX_BODY = 32 * 1024 * 1024
raw = resp.read(MAX_BODY + 1)
if len(raw) > MAX_BODY:
    raise APIError(resp.status, "too_large", "response exceeded 32 MiB")
```

In `agent.py`, accumulate into a `bytearray` (removing the O(n²)) and enforce the same cap
inside the `recv` loop.

---

## PY-005 — `quote()` default `safe="/"` lets a caller-supplied id escape its path segment

- **Severity:** MEDIUM · **Confidence:** 80
- **CWE:** CWE-88 (Argument Injection), CWE-73
- **File:** `sdk/python/agezt/client.py:146`, `:204`, `:210`

```python
# sdk/python/agezt/client.py:146
return self._get("/api/v1/runs/" + urllib.parse.quote(correlation_id))
# sdk/python/agezt/client.py:204, :210
"/api/v1/mailbox/messages/" + urllib.parse.quote(message_id) + "/ack"
```

`urllib.parse.quote` defaults to `safe="/"`, so **`/` is deliberately left unescaped**,
and `.` is in the always-safe set. A `correlation_id` or `message_id` of
`../../v1/mailbox/messages` therefore rewrites which endpoint is called rather than being
carried as one opaque segment.

Impact is bounded — every `/api/v1/*` route requires the same bearer token the client
already holds, so this is endpoint *confusion*, not privilege escalation. It is
nevertheless a correctness/robustness defect on the client's most attacker-adjacent
inputs (`message_id` values come off the shared inter-agent mailbox).

**Why not a false positive.** Verified against the documented `quote` signature
(`safe='/'` default); the Rust SDK's `percent_encode` (`sdk/rust/src/client.rs:543-554`)
escapes everything outside the unreserved set, including `/`, and has a test asserting
`"a/b c" -> "a%2Fb%20c"` (`client.rs:607`). The two SDKs disagree; Rust is right.

**Remediation.** `urllib.parse.quote(correlation_id, safe="")` at all three sites.

---

## PY-006 — No scheme validation on `base_url`; plaintext HTTP silently accepted

- **Severity:** MEDIUM · **Confidence:** 82
- **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information), CWE-295
- **File:** `sdk/python/agezt/client.py:101`

```python
# sdk/python/agezt/client.py:101
self.base_url = base_url.rstrip("/")
```

That is the entirety of the validation. Consequences:

- **`http://` is accepted with no warning**, sending `Authorization: Bearer <admin token>`
  in cleartext. The class docstring's own example is `http://127.0.0.1:8800`
  (`client.py:88`), so plaintext is the documented default; nothing distinguishes a
  loopback URL (where that is fine) from a remote one (where it leaks the token).
  This is also the enabler for PY-002 — a passive network attacker becomes an active one
  by injecting a 302.
- Non-HTTP schemes reach `urllib.request`'s default opener, which handles `file://` and
  `ftp://`. `Client("file:///C:/Users/x/.agezt/", token).health()` reads from the local
  filesystem.
- No certificate pinning; `urlopen` uses `ssl.create_default_context()` for `https`, which
  *does* verify certs and hostnames correctly — no `verify=False`-equivalent exists
  anywhere in the SDK (see Clean, below).

**Why not a false positive.** No scheme check, allowlist, or opener restriction appears
anywhere in `sdk/python/`. Contrast the Rust SDK, which explicitly parses and rejects
non-`http` schemes (`sdk/rust/src/http.rs:25-37`).

**Remediation.** Validate in `__init__` and require TLS off-loopback:

```python
p = urllib.parse.urlparse(base_url)
if p.scheme not in ("http", "https"):
    raise ValueError(f"base_url scheme must be http or https, got {p.scheme!r}")
if p.scheme == "http" and p.hostname not in ("127.0.0.1", "::1", "localhost"):
    raise ValueError("refusing to send a bearer token in cleartext to a non-loopback host")
```

---

## PY-007 — `arc.py` tar extraction guard never inspects `linkname`

- **Severity:** LOW · **Confidence:** 90
- **CWE:** CWE-22 (Path Traversal), CWE-59 (Link Following)
- **File:** `plugins/builtinskills/archivetools/scripts/arc.py:60-63`, guard at `:35-39`

```python
# plugins/builtinskills/archivetools/scripts/arc.py:60-63
for m in t.getmembers():
    if not _within(dest, os.path.join(dest, m.name)):
        raise ValueError(f"unsafe path in archive (zip slip): {m.name}")
t.extractall(dest)
```

The guard checks `m.name` only, never `m.linkname`, and it runs *before* extraction — so
`os.path.realpath` cannot see a symlink that does not exist yet. A tar containing a
symlink member `link -> /abs/outside` followed by `link/escaped.txt` passes the guard
completely.

**Verified** by replaying `_within` and the loop verbatim against such a tar:

```
guard check member='link'             -> within=True
guard check member='link/escaped.txt' -> within=True
ALL MEMBERS PASSED arc.py GUARD
extractall() raised AbsoluteLinkError: 'link' is a link to an absolute path
ESCAPED FILE WRITTEN OUTSIDE dest/ : False
```

So **arc.py's own guard provides no protection here** — what stopped the escape was
CPython 3.14.6's `tarfile` extraction filter, not this code. Per PEP 706, the permissive
`fully_trusted` behaviour is the default on Python 3.9–3.13 (emitting only a
`DeprecationWarning`), and `arc.py` never passes `filter=`. I verified the block on
3.14.6 only; I did not have 3.9–3.13 available to test the unguarded case.

**Severity is LOW deliberately.** `arc.py` is a built-in *skill script* the agent invokes
on the daemon host, and per `architecture.md` §5.2 that agent already holds `shell` at L4
by default — so this crosses no privilege boundary that is not already open. It is a
defence-in-depth gap, not an escalation.

**Remediation.** Make the protection explicit rather than version-dependent:

```python
t.extractall(dest, filter="data")   # Python 3.12+; also reject m.linkname escapes
```

and extend `_within` to validate `m.linkname` (resolved against the member's own
directory) for `m.issym() or m.islnk()`.

---

## PY-008 — Chunked-encoding detection matches anywhere in the header block

- **Severity:** LOW · **Confidence:** 78
- **CWE:** CWE-444
- **File:** `sdk/python/agezt/agent.py:229`

```python
# sdk/python/agezt/agent.py:229
if "chunked" in headers.lower():
    body = self._decode_chunked(body)
```

`headers` is the whole raw header blob, not a parsed `Transfer-Encoding` value. Any
response carrying the substring `chunked` in *any* header — a `Location`, an echoed
value, an error string — flips the client into chunked decoding of a body that is not
chunked, corrupting it. `_decode_chunked` then calls `int(..., 16)` at `agent.py:252` on
whatever it finds, raising an uncaught `ValueError` that escapes as a non-`AgentError`
exception type the caller is not documented to expect.

Contrast the Rust client, which parses the header properly:
`key == "transfer-encoding" && val.eq_ignore_ascii_case("chunked")`
(`sdk/rust/src/http.rs:149`).

**Remediation.** Parse headers into a dict and test the `transfer-encoding` value only;
wrap `_decode_chunked`'s `int(...)` in a `try` that raises `AgentError("INVALID_RESPONSE", …)`.

---

## Python — checklist coverage

| Category (sc-lang-python) | Result |
|---|---|
| 1. Pickle deserialization RCE | **CLEAN** — no `pickle`/`joblib`/`torch.load`/`numpy.load` anywhere |
| 2. YAML unsafe loading | **CLEAN** — no `yaml` import in the tree |
| 3. `eval` / `exec` / `compile` | **CLEAN** — zero occurrences across all 27 `.py` files |
| 4. Subprocess shell injection | **CLEAN** — the only `subprocess` users are `scripts/dev/*.py`, all list-form argv, no `shell=True`, no `os.system`/`os.popen` |
| 5. Django ORM raw/extra | **N/A** — no Django |
| 6. Django misconfiguration | **N/A** |
| 7. Flask/Jinja2 SSTI | **N/A** — no Flask/Jinja2 |
| 8. Flask debug / secret key | **N/A** |
| 9. TLS bypass & SSRF | **PARTIAL** — no `verify=False` or custom `SSLContext` anywhere (clean); but no scheme validation → **PY-006** |
| 10. Weak password hashing | **N/A** — SDK stores/derives no passwords |
| 11. Insecure randomness | **CLEAN** — no `random`/`uuid1` in any security context |
| 12. Format-string injection | **PARTIAL** — f-strings into an HTTP request line → **PY-001** |
| 13. Dynamic import abuse | **CLEAN** — the only dynamic imports are fixed literals (`agent.py:467`, `:472`, `:514`) |
| 14. FastAPI DI bypass | **N/A** |
| 15. Pydantic validation escape | **N/A** — no Pydantic; responses are hand-mapped |
| 16. Packaging supply chain | **CLEAN** — `pyproject.toml` is fully declarative, `dependencies = []` (`:16`), no `setup.py`, no build hook / `cmdclass`, no `dependency_links`. `[tool.setuptools.packages.find] include = ["agezt*"]` (`:24`) correctly excludes `tests/` and `examples/` from the wheel |
| 17. `ast.literal_eval` misuse | **CLEAN** — not used; no eval fallback |
| 18. marshal / shelve | **CLEAN** — absent |
| 19. SQL injection | **N/A** — no `database/sql` equivalent in the SDK. `plugins/builtinskills/sqldb/scripts/db.py:63`, `:81` use SQLAlchemy `text(sql)` with bound `params`; the SQL string is agent-authored **by design** (it is a database tool), governed upstream by Edict, so not filed |
| 20. Path traversal via `os.path.join` | **PARTIAL** — → **PY-005** (URL segments), **PY-007** (tar) |
| 21. XXE | **CLEAN** — no XML parsing in the SDK |
| 22. Timing attacks on comparison | **CLEAN** — the SDK performs no secret comparison; it only presents a token |
| 23. Insecure temp files | **CLEAN** — no `tempfile.mktemp`. `plugins/builtinskills/computeruse/scripts/desktop.py:95` uses `tempfile.mkdtemp` (safe, 0700) |
| 24. ReDoS | **CLEAN** — no `re.compile` on caller-supplied patterns in the SDK |
| Token handling (URL / repr / logs) | **CLEAN except redirects** — the token never enters a URL query string, is never logged, and `Client`/`AgentClient` are plain classes (not dataclasses), so no auto-`__repr__` leaks it. `APIError` (`errors.py:20-25`) formats only status/type/message. → but **PY-002** |
| Network timeouts | **CLEAN** — every network call passes an explicit timeout: `client.py:138`, `:252`, `:287`; `agent.py:168`, `:571` |
| File permissions on written credentials | **N/A** — the SDK writes no files |

---

# RUST

## unsafe inventory

| Site | Invariant required | Upheld? |
|---|---|---|
| — | — | — |

**There is no `unsafe` in the crate.** `sdk/rust/src/lib.rs:46` declares
`#![forbid(unsafe_code)]`, which is compiler-enforced crate-wide (`forbid` cannot be
locally overridden by an inner `allow`). A grep for `unsafe`, `transmute`, `from_raw`,
`static mut`, `Box::leak`, and `impl Drop` across `sdk/rust/src/` returns exactly one hit
— the `forbid` attribute itself. The crate compiles clean, so the lint is satisfied.

This closes checklist categories 1, 2, 8, 9, 13, 14 and 18 by construction.

## Dependency posture — the "zero dependencies" claim is TRUE

Verified, as requested:

- `sdk/rust/Cargo.toml` — `[dependencies]` is present and **empty** (comment at the same
  point states the intent).
- `sdk/rust/Cargo.lock` — contains exactly **one** `[[package]]`: `agezt 1.0.0` itself.
- `cargo build --offline` succeeds with no network access, confirming nothing is fetched.

Consequence: there is **no third-party advisory surface**, no `build.rs`, and no proc
macro in the tree, so the absence of `cargo audit` (not installed) costs nothing here.

**How it does HTTP and TLS — the important half of the question.** It hand-rolls HTTP/1.1
over `std::net::TcpStream` (`sdk/rust/src/http.rs`) and hand-rolls a JSON
parser/serializer (`sdk/rust/src/json.rs`). It does **not** hand-roll TLS or any crypto —
it has none at all, and rejects `https://` outright (`http.rs:29-33`). That is the right
call versus a homegrown TLS stack, but it has a real consequence, filed as **RS-002**.

---

## RS-001 — Unbounded recursion in the JSON parser aborts the consumer's process

- **Severity:** HIGH · **Confidence:** 96
- **CWE:** CWE-674 (Uncontrolled Recursion), CWE-400
- **File:** `sdk/rust/src/json.rs:199-211` → `:222-250` / `:252-276` (mutual recursion)

`parse_value` dispatches to `parse_object` (`json.rs:202`) and `parse_array`
(`json.rs:203`), each of which calls `parse_value` again (`json.rs:235`, `:261`). There is
**no depth counter and no recursion limit** anywhere in the module. `serde_json`, which
this parser replaces, ships a 128-level default limit precisely for this reason.

**Verified empirically** by adding the published crate as a path dependency in the
scratchpad and calling the public `agezt::Value::parse`:

```
--> trying depth 100
    depth 100: ok=true
--> trying depth 1000

thread 'main' (27356) has overflowed its stack
error: process didn't exit successfully: (exit code: 0xc00000fd, STATUS_STACK_OVERFLOW)
```

A payload of ~1000 `[` characters is enough on the main thread's default stack.

**Exploitation path.** `Value::parse` is called on every response body the daemon returns:
`client.rs:373` (`read_json`, i.e. every unary call), `client.rs:471` (`make_event`, every
SSE frame), and `client.rs:521` (`api_error`, every **error** body — so even a 500 reaches
it). A malicious or compromised daemon returns roughly 2 KB of nested brackets and the
consumer's process **dies immediately**.

The severity driver is that this is not a catchable failure. A Rust stack overflow is not
a panic: it is `SIGSEGV`/`STATUS_STACK_OVERFLOW` and it **aborts the process**. It cannot
be caught by `catch_unwind`, it ignores `panic = "abort"` settings, and the `Result` this
API returns is never reached. A library that takes down its host application on hostile
input is the exact anti-pattern in checklist category 6, escalated — the caller has no
defence available to them at all.

**Why not a false positive.** `Value` and `Value::parse` are public API (`lib.rs:56`,
re-exported), the recursion is unconditional, and I triggered the abort through the
crate's own public surface. Depth 100 succeeds and depth 1000 aborts, so the threshold is
comfortably inside what a single small response can carry.

**Remediation.** Thread a depth budget through the parser and return `Err` instead of
recursing:

```rust
const MAX_DEPTH: u32 = 128;

fn parse_value(&mut self, depth: u32) -> Result<Value, String> {
    if depth > MAX_DEPTH {
        return Err(format!("maximum nesting depth {MAX_DEPTH} exceeded"));
    }
    // ... pass depth + 1 to parse_object / parse_array
}
```

Add a regression test asserting `Value::parse(&"[".repeat(10_000)).is_err()`.

---

## RS-002 — No TLS support at all: the bearer token is always sent in cleartext

- **Severity:** MEDIUM · **Confidence:** 90
- **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
- **File:** `sdk/rust/src/http.rs:25-37`, `:106-111`; `sdk/rust/src/client.rs:384`; doc at `lib.rs:41-45`

```rust
// sdk/rust/src/http.rs:29-33
if base_url.starts_with("https://") {
    return Err(invalid(
        "https is not supported (the std-only client has no TLS); \
         use http:// behind a TLS-terminating proxy",
    ));
}
```

```rust
// sdk/rust/src/client.rs:384
let auth = format!("Bearer {}", self.token);
```

Every request writes `Authorization: Bearer <token>` over a raw `TcpStream`. There is no
way for a consumer to opt into TLS — `https://` is a hard error, not a fallback.

**Assessment.** This is *honestly documented* in three places (`http.rs:7-9`,
`lib.rs:41-45`, `Cargo.toml` comment) and is a deliberate consequence of the
zero-dependency goal, so it is **not** filed as a comment-vs-code divergence — the docs
and the code agree. It is filed because the residual risk is real and belongs in the
threat model: a consumer who follows the documented "front it with a reverse proxy"
advice has a plaintext hop between the SDK and that proxy, and any consumer who points
this crate at a non-loopback daemon transmits an admin-equivalent token in the clear.
Combined with the absence of certificate validation (there is nothing to validate), a
network attacker both reads the token and can rewrite responses — which is the delivery
vector for **RS-001**.

**Remediation.** Either (a) put the TLS story in the crate-level docs as a *hard*
constraint — e.g. reject any `base_url` whose host is not loopback unless an explicit
`allow_cleartext_remote()` builder method is called — or (b) add an optional,
feature-gated `rustls` dependency so `https://` works, keeping the default build at zero
dependencies. Option (a) preserves the zero-dep promise and is the smaller change.

---

## RS-003 — Unbounded response body read

- **Severity:** MEDIUM · **Confidence:** 84
- **CWE:** CWE-400, CWE-789
- **File:** `sdk/rust/src/http.rs:73-77`, `:163`, `:203`; callers `client.rs:194`, `:330`, `:366`

```rust
// sdk/rust/src/http.rs:73-77
pub fn read_text(mut self) -> io::Result<String> {
    let mut s = String::new();
    self.body.read_to_string(&mut s)?;   // no cap
    Ok(s)
}
```

When the daemon sends neither `Content-Length` nor `Transfer-Encoding: chunked`, the
framing falls through to `BodyMode::Eof` (`http.rs:163`), whose `read` is a bare
passthrough (`http.rs:203`). `read_to_string` then grows until the peer closes — or
forever. `BodyMode::Chunked` is equally uncapped.

This is reachable on the **error** path as well as the success path:
`api_error(resp.status, &resp.read_text()?)` at `client.rs:194` and `:330`, and
`read_json` at `client.rs:366`. So a hostile daemon answering any request with an
unframed, endless body grows the consumer's allocation without bound.

**Why not a false positive.** `set_read_timeout` (`http.rs:95`) bounds how long a single
`read` may *block*, not how much total data may arrive; a steadily-streaming peer never
trips it. `BodyMode::Length` is bounded by the advertised `Content-Length` (`http.rs:204-211`)
— but the daemon chooses that value, and the two other modes have no bound at all.

**Remediation.** Cap the read and surface a typed error:

```rust
const MAX_BODY: u64 = 32 * 1024 * 1024;
pub fn read_text(self) -> io::Result<String> {
    let mut s = String::new();
    let mut limited = self.body.take(MAX_BODY + 1);
    limited.read_to_string(&mut s)?;
    if s.len() as u64 > MAX_BODY {
        return Err(invalid("response body exceeded 32 MiB"));
    }
    Ok(s)
}
```

---

## RS-004 — Header value injection via an unvalidated tenant id (and `base_url` host)

- **Severity:** LOW · **Confidence:** 75
- **CWE:** CWE-93 (CRLF Injection), CWE-113
- **File:** `sdk/rust/src/http.rs:109-111`, `:107`; source `client.rs:390`, `:131-134`

```rust
// sdk/rust/src/http.rs:109-111
for (k, v) in headers {
    write!(req, "{k}: {v}\r\n")?;
}
```

Header values are written verbatim with no control-character rejection. Two reach it from
caller-controlled data:

- `("X-Agezt-Tenant", t)` at `client.rs:390`, where `t` comes from
  `Client::with_tenant(impl Into<String>)` (`client.rs:131-134`) with no validation.
- `Host: {host_header}` at `http.rs:107`, derived from `base_url` via `Target::parse`,
  which validates the *port* (`http.rs:47-49`) but never the host for `\r`/`\n`.

A tenant id containing `\r\n` injects arbitrary headers or a smuggled request — the same
class as **PY-001**.

**Severity is LOW, not critical, deliberately.** Unlike the Python case, the injectable
values here are *configuration* (a tenant id, a base URL) that a consumer normally sets
from a constant, not per-call data derived from LLM output or board messages. The
Rust SDK's **path** construction, by contrast, is correct: every caller-supplied path
segment goes through `percent_encode` (`client.rs:204`, `:257`, `:275`, `:286`, `:301`,
`:322`), which escapes everything outside the unreserved set (`client.rs:543-554`), and
`limit` is a `u32` so it can only render digits. Path injection is genuinely closed here.

**Remediation.** Reject control characters in `Target::parse` and in `request`:

```rust
if headers.iter().any(|(k, v)| k.contains(['\r','\n']) || v.contains(['\r','\n'])) {
    return Err(invalid("control characters in header"));
}
```

---

## RS-005 — Saturating `f64 as i64` cast silently corrupts out-of-range numbers

- **Severity:** LOW · **Confidence:** 88
- **CWE:** CWE-681 (Incorrect Conversion Between Numeric Types), CWE-197
- **File:** `sdk/rust/src/json.rs:79`

```rust
// sdk/rust/src/json.rs:79
Value::Float(f) if f.fract() == 0.0 => Some(*f as i64),
```

Since Rust 1.45 a float→int `as` cast **saturates** rather than being UB, so a daemon
returning `"ts_unix_ms": 1e300` yields `i64::MAX` rather than an error. `as_i64` is used
for `Mail::ts_unix_ms` (`client.rs:505`), `Health::model_count` (`client.rs:145`), and
`RunArc::count` (`client.rs:213`) — the caller receives a plausible-looking number with no
signal that the value was out of range.

Not memory-unsafe (no UB), and `NaN`/`inf` are excluded because their `.fract()` is `NaN`,
which fails the `== 0.0` guard. Purely a silent data-integrity issue.

**Remediation.**

```rust
Value::Float(f) if f.fract() == 0.0 && *f >= i64::MIN as f64 && *f <= i64::MAX as f64
    => Some(*f as i64),
```

---

## RS-006 — Non-finite floats serialize to invalid JSON

- **Severity:** LOW · **Confidence:** 85
- **CWE:** CWE-20 (Improper Input Validation)
- **File:** `sdk/rust/src/json.rs:129`; parser side `json.rs:400-402`

```rust
// sdk/rust/src/json.rs:129
Value::Float(f) => write!(out, "{f}"),
```

`parse_number` accepts `1e999`, which `text.parse::<f64>()` (`json.rs:400`) turns into
`f64::INFINITY` without error. Re-serializing that `Value` emits the bare token `inf`
(or `NaN`), which is not valid JSON — breaking the round-trip property the crate's own
test asserts (`json.rs:444-451`) and producing a malformed request body if such a value is
ever echoed back to the daemon.

**Remediation.** Reject non-finite values at parse time (`return Err` when
`!n.is_finite()`), or emit `null` for them in `write_json`, matching `serde_json`'s
behaviour.

---

## Rust — checklist coverage

| Category (sc-lang-rust) | Result |
|---|---|
| 1. Unsafe block audit | **CLEAN** — `#![forbid(unsafe_code)]`, `lib.rs:46`; zero `unsafe` in the crate |
| 2. FFI boundary validation | **CLEAN** — no `extern`, no `c_char`, no FFI |
| 3. Command injection | **CLEAN** — no `std::process::Command` anywhere in the crate |
| 4. Path traversal | **CLEAN** — the crate touches no filesystem; no `PathBuf`/`fs::` |
| 5. Integer overflow | **PARTIAL** — arithmetic in `http.rs:210`, `:243` is safe (`*remaining -= n` where `n <= want <= remaining` by the `min` at `:208`/`:241`); the `as usize` casts are bounded by `buf.len()`. → **RS-005** for the float cast |
| 6. `panic!()` in library code | **CLEAN** — every `unwrap`/`expect`/`panic!` is inside `#[cfg(test)]` (`client.rs:585`, `:594`; `http.rs:272-300`; `json.rs:412-468`) or a doc example. Production code uses `unwrap_or`, `unwrap_or_default`, `unwrap_or_else`, `ok_or_else` and `?` throughout. No slice indexing that can panic: `json.rs:357`, `:369` use `starts_with` on a range-checked slice; `read_hex4` bounds-checks at `json.rs:346`; `http.rs:39` splits on an ASCII byte index. **But see RS-001 — stack overflow is not a panic and is not covered by this category** |
| 7. Rc/Arc reference cycles | **CLEAN** — no `Rc`/`Arc`/`RefCell` in the crate |
| 8. Send/Sync misuse | **CLEAN** — no `unsafe impl`; all auto-derived |
| 9. Interior mutability data race | **CLEAN** — no `Cell`/`RefCell`/`Mutex`; `Client` is `Clone` + plain data |
| 10. Deserialization bombs | **FAIL** → **RS-001** (depth), **RS-003** (size) |
| 11. actix-web/axum security | **N/A** — client only, no server |
| 12. Cargo supply chain | **CLEAN** — zero dependencies (verified in `Cargo.toml` + `Cargo.lock` + offline build), no `build.rs`, no proc macro, `Cargo.lock` committed |
| 13. Pin/Unpin unsoundness | **CLEAN** — no `Pin`, no async, no self-referential types |
| 14. MaybeUninit UB | **CLEAN** — absent |
| 15. Tokio cancellation safety | **N/A** — fully synchronous, no async runtime |
| 16. `.await` in Drop | **N/A** — no `impl Drop` at all |
| 17. Regex DoS | **CLEAN** — no regex crate, no pattern compilation |
| 18. Unsafe trait impls | **CLEAN** — the two `Iterator` impls (`client.rs:402`, `:455`) are safe trait impls with no `size_hint` override, so no `TrustedLen`-style unsoundness |
| 19. Memory leaks via `Box::leak` | **CLEAN** — absent |
| 20. Error-handling info disclosure | **CLEAN** — `api_error` (`client.rs:518-540`) maps server errors into a typed `Error::Api`; nothing is echoed back to a third party (client-side crate) |
| Token handling (URL / logs / redirects) | **CLEAN** — the token never enters a URL or a log; `#[derive(Debug)]` on `Client` (`client.rs:102`) *would* print it, but only if a consumer explicitly `{:?}`-formats the client, which is a consumer choice, not an SDK leak. **No redirect following at all** — `Connection: close`, one request per connection, 3xx is surfaced as a status code — so the Python redirect flaw (PY-002) has no Rust analogue |
| URL / path construction | **CLEAN** — `percent_encode` (`client.rs:543-554`) escapes everything outside the unreserved set, applied at all six caller-supplied-segment sites; scheme is validated and non-`http` rejected (`http.rs:25-37`) |
| Network timeouts | **CLEAN** — connect, read and write timeouts all set (`http.rs:94-96`), default 30 s (`client.rs:118`), tunable via `with_timeout` |

---

## Tooling output

Everything below is verbatim from commands actually executed. Nothing was installed; no
`pip install` or `cargo fetch` was run; no daemon was started.

### Availability probe (`command -v`)

| Tool | Result |
|---|---|
| `ruff` | **NOT FOUND** |
| `bandit` | **NOT FOUND** |
| `mypy` | **NOT FOUND** |
| `semgrep` | **NOT FOUND** |
| `cargo` | `/c/Users/ersin/.cargo/bin/cargo` — **present**, `cargo 1.93.0 (083ac5135 2025-12-15)` |
| `python` | `/c/Python314/python` — **present**, `3.14.6` |
| `pip` | present (deliberately unused) |

### `cargo clippy --version`

```
error: the 'cargo-clippy.exe' binary, normally provided by the 'clippy' component,
is not applicable to the 'stable-x86_64-pc-windows-msvc' toolchain
```

Clippy component not installed — **not run**. Rust findings below are all from manual
review plus the build/test runs.

### `cargo audit --version`

```
error: no such command: `audit`
```

Not installed — **not run**. Mitigated: the dependency graph is provably empty
(`Cargo.lock` contains only `agezt 1.0.0`), so there is no advisory surface to audit.

### `cargo build --offline` (CARGO_BUILD_JOBS=3)

```
    Finished `dev` profile [unoptimized + debuginfo] target(s) in 0.04s
```

Clean — and the fact that `--offline` succeeds independently confirms zero dependencies.

### `cargo test --offline` (CARGO_BUILD_JOBS=3)

```
test result: ok. 13 passed; 0 failed; 0 ignored   (unit tests, src/)
test result: ok. 13 passed; 0 failed; 0 ignored   (tests/client.rs, integration)
test result: ok.  3 passed; 0 failed; 0 ignored   (doc-tests)
```

29/29 pass. Notably the integration suite includes `mailbox_inbox_encodes_query` and
`tenant_header_is_transmitted`, i.e. path encoding is already regression-tested (which is
why RS-004 is limited to *header* values, not paths).

### Manual scans (no scanner available)

- `grep` for `eval|exec|compile|pickle|marshal|shelve|yaml.load|os.system|os.popen|shell=True|subprocess|__import__|importlib|verify=False|mktemp|input(` across all 27 tracked `.py` files → only list-form `subprocess.run` in `scripts/dev/*.py`; no other hit.
- `grep` for `unsafe|static mut|transmute|Box::leak|impl Drop|from_raw|unwrap()|expect(|panic!` across `sdk/rust/src/` → all hits accounted for in the tables above.

### Empirical proofs-of-concept (scratchpad only)

1. **RS-001** — added the crate as a path dependency and called `agezt::Value::parse` on
   nested arrays: depth 100 OK, **depth 1000 → `STATUS_STACK_OVERFLOW` (0xc00000fd)**,
   process aborted.
2. **PY-002** — called `urllib.request.HTTPRedirectHandler.redirect_request` directly on a
   `Request` built exactly as `client.py:269-271` builds it: the `Authorization` header
   **is** carried to a different host. No network involved.
3. **PY-001** — replayed `agent.py:173-191` + `agent.py:344` verbatim on a `\r\n`-bearing
   query and printed the bytes that would reach `sendall`: **two well-formed HTTP request
   lines**. Nothing was sent.
4. **PY-007** — replayed `arc.py:35-39` + `:58-64` against a symlink-escape tar: **the
   guard passed all members**; the escape was stopped only by CPython 3.14.6's `tarfile`
   filter (`AbsoluteLinkError`), not by `arc.py`.

---

## What came back clean — the short version

**Python.** No `eval`/`exec`/`compile`; no `pickle`/`marshal`/`shelve`; no YAML; no
`shell=True`, `os.system` or `os.popen`; no `verify=False` or custom `SSLContext`; no
insecure randomness; no `tempfile.mktemp`; no XXE surface; no dynamic import of
caller-controlled names; no secret comparison (so no timing-attack surface); explicit
timeouts on **every** network call; a fully declarative `pyproject.toml` with zero
dependencies, no `setup.py`, no build-time hook, and a package-find filter that keeps
`tests/` and `examples/` out of the wheel.

**Rust.** No `unsafe` at all (compiler-enforced via `forbid`); no FFI, no `Command`, no
filesystem access, no `Rc`/`RefCell`, no `unsafe impl Send/Sync`, no `static mut`, no
`transmute`, no `Box::leak`, no `impl Drop`, no async and therefore no cancellation or
`.await`-in-`Drop` hazards, no regex. Zero third-party crates — **verified**, and the
crate does *not* hand-roll TLS or crypto (it has none). No panic-capable code outside
`#[cfg(test)]`. Path/query construction is correctly percent-encoded and already
regression-tested. No redirect following, so the token cannot be replayed to another host.

**The pattern worth naming:** on the two questions the SDKs answer differently — URL
segment encoding and scheme validation — **Rust is right and Python is wrong** (PY-005,
PY-006). On the two where Python's stdlib does the work, Rust had to hand-roll it and
picked up the bugs (RS-001 depth, RS-003 size). Cross-porting each SDK's correct answer to
the other would close four of the eleven findings.
