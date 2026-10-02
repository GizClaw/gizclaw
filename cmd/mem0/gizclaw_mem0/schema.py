"""Load the canonical OpenAPI contract and inspect the implementation schema."""

import json
from pathlib import Path

from fastapi.openapi.utils import get_openapi


def contract_path() -> Path:
    packaged = Path(__file__).with_name("openapi.json")
    if packaged.is_file():
        return packaged
    return Path(__file__).resolve().parents[3] / "api/http/mem0.json"


def contract() -> dict:
    return json.loads(contract_path().read_text(encoding="utf-8"))


def normalize(value):
    """Compare OpenAPI 3.0 wire shapes with FastAPI's JSON Schema output."""
    if isinstance(value, list):
        return [normalize(item) for item in value]
    if not isinstance(value, dict):
        return value
    result = {key: normalize(item) for key, item in value.items()
              if key not in ("title", "description") and not (key == "default" and item is None)}
    if isinstance(result.get("required"), list):
        result["required"] = sorted(result["required"])
    variants = result.get("anyOf")
    if variants and len(variants) == 2 and {"type": "null"} in variants:
        concrete = next(item for item in variants if item != {"type": "null"})
        result.pop("anyOf")
        result.update(concrete)
        result["nullable"] = True
    return result


def implementation(app) -> dict:
    return normalize(get_openapi(title=app.title, version=app.version, routes=app.routes))
