from __future__ import annotations

import importlib
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[2]
RESEARCH_SRC = ROOT / "research-service"
FUTURES_SRC = ROOT / "futures-service" / "src"


class FakeExecutor:
    async def execute(self, task):
        return {"candidate": {"ok": True}}


class FakeGrants:
    async def validate(self, task, grant):
        return True


def load_research():
    sys.path.insert(0, str(RESEARCH_SRC))
    sys.modules.pop("app", None)
    sys.modules.pop("app.main", None)
    return importlib.import_module("app.main")


def load_futures():
    sys.path.insert(0, str(FUTURES_SRC))
    sys.modules.pop("futures_worker", None)
    sys.modules.pop("futures_worker.app", None)
    return importlib.import_module("futures_worker.app")


class WorkerOpsContractTests(unittest.TestCase):
    def test_research_health_and_quiesce_ack(self):
        module = load_research()
        app = module.create_app(service_token="worker-test-token")
        client = TestClient(app)
        self.assertEqual(client.get("/livez").status_code, 200)
        ready = client.get("/readyz")
        self.assertIn(ready.status_code, (200, 503))
        auth = {"Authorization": "Bearer worker-test-token"}
        self.assertEqual(client.post("/internal/ops/quiesce/begin", json={"epoch": "backup-1"}, headers=auth).status_code, 200)
        status = client.get("/internal/ops/quiesce/status", params={"epoch": "backup-1"}, headers=auth)
        self.assertEqual(status.status_code, 200)
        self.assertTrue(status.json()["acknowledged"])
        self.assertEqual(status.json()["epoch"], "backup-1")
        self.assertIn("last_sequence", status.json())
        self.assertIn("persisted", status.json())
        self.assertEqual(client.post("/internal/ops/quiesce/end", json={"epoch": "backup-1"}, headers=auth).status_code, 200)

    def test_futures_health_and_quiesce_requires_service_token(self):
        module = load_futures()
        app = module.create_app(executor=FakeExecutor(), service_token="worker-test-token", grant_validator=FakeGrants())
        client = TestClient(app)
        self.assertEqual(client.get("/livez").status_code, 200)
        self.assertIn(client.get("/readyz").status_code, (200, 503))
        self.assertEqual(client.post("/internal/ops/quiesce/begin", json={"epoch": "backup-2"}).status_code, 401)
        auth = {"X-Futures-Service-Token": "worker-test-token"}
        self.assertEqual(client.post("/internal/ops/quiesce/begin", json={"epoch": "backup-2"}, headers=auth).status_code, 200)
        status = client.get("/internal/ops/quiesce/status", params={"epoch": "backup-2"}, headers=auth)
        self.assertEqual(status.status_code, 200)
        self.assertTrue(status.json()["acknowledged"])
        self.assertEqual(client.post("/internal/ops/quiesce/end", json={"epoch": "backup-2"}, headers=auth).status_code, 200)

    def test_production_workers_reject_missing_or_fixture_configuration(self):
        research = load_research()
        with patch.dict(os.environ, {"ZHIGU_DEPLOYMENT_ENVIRONMENT": "production", "ZHIGU_RESEARCH_EXECUTOR": "fixture", "ZHIGU_GO_INTERNAL_URL": ""}, clear=False):
            with self.assertRaises(RuntimeError):
                research.create_app(service_token="too-short")
        futures = load_futures()
        with patch.dict(os.environ, {"ZHIGU_DEPLOYMENT_ENVIRONMENT": "production", "FUTURES_GO_BASE_URL": ""}, clear=False):
            with self.assertRaises(RuntimeError):
                futures.create_app(executor=FakeExecutor(), service_token="too-short", grant_validator=FakeGrants())

    def test_worker_dockerfiles_are_multistage_nonroot_and_do_not_install_at_runtime(self):
        for relative in ("research-service/Dockerfile", "futures-service/Dockerfile"):
            text = (ROOT / relative).read_text()
            stages = [part for part in text.split("\nFROM ") if part.strip()]
            self.assertGreaterEqual(len(stages), 2, relative)
            final = stages[-1]
            self.assertNotRegex(final, r"(?i)\b(pip|uv|apt-get|apk add|npm|go mod)\b")
            self.assertRegex(text, r"(?m)^USER\s+\S+")
            self.assertRegex(text, r"(?m)^LABEL\s+org\.opencontainers\.image\.(revision|source|created|version)=")

    def test_worker_images_precreate_writable_volume_mountpoints(self):
        research = (ROOT / "research-service/Dockerfile").read_text()
        futures = (ROOT / "futures-service/Dockerfile").read_text()
        self.assertIn("install -d -o zhigu -g zhigu /data /data/documents /data/storage", research)
        self.assertIn("install -d -o zhigu -g zhigu /data /documents", futures)


if __name__ == "__main__":
    unittest.main()
