import os
from typing import Any, Mapping, Protocol

import httpx

from .executor import ExecutionError, ResearchExecutor
from .ports import ExecutionScope


class GrantValidator(Protocol):
    async def validate(self, task: Mapping[str, Any], grant: str) -> bool: ...


class StaticGrantValidator:
    def __init__(self, valid_tokens: set[str]):
        self.valid_tokens = set(valid_tokens)

    async def validate(self, task: Mapping[str, Any], grant: str) -> bool:
        return grant in self.valid_tokens


class HTTPGrantValidator:
    def __init__(self, base_url: str, service_token: str, client: httpx.AsyncClient | None = None):
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(10, read=20))

    async def validate(self, task: Mapping[str, Any], grant: str) -> bool:
        response = await self.client.post(
            f"{self.base_url}/internal/futures/v1/grants/consume",
            headers={
                "X-Futures-Service-Token": self.service_token,
                "Authorization": f"Bearer {grant}",
            },
            json=dict(task),
        )
        return response.status_code == 200


class HTTPGrantBroker:
    def __init__(self, base_url: str, service_token: str, client: httpx.AsyncClient | None = None):
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(10, read=20))

    async def issue(self, scope: ExecutionScope, stage: str, tool_name: str | None, arguments: Mapping[str, Any]) -> str:
        from .grants import hash_arguments
        response = await self.client.post(
            f"{self.base_url}/internal/futures/v1/grants",
            headers={"X-Futures-Service-Token": self.service_token},
            json={
                "domain": scope.domain, "owner_id": scope.owner_id, "mode": scope.mode,
                "run_id": scope.run_id, "task_id": scope.task_id, "generation": scope.generation,
                "manifest_id": scope.manifest_id, "stage": stage, "tool_name": tool_name or "",
                "args_hash": hash_arguments(arguments),
            },
        )
        if response.status_code != 201:
            raise ExecutionError("FUTURES_GRANT_DENIED")
        return response.json()["data"]["grant"]


class HTTPToolGateway:
    def __init__(self, base_url: str, service_token: str, client: httpx.AsyncClient | None = None):
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(10, read=20))

    async def call(self, scope: ExecutionScope, name: str, arguments: Mapping[str, Any], grant: str) -> Mapping[str, Any]:
        response = await self.client.post(
            f"{self.base_url}/internal/futures/v1/tools",
            headers={"X-Futures-Service-Token": self.service_token, "Authorization": f"Bearer {grant}"},
            json=self._body(scope, name, arguments),
        )
        if response.status_code != 200:
            raise ExecutionError("FUTURES_TOOL_EXECUTION_FAILED")
        return response.json()["data"]

    @staticmethod
    def _body(scope: ExecutionScope, name: str, arguments: Mapping[str, Any]) -> dict[str, Any]:
        return {
            "scope": {"domain": scope.domain, "mode": scope.mode, "owner_id": scope.owner_id, "run_id": scope.run_id},
            "task_id": scope.task_id, "generation": scope.generation, "manifest_id": scope.manifest_id,
            "name": name, "arguments": dict(arguments),
        }


class HTTPModelGateway:
    def __init__(self, base_url: str, service_token: str, client: httpx.AsyncClient | None = None):
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(20, read=30))

    async def complete(self, scope: ExecutionScope, stage: str, messages: list[Mapping[str, str]], grant: str) -> Mapping[str, Any]:
        response = await self.client.post(
            f"{self.base_url}/internal/futures/v1/model",
            headers={"X-Futures-Service-Token": self.service_token, "Authorization": f"Bearer {grant}"},
            json={
                "scope": {"domain": scope.domain, "mode": scope.mode, "owner_id": scope.owner_id, "run_id": scope.run_id},
                "task_id": scope.task_id, "generation": scope.generation, "manifest_id": scope.manifest_id,
                "name": "futures_model", "arguments": {"stage": stage, "messages": messages},
            },
        )
        if response.status_code != 200:
            raise ExecutionError("FUTURES_MODEL_EXECUTION_FAILED")
        return response.json()["data"]


def default_executor() -> ResearchExecutor:
    base_url = os.getenv("FUTURES_GO_BASE_URL", "http://127.0.0.1:8080")
    service_token = os.getenv("FUTURES_SERVICE_TOKEN")
    if not service_token:
        raise RuntimeError("FUTURES_SERVICE_TOKEN is required")
    tools = HTTPToolGateway(base_url, service_token)
    models = HTTPModelGateway(base_url, service_token)
    grants = HTTPGrantBroker(base_url, service_token)
    return ResearchExecutor(tools, models, grants)
