"""Tests for the Agezt asyncio client, against the same stdlib http.server mock
used by the sync client tests. No third-party dependencies: run with
``python -m unittest`` from sdk/python.
"""

import threading
import unittest
from http.server import HTTPServer

import sys
from pathlib import Path

# Make the package importable when run from the repo without installation.
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from agezt import APIError, AsyncClient  # noqa: E402
from test_client import _Handler  # noqa: E402  (reuse the sync test's mock handler)


class AsyncClientTest(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.srv = HTTPServer(("127.0.0.1", 0), _Handler)
        self.srv.last_body = None
        self.t = threading.Thread(target=self.srv.serve_forever, daemon=True)
        self.t.start()
        port = self.srv.server_address[1]
        self.c = AsyncClient(f"http://127.0.0.1:{port}", token="testtoken", timeout=5)

    def tearDown(self):
        self.srv.shutdown()
        self.srv.server_close()

    async def test_health_and_models(self):
        h = await self.c.health()
        self.assertEqual(h["status"], "ok")
        self.assertEqual(h["version"], "test")
        m = await self.c.models()
        self.assertEqual(m["default"], "m")
        self.assertIn("n", m["models"])

    async def test_run_sync(self):
        r = await self.c.run("ping", model="m")
        self.assertEqual(r.status, "completed")
        self.assertEqual(r.answer, "pong")
        self.assertEqual(r.correlation_id, "c4")
        self.assertEqual(self.srv.last_body.get("model"), "m")

    async def test_run_failure_raises(self):
        with self.assertRaises(APIError) as cm:
            await self.c.run("boom")
        self.assertEqual(cm.exception.status, 502)
        self.assertIn("provider exploded", cm.exception.message)

    async def test_run_stream(self):
        events = [ev async for ev in self.c.run_stream("hi")]
        kinds = [e.event for e in events]
        self.assertEqual(kinds, ["start", "token", "token", "done"])
        tokens = "".join(e.data.get("text", "") for e in events if e.event == "token")
        self.assertEqual(tokens, "hello")
        self.assertEqual(events[-1].data.get("answer"), "hello")

    async def test_get_run(self):
        arc = await self.c.get_run("c1")
        self.assertEqual(arc["count"], 2)
        self.assertEqual(len(arc["events"]), 2)

    async def test_bad_token_raises_401(self):
        async with AsyncClient(self.c.base_url, token="WRONG", timeout=5) as bad:
            with self.assertRaises(APIError) as cm:
                await bad.health()
        self.assertEqual(cm.exception.status, 401)
        self.assertEqual(cm.exception.type, "unauthorized")

    async def test_context_manager_returns_client(self):
        async with AsyncClient(self.c.base_url, token="testtoken", timeout=5) as c:
            self.assertIsInstance(c, AsyncClient)
            self.assertEqual((await c.health())["status"], "ok")


if __name__ == "__main__":
    unittest.main()
