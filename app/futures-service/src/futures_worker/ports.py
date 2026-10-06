"""Implement these ports without importing app/research-service or its policies."""
from dataclasses import dataclass
from typing import Any, Mapping, Protocol


@dataclass(frozen=True)
class ExecutionScope:
    domain: str
    mode: str
    owner_id: int
    run_id: str
    task_id: str
    generation: int
    manifest_id: str
    as_of: str


class ToolGateway(Protocol):
    async def call(
        self, scope: ExecutionScope, name: str, arguments: Mapping[str, Any], grant: str
    ) -> Mapping[str, Any]: ...


class ModelGateway(Protocol):
    async def complete(
        self, scope: ExecutionScope, stage: str, messages: list[Mapping[str, str]], grant: str
    ) -> Mapping[str, Any]: ...


class ResearchExecutor(Protocol):
    async def execute(self, task: Mapping[str, Any]) -> Mapping[str, Any]: ...
