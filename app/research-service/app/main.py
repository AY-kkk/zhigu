from __future__ import annotations

import os
from contextlib import asynccontextmanager

from fastapi import FastAPI, Header, HTTPException

from app import jobs
from app.schemas import ResearchTask

SERVICE_TOKEN = os.environ.get("ZHIGU_INTERNAL_TOKEN", "zhigu-internal-dev")
CONTRACT_VERSION = "2"


@asynccontextmanager
async def lifespan(_: FastAPI):
    jobs.recover_on_start()
    yield


app = FastAPI(title="zhigu-research-service", lifespan=lifespan)


def _auth(authorization: str | None, contract: str | None) -> None:
    if authorization != f"Bearer {SERVICE_TOKEN}":
        raise HTTPException(status_code=401, detail={"code": "UNAUTHENTICATED", "message": "内部凭据无效"})
    if contract != CONTRACT_VERSION:
        raise HTTPException(status_code=409, detail={"code": "CONTRACT_VERSION_MISMATCH", "message": "控制协议版本不匹配"})


@app.post("/internal/research/tasks", status_code=202)
def submit(
    task: ResearchTask,
    authorization: str | None = Header(default=None),
    x_zhigu_task_token: str | None = Header(default=None),
    x_zhigu_contract_version: str | None = Header(default=None),
    x_zhigu_model_protocol: str | None = Header(default=None),
):
    _auth(authorization, x_zhigu_contract_version)
    _ = x_zhigu_model_protocol
    try:
        return jobs.submit(task, task_token=x_zhigu_task_token or "")
    except jobs.Conflict as exc:
        raise HTTPException(status_code=409, detail={"code": exc.code, "message": "任务幂等冲突"}) from exc


@app.get("/internal/research/tasks/{task_id}")
def get_task(
    task_id: str,
    authorization: str | None = Header(default=None),
    x_zhigu_contract_version: str | None = Header(default=None),
):
    _auth(authorization, x_zhigu_contract_version)
    try:
        return jobs.get(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc


@app.post("/internal/research/tasks/{task_id}/cancel", status_code=202)
def cancel(
    task_id: str,
    authorization: str | None = Header(default=None),
    x_zhigu_contract_version: str | None = Header(default=None),
):
    _auth(authorization, x_zhigu_contract_version)
    try:
        return jobs.cancel(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc


@app.post("/internal/research/tasks/{task_id}/purge", status_code=204)
def purge(
    task_id: str,
    authorization: str | None = Header(default=None),
    x_zhigu_contract_version: str | None = Header(default=None),
):
    _auth(authorization, x_zhigu_contract_version)
    try:
        jobs.purge_task(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc


@app.post("/internal/research/tasks/{task_id}/ack", status_code=204)
def ack(
    task_id: str,
    authorization: str | None = Header(default=None),
    x_zhigu_contract_version: str | None = Header(default=None),
):
    _auth(authorization, x_zhigu_contract_version)
    try:
        jobs.ack_task(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail={"code": "TASK_NOT_FOUND", "message": "任务不存在"}) from exc


@app.get("/healthz")
def healthz():
    executor = os.environ.get("ZHIGU_RESEARCH_EXECUTOR", "").strip().lower()
    if executor == "":
        executor = "harness"
    return {
        "status": "ok",
        "executor": executor,
        "mode": "fixture" if executor == "fixture" else "harness",
        "go_gateway_configured": bool(os.environ.get("ZHIGU_GO_INTERNAL_URL", "").strip()),
    }
