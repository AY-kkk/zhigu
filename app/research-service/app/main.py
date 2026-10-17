from __future__ import annotations

import hmac
import os
from pathlib import Path
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, Header, HTTPException, Query
from pydantic import BaseModel, ConfigDict

from app import jobs
from app.schemas import ResearchTask
from app.worker_ops import OpsState, QuiesceConflict, production_environment, require_production_config

CONTRACT_VERSION = "2"


def secret_env(name: str) -> str:
    value = os.environ.get(name, "")
    if value:
        return value
    path = os.environ.get(name + "_FILE", "")
    if path:
        try:
            return Path(path).read_text(encoding="utf-8").strip()
        except OSError as exc:
            raise RuntimeError(f"{name}_FILE cannot be read") from exc
    return ""


def healthz() -> dict[str, object]:
    executor = os.environ.get("ZHIGU_RESEARCH_EXECUTOR", "").strip().lower() or "harness"
    return {
        "status": "ok",
        "executor": executor,
        "mode": "fixture" if executor == "fixture" else "harness",
        "go_gateway_configured": bool(os.environ.get("ZHIGU_GO_INTERNAL_URL", "").strip()),
    }


class QuiesceRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    epoch: str


def create_app(service_token: str | None = None) -> FastAPI:
    token = service_token or secret_env("ZHIGU_INTERNAL_TOKEN")
    executor = os.environ.get("ZHIGU_RESEARCH_EXECUTOR", "").strip().lower()
    if not token:
        token = "zhigu-internal-dev" if not production_environment() else ""
    require_production_config(token, os.environ.get("ZHIGU_GO_INTERNAL_URL", ""), executor)
    ops = OpsState()

    @asynccontextmanager
    async def lifespan(_: FastAPI):
        jobs.recover_on_start()
        yield
        jobs.checkpoint()

    app = FastAPI(title="zhigu-research-service", lifespan=lifespan)

    def auth(authorization: str | None, contract: str | None = None) -> None:
        expected = f"Bearer {token}"
        if authorization is None or not hmac.compare_digest(authorization, expected):
            raise HTTPException(status_code=401, detail={"code": "UNAUTHENTICATED", "message": "内部凭据无效"})
        if contract is not None and contract != CONTRACT_VERSION:
            raise HTTPException(status_code=409, detail={"code": "CONTRACT_VERSION_MISMATCH", "message": "控制协议版本不匹配"})

    def ops_auth(authorization: str | None = Header(default=None)) -> None:
        auth(authorization)

    @app.post("/internal/research/tasks", status_code=202)
    def submit(
        task: ResearchTask,
        authorization: str | None = Header(default=None),
        x_zhigu_task_token: str | None = Header(default=None),
        x_zhigu_contract_version: str | None = Header(default=None),
        x_zhigu_model_protocol: str | None = Header(default=None),
    ):
        auth(authorization, x_zhigu_contract_version)
        if not ops.accepting:
            raise HTTPException(status_code=503, detail={"code": "WORKER_QUIESCING", "message": "worker 正在静默"})
        _ = x_zhigu_model_protocol
        try:
            return jobs.submit(task, task_token=x_zhigu_task_token or "")
        except jobs.Conflict as exc:
            raise HTTPException(status_code=409, detail={"code": exc.code, "message": "任务幂等冲突"}) from exc

    @app.get("/internal/research/tasks/{task_id}")
    def get_task(task_id: str, authorization: str | None = Header(default=None), x_zhigu_contract_version: str | None = Header(default=None)):
        auth(authorization, x_zhigu_contract_version)
        try:
            return jobs.get(task_id)
        except KeyError as exc:
            raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc

    @app.post("/internal/research/tasks/{task_id}/cancel", status_code=202)
    def cancel(task_id: str, authorization: str | None = Header(default=None), x_zhigu_contract_version: str | None = Header(default=None)):
        auth(authorization, x_zhigu_contract_version)
        try:
            return jobs.cancel(task_id)
        except KeyError as exc:
            raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc

    @app.post("/internal/research/tasks/{task_id}/purge", status_code=204)
    def purge(task_id: str, authorization: str | None = Header(default=None), x_zhigu_contract_version: str | None = Header(default=None)):
        auth(authorization, x_zhigu_contract_version)
        try:
            jobs.purge_task(task_id)
        except KeyError as exc:
            raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc

    @app.post("/internal/research/tasks/{task_id}/ack", status_code=204)
    def ack(task_id: str, authorization: str | None = Header(default=None), x_zhigu_contract_version: str | None = Header(default=None)):
        auth(authorization, x_zhigu_contract_version)
        try:
            jobs.ack_task(task_id)
        except KeyError as exc:
            raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc

    @app.get("/livez")
    def livez() -> dict[str, str]:
        return {"service": "research-worker", "status": "ok"}

    @app.get("/healthz")
    def health_route() -> dict[str, object]:
        return healthz()

    @app.get("/readyz")
    def readyz() -> dict:
        try:
            jobs.connect().execute("SELECT 1").fetchone()
        except Exception:
            raise HTTPException(status_code=503, detail={"code": "WORKER_NOT_READY", "message": "持久化依赖不可用"})
        healthy = ops.accepting
        return {
            "service": "research-worker",
            "status": "ready" if healthy else "quiescing",
            "health": "ready" if healthy else "degraded",
            "operation_mode": "read_only" if healthy else "off",
            "data_provenance": "fixture" if os.environ.get("ZHIGU_RESEARCH_EXECUTOR", "").lower() == "fixture" else "real",
            "reason": "" if healthy else "WORKER_QUIESCING",
        }

    @app.post("/internal/ops/quiesce/begin", dependencies=[Depends(ops_auth)])
    async def quiesce_begin(body: QuiesceRequest) -> dict:
        try:
            jobs.set_quiescing(True)
            return await ops.begin(body.epoch)
        except QuiesceConflict as exc:
            jobs.set_quiescing(not ops.accepting)
            raise HTTPException(status_code=409, detail={"code": str(exc), "message": "静默状态冲突"}) from exc

    @app.get("/internal/ops/quiesce/status", dependencies=[Depends(ops_auth)])
    async def quiesce_status(epoch: str = Query(min_length=1, max_length=128)) -> dict:
        try:
            ack = await ops.status(epoch, jobs.active_count, jobs.checkpoint)
        except QuiesceConflict as exc:
            raise HTTPException(status_code=409, detail={"code": str(exc), "message": "静默状态冲突"}) from exc
        return ack.__dict__

    @app.post("/internal/ops/quiesce/end", dependencies=[Depends(ops_auth)])
    async def quiesce_end(body: QuiesceRequest) -> dict:
        try:
            result = await ops.end(body.epoch)
            jobs.set_quiescing(False)
            return result
        except QuiesceConflict as exc:
            raise HTTPException(status_code=409, detail={"code": str(exc), "message": "静默状态冲突"}) from exc

    return app


app = create_app()
