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
"""

from .client import Client, RunResult, StreamEvent
from .errors import AgeztError, APIError

__all__ = ["Client", "RunResult", "StreamEvent", "AgeztError", "APIError"]
__version__ = "1.0.0"
