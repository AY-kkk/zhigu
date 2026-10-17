#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PREVIOUS_IMAGE="${ZHIGU_PREVIOUS_SERVER_IMAGE:?set ZHIGU_PREVIOUS_SERVER_IMAGE}"
COMPOSE=(-f "$ROOT/app/deploy/compose.prod.yaml" -f "$ROOT/app/deploy/compose.rollback.yaml")
docker compose "${COMPOSE[@]}" up -d --no-build server
curl -fsS --retry 60 --retry-delay 2 http://127.0.0.1:8080/readyz
printf '{"timestamp":"%s","service":"zhigu-rollback","previous_image":"%s","database_preserved":true}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$PREVIOUS_IMAGE"
