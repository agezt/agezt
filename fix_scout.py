import subprocess

# Use a single string without commas to avoid CSV splitting
instructions = (
    "Report only - do not modify files. "
    "Stay within the assigned workdir scope. "
    "Return structured findings (files patterns anomalies)."
)

result = subprocess.run(
    ["agt", "agent", "set", "--instructions", instructions, "scout"],
    capture_output=True, text=True, timeout=30
)
print("STDOUT:", result.stdout)
print("STDERR:", result.stderr)

# Verify
result2 = subprocess.run(
    ["agt", "agent", "show", "scout", "--json"],
    capture_output=True, text=True, timeout=30
)
import json
data = json.loads(result2.stdout)
print("\nInstructions:", data.get("instructions", []))
