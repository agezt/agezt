"""Exceptions raised by the Agezt client."""

from __future__ import annotations


class AgeztError(Exception):
    """Base class for all errors raised by this package."""


class APIError(AgeztError):
    """The daemon returned a non-2xx response.

    Attributes:
        status: the HTTP status code.
        type: the machine-readable error type from the body
            (``error.type``), or ``""`` when absent.
        message: the human-readable message from the body, or the raw body.
    """

    def __init__(self, status: int, type: str = "", message: str = "") -> None:
        self.status = status
        self.type = type
        self.message = message
        detail = message or type or "request failed"
        super().__init__(f"agezt: HTTP {status}: {detail}")
