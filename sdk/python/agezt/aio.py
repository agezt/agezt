"""Asyncio client for the Agezt REST API (``/api/v1``). Standard library only.

``AsyncClient`` mirrors the synchronous :class:`agezt.Client` method-for-method,
but every call is awaitable and never blocks the event loop. It is built on the
stdlib (``urllib`` + ``asyncio``) with no third-party dependency, matching
Agezt's stdlib-first ethos: the blocking HTTP work runs in a thread executor, and
the streaming run is bridged to an async iterator through an ``asyncio.Queue`` so
tokens are delivered to ``async for`` as the agent produces them.

    import asyncio
    from agezt import AsyncClient

    async def main():
        async with AsyncClient("http://127.0.0.1:8800", token="…") as c:
            print((await c.health())["version"])
            result = await c.run("summarise the latest commits")
            print(result.answer)

            async for ev in c.run_stream("write a haiku about Go"):
                if ev.event == "token":
                    print(ev.data.get("text", ""), end="", flush=True)

    asyncio.run(main())
"""

from __future__ import annotations

import asyncio
from typing import Any, AsyncIterator, Dict, Optional

from typing import Dict as _Dict, List as _List

from .client import Client, Mail, RunResult, StreamEvent

__all__ = ["AsyncClient"]


class AsyncClient:
    """An asyncio client for a running Agezt daemon's REST API.

    Same constructor as :class:`agezt.Client`:

    Args:
        base_url: the daemon's REST address, e.g. ``http://127.0.0.1:8800``.
        token: the bearer token (the daemon's admin token or a tenant token).
        timeout: per-request timeout in seconds.
        tenant: optional tenant id, sent as the ``X-Agezt-Tenant`` header.

    The client holds no persistent connection, so it is cheap to create and
    needs no teardown; :meth:`aclose` and ``async with`` are provided for
    symmetry and forward-compatibility.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        timeout: float = 30.0,
        tenant: Optional[str] = None,
    ) -> None:
        # Reuse the synchronous client's request building, error mapping, and SSE
        # parsing verbatim — the async layer only governs *when* that work runs
        # (off the event loop), not *how* the protocol is spoken.
        self._sync = Client(base_url, token, timeout=timeout, tenant=tenant)

    # Expose the same read-only attributes as the sync client.
    @property
    def base_url(self) -> str:
        return self._sync.base_url

    @property
    def token(self) -> str:
        return self._sync.token

    @property
    def timeout(self) -> float:
        return self._sync.timeout

    @property
    def tenant(self) -> Optional[str]:
        return self._sync.tenant

    # --- public API -------------------------------------------------------

    async def health(self) -> Dict[str, Any]:
        """Return the daemon's liveness + version summary."""
        return await self._in_thread(self._sync.health)

    async def models(self) -> Dict[str, Any]:
        """Return ``{"default": id, "models": [ids…]}``."""
        return await self._in_thread(self._sync.models)

    async def run(self, intent: str, model: Optional[str] = None) -> RunResult:
        """Execute an intent and return the final answer.

        Raises APIError if the run fails or the request is rejected.
        """
        return await self._in_thread(lambda: self._sync.run(intent, model))

    async def get_run(self, correlation_id: str) -> Dict[str, Any]:
        """Return the journaled event arc of a past run."""
        return await self._in_thread(lambda: self._sync.get_run(correlation_id))

    async def run_stream(
        self, intent: str, model: Optional[str] = None
    ) -> AsyncIterator[StreamEvent]:
        """Execute an intent, yielding StreamEvents (start/token/done/error) to
        ``async for`` as the agent produces them.

        The blocking SSE read runs in a worker thread; each parsed event is
        handed to the event loop through an ``asyncio.Queue``, so awaiting the
        next event never blocks the loop. An error raised mid-stream (e.g. a
        non-2xx response) is re-raised in the consuming coroutine.
        """
        loop = asyncio.get_running_loop()
        queue: "asyncio.Queue[Any]" = asyncio.Queue()
        done = object()  # unique end-of-stream sentinel

        def produce() -> None:
            try:
                for ev in self._sync.run_stream(intent, model):
                    loop.call_soon_threadsafe(queue.put_nowait, ev)
            except BaseException as exc:  # surface to the consumer, don't swallow
                loop.call_soon_threadsafe(queue.put_nowait, exc)
            finally:
                loop.call_soon_threadsafe(queue.put_nowait, done)

        # Fire-and-forget: the producer runs to completion as the consumer drains
        # the queue. produce() routes all outcomes (events, error, end) through
        # the queue, so the executor future never carries an exception itself.
        loop.run_in_executor(None, produce)

        while True:
            item = await queue.get()
            if item is done:
                return
            if isinstance(item, BaseException):
                raise item
            yield item

    # --- mailbox (mirrors the sync client) ---------------------------------

    async def mailbox_send(
        self,
        text: str,
        *,
        from_: str = "",
        to: str = "",
        topic: str = "",
        reply_to: str = "",
        help: bool = False,
    ) -> Mail:
        """Leave a message on the shared mailbox (see :meth:`Client.mailbox_send`)."""
        return await self._in_thread(
            lambda: self._sync.mailbox_send(
                text, from_=from_, to=to, topic=topic, reply_to=reply_to, help=help
            )
        )

    async def mailbox_broadcast(self, from_: str, text: str) -> Mail:
        """Send an announcement to EVERY inbox except the sender's."""
        return await self._in_thread(lambda: self._sync.mailbox_broadcast(from_, text))

    async def mailbox_inbox(
        self, name: str, include_read: bool = False, limit: int = 0
    ) -> "_List[Mail]":
        """Return what waits for ``name`` (see :meth:`Client.mailbox_inbox`)."""
        return await self._in_thread(
            lambda: self._sync.mailbox_inbox(name, include_read, limit)
        )

    async def mailbox_ack(self, message_id: str, by: str) -> None:
        """Mark a message read for one reader."""
        return await self._in_thread(lambda: self._sync.mailbox_ack(message_id, by))

    async def mailbox_replies(self, message_id: str, limit: int = 0) -> "_List[Mail]":
        """Return the answers to a sent message, oldest first."""
        return await self._in_thread(lambda: self._sync.mailbox_replies(message_id, limit))

    async def mailbox_messages(self, topic: str = "", limit: int = 0) -> "_List[Mail]":
        """Return recent mailbox messages, newest first."""
        return await self._in_thread(lambda: self._sync.mailbox_messages(topic, limit))

    async def mailbox_topics(self) -> "_Dict[str, int]":
        """Return the mailbox's topics with their message counts."""
        return await self._in_thread(self._sync.mailbox_topics)

    async def mailbox_watch(self, name: str = "", topic: str = "") -> AsyncIterator[Mail]:
        """Stream new mail as it lands (see :meth:`Client.mailbox_watch`),
        yielding to ``async for`` without blocking the event loop — the same
        queue bridge as :meth:`run_stream`."""
        loop = asyncio.get_running_loop()
        queue: "asyncio.Queue[Any]" = asyncio.Queue()
        done = object()

        def produce() -> None:
            try:
                for mail in self._sync.mailbox_watch(name, topic):
                    loop.call_soon_threadsafe(queue.put_nowait, mail)
            except BaseException as exc:
                loop.call_soon_threadsafe(queue.put_nowait, exc)
            finally:
                loop.call_soon_threadsafe(queue.put_nowait, done)

        loop.run_in_executor(None, produce)

        while True:
            item = await queue.get()
            if item is done:
                return
            if isinstance(item, BaseException):
                raise item
            yield item

    async def aclose(self) -> None:
        """No-op: the client holds no persistent resources. Provided for symmetry."""
        return None

    async def __aenter__(self) -> "AsyncClient":
        return self

    async def __aexit__(self, *exc: Any) -> None:
        await self.aclose()

    # --- internals --------------------------------------------------------

    @staticmethod
    async def _in_thread(fn: Any) -> Any:
        """Run a blocking callable in the default executor, off the event loop."""
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(None, fn)
