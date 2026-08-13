"""Regression tests for the agent-gateway transport (SDK-002).

These assert the CALL SITE, deliberately. The reason the bug shipped twice is
that nothing here ever imported ``agezt.agent`` and the TypeScript suite tested
``resolveSocketPath`` as a pure function — so both suites stayed green while
``_subscribe`` connected to the raw, unresolved default and handed the
capability token to whoever was listening at ``./@agezt/agentgw.sock``. Testing
the helper proves nothing; assert what each connect site actually passes.

No third-party dependencies and no socket is ever opened: the ``socket`` module
the agent client sees is replaced with a recorder. Run with
``python -m unittest discover -s tests`` from sdk/python.
"""

import socket as _real_socket
import sys
import unittest
from pathlib import Path
from unittest import mock

# Make the package importable when run from the repo without installation.
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from agezt.agent import DEFAULT_SOCKET_PATH, AgentClient  # noqa: E402

# What _resolve_socket_path must produce for the shipped default on Linux: the
# abstract-namespace address, which no filesystem entry can shadow.
ABSTRACT_DEFAULT = b"\0agezt/agentgw.sock"

TOKEN = "CAPABILITY-TOKEN"


class _RecordingSocket:
    """Stands in for a connected socket and records what it was asked to do."""

    def __init__(self, log, family, type_):
        self._log = log
        self.family = family
        self.type = type_
        self._chunks = [b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}"]

    def settimeout(self, t):
        self._log["timeouts"].append(t)

    def connect(self, address):
        self._log["connected"].append(address)

    def sendall(self, data):
        self._log["sent"].append(data)

    def recv(self, _n):
        return self._chunks.pop(0) if self._chunks else b""

    def close(self):
        self._log["closed"] += 1


class _RecordingSocketModule:
    """A stand-in for the ``socket`` module as ``agezt.agent`` sees it.

    AF_UNIX is defined here even on Windows on purpose: without it the buggy
    code raises AttributeError and the test would fail for an incidental
    platform reason instead of on the address it leaked.
    """

    AF_UNIX = "AF_UNIX"
    AF_INET = "AF_INET"
    SOCK_STREAM = "SOCK_STREAM"
    timeout = _real_socket.timeout
    error = OSError

    def __init__(self):
        self.log = {"connected": [], "sent": [], "timeouts": [], "closed": 0}

    def socket(self, family, type_):
        return _RecordingSocket(self.log, family, type_)


class AgentSocketPathTest(unittest.TestCase):
    """Every connect site must go through _resolve_socket_path."""

    def setUp(self):
        self.fake = _RecordingSocketModule()
        # Force the Linux branch of _resolve_socket_path: off Linux it is the
        # identity, so an assertion made there cannot tell the fix from the bug.
        patches = [
            mock.patch("agezt.agent.socket", self.fake),
            mock.patch("agezt.agent.sys.platform", "linux"),
        ]
        for p in patches:
            p.start()
            self.addCleanup(p.stop)
        self.client = AgentClient(token=TOKEN)

    def _sent(self):
        return b"".join(self.fake.log["sent"])

    def test_subscribe_connects_to_the_resolved_address(self):
        # subscribe() is a generator; the documented usage iterates it, which is
        # what opens the connection.
        list(self.client.eventbus.subscribe("x.>"))

        self.assertEqual(self.fake.log["connected"], [ABSTRACT_DEFAULT])
        connected = self.fake.log["connected"][0]
        self.assertNotEqual(
            connected,
            DEFAULT_SOCKET_PATH,
            "connected to the raw '@…' path — on Linux that is a CWD-relative "
            "FILE an attacker can plant a listener at (SDK-002)",
        )
        self.assertIsInstance(connected, bytes)
        self.assertTrue(connected.startswith(b"\0"))

    def test_subscribe_sends_the_capability_token(self):
        # The assertion above is only meaningful because this request carries
        # the credential the wrong address would have leaked.
        list(self.client.eventbus.subscribe("x.>"))
        self.assertIn(b"Authorization: Bearer " + TOKEN.encode(), self._sent())

    def test_unary_requests_connect_to_the_resolved_address(self):
        self.client.memory.write(type="fact", subject="s", content="c")
        self.assertEqual(self.fake.log["connected"], [ABSTRACT_DEFAULT])

    def test_socket_path_has_one_owner(self):
        # The structural cause: _AgentClient used to hold its own copy of the
        # address next to the transport's, and _subscribe connected to the copy.
        # Read-only, and read straight from the transport.
        self.assertIs(self.client.socket_path, self.client._sock.socket_path)
        with self.assertRaises(AttributeError):
            self.client.socket_path = "/tmp/planted.sock"

    def test_a_custom_path_is_honoured_and_still_resolved(self):
        client = AgentClient(token=TOKEN, socket_path="/run/agezt/gw.sock")
        list(client.eventbus.subscribe(">"))
        # No leading "@", so the resolver leaves it alone — but it still went
        # through the resolver rather than around it.
        self.assertEqual(self.fake.log["connected"], ["/run/agezt/gw.sock"])


if __name__ == "__main__":
    unittest.main()
