# Build with repository root as context: docker build -f app/research-service/Dockerfile .
# syntax=docker/dockerfile:1
ARG PYTHON_IMAGE=python:3.12.11-slim-bookworm
FROM ${PYTHON_IMAGE} AS build
ENV UV_LINK_MODE=copy UV_PROJECT_ENVIRONMENT=/opt/venv
COPY --from=ghcr.io/astral-sh/uv:0.8.4 /uv /usr/local/bin/uv
WORKDIR /src/app/research-service
COPY app/research-service/pyproject.toml app/research-service/uv.lock ./
RUN uv sync --frozen --no-dev --no-editable
COPY app/research-service/app ./app

FROM ${PYTHON_IMAGE}
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
ARG VERSION=dev
LABEL org.opencontainers.image.revision=${VCS_REF} \
      org.opencontainers.image.source=zhigu \
      org.opencontainers.image.created=${BUILD_DATE} \
      org.opencontainers.image.version=${VERSION}
RUN useradd --system --uid 10001 --create-home zhigu \
  && install -d -o zhigu -g zhigu /data
WORKDIR /app
COPY --from=build /opt/venv /opt/venv
COPY --from=build /src/app/research-service/app /app/app
ENV PYTHONPATH=/app PATH=/opt/venv/bin:$PATH
USER zhigu
EXPOSE 8091
HEALTHCHECK --interval=10s --timeout=3s --retries=10 CMD ["python","-c","import urllib.request; urllib.request.urlopen('http://127.0.0.1:8091/livez')"]
CMD ["uvicorn","app.main:app","--host","0.0.0.0","--port","8091"]
