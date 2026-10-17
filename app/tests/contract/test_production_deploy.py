from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]

class ProductionDeployContractTests(unittest.TestCase):
    def test_prod_compose_is_isolated_complete_and_hardened(self):
        text = (ROOT / "deploy/compose.prod.yaml").read_text()
        for service in ("postgres:", "research:", "futures:", "server:", "web:"):
            self.assertIn("\n  " + service, text)
        for volume in ("postgres-data:", "research-data:", "futures-data:", "futures-documents:"):
            self.assertIn("\n  " + volume, text)
        self.assertNotIn("fixture", text)
        self.assertNotIn("latest", text)
        self.assertNotIn("../..:/work", text)
        self.assertIn("internal: true", text)
        self.assertIn("restart: unless-stopped", text)
        self.assertIn("pids_limit", text)
        self.assertEqual(text.count("pids_limit: 256"), text.count("pids: 256"))
        self.assertEqual(text.count("pids_limit: 512"), text.count("pids: 512"))
        self.assertIn("max-size", text)
        for variable in ("ZHIGU_RESEARCH_DOCUMENT_DIR", "ZHIGU_RESEARCH_STORAGE_DIR", "ZHIGU_FUTURES_STORAGE_DIR"):
            self.assertIn(variable, text)
        self.assertIn('ZHIGU_PERSISTENT_MOUNTS: \'["/data","/futures-documents"]\'', text)
        self.assertIn("research-data:/data", text)
        self.assertIn("futures-documents:/futures-documents", text)
        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        self.assertIn("compose.bootstrap.yaml", workflow)
        self.assertIn("Generate disposable production secrets", workflow)
        bootstrap = (ROOT / "deploy/compose.bootstrap.yaml").read_text()
        self.assertIn("ZHIGU_BOOTSTRAP_OWNER_FILE", bootstrap)
        self.assertNotIn("ZHIGU_BOOTSTRAP_OWNER_FILE", text)
        for binding in ('127.0.0.1:5432:5432', '127.0.0.1:8080:8080', '127.0.0.1:8091:8091', '127.0.0.1:8092:8092'):
            self.assertIn(binding, text)

    def test_test_compose_is_separate_and_fixture_only(self):
        text = (ROOT / "deploy/compose.test.yaml").read_text()
        self.assertIn("name: zhigu-test", text)
        self.assertIn("ZHIGU_DEPLOYMENT_ENVIRONMENT: test", text)
        self.assertIn("ZHIGU_RESEARCH_EXECUTOR: fixture", text)
        self.assertIn('127.0.0.1:5173:8080', text)
        self.assertNotIn('127.0.0.1:5173:80"', text)
        self.assertNotIn("name: zhigu-production", text)

    def test_tls_proxy_and_operational_scripts_exist(self):
        nginx = (ROOT / "deploy/nginx.prod.conf").read_text()
        self.assertIn("listen 8443 ssl", nginx)
        self.assertIn("client_max_body_size 21m", nginx)
        self.assertIn("/metrics { return 404; }", nginx)
        backup = (ROOT / "scripts/backup.sh").read_text()
        self.assertIn("quiesce/begin", backup)
        self.assertIn("pg_dump --format=custom", backup)
        self.assertTrue(any(token in backup for token in ("age -r", "openssl enc")))
        self.assertIn("/internal/ops/v1/backup/begin", backup)
        self.assertIn("/internal/ops/v1/backup/status", backup)
        self.assertIn("BARRIER_CLOSED", backup)
        self.assertIn("restore-verify", (ROOT / "scripts/restore-verify.sh").read_text())
        self.assertIn("docker compose", (ROOT / "scripts/maintenance-watchdog.sh").read_text())
        maintenance = (ROOT / "scripts/maintenance-run.sh").read_text()
        self.assertIn("flock -n", maintenance)
        self.assertIn("timeout 300s", maintenance)
        self.assertIn("backup_age", maintenance)
        self.assertIn("certificate", maintenance)
        for dockerfile in ("research-service/Dockerfile", "futures-service/Dockerfile", "server/Dockerfile", "web/Dockerfile"):
            docker_text = (ROOT / dockerfile).read_text()
            self.assertIn("org.opencontainers.image.revision", docker_text)
            self.assertIn("USER", docker_text)
        research_image = (ROOT / "research-service/Dockerfile").read_text()
        self.assertNotIn("COPY references/", research_image)
        for unit in ("zhigu-maintenance.service", "zhigu-maintenance.timer", "zhigu-watchdog.service"):
            self.assertTrue((ROOT / "deploy/systemd" / unit).is_file(), unit)
        timer = (ROOT / "deploy/systemd/zhigu-maintenance.timer").read_text()
        self.assertIn("OnCalendar=", timer)
        self.assertIn("Persistent=true", timer)
        upgrade = (ROOT / "scripts/upgrade-verify.sh").read_text()
        self.assertIn("PREVIOUS_DB_DUMP", upgrade)
        self.assertIn("ZHIGU_PREVIOUS_SERVER_IMAGE", upgrade)
        self.assertIn("pg_restore", upgrade)
        self.assertIn("N/N-1", upgrade)
        performance = (ROOT / "tests/performance/http_load.py").read_text()
        self.assertIn("--runs", performance)
        self.assertIn("p95_ms", performance)
        self.assertIn("error_rate", performance)

    def test_preflight_fails_closed_without_docker(self):
        script = (ROOT / "scripts/production-preflight.sh").read_text()
        self.assertIn("NOT RUN: Docker unavailable", script)
        self.assertIn("fixture|latest", script)
        self.assertIn("ZHIGU_ALLOW_LOOPBACK_SMOKE", script)
        self.assertLess(script.index("stat -c '%a'"), script.index("stat -f '%Lp'"))

    def test_ci_contract_commands_do_not_depend_on_shadowed_or_missing_venvs(self):
        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        production = workflow.split("  production-contracts:", 1)[1].split("\n  go-race-futures:", 1)[0]
        self.assertIn("app/scripts/verify-production.sh --static", production)
        self.assertNotIn("app.tests.contract", production)
        backup = workflow.split("  backup-restore:", 1)[1].split("\n  security:", 1)[0]
        self.assertNotIn("app/research-service/.venv/bin/python", backup)
        self.assertIn("python -m unittest discover -s app/tests/contract", backup)

    def test_production_smoke_cleanup_keeps_required_compose_environment(self):
        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        smoke = workflow.split("  production-smoke:", 1)[1].split("\n  upgrade:", 1)[0]
        self.assertGreaterEqual(smoke.count("ZHIGU_PUBLIC_ORIGIN:"), 3)
        self.assertGreaterEqual(smoke.count("ZHIGU_TLS_CERT_DIR:"), 3)
        self.assertIn("chown 10001:10001 app/deploy/secrets/", smoke)
        self.assertIn("app/deploy/secrets/session_keys", smoke)
        self.assertIn("printf '[\"%s\",\"%s\"]'", smoke)
        self.assertIn("tr '/+' '_-'", smoke)
        self.assertNotIn("ZHIGU_PUBLIC_ORIGIN: https://127.0.0.1", smoke)
        self.assertIn("--resolve smoke.zhigu.example.com:443:127.0.0.1", smoke)
        self.assertIn("chown 101:101 /tmp/zhigu-tls/", smoke)
        self.assertIn("--retry-all-errors", smoke)

    def test_preflight_requires_runtime_secret_owner(self):
        script = (ROOT / "scripts/production-preflight.sh").read_text()
        self.assertIn("secret $secret must be owned by uid 10001", script)
        self.assertIn("TLS private key must be owned by uid 101", script)
