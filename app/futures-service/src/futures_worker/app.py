import hmac
import os

from fastapi import Depends, FastAPI, Header, HTTPException, status
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse

from .executor import ExecutionError, ResearchExecutor
from .schema import TaskModel
from .transport import HTTPGrantValidator, StaticGrantValidator, default_executor


def create_app(
    executor: ResearchExecutor | None = None,
    service_token: str | None = None,
    grant_validator=None,
) -> FastAPI:
    token = service_token or os.getenv("FUTURES_SERVICE_TOKEN")
    if not token:
        raise RuntimeError("FUTURES_SERVICE_TOKEN is required")
    default = executor is None
    executor = executor or default_executor()
    if grant_validator is None:
        if default:
            grant_validator = HTTPGrantValidator(os.getenv("FUTURES_GO_BASE_URL", "http://127.0.0.1:8080"), token)
        else:
            grant_validator = StaticGrantValidator(set())

    app = FastAPI(title="Zhigu Futures Worker", version="1.0", docs_url=None, redoc_url=None)

    def authorize(x_futures_service_token: str = Header(default=""), authorization: str = Header(default="")) -> None:
        if not hmac.compare_digest(x_futures_service_token, token):
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="FUTURES_SERVICE_TOKEN_INVALID")
        if not authorization.startswith("Bearer ") or len(authorization) == len("Bearer "):
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="FUTURES_TASK_GRANT_MISSING")

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

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"service": "futures-worker", "status": "ok"}

    @app.post("/internal/futures/v1/execute", dependencies=[Depends(authorize)])
    async def execute(task: TaskModel, authorization: str = Header(default="")) -> dict:
        grant = authorization.removeprefix("Bearer ")
        if not await grant_validator.validate(task.as_mapping(), grant):
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="FUTURES_TASK_GRANT_INVALID")
        try:
            result = await executor.execute(task.as_mapping())
        except ExecutionError as exc:
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail=str(exc)) from exc
        return result["candidate"]

    return app
