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


class ConfigAccessError(AgeztError):
    """Access to a config value was denied.

    This is raised when:
    - The config key has rating=secret (always denied)
    - HITL approval was denied or timed out
    - The agent lacks the config.access capability

    Attributes:
        key: The config key that was denied.
        code: Machine-readable error code (e.g., 'ACCESS_DENIED', 'KEY_NOT_FOUND').
        message: Human-readable explanation.
        status: HTTP status code (403, 404, 429, etc.).
    """

    def __init__(self, key: str, code: str, message: str, status: int = 403) -> None:
        self.key = key
        self.code = code
        self.status = status
        self.message = message
        super().__init__(f"config: {code}: {message}")
