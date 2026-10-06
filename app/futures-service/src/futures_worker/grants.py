import hashlib
import json
from typing import Any


def model_arguments(stage: str, messages: list[dict[str, Any]]) -> dict[str, Any]:
    return {"stage": stage, "messages": messages}


def hash_arguments(arguments: Any) -> str:
    raw = json.dumps(arguments, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(raw).hexdigest()
