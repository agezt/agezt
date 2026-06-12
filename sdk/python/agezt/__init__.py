"""Official Python client for the Agezt agentic OS.

Talks to a running Agezt daemon's native REST API (``/api/v1``) — the same
governed kernel loop ``agt run`` uses (Edict policy, the journal, cost
governance) — over plain HTTP with a bearer token. Standard library only.

    from agezt import Client

    c = Client("http://127.0.0.1:8800", token="…")
    print(c.health()["version"])
    result = c.run("summarise the latest commits")
    print(result.answer)

    # stream tokens as they are produced
    for ev in c.run_stream("write a haiku about Go"):
        if ev.event == "token":
            print(ev.data.get("text", ""), end="", flush=True)

There is also an asyncio client with the same surface — every call is awaitable
and never blocks the event loop::

    import asyncio
    from agezt import AsyncClient

    async def main():
        async with AsyncClient("http://127.0.0.1:8800", token="…") as c:
            print((await c.run("summarise the latest commits")).answer)
            async for ev in c.run_stream("write a haiku about Go"):
                if ev.event == "token":
                    print(ev.data.get("text", ""), end="", flush=True)

    asyncio.run(main())
"""

from .aio import AsyncClient
from .agent import AgentClient, Capability, AgentError
from .client import Client, Mail, RunResult, StreamEvent
from .errors import AgeztError, APIError, ConfigAccessError

__all__ = [
    "Client",
    "AsyncClient",
    "Mail",
    "AgentClient",
    "Capability",
    "AgentError",
    "RunResult",
    "StreamEvent",
    "AgeztError",
    "APIError",
    "ConfigAccessError",
]
__version__ = "1.1.0"
