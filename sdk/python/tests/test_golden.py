"""The SDK parses what the DAEMON answered.

The fixtures in ``sdk/fixtures/`` are recorded from the daemon's own REST
handlers (``kernel/restapi``'s golden test), so these tests check the SDK's
parsing against the server's bytes — not against a payload this test wrote
itself. A change in the daemon's wire shape moves the fixture and fails here.

Regenerate the fixtures with::

    AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures
"""

import json
import os
import unittest

from agezt import Mail, RunResult
from agezt._generated import Paths

FIXTURES = os.path.normpath(
    os.path.join(os.path.dirname(__file__), "..", "..", "fixtures")
)


def load(name):
    """Read one fixture file (the server's own bytes)."""
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as handle:
        return json.load(handle)


class GoldenFixtureTest(unittest.TestCase):
    """The SDK's types against the server's answers."""

    def test_every_fixture_path_matches_the_generated_catalog(self):
        """The SDK's generated paths and the server's templates are one set."""
        server_paths = {entry["path"] for entry in load("manifest.json")["fixtures"]}
        sdk_paths = {
            value
            for name, value in vars(Paths).items()
            if not name.startswith("_") and isinstance(value, str)
        }
        self.assertEqual(
            sdk_paths,
            server_paths,
            "the SDK's generated paths and the server's fixtures disagree",
        )

    def test_health_and_models(self):
        health = load("health.json")
        self.assertEqual(health["status"], "ok")
        self.assertEqual(health["version"], "9.9.9")
        self.assertEqual(health["default_model"], "model-a")
        self.assertEqual(health["model_count"], 2)
        models = load("models.json")
        self.assertEqual(models["default"], "model-a")
        self.assertEqual(models["models"], ["model-a", "model-b"])

    def test_run_result(self):
        result = RunResult.from_dict(load("run.json"))
        self.assertEqual(result.correlation_id, "corr-golden")
        self.assertEqual(result.model, "model-a")
        self.assertEqual(result.status, "completed")
        self.assertEqual(result.answer, "golden answer")

    def test_run_arc(self):
        arc = load("run_arc.json")
        self.assertEqual(arc["correlation_id"], "corr-golden")
        self.assertEqual(arc["count"], 1)
        self.assertEqual(arc["events"][0]["kind"], "llm.token")

    def test_mailbox_send(self):
        sent = load("mailbox_send.json")
        mail = Mail.from_dict(sent["message"])
        self.assertEqual(mail.id, "m-3")
        self.assertEqual(mail.topic, "dm")
        self.assertEqual(mail.text, "golden message")
        self.assertEqual(mail.from_, "agent-a")
        self.assertEqual(mail.to, "agent-b")
        self.assertEqual(mail.ts_unix_ms, 1700000000002)
        self.assertFalse(mail.help)

    def test_mailbox_messages_and_inbox(self):
        listed = load("mailbox_messages.json")
        self.assertEqual(listed["count"], 1)
        mail = Mail.from_dict(listed["messages"][0])
        self.assertEqual(mail.id, "m-1")
        self.assertEqual(mail.topic, "dm")
        self.assertEqual(mail.ts_unix_ms, 1700000000000)

        inbox = load("mailbox_inbox.json")
        self.assertEqual(inbox["name"], "agent-b")
        self.assertEqual(inbox["count"], 2)
        waiting = [Mail.from_dict(m) for m in inbox["waiting"]]
        self.assertEqual([m.id for m in waiting], ["m-2", "m-1"])
        self.assertEqual(waiting[0].to, "*")

    def test_mailbox_replies_ack_and_topics(self):
        replies = load("mailbox_replies.json")
        self.assertEqual(replies["id"], "m-1")
        self.assertEqual(replies["count"], 0)
        self.assertEqual(replies["replies"], [])

        acked = load("mailbox_ack.json")
        self.assertTrue(acked["acked"])
        self.assertEqual(acked["id"], "m-1")
        self.assertEqual(acked["by"], "agent-b")

        topics = load("mailbox_topics.json")
        self.assertEqual(topics["topics"], {"dm": 1, "status": 1})


if __name__ == "__main__":
    unittest.main()
