import json
import os
import sys
from pathlib import Path

base_dir = Path(os.environ.get("AGEZT_HOME") or Path.home() / ".agezt")
catalog_path = base_dir / "catalog" / "api.json"
model_id = sys.argv[1] if len(sys.argv) > 1 else "claude-3-5-sonnet-20241022"

with catalog_path.open("r", encoding="utf-8") as f:
    catalog = json.load(f)

found = []
for provider_id, provider_data in catalog.items():
    models = provider_data.get('models', {})
    for model_key, model in models.items():
        if model_id in model_key or model_id == model.get('id'):
            found.append({
                'provider': provider_id,
                'name': provider_data.get('name'),
                'env': provider_data.get('env', []),
                'api': provider_data.get('api', ''),
                'model': model
            })

print(f"Found {len(found)} providers for {model_id} in {catalog_path}:")
for f in found:
    print(f"  Provider: {f['provider']} ({f['name']})")
    print(f"  Env vars: {f['env']}")
    print(f"  API: {f['api']}")
    print(f"  Model ID: {f['model'].get('id')}")
    print()
