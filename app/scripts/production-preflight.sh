#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE="$ROOT/app/deploy/compose.prod.yaml"
TEST_COMPOSE="$ROOT/app/deploy/compose.test.yaml"
SECRETS="$ROOT/app/deploy/secrets"
fail() { echo "PREFLIGHT FAIL: $*" >&2; exit 1; }

[[ -f "$COMPOSE" ]] || fail "missing compose.prod.yaml"
[[ -f "$TEST_COMPOSE" ]] || fail "missing compose.test.yaml"
grep -q '^name: zhigu-production$' "$COMPOSE" || fail "production project name is not fixed"
grep -q '^name: zhigu-test$' "$TEST_COMPOSE" || fail "test project is not isolated"
for secret in postgres_password postgres_dsn internal_token futures_token jwt_secret session_keys master_key; do
  if [[ ! -s "$SECRETS/$secret" ]]; then
    echo "NOT RUN: missing production secret $secret; clean-host deployment remains BLOCKED" >&2
    exit 2
  fi
  [[ "$(stat -c '%a' "$SECRETS/$secret" 2>/dev/null || stat -f '%Lp' "$SECRETS/$secret")" == "600" ]] || fail "secret $secret must be mode 600"
done
if grep -Eq 'fixture|latest|:/work|\\.\\.:/|/\\.env' "$COMPOSE"; then
  fail "production compose contains fixture/default/source bind/weak-secret pattern"
fi
for service in postgres research futures server web; do
  grep -q "^  $service:" "$COMPOSE" || fail "missing service $service"
done
for volume in postgres-data research-data futures-data futures-documents; do
  grep -q "^  $volume:" "$COMPOSE" || fail "missing volume $volume"
done
grep -q 'internal: true' "$COMPOSE" || fail "private backend network missing"
grep -q 'client_max_body_size 21m' "$ROOT/app/deploy/nginx.prod.conf" || fail "upload limit is not 21 MiB"
grep -q 'listen 8443 ssl' "$ROOT/app/deploy/nginx.prod.conf" || fail "TLS listener missing"
[[ -n "${ZHIGU_PUBLIC_ORIGIN:-}" ]] || fail "ZHIGU_PUBLIC_ORIGIN is required"
[[ "${ZHIGU_PUBLIC_ORIGIN}" == https://* ]] || fail "ZHIGU_PUBLIC_ORIGIN must use HTTPS"
case "${ZHIGU_PUBLIC_ORIGIN}" in
  https://localhost|https://localhost:*|https://127.*|https://10.*|https://192.168.*|https://172.1[6-9].*|https://172.2[0-9].*|https://172.3[0-1].*)
    [[ "${ZHIGU_ALLOW_LOOPBACK_SMOKE:-}" == "1" ]] || fail "production ZHIGU_PUBLIC_ORIGIN must not be loopback/private" ;;
esac
[[ -n "${ZHIGU_TLS_CERT_DIR:-}" ]] || fail "ZHIGU_TLS_CERT_DIR is required"
[[ -s "${ZHIGU_TLS_CERT_DIR}/fullchain.pem" && -s "${ZHIGU_TLS_CERT_DIR}/privkey.pem" ]] || fail "TLS certificate files are missing"
if command -v docker >/dev/null 2>&1; then
  compose_env=()
  [[ -n "${ZHIGU_ENV_FILE:-}" ]] && compose_env=(--env-file "$ZHIGU_ENV_FILE")
  docker compose "${compose_env[@]}" -f "$COMPOSE" config >/dev/null
  echo "PASS: production compose config"
else
  echo "NOT RUN: Docker unavailable; production compose and clean-host smoke remain BLOCKED" >&2
  exit 2
fi
echo "PASS: production preflight static checks"
