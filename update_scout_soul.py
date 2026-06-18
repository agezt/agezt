import subprocess

soul = (
    "You are Scout - a lightweight reconnaissance agent. "
    "Your job is to investigate unfamiliar codebases, explore packages, "
    "and report structured findings (file layout, key symbols, patterns, anomalies) "
    "back to your owner. Fast, focused, minimal footprint. "
    "Observe and summarize only - do not refactor or write code."
)

instructions = (
    "Report only - do not modify files. "
    "Stay within the assigned workdir scope. "
    "Return structured findings: files, patterns, anomalies."
)

# Set soul via agt
result1 = subprocess.run(
    ["agt", "agent", "set", "--soul", soul, "scout"],
    capture_output=True, text=True, timeout=30
)
print("Soul set result:", result1.stdout, result1.stderr)

# Set instructions via agt
result2 = subprocess.run(
    ["agt", "agent", "set", "--instructions", instructions, "scout"],
    capture_output=True, text=True, timeout=30
)
print("Instructions set result:", result2.stdout, result2.stderr)

# Verify
result3 = subprocess.run(
    ["agt", "agent", "show", "scout", "--json"],
    capture_output=True, text=True, timeout=30
)
import json
data = json.loads(result3.stdout)
print("\nSoul:", data.get("soul", "N/A"))
print("Instructions:", data.get("instructions", []))
