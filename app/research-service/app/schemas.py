from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class ClaimItem(BaseModel):
    model_config = ConfigDict(extra="forbid")
    claim_id: str
    text: str
    claim_type: Literal["fact", "inference", "assumption"]


class Claim(BaseModel):
    model_config = ConfigDict(extra="forbid")
    text: str
    horizon: str
    items: list[ClaimItem]


class ResearchTask(BaseModel):
    model_config = ConfigDict(extra="forbid")
    schema_version: Literal["1.0"]
    run_id: str
    task_id: str
    role: Literal["supporter", "challenger"]
    claim: Claim
    instrument_id: str
    as_of: str
    mode: Literal["fixture", "live"]
    source_policy_version: str
    model_config_version: str
    prompt_version: str
    max_model_calls: int = Field(ge=1, le=4)
    max_tool_calls: int = Field(ge=1, le=6)
    deadline_at: str


class Argument(BaseModel):
    model_config = ConfigDict(extra="forbid")
    claim_type: Literal["fact", "inference", "assumption"]
    text: str
    evidence_ids: list[str]


class Usage(BaseModel):
    model_config = ConfigDict(extra="forbid")
    model_calls: int
    tool_calls: int
    input_tokens: int
    output_tokens: int
    usage_unknown: bool
    simulated: bool = False


class ResultError(BaseModel):
    model_config = ConfigDict(extra="forbid")
    code: str
    message: str
    retryable: bool


class ResearchResult(BaseModel):
    model_config = ConfigDict(extra="forbid")
    schema_version: Literal["1.0"] = "1.0"
    run_id: str
    task_id: str
    status: Literal["succeeded", "insufficient", "failed", "canceled"]
    arguments: list[Argument]
    evidence_ids: list[str]
    unknowns: list[str]
    counterevidence: list[Argument]
    usage: Usage
    errors: list[ResultError]


ALLOWED_TOOLS = frozenset({"get_financials", "search_filings", "calculate_metric"})
