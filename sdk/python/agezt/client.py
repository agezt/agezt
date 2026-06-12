"""HTTP client for the Agezt REST API (``/api/v1``). Standard library only."""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Dict, Iterator, List, Optional

from .errors import APIError

__all__ = ["Client", "Mail", "RunResult", "StreamEvent"]


@dataclass
class RunResult:
    """A completed (non-streaming) run."""

    correlation_id: str
    model: str
    status: str
    answer: str

    @classmethod
    def _from(cls, d: Dict[str, Any]) -> "RunResult":
        return cls(
            correlation_id=d.get("correlation_id", ""),
            model=d.get("model", ""),
            status=d.get("status", ""),
            answer=d.get("answer", ""),
        )


@dataclass
class Mail:
    """One message on the daemon's shared mailbox (the inter-agent board).

    Agents and SDK apps leave messages for each other by name (``to``),
    broadcast to every inbox (``to="*"``), or post under a ``topic``;
    ``reply_to`` threads an answer back to the message it answers.
    """

    id: str = ""
    topic: str = ""
    text: str = ""
    from_: str = ""
    to: str = ""
    reply_to: str = ""
    help: bool = False
    ts_unix_ms: int = 0

    @classmethod
    def _from(cls, d: Dict[str, Any]) -> "Mail":
        return cls(
            id=d.get("id", ""),
            topic=d.get("topic", ""),
            text=d.get("text", ""),
            from_=d.get("from", ""),
            to=d.get("to", ""),
            reply_to=d.get("reply_to", ""),
            help=bool(d.get("help", False)),
            ts_unix_ms=int(d.get("ts_unix_ms", 0)),
        )


def _mails(raw: Any) -> List[Mail]:
    return [Mail._from(m) for m in raw or [] if isinstance(m, dict)]


@dataclass
class StreamEvent:
    """One Server-Sent Event from a streaming run.

    ``event`` is one of ``start``, ``token``, ``done``, ``error``; ``data`` is
    the decoded JSON payload (e.g. ``{"text": "…"}`` for ``token``).
    """

    event: str
    data: Dict[str, Any] = field(default_factory=dict)


class Client:
    """A client for a running Agezt daemon's REST API.

    Args:
        base_url: the daemon's REST address, e.g. ``http://127.0.0.1:8800``.
        token: the bearer token (the daemon's admin token or a tenant token).
        timeout: per-request timeout in seconds.
        tenant: optional tenant id, sent as the ``X-Agezt-Tenant`` header.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        timeout: float = 30.0,
        tenant: Optional[str] = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self.tenant = tenant

    # --- public API -------------------------------------------------------

    def health(self) -> Dict[str, Any]:
        """Return the daemon's liveness + version summary."""
        return self._get("/api/v1/health")

    def models(self) -> Dict[str, Any]:
        """Return ``{"default": id, "models": [ids…]}``."""
        return self._get("/api/v1/models")

    def run(self, intent: str, model: Optional[str] = None) -> RunResult:
        """Execute an intent and return the final answer (blocking).

        Raises APIError if the run fails or the request is rejected.
        """
        body: Dict[str, Any] = {"intent": intent}
        if model:
            body["model"] = model
        return RunResult._from(self._post_json("/api/v1/runs", body))

    def run_stream(
        self, intent: str, model: Optional[str] = None
    ) -> Iterator[StreamEvent]:
        """Execute an intent, yielding StreamEvents (start/token/done/error) as
        the agent produces them.
        """
        body: Dict[str, Any] = {"intent": intent, "stream": True}
        if model:
            body["model"] = model
        data = json.dumps(body).encode("utf-8")
        req = self._request("POST", "/api/v1/runs", data=data, accept="text/event-stream")
        try:
            resp = urllib.request.urlopen(req, timeout=self.timeout)
        except urllib.error.HTTPError as e:
            raise self._api_error(e) from None
        with resp:
            yield from _parse_sse(resp)

    def get_run(self, correlation_id: str) -> Dict[str, Any]:
        """Return the journaled event arc of a past run."""
        return self._get("/api/v1/runs/" + urllib.parse.quote(correlation_id))

    # --- mailbox (the shared inter-agent message board) --------------------

    def mailbox_send(
        self,
        text: str,
        *,
        from_: str = "",
        to: str = "",
        topic: str = "",
        reply_to: str = "",
        help: bool = False,
    ) -> Mail:
        """Leave a message on the shared mailbox.

        Addressing: ``to`` names a recipient agent/app (topic defaults to
        ``dm``); ``to="*"`` broadcasts to every inbox; ``to`` empty with a
        ``topic`` is a plain post; ``reply_to`` answers a message (it goes back
        to the original sender); ``help=True`` raises an assistance request.
        A directed message wakes a standing order watching ``board.dm.<name>``.
        """
        body: Dict[str, Any] = {"text": text}
        if from_:
            body["from"] = from_
        if to:
            body["to"] = to
        if topic:
            body["topic"] = topic
        if reply_to:
            body["reply_to"] = reply_to
        if help:
            body["help"] = True
        res = self._post_json("/api/v1/mailbox/messages", body)
        return Mail._from(res.get("message", {}))

    def mailbox_broadcast(self, from_: str, text: str) -> Mail:
        """Send an announcement to EVERY inbox except the sender's."""
        return self.mailbox_send(text, from_=from_, to="*")

    def mailbox_inbox(
        self, name: str, include_read: bool = False, limit: int = 0
    ) -> List[Mail]:
        """Return what waits for ``name``, newest first: messages addressed to
        it plus broadcasts it didn't send. Answered/acked messages are dropped
        unless ``include_read``."""
        q = {"name": name}
        if include_read:
            q["all"] = "true"
        if limit > 0:
            q["limit"] = str(limit)
        res = self._get("/api/v1/mailbox/inbox?" + urllib.parse.urlencode(q))
        return _mails(res.get("waiting"))

    def mailbox_ack(self, message_id: str, by: str) -> None:
        """Mark a message read for one reader (it leaves that reader's inbox
        without a reply). Per-reader and idempotent."""
        self._post_json(
            "/api/v1/mailbox/messages/" + urllib.parse.quote(message_id) + "/ack",
            {"by": by},
        )

    def mailbox_replies(self, message_id: str, limit: int = 0) -> List[Mail]:
        """Return the answers to a sent message, oldest first."""
        path = "/api/v1/mailbox/messages/" + urllib.parse.quote(message_id) + "/replies"
        if limit > 0:
            path += "?limit=" + str(limit)
        return _mails(self._get(path).get("replies"))

    def mailbox_messages(self, topic: str = "", limit: int = 0) -> List[Mail]:
        """Return recent mailbox messages, newest first, optionally filtered to
        one topic."""
        q: Dict[str, str] = {}
        if topic:
            q["topic"] = topic
        if limit > 0:
            q["limit"] = str(limit)
        path = "/api/v1/mailbox/messages"
        if q:
            path += "?" + urllib.parse.urlencode(q)
        return _mails(self._get(path).get("messages"))

    def mailbox_topics(self) -> Dict[str, int]:
        """Return the mailbox's topics with their message counts."""
        return self._get("/api/v1/mailbox/topics").get("topics", {})

    def mailbox_watch(self, name: str = "", topic: str = "") -> Iterator[Mail]:
        """Stream new mail the moment it lands — the push counterpart of
        polling :meth:`mailbox_inbox`.

        ``name`` watches one agent/app's mail (messages addressed to it plus
        broadcasts it didn't send); ``topic`` watches one topic; neither tails
        every board message. Blocks on the SSE stream until the server closes
        it (or the caller abandons the iterator). The server's first frame is
        a ``ready`` marker — messages sent after it are guaranteed delivered.
        """
        q: Dict[str, str] = {}
        if name:
            q["name"] = name
        if topic:
            q["topic"] = topic
        path = "/api/v1/mailbox/watch"
        if q:
            path += "?" + urllib.parse.urlencode(q)
        req = self._request("GET", path, accept="text/event-stream")
        try:
            resp = urllib.request.urlopen(req, timeout=self.timeout)
        except urllib.error.HTTPError as e:
            raise self._api_error(e) from None
        with resp:
            for ev in _parse_sse(resp):
                if ev.event == "mail":
                    yield Mail._from(ev.data)

    # --- internals --------------------------------------------------------

    def _request(
        self,
        method: str,
        path: str,
        data: Optional[bytes] = None,
        accept: str = "application/json",
    ) -> urllib.request.Request:
        req = urllib.request.Request(self.base_url + path, data=data, method=method)
        req.add_header("Authorization", "Bearer " + self.token)
        req.add_header("Accept", accept)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        if self.tenant:
            req.add_header("X-Agezt-Tenant", self.tenant)
        return req

    def _get(self, path: str) -> Dict[str, Any]:
        return self._do(self._request("GET", path))

    def _post_json(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        data = json.dumps(body).encode("utf-8")
        return self._do(self._request("POST", path, data=data))

    def _do(self, req: urllib.request.Request) -> Dict[str, Any]:
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
        except urllib.error.HTTPError as e:
            raise self._api_error(e) from None
        if not raw:
            return {}
        return json.loads(raw.decode("utf-8"))

    @staticmethod
    def _api_error(e: urllib.error.HTTPError) -> APIError:
        typ, msg = "", ""
        try:
            body = json.loads(e.read().decode("utf-8"))
            err = body.get("error")
            if isinstance(err, dict):  # {"error": {"type", "message"}}
                typ = err.get("type", "")
                msg = err.get("message", "")
            elif isinstance(err, str):  # failed-run body: {"status":"failed","error": "…"}
                typ = body.get("status", "")
                msg = err
        except Exception:
            pass
        return APIError(e.code, typ, msg)


def _parse_sse(stream) -> Iterator[StreamEvent]:
    """Parse a text/event-stream into StreamEvents. Each event is the lines up to
    a blank line: ``event:`` names it, ``data:`` (possibly multi-line) is its
    JSON payload."""
    event = "message"
    data_lines: List[str] = []
    for raw in stream:
        line = raw.decode("utf-8").rstrip("\n").rstrip("\r")
        if line == "":
            if data_lines:
                payload: Dict[str, Any] = {}
                joined = "\n".join(data_lines)
                try:
                    payload = json.loads(joined)
                except ValueError:
                    payload = {"raw": joined}
                yield StreamEvent(event=event, data=payload)
            event = "message"
            data_lines = []
            continue
        if line.startswith(":"):  # SSE comment / heartbeat
            continue
        if line.startswith("event:"):
            event = line[len("event:"):].strip()
        elif line.startswith("data:"):
            data_lines.append(line[len("data:"):].lstrip())
    # Flush a trailing event with no terminating blank line.
    if data_lines:
        joined = "\n".join(data_lines)
        try:
            yield StreamEvent(event=event, data=json.loads(joined))
        except ValueError:
            yield StreamEvent(event=event, data={"raw": joined})
