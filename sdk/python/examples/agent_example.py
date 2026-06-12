#!/usr/bin/env python3
"""
Example: AI Agent Subprocess Using the Agezt Agent SDK

This example shows how an AI agent subprocess can use the Agent SDK
to communicate with the AGEZT kernel via scoped capability tokens.

Usage:
    # Token is passed by the parent agent via environment
    export AGEZT_AGENT_TOKEN="eyJ..."
    python agent_example.py
"""

import os
import sys

from agezt import AgentClient, Capability

def main():
    # Get token from environment (set by parent agent)
    token = os.environ.get("AGEZT_AGENT_TOKEN")
    if not token:
        print("ERROR: AGEZT_AGENT_TOKEN environment variable not set")
        print("The parent agent should set this before spawning the subprocess.")
        sys.exit(1)

    # Create the agent client
    client = AgentClient(token=token)

    print("=== Agezt Agent SDK Demo ===\n")

    # --- Memory Operations ---
    print("1. Remembering a fact...")
    record = client.memory.write(
        type="fact",
        subject="Agezt Agent SDK",
        content="The Agent SDK allows AI agents to securely communicate with AGEZT."
    )
    print(f"   Created: {record['id']}")

    print("\n2. Searching memories...")
    results = client.memory.search("Agezt Agent SDK")
    for r in results:
        print(f"   - [{r['score']:.2f}] {r['subject']}: {r['content'][:50]}...")

    # --- Eventbus Operations ---
    print("\n3. Publishing an event...")
    client.eventbus.publish("agent.demo", {
        "message": "Hello from agent subprocess!",
        "timestamp": "2024-01-01T00:00:00Z"
    })
    print("   Event published!")

    # --- Logging ---
    print("\n4. Writing logs...")
    client.log.write("Agent subprocess started", level="info")
    client.log.write("Processing complete", level="info", meta={"items": 42})
    print("   Logs written!")

    # --- Agent Operations ---
    print("\n5. Listing agents...")
    agents = client.agent.list()
    print(f"   Found {len(agents)} agents:")
    for a in agents[:5]:
        print(f"   - {a['name']} ({a['model']})")

    print("\n=== Demo Complete ===")
    print("\nFor AI agents writing code, use:")
    print("  from agezt import AgentClient")
    print("  client = AgentClient(token=os.environ['AGEZT_AGENT_TOKEN'])")
    print("  client.memory.write(type='fact', subject='...', content='...')")
    print("  results = client.memory.search('query')")

if __name__ == "__main__":
    main()
