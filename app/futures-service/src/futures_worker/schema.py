from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class Strict(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ScopeModel(Strict):
    domain: Literal["futures"]
    mode: Literal["live"]
    owner_id: int = Field(gt=0)


class ClaimModel(Strict):
    id: str = Field(min_length=1)
    kind: Literal["fact", "inference", "hypothesis"]
    text: str = Field(min_length=1)
    locator: dict[str, Any] | None


class VersionsModel(Strict):
    template: str = Field(min_length=1)
    formula: str = Field(min_length=1)
    policy: str = Field(min_length=1)
    model_config_version: str = Field(min_length=1, alias="model_config")
    source_manifest: str = Field(min_length=1)


class LimitsModel(Strict):
    max_model_calls: int = Field(ge=0, le=8)
    max_tool_calls: int = Field(ge=0, le=24)
    remaining_tokens: int = Field(ge=0, le=48000)
    deadline: str = Field(min_length=1)


class TaskModel(Strict):
    schema_version: Literal["futures.task.v1"]
    scope: ScopeModel
    task_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    generation: int = Field(ge=1)
    draft_id: str = Field(min_length=1)
    draft_revision: int = Field(ge=1)
    manifest_id: str = Field(min_length=1)
    product_id: str = Field(min_length=1)
    contract_id: str | None
    claims: list[ClaimModel] = Field(min_length=1, max_length=12)
    as_of: str = Field(min_length=1)
    horizon_end: str = Field(min_length=1)
    versions: VersionsModel
    limits: LimitsModel
    allowed_tools: list[Literal[
        "futures_get_observations", "futures_get_evidence", "futures_calculate"
    ]] = Field(min_length=1)

    def as_mapping(self) -> dict[str, Any]:
        return self.model_dump(mode="json", by_alias=True)
