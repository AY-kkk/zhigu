import os
import json
import asyncio
from pathlib import Path
import unittest

import httpx

from fastapi.testclient import TestClient

from futures_worker.app import create_app
from futures_worker.executor import ExecutionError, ResearchExecutor
from futures_worker.grants import hash_arguments
from futures_worker.ports import ExecutionScope
from futures_worker.transport import HTTPGrantBroker, HTTPModelGateway
from futures_worker.transport import StaticGrantValidator
from futures_worker.schema import TaskModel

from test_executor import TASK, ModelGateway, ToolGateway, report


class FakeExecutor(ResearchExecutor):
    async def execute(self, task):
        return {"candidate": report("verify")}


class TransportTests(unittest.TestCase):
    def setUp(self):
        os.environ["FUTURES_SERVICE_TOKEN"] = "test-service-token"
        self.client = TestClient(create_app(
            FakeExecutor(ToolGateway(), ModelGateway([])),
            grant_validator=StaticGrantValidator({"task-grant"}),
        ))

    def test_execute_requires_service_token_and_strict_task(self):
        response = self.client.post("/internal/futures/v1/execute", json=TASK)
        self.assertEqual(response.status_code, 401)
        bad = dict(TASK, unknown=True)
        response = self.client.post(
            "/internal/futures/v1/execute", json=bad,
            headers={"X-Futures-Service-Token": "test-service-token"},
        )
        self.assertEqual(response.status_code, 401)
        response = self.client.post(
            "/internal/futures/v1/execute", json=bad,
            headers={"X-Futures-Service-Token": "test-service-token", "Authorization": "Bearer task-grant"},
        )
        self.assertEqual(response.status_code, 422)

    def test_health_is_worker_local_and_never_reports_live_data(self):
        response = self.client.get("/healthz")
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["service"], "futures-worker")
        self.assertNotIn("live_data", response.json())

    def test_pydantic_task_accepts_contract_fixture_and_rejects_nested_unknown(self):
        path = Path(__file__).resolve().parents[3] / "contracts" / "futures" / "v1" / "fixtures" / "task-offline.json"
        task = json.loads(path.read_text())
        TaskModel.model_validate(task)
        task["limits"]["unexpected"] = 1
        with self.assertRaises(ValueError):
            TaskModel.model_validate(task)

    def test_candidate_is_returned_but_never_marked_published(self):
        response = self.client.post(
            "/internal/futures/v1/execute", json=TASK,
            headers={"X-Futures-Service-Token": "test-service-token", "Authorization": "Bearer task-grant"},
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["schema_version"], "futures.report.v1")
        self.assertNotIn("published", response.json())

    def test_model_grant_and_request_use_the_same_canonical_arguments(self):
        issued = {}

        async def handle(request):
            body = json.loads(request.content)
            if request.url.path.endswith("/grants"):
                issued.update(body)
                return httpx.Response(201, json={"data": {"grant": "offline-grant"}})
            self.assertEqual(issued["args_hash"], hash_arguments(body["arguments"]))
            return httpx.Response(200, json={"data": {"content": "{}"}})

        async def run():
            scope = ExecutionScope(
                domain="futures", mode="live", owner_id=1, run_id="r",
                task_id="t", generation=1, manifest_id="m",
                as_of="2026-10-05T00:00:00Z",
            )
            messages = [{"role": "system", "content": "support"}]
            async with httpx.AsyncClient(transport=httpx.MockTransport(handle)) as client:
                broker = HTTPGrantBroker("http://offline.invalid", "test", client)
                model = HTTPModelGateway("http://offline.invalid", "test", client)
                grant = await broker.issue(scope, "support", None, {"stage": "support", "messages": messages})
                await model.complete(scope, "support", messages, grant)

        asyncio.run(run())

    def test_execute_rejects_unverified_task_grant(self):
        response = self.client.post(
            "/internal/futures/v1/execute", json=TASK,
            headers={"X-Futures-Service-Token": "test-service-token", "Authorization": "Bearer invented-grant"},
        )
        self.assertEqual(response.status_code, 403)

    def test_canonical_hash_vector_is_stable(self):
        fixture = json.loads((Path(__file__).resolve().parents[3] / "contracts" / "futures" / "v1" / "test-vectors" / "canonical-arguments.json").read_text())
        self.assertEqual(hash_arguments(fixture["arguments"]), fixture["sha256"])
