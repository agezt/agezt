import subprocess, json

# Try decrypting vault via agt vault status to get entry names
result = subprocess.run(['agt', 'vault', 'status', '--json'], capture_output=True, text=True)
print("STDOUT:", result.stdout)
print("STDERR:", result.stderr[:500] if result.stderr else "")

# The vault is encrypted. We can try to see what keys are configured via env
import os
env_keys = [k for k in os.environ if 'ANTHROPIC' in k or 'LLMGATEWAY' in k or 'API_KEY' in k.upper()]
print("Relevant env vars:", env_keys)
