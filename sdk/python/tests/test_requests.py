"""The SDK's request rows are the daemon's contract.

The request bodies in ``sdk/fixtures/requests.json`` are rendered by the
daemon's own REST contract (``kernel/contract/restapiapi``'s ``RowBody``), so
these tests check that the SDK's request rows serialize to the very bytes the
contract declares — not to a payload this test wrote itself. A change in the
contract's request vocabulary moves the fixture and fails here.

Regenerate the fixture with::

    AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures
"""

import json
import os
import unittest

from agezt._generated import MailAck, MailDraft, RunRequest

FIXTURES = os.path.normpath(
    os.path.join(os.path.dirname(__file__), "..", "..", "fixtures")
)


def load(name):
    """Read one fixture file (the contract's own bytes)."""
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as handle:
        return json.load(handle)


def row(name):
    """The contract's request body for one row."""
    return load("requests.json")["rows"][name]


class RequestRowTest(unittest.TestCase):
    """The SDK's request rows against the contract's bodies."""

    def test_run_request(self):
        """A run request serializes to the contract's body."""
        body = row("run_request")
        self.assertEqual(RunRequest.from_dict(body).to_wire(), body)

    def test_mail_draft(self):
        """A message to leave serializes to the contract's body."""
        body = row("mail_draft")
        self.assertEqual(MailDraft.from_dict(body).to_wire(), body)

    def test_mail_ack(self):
        """An ack serializes to the contract's body."""
        body = row("mail_ack")
        self.assertEqual(MailAck.from_dict(body).to_wire(), body)

    def test_an_unset_optional_field_is_not_sent(self):
        """An optional field the row does not set is absent from the body.

        The daemon reads an absent optional field and an empty one as the same
        thing, but a request that says ``"help": false`` is not the request
        that says nothing about help.
        """
        draft = MailDraft(text="hello")
        self.assertEqual(draft.to_wire(), {"text": "hello"})


if __name__ == "__main__":
    unittest.main()
