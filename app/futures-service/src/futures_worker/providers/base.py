from dataclasses import dataclass
from typing import Any, Mapping, Protocol, Sequence


class ProviderError(ValueError):
    pass


@dataclass(frozen=True)
class FetchRequest:
    scope: str
    product_id: str
    metric: str
    start: str
    end: str
    cursor: str | None


@dataclass(frozen=True)
class SourceManifest:
    source_id: str
    version: str
    publisher: str
    url: str
    source_type: str
    rights: Mapping[str, Any]
    metrics: Sequence[str]


class Provider(Protocol):
    async def fetch(self, request: FetchRequest) -> Mapping[str, Any]: ...
