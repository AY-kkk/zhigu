import hmac
import os
from pathlib import Path

from fastapi import Depends, FastAPI, Header, HTTPException, Query, status
from pydantic import BaseModel, ConfigDict
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse

from .executor import ExecutionError, ResearchExecutor
from .ops import OpsState, QuiesceConflict, require_production_config
from .schema import TaskModel
from .transport import HTTPGrantValidator, StaticGrantValidator, default_executor


def secret_env(name: str) -> str:
    value = os.getenv(name, "")
    if value:
        return value
    path = os.getenv(name + "_FILE", "")
    if not path:
        return ""
    try:
        return Path(path).read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise RuntimeError(f"{name}_FILE cannot be read") from exc


class QuiesceRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    epoch: str


def create_app(
    executor: ResearchExecutor | None = None,
    service_token: str | None = None,
    grant_validator=None,
) -> FastAPI:
    token = service_token or secret_env("FUTURES_SERVICE_TOKEN")
    if not token:
        raise RuntimeError("FUTURES_SERVICE_TOKEN is required")
    default = executor is None
    executor = executor or default_executor()
    if grant_validator is None:
        if default:
            grant_validator = HTTPGrantValidator(os.getenv("FUTURES_GO_BASE_URL", "http://127.0.0.1:8080"), token)
        else:
            grant_validator = StaticGrantValidator(set())
    require_production_config(token, os.getenv("FUTURES_GO_BASE_URL", ""))
    ops = OpsState()
    app = FastAPI(title="Zhigu Futures Worker", version="1.0", docs_url=None, redoc_url=None)

    def authorize(x_futures_service_token: str = Header(default=""), authorization: str = Header(default="")) -> None:
        if not hmac.compare_digest(x_futures_service_token, token):
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="FUTURES_SERVICE_TOKEN_INVALID")
        if not authorization.startswith("Bearer ") or len(authorization) == len("Bearer "):
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="FUTURES_TASK_GRANT_MISSING")

    def ops_authorize(x_futures_service_token: str = Header(default="")) -> None:
        if not hmac.compare_digest(x_futures_service_token, token):
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="FUTURES_SERVICE_TOKEN_INVALID")

    class InternalAuthMiddleware(BaseHTTPMiddleware):
        async def dispatch(self, request: Request, call_next):
            if request.url.path == "/internal/futures/v1/execute":
                provided = request.headers.get("x-futures-service-token", "")
                grant = request.headers.get("authorization", "")
                if not hmac.compare_digest(provided, token):
                    return JSONResponse({"detail": "FUTURES_SERVICE_TOKEN_INVALID"}, status_code=401)
                if not grant.startswith("Bearer ") or len(grant) == len("Bearer "):
                    return JSONResponse({"detail": "FUTURES_TASK_GRANT_MISSING"}, status_code=401)
            return await call_next(request)

    app.add_middleware(InternalAuthMiddleware)

    @app.get("/livez")
    async def livez() -> dict[str, str]:
        return {"service": "futures-worker", "status": "ok"}

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"service": "futures-worker", "status": "ok"}

    @app.get("/readyz")
    async def readyz() -> dict:
        healthy = ops.accepting
        return {
            "service": "futures-worker",
            "status": "ready" if healthy else "quiescing",
            "health": "ready" if healthy else "degraded",
            "operation_mode": "read_only" if healthy else "off",
            "data_provenance": "fixture" if os.getenv("FUTURES_EXECUTOR", "").lower() == "fixture" else "real",
            "reason": "" if healthy else "WORKER_QUIESCING",
        }

    @app.post("/internal/ops/quiesce/begin", dependencies=[Depends(ops_authorize)])
    async def quiesce_begin(body: QuiesceRequest) -> dict:
        try:
            return await ops.begin(body.epoch)
        except QuiesceConflict as exc:
            raise HTTPException(status_code=409, detail=str(exc)) from exc

    @app.get("/internal/ops/quiesce/status", dependencies=[Depends(ops_authorize)])
    async def quiesce_status(epoch: str = Query(min_length=1, max_length=128)) -> dict:
        try:
            ack = await ops.status(epoch)
        except QuiesceConflict as exc:
            raise HTTPException(status_code=409, detail=str(exc)) from exc
        return ack.__dict__

    @app.post("/internal/ops/quiesce/end", dependencies=[Depends(ops_authorize)])
    async def quiesce_end(body: QuiesceRequest) -> dict:
        try:
            return await ops.end(body.epoch)
        except QuiesceConflict as exc:
            raise HTTPException(status_code=409, detail=str(exc)) from exc

    @app.post("/internal/futures/v1/execute", dependencies=[Depends(authorize)])
    async def execute(task: TaskModel, authorization: str = Header(default="")) -> dict:
        if not ops.start_task():
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="FUTURES_WORKER_QUIESCING")
        try:
            grant = authorization.removeprefix("Bearer ")
            if not await grant_validator.validate(task.as_mapping(), grant):
                raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="FUTURES_TASK_GRANT_INVALID")
            try:
                result = await executor.execute(task.as_mapping())
            except ExecutionError as exc:
                raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail=str(exc)) from exc
            return result["candidate"]
        finally:
            ops.finish_task()

    return app
