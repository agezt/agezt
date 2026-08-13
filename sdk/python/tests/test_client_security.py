"""Transport-security regression tests for agezt.Client (PY-002, PY-005, PY-006).

Everything here is hermetic: the only servers are stdlib HTTPServers bound to
127.0.0.1 on an ephemeral port, and nothing leaves the machine. Run with
``python -m unittest discover -s tests`` from sdk/python.
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

TOKEN = "SECRET-DAEMON-TOKEN"


def _serve(handler_cls):
    """Start a throwaway loopback HTTP server; returns (server, port)."""
    srv = HTTPServer(("127.0.0.1", 0), handler_cls)
    srv.seen = []
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, srv.server_address[1]


class _Collector(BaseHTTPRequestHandler):
    """The redirect TARGET. Records anything it is handed, answers 200."""

    def log_message(self, *a):
        pass

    def do_GET(self):
        self.server.seen.append((self.path, dict(self.headers)))
        body = b'{"stolen": true}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class RedirectTokenTest(unittest.TestCase):
    """PY-002: the bearer token must not survive a redirect off our origin.

    urllib's default HTTPRedirectHandler copies every header except
    Content-Length/Content-Type onto the redirected request with no
    same-origin check, so `add_header("Authorization", …)` hands full
    agent-level control of the daemon to whatever host answers a 302.
    """

    def setUp(self):
        self.collector, collector_port = _serve(_Collector)
        self.addCleanup(self.collector.server_close)
        self.addCleanup(self.collector.shutdown)
        target = "http://127.0.0.1:%d/collect" % collector_port

        class _Redirector(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def _redirect(self):
                length = int(self.headers.get("Content-Length", "0"))
                if length:
                    self.rfile.read(length)
                self.server.seen.append((self.path, dict(self.headers)))
                self.send_response(302)
                self.send_header("Location", target)
                self.send_header("Content-Length", "0")
                self.end_headers()

            do_GET = _redirect
            do_POST = _redirect

        self.daemon, daemon_port = _serve(_Redirector)
        self.addCleanup(self.daemon.server_close)
        self.addCleanup(self.daemon.shutdown)
        self.client = Client(
            "http://127.0.0.1:%d" % daemon_port, token=TOKEN, timeout=5
        )

    def test_cross_origin_redirect_is_refused_and_leaks_nothing(self):
        with self.assertRaises(APIError) as cm:
            self.client.health()
        self.assertEqual(cm.exception.type, "cross_origin_redirect")

        # The point of the test: the redirect target was never contacted, so it
        # never saw the token.
        self.assertEqual(self.collector.seen, [])

        # And the original request did carry the token, so the assertion above
        # is about the redirect and not about an unauthenticated client.
        self.assertEqual(len(self.daemon.seen), 1)
        self.assertEqual(self.daemon.seen[0][1].get("Authorization"), "Bearer " + TOKEN)

    def test_a_302_to_our_own_origin_still_carries_no_token(self):
        # Belt and braces: even a redirect the guard allows must not re-send the
        # credential, because a daemon that can choose the Location can choose a
        # path too. The daemon's own API never redirects.
        req = self.client._request("GET", "/api/v1/health")
        redirected = self.client._opener.handlers  # opener is the only door
        self.assertTrue(
            any(type(h).__name__ == "_SameOriginRedirectHandler" for h in redirected)
        )
        self.assertNotIn("Authorization", req.headers)

    def test_authorization_is_attached_unredirected(self):
        # The call-site assertion, independent of any server: urllib only
        # copies req.headers onto a redirect, never req.unredirected_hdrs.
        req = self.client._request("GET", "/api/v1/health")
        self.assertEqual(
            req.unredirected_hdrs.get("Authorization"), "Bearer " + TOKEN
        )
        self.assertNotIn("Authorization", req.headers)

    def test_every_urlopen_site_goes_through_the_guarded_opener(self):
        # run_stream and mailbox_watch open their own connections; if either
        # reached for the module-level urlopen it would use the default opener,
        # whose redirect handler has no same-origin check at all.
        for call in (
            lambda: list(self.client.run_stream("hi")),
            lambda: list(self.client.mailbox_watch(name="x")),
        ):
            with self.assertRaises(APIError) as cm:
                call()
            self.assertEqual(cm.exception.type, "cross_origin_redirect")
        self.assertEqual(self.collector.seen, [])


class SchemeValidationTest(unittest.TestCase):
    """PY-006: non-HTTP schemes reach urllib's default opener, which handles
    file:// and ftp:// locally."""

    def test_non_http_schemes_are_rejected(self):
        for url in ("file:///C:/Users/x/.agezt/", "ftp://example.invalid/", "/api", ""):
            with self.assertRaises(ValueError):
                Client(url, token=TOKEN)

    def test_http_and_https_are_accepted(self):
        for url in ("http://127.0.0.1:8800", "https://agezt.example.invalid"):
            self.assertTrue(Client(url, token=TOKEN).base_url)


class PathEncodingTest(unittest.TestCase):
    """PY-005: quote()'s default safe="/" lets a caller-supplied id escape its
    path segment and rewrite which endpoint is called. Mailbox ids come off the
    shared inter-agent board, the SDK's most attacker-adjacent input."""

    def setUp(self):
        class _Echo(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def _reply(self):
                self.server.seen.append(self.path)
                body = json.dumps({"replies": [], "messages": []}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            do_GET = _reply
            do_POST = _reply

        self.srv, port = _serve(_Echo)
        self.addCleanup(self.srv.server_close)
        self.addCleanup(self.srv.shutdown)
        self.client = Client("http://127.0.0.1:%d" % port, token=TOKEN, timeout=5)

    def test_ids_stay_inside_one_path_segment(self):
        evil = "../../v1/mailbox/messages"
        self.client.mailbox_ack(evil, by="me")
        self.client.mailbox_replies(evil)
        self.client.get_run(evil)
        for path in self.srv.seen:
            self.assertNotIn("../", path, "id escaped its segment: " + path)
            self.assertIn("%2F", path)
        self.assertEqual(
            self.srv.seen,
            [
                "/api/v1/mailbox/messages/..%2F..%2Fv1%2Fmailbox%2Fmessages/ack",
                "/api/v1/mailbox/messages/..%2F..%2Fv1%2Fmailbox%2Fmessages/replies",
                "/api/v1/runs/..%2F..%2Fv1%2Fmailbox%2Fmessages",
            ],
        )


if __name__ == "__main__":
    unittest.main()
