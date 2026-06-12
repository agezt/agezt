"""Tests for the mailbox surface of the Agezt Python client (M937).

Stdlib-only, like test_client.py: a tiny http.server stub answers the
/api/v1/mailbox routes with canned shapes; the tests assert the client sends
the right bodies/queries and maps the responses to Mail values.
"""

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from agezt import APIError, Client, Mail  # noqa: E402

_MSG = {
    "id": "m-1",
    "topic": "dm",
    "from": "myapp",
    "to": "researcher",
    "text": "deploy target?",
    "ts_unix_ms": 1700000000000,
}


class _Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def _json(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        u = urlparse(self.path)
        self.server.last_query = parse_qs(u.query)
        if u.path == "/api/v1/mailbox/inbox":
            self._json(200, {"name": "researcher", "waiting": [_MSG], "count": 1})
        elif u.path == "/api/v1/mailbox/messages/m-1/replies":
            rep = dict(_MSG, id="m-2", reply_to="m-1", to="myapp", **{"from": "researcher"})
            self._json(200, {"id": "m-1", "replies": [rep], "count": 1})
        elif u.path == "/api/v1/mailbox/messages":
            self._json(200, {"messages": [_MSG], "count": 1})
        elif u.path == "/api/v1/mailbox/topics":
            self._json(200, {"topics": {"dm": 2, "status": 1}})
        else:
            self._json(404, {"error": {"type": "not_found", "message": "nope"}})

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length) or b"{}")
        self.server.last_body = body
        self.server.last_path = self.path
        if self.path == "/api/v1/mailbox/messages":
            sent = dict(_MSG)
            sent.update({k: v for k, v in body.items() if k != "from"})
            if "from" in body:
                sent["from"] = body["from"]
            self._json(201, {"message": sent})
        elif self.path == "/api/v1/mailbox/messages/m-1/ack":
            self._json(200, {"acked": True, "id": "m-1", "by": body.get("by", "")})
        elif self.path.endswith("/ack"):
            self._json(404, {"error": {"type": "not_found", "message": "no message"}})
        else:
            self._json(404, {"error": {"type": "not_found", "message": "nope"}})


class MailboxTest(unittest.TestCase):
    def setUp(self):
        self.srv = HTTPServer(("127.0.0.1", 0), _Handler)
        self.srv.last_body = None
        self.srv.last_query = None
        self.srv.last_path = None
        self.t = threading.Thread(target=self.srv.serve_forever, daemon=True)
        self.t.start()
        port = self.srv.server_address[1]
        self.c = Client(f"http://127.0.0.1:{port}", token="testtoken", timeout=5)

    def tearDown(self):
        self.srv.shutdown()
        self.srv.server_close()

    def test_send_dm(self):
        m = self.c.mailbox_send("deploy target?", from_="myapp", to="researcher")
        self.assertIsInstance(m, Mail)
        self.assertEqual(m.id, "m-1")
        self.assertEqual(m.from_, "myapp")
        self.assertEqual(self.srv.last_body, {"text": "deploy target?", "from": "myapp", "to": "researcher"})

    def test_broadcast_and_help_flags(self):
        self.c.mailbox_broadcast("myapp", "heads-up")
        self.assertEqual(self.srv.last_body["to"], "*")
        self.c.mailbox_send("stuck", from_="w", help=True)
        self.assertTrue(self.srv.last_body["help"])

    def test_inbox(self):
        mails = self.c.mailbox_inbox("researcher", include_read=True, limit=5)
        self.assertEqual(len(mails), 1)
        self.assertEqual(mails[0].text, "deploy target?")
        self.assertEqual(self.srv.last_query, {"name": ["researcher"], "all": ["true"], "limit": ["5"]})

    def test_ack(self):
        self.c.mailbox_ack("m-1", "researcher")
        self.assertEqual(self.srv.last_path, "/api/v1/mailbox/messages/m-1/ack")
        self.assertEqual(self.srv.last_body, {"by": "researcher"})
        with self.assertRaises(APIError):
            self.c.mailbox_ack("nope", "researcher")

    def test_replies_messages_topics(self):
        reps = self.c.mailbox_replies("m-1")
        self.assertEqual(reps[0].reply_to, "m-1")
        msgs = self.c.mailbox_messages(topic="dm", limit=3)
        self.assertEqual(len(msgs), 1)
        self.assertEqual(self.srv.last_query, {"topic": ["dm"], "limit": ["3"]})
        self.assertEqual(self.c.mailbox_topics(), {"dm": 2, "status": 1})


if __name__ == "__main__":
    unittest.main()
