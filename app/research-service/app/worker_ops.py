from __future__ import annotations

import asyncio
import os
import time
from dataclasses import dataclass
from typing import Callable


class QuiesceConflict(Exception):
    pass


@dataclass
class QuiesceAck:
    epoch: str
    status: str
    acknowledged: bool
    last_sequence: int
    persisted: bool


class OpsState:
    def __init__(self) -> None:
        self._lock = asyncio.Lock()
        self.epoch: str | None = None
        self.accepting = True
        self.last_sequence = 0

    async def begin(self, epoch: str) -> dict:
        if not epoch or len(epoch) > 128:
            raise QuiesceConflict("INVALID_EPOCH")
        async with self._lock:
            if self.epoch not in (None, epoch):
                raise QuiesceConflict("QUIESCE_EPOCH_CONFLICT")
            self.epoch = epoch
            self.accepting = False
            self.last_sequence += 1
            return {"epoch": epoch, "status": "quiescing", "accepting": False, "last_sequence": self.last_sequence}

    async def status(self, epoch: str, active_count: Callable[[], int], persist: Callable[[], bool]) -> QuiesceAck:
        async with self._lock:
            if self.epoch != epoch:
                raise QuiesceConflict("QUIESCE_EPOCH_MISMATCH")
            active = active_count()
            acknowledged = active == 0
            persisted = False
            if acknowledged:
                persisted = persist()
            return QuiesceAck(epoch, "quiesced" if acknowledged else "quiescing", acknowledged, self.last_sequence, persisted)

    async def end(self, epoch: str) -> dict:
        async with self._lock:
            if self.epoch != epoch:
                raise QuiesceConflict("QUIESCE_EPOCH_MISMATCH")
            self.epoch = None
            self.accepting = True
            self.last_sequence += 1
            return {"epoch": epoch, "status": "ready", "accepting": True, "last_sequence": self.last_sequence}


def production_environment() -> bool:
    return os.environ.get("ZHIGU_DEPLOYMENT_ENVIRONMENT", os.environ.get("ZHIGU_ENV", "")).strip().lower() == "production"


def require_production_config(token: str, go_url: str, executor: str) -> None:
    if not production_environment():
        return
    weak = {"", "zhigu-internal-dev", "changeme", "test", "secret"}
    if len(token) < 32 or token.lower() in weak:
        raise RuntimeError("PRODUCTION_INTERNAL_TOKEN_REQUIRED")
    if not go_url:
        raise RuntimeError("PRODUCTION_GO_INTERNAL_URL_REQUIRED")
    if executor in {"", "fixture", "demo"}:
        raise RuntimeError("PRODUCTION_FIXTURE_EXECUTOR_FORBIDDEN")
