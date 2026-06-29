"""Agent SDK client for AI agent subprocess code to communicate with AGEZT.

This module provides an ``AgentClient`` that connects to the AGEZT Agent Gateway
via a Unix domain socket. The gateway accepts scoped JWT tokens that limit
what capabilities the agent subprocess can access.

Example usage::

    import os
    from agezt.agent import AgentClient

    # The token is passed via environment by the parent agent
    token = os.environ["AGEZT_AGENT_TOKEN"]
    client = AgentClient(token=token)

    # Remember a fact
    client.memory.write(type="fact", subject="test", content="This is a test")

    # Recall memories
    results = client.memory.search("test")
    for r in results:
        print(r["subject"], r["content"])

    # Publish an event
    client.eventbus.publish("my.event", {"key": "value"})

    # Subscribe to events
    for event in client.eventbus.subscribe("my.>"):
        print(event)

The client is thread-safe for basic use. For async usage, use ``AsyncAgentClient``.
"""

from __future__ import annotations

import json
import socket
import threading
from dataclasses import dataclass
from typing import Any, Dict, Iterator, List, Optional

__all__ = ["AgentClient", "AsyncAgentClient", "AgentError", "Capability"]

# Default socket path for the agent gateway
DEFAULT_SOCKET_PATH = "@agezt/agentgw.sock"


@dataclass
class Capability:
    """Represents a capability that can be granted to an agent subprocess."""

    # Eventbus capabilities
    EVENTBUS_PUBLISH = "eventbus.publish"
    EVENTBUS_SUBSCRIBE = "eventbus.subscribe"

    # Memory capabilities
    MEMORY_READ = "memory.read"
    MEMORY_WRITE = "memory.write"
    MEMORY_DELETE = "memory.delete"
    MEMORY_SEARCH = "memory.search"
    MEMORY_LIST = "memory.list"

    # Log capabilities
    LOG_READ = "log.read"
    LOG_WRITE = "log.write"

    # Agent capabilities
    AGENT_LIST = "agent.list"
    AGENT_QUERY = "agent.query"


class AgentError(Exception):
    """Raised when the agent gateway returns an error."""

    def __init__(self, code: str, message: str, status_code: int = 500) -> None:
        self.code = code
        self.message = message
        self.status_code = status_code
        super().__init__(f"[{code}] {message}")


class _SocketClient:
    """Low-level socket client for the agent gateway.
    
    Supports both Unix domain sockets and TCP sockets.
    Use socket_path in the form:
      - /path/to.sock for Unix domain socket
      - host:port for TCP socket (e.g., localhost:18765)
      - tcp://host:port for explicit TCP
    """

    def __init__(self, socket_path: str, timeout: float = 30.0) -> None:
        self.socket_path = socket_path
        self.timeout = timeout
        self._lock = threading.Lock()
        self._is_tcp = self._detect_tcp()

    def _detect_tcp(self) -> bool:
        """Detect if the socket path is a TCP address."""
        # If it starts with tcp:// it's TCP
        if self.socket_path.startswith("tcp://"):
            return True
        # If it looks like host:port (contains : but not at start), it's TCP
        if ":" in self.socket_path and not self.socket_path.startswith("/"):
            return True
        # Otherwise assume Unix socket
        return False

    def _create_socket(self) -> socket.socket:
        """Create the appropriate type of socket."""
        if self._is_tcp:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        else:
            # Try Unix socket, fall back to TCP on Windows
            try:
                sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            except (AttributeError, OSError):
                # AF_UNIX not available on Windows - use TCP loopback
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        return sock

    def _connect(self, sock: socket.socket) -> None:
        """Connect the socket to the gateway."""
        if self._is_tcp:
            if self.socket_path.startswith("tcp://"):
                addr = self.socket_path[6:]
            else:
                addr = self.socket_path
            host, port_str = addr.rsplit(":", 1)
            sock.connect((host, int(port_str)))
        else:
            sock.connect(self.socket_path)

    def _request(
        self,
        method: str,
        path: str,
        body: Optional[Dict[str, Any]] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Send a request and return the parsed JSON response."""
        with self._lock:
            sock = self._create_socket()
            sock.settimeout(self.timeout)
            try:
                self._connect(sock)
                try:
                    # Build HTTP request
                    req_lines = [f"{method} {path} HTTP/1.1"]
                    req_lines.append("Host: localhost")
                    req_lines.append("Accept: application/json")

                    if headers:
                        for k, v in headers.items():
                            req_lines.append(f"{k}: {v}")

                    if body is not None:
                        body_bytes = json.dumps(body).encode("utf-8")
                        req_lines.append(f"Content-Type: application/json")
                        req_lines.append(f"Content-Length: {len(body_bytes)}")
                    else:
                        body_bytes = b""

                    req_lines.append("")
                    req_lines.append("")

                    sock.sendall("\r\n".join(req_lines).encode("utf-8"))
                    if body_bytes:
                        sock.sendall(body_bytes)

                    # Read response
                    response = b""
                    while True:
                        chunk = sock.recv(4096)
                        if not chunk:
                            break
                        response += chunk

                    return self._parse_http_response(response)
                finally:
                    sock.close()
            except socket.timeout as e:
                raise AgentError("TIMEOUT", f"Request timed out after {self.timeout}s", 408) from e
            except OSError as e:
                raise AgentError("CONNECTION_ERROR", f"Cannot connect to gateway: {e}", 503) from e

    def _parse_http_response(self, response: bytes) -> Dict[str, Any]:
        """Parse HTTP response and return JSON body."""
        # Split headers and body
        parts = response.split(b"\r\n\r\n", 1)
        if len(parts) < 2:
            raise AgentError("INVALID_RESPONSE", "Invalid HTTP response from gateway", 500)

        headers = parts[0].decode("utf-8", errors="replace")
        body = parts[1]

        # Parse status line
        status_line = headers.split("\r\n")[0]
        try:
            status_code = int(status_line.split()[1])
        except (IndexError, ValueError):
            raise AgentError("INVALID_RESPONSE", f"Cannot parse status: {status_line}", 500)

        # Check for chunked encoding
        if "chunked" in headers.lower():
            body = self._decode_chunked(body)

        # Remove trailing \r\n
        body = body.rstrip(b"\r\n")

        if not body:
            return {}

        try:
            return json.loads(body.decode("utf-8"))
        except json.JSONDecodeError as e:
            raise AgentError("INVALID_RESPONSE", f"Cannot parse JSON: {e}", 500) from e

    def _decode_chunked(self, body: bytes) -> bytes:
        """Decode chunked transfer encoding."""
        result = b""
        pos = 0
        while pos < len(body):
            # Read chunk size line
            line_end = body.find(b"\r\n", pos)
            if line_end == -1:
                break
            chunk_size = int(body[pos:line_end].split(b";")[0], 16)
            if chunk_size == 0:
                break
            pos = line_end + 2
            result += body[pos : pos + chunk_size]
            pos += chunk_size + 2  # Skip \r\n after chunk
        return result


class _EventbusHandle:
    """Handle for eventbus operations."""

    def __init__(self, client: "_AgentClient") -> None:
        self._client = client

    def publish(self, event: str, payload: Optional[Dict[str, Any]] = None) -> None:
        """Publish an event to the bus.

        Args:
            event: The event type (subject) for routing.
            payload: Optional JSON-serializable payload.
        """
        body: Dict[str, Any] = {"event": event}
        if payload is not None:
            body["payload"] = payload

        self._client._post("/v1/eventbus/publish", body)

    def subscribe(self, pattern: str = ">") -> Iterator[Dict[str, Any]]:
        """Subscribe to events matching a pattern.

        Uses SSE (Server-Sent Events) for streaming.

        Args:
            pattern: Event pattern to match. Use ">" for all events.

        Yields:
            Dict: Each event as a dictionary.

        Example::

            for ev in client.eventbus.subscribe("my.>"):
                print(ev["subject"], ev["payload"])
        """
        return self._client._subscribe("/v1/eventbus/subscribe?pattern=" + pattern)


class _MemoryHandle:
    """Handle for memory operations."""

    def __init__(self, client: "_AgentClient") -> None:
        self._client = client

    def write(
        self,
        type: str = "fact",
        subject: str = "",
        content: str = "",
        tags: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Remember (write) a memory record.

        Args:
            type: Memory type (e.g., "fact", "idea", "todo", "learned").
            subject: Brief subject/title for the memory.
            content: The actual memory content.
            tags: Optional key-value tags.

        Returns:
            Dict: The created memory record with ID.
        """
        body: Dict[str, Any] = {
            "type": type,
            "subject": subject,
            "content": content,
        }
        if tags:
            body["tags"] = tags

        result = self._client._post("/v1/memory/write", body)
        return result.get("record", {})

    def search(self, query: str, limit: int = 20) -> List[Dict[str, Any]]:
        """Recall (search) memories matching a query.

        Args:
            query: Natural language search query.
            limit: Maximum number of results.

        Returns:
            List[Dict]: Matching memory records with relevance scores.
        """
        result = self._client._get(f"/v1/memory/search?q={query}&limit={limit}")
        return result.get("results", [])

    def delete(self, id: str) -> bool:
        """Forget (delete) a specific memory record.

        Args:
            id: The memory record ID to delete.

        Returns:
            bool: True if deleted, False if not found.
        """
        result = self._client._delete(f"/v1/memory/delete?id={id}")
        return result.get("deleted", False)


class _LogHandle:
    """Handle for logging operations."""

    def __init__(self, client: "_AgentClient") -> None:
        self._client = client

    def write(
        self, message: str, level: str = "info", meta: Optional[Dict[str, Any]] = None
    ) -> None:
        """Write a log entry.

        Args:
            message: The log message.
            level: Log level (debug, info, warn, error). Default: info.
            meta: Optional metadata dictionary.
        """
        body: Dict[str, Any] = {
            "level": level,
            "message": message,
        }
        if meta:
            body["meta"] = meta

        self._client._post("/v1/log/write", body)


class _AgentHandle:
    """Handle for agent operations."""

    def __init__(self, client: "_AgentClient") -> None:
        self._client = client

    def list(self) -> List[Dict[str, Any]]:
        """List all agents in the roster.

        Returns:
            List[Dict]: Agent profiles.
        """
        result = self._client._get("/v1/agent/list")
        return result.get("agents", [])

    def query(self, id: str) -> Dict[str, Any]:
        """Query a specific agent's status.

        Args:
            id: The agent ID or slug to query.

        Returns:
            Dict: Agent profile.
        """
        result = self._client._get(f"/v1/agent/query?id={id}")
        return result


class _ConfigHandle:
    """Access to the Config Center.

    Config values are rated based on their sensitivity:
    - ``public``: Endpoint URLs, version info (auto-allowed)
    - ``internal``: Application settings (auto-allowed)
    - ``restricted``: Semi-sensitive values (requires HITL approval)
    - ``secret``: API keys, tokens, passwords (always denied)

    Example::

        client = AgentClient(token=os.environ["AGEZT_AGENT_TOKEN"])

        # Public/internal values — automatic access
        endpoint = client.config.get("analytics:endpoint")
        version = client.config.get("app:version")

        # Secret values — always denied
        try:
            api_key = client.config.get("github:token")  # raises ConfigAccessError
        except ConfigAccessError as e:
            print(f"Access denied: {e.code}")

        # Restricted values — may require HITL approval
        try:
            aws_key = client.config.get(
                "aws:readonly_key",
                reason="Need AWS credentials to read S3 data"
            )
        except ConfigAccessError as e:
            print(f"Approval denied or timed out: {e.message}")
    """

    def __init__(self, client: "_AgentClient") -> None:
        self._client = client

    def get(self, key: str, reason: str = "") -> str:
        """Get a config value by key.

        Args:
            key: Config key (e.g., "github:endpoint", "analytics:api_key").
            reason: Why the agent needs this value. Shown to operator for
                HITL approval requests.

        Returns:
            str: The config value.

        Raises:
            ConfigAccessError: When access is denied. Possible codes:
                - ``ACCESS_DENIED``: Rating=secret or HITL denied/timeout
                - ``KEY_NOT_FOUND``: Config key does not exist
                - ``RATE_LIMITED``: Too many config requests
        """
        from agezt.errors import ConfigAccessError

        path = f"/v1/config/{key}"
        if reason:
            # URL-encode the reason
            import urllib.parse

            path += f"?{urllib.parse.urlencode({'reason': reason})}"

        try:
            result = self._client._get(path)
            return result.get("value", "")
        except Exception as e:
            # Re-raise as ConfigAccessError if it's an API error
            if isinstance(e, ConfigAccessError):
                raise
            # For other errors (network, etc.), wrap them
            raise ConfigAccessError(
                key=key,
                code="INTERNAL_ERROR",
                message=str(e),
                status=500,
            )

    def list_keys(self) -> List[str]:
        """List accessible config keys.

        Returns only keys with ``public`` or ``internal`` ratings.
        ``secret`` and ``restricted`` keys are excluded.

        Returns:
            List[str]: List of accessible config keys.
        """
        result = self._client._get("/v1/config")
        return result.get("keys", [])

    def search(self, query: str) -> List[Dict[str, Any]]:
        """Search config keys by prefix.

        Only ``public`` rating keys are searchable.

        Args:
            query: Search prefix (e.g., "github:", "analytics:").

        Returns:
            List[Dict]: Matching entries with ``key`` and ``description``.
        """
        import urllib.parse

        path = f"/v1/config/search?{urllib.parse.urlencode({'q': query})}"
        result = self._client._get(path)
        return result.get("results", [])


class _AgentClient:
    """Base class for agent client - handles the HTTP-over-Unix transport."""

    def __init__(
        self,
        token: str,
        socket_path: str = DEFAULT_SOCKET_PATH,
        timeout: float = 30.0,
    ) -> None:
        self.token = token
        self.socket_path = socket_path
        self.timeout = timeout
        self._sock = _SocketClient(socket_path, timeout)

        # Sub-handles
        self.eventbus = _EventbusHandle(self)
        self.memory = _MemoryHandle(self)
        self.log = _LogHandle(self)
        self.agent = _AgentHandle(self)
        self.config = _ConfigHandle(self)

    def _get(self, path: str) -> Dict[str, Any]:
        """Send GET request."""
        return self._sock._request(
            "GET",
            path,
            headers={"Authorization": f"Bearer {self.token}"},
        )

    def _post(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        """Send POST request."""
        return self._sock._request(
            "POST",
            path,
            body=body,
            headers={"Authorization": f"Bearer {self.token}"},
        )

    def _delete(self, path: str) -> Dict[str, Any]:
        """Send DELETE request."""
        return self._sock._request(
            "DELETE",
            path,
            headers={"Authorization": f"Bearer {self.token}"},
        )

    def _subscribe(self, path: str) -> Iterator[Dict[str, Any]]:
        """Subscribe to SSE stream."""
        # SSE subscription requires special handling
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(self.timeout)
        sock.connect(self.socket_path)

        try:
            # Build HTTP request for SSE
            req = (
                f"GET {path} HTTP/1.1\r\n"
                f"Host: localhost\r\n"
                f"Accept: text/event-stream\r\n"
                f"Authorization: Bearer {self.token}\r\n"
                f"Cache-Control: no-cache\r\n"
                f"\r\n"
            )
            sock.sendall(req.encode("utf-8"))

            # Read SSE stream
            buffer = b""
            while True:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                buffer += chunk

                # Process SSE events
                while b"\r\n\r\n" in buffer:
                    header_end = buffer.index(b"\r\n\r\n")
                    headers = buffer[:header_end].decode("utf-8", errors="replace")
                    buffer = buffer[header_end + 4 :]

                    # Look for SSE data
                    for line in headers.split("\r\n"):
                        if line.startswith("data:"):
                            data = line[5:].strip()
                            if data:
                                try:
                                    yield json.loads(data)
                                except json.JSONDecodeError:
                                    pass
        finally:
            sock.close()


class AgentClient(_AgentClient):
    """Synchronous client for AI agent subprocess code to access AGEZT.

    Connects to the AGEZT Agent Gateway via a Unix domain socket and authenticates
    using a scoped JWT token. The token is typically provided by the parent agent
    via environment variable ``AGEZT_AGENT_TOKEN``.

    Args:
        token: JWT capability token from the parent agent.
        socket_path: Path to the agent gateway Unix socket.
        timeout: Request timeout in seconds.

    Example::

        import os
        from agezt import AgentClient

        client = AgentClient(token=os.environ["AGEZT_AGENT_TOKEN"])

        # Remember
        client.memory.write(type="fact", subject="API", content="Use POST /v1/memory/write")

        # Search
        results = client.memory.search("API")
        for r in results:
            print(r["subject"], r["content"])

        # Publish event
        client.eventbus.publish("agent.progress", {"step": 1, "total": 3})
    """

    pass


# Note: AsyncAgentClient would require aiohttp or similar for proper async socket support.
# For now, the sync AgentClient works well with threading in async frameworks like asyncio.
# A future version can add async support when needed.
