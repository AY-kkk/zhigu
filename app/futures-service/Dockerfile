# Build with repository root as context: docker build -f app/futures-service/Dockerfile .
# syntax=docker/dockerfile:1
ARG PYTHON_IMAGE=python:3.12.11-slim-bookworm
FROM ${PYTHON_IMAGE} AS build
ENV UV_LINK_MODE=copy UV_PROJECT_ENVIRONMENT=/opt/venv
COPY --from=ghcr.io/astral-sh/uv:0.8.4 /uv /usr/local/bin/uv
WORKDIR /src/app/futures-service
COPY app/futures-service/pyproject.toml app/futures-service/uv.lock ./
RUN uv sync --frozen --no-dev --no-editable
COPY app/futures-service/src ./src

FROM ${PYTHON_IMAGE}
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
ARG VERSION=dev
LABEL org.opencontainers.image.revision=${VCS_REF} \
      org.opencontainers.image.source=zhigu \
      org.opencontainers.image.created=${BUILD_DATE} \
      org.opencontainers.image.version=${VERSION}
RUN useradd --system --uid 10001 --create-home zhigu
WORKDIR /app
COPY --from=build /opt/venv /opt/venv
COPY --from=build /src/app/futures-service/src /app/src
ENV PYTHONPATH=/app/src PATH=/opt/venv/bin:$PATH
USER zhigu
EXPOSE 8092
HEALTHCHECK --interval=10s --timeout=3s --retries=10 CMD ["python","-c","import urllib.request; urllib.request.urlopen('http://127.0.0.1:8092/livez')"]
CMD ["uvicorn","--factory","futures_worker.app:create_app","--host","0.0.0.0","--port","8092"]
