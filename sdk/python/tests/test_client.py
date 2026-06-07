"""Tests for the Agezt Python client, against a stdlib http.server mock.

No third-party dependencies: run with ``python -m unittest`` from sdk/python.
"""

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

import sys
from pathlib import Path

# Make the package importable when run from the repo without installation.
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from agezt import APIError, Client  # noqa: E402


class _Handler(BaseHTTPRequestHandler):
    # Set per-test on the server instance.
    def log_message(self, *args):  # silence test noise
        pass

    def _auth_ok(self):
        return self.headers.get("Authorization") == "Bearer testtoken"

    def _json(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if not self._auth_ok():
            self._json(401, {"error": {"type": "unauthorized", "message": "missing or invalid token"}})
            return
        if self.path == "/api/v1/health":
            self._json(200, {"status": "ok", "version": "test", "default_model": "m", "model_count": 1})
        elif self.path == "/api/v1/models":
            self._json(200, {"default": "m", "models": ["m", "n"]})
        elif self.path.startswith("/api/v1/runs/"):
            self._json(200, {"correlation_id": "c1", "count": 2, "events": [{"seq": 1}, {"seq": 2}]})
        else:
            self._json(404, {"error": {"type": "not_found", "message": "nope"}})

    def do_POST(self):
        if not self._auth_ok():
            self._json(401, {"error": {"type": "unauthorized", "message": "bad token"}})
            return
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length) or b"{}")
        self.server.last_body = body
        if body.get("intent") == "boom":
            self._json(502, {"correlation_id": "c2", "model": "m", "status": "failed", "error": "provider exploded"})
            return
        if body.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for frame in (
                'event: start\ndata: {"correlation_id": "c3", "model": "m"}\n\n',
                'event: token\ndata: {"text": "hel"}\n\n',
                ": heartbeat\n\n",
                'event: token\ndata: {"text": "lo"}\n\n',
                'event: done\ndata: {"correlation_id": "c3", "status": "completed", "answer": "hello"}\n\n',
            ):
                self.wfile.write(frame.encode())
            return
        self._json(200, {"correlation_id": "c4", "model": body.get("model", "m"), "status": "completed", "answer": "pong"})


class ClientTest(unittest.TestCase):
    def setUp(self):
        self.srv = HTTPServer(("127.0.0.1", 0), _Handler)
        self.srv.last_body = None
        self.t = threading.Thread(target=self.srv.serve_forever, daemon=True)
        self.t.start()
        port = self.srv.server_address[1]
        self.c = Client(f"http://127.0.0.1:{port}", token="testtoken", timeout=5)

    def tearDown(self):
        self.srv.shutdown()
        self.srv.server_close()

    def test_health(self):
        h = self.c.health()
        self.assertEqual(h["status"], "ok")
        self.assertEqual(h["version"], "test")

    def test_models(self):
        m = self.c.models()
        self.assertEqual(m["default"], "m")
        self.assertIn("n", m["models"])

    def test_run_sync(self):
        r = self.c.run("ping", model="m")
        self.assertEqual(r.status, "completed")
        self.assertEqual(r.answer, "pong")
        self.assertEqual(r.correlation_id, "c4")
        # the model option is forwarded
        self.assertEqual(self.srv.last_body.get("model"), "m")

    def test_run_failure_raises(self):
        with self.assertRaises(APIError) as cm:
            self.c.run("boom")
        self.assertEqual(cm.exception.status, 502)
        self.assertIn("provider exploded", cm.exception.message)

    def test_run_stream(self):
        events = list(self.c.run_stream("hi"))
        kinds = [e.event for e in events]
        self.assertEqual(kinds, ["start", "token", "token", "done"])
        tokens = "".join(e.data.get("text", "") for e in events if e.event == "token")
        self.assertEqual(tokens, "hello")
        self.assertEqual(events[-1].data.get("answer"), "hello")

    def test_get_run(self):
        arc = self.c.get_run("c1")
        self.assertEqual(arc["count"], 2)
        self.assertEqual(len(arc["events"]), 2)

    def test_bad_token_raises_401(self):
        bad = Client(self.c.base_url, token="WRONG", timeout=5)
        with self.assertRaises(APIError) as cm:
            bad.health()
        self.assertEqual(cm.exception.status, 401)
        self.assertEqual(cm.exception.type, "unauthorized")


if __name__ == "__main__":
    unittest.main()
