from typing import Any, Mapping

import httpx

from .base import ProviderError


class HTTPIngestGateway:
    def __init__(self, base_url: str, service_token: str, owner_id: int, mode: str = "live", client: httpx.AsyncClient | None = None):
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.owner_id = owner_id
        self.mode = mode
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(20, read=30))

    async def ingest(self, records: list[Mapping[str, Any]]) -> Mapping[str, Any]:
        response = await self.client.post(
            f"{self.base_url}/internal/futures/v1/observations",
            headers={"X-Futures-Service-Token": self.service_token},
            json={"owner_id": self.owner_id, "mode": self.mode, "records": list(records)},
        )
        if response.status_code != 200:
            raise ProviderError("FUTURES_OBSERVATION_INGEST_FAILED")
        return response.json().get("data", {})
