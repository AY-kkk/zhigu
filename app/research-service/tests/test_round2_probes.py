"""Round-2 review probes absorbed as Python regression tests."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
from unittest.mock import patch

import jsonschema
import pytest
from fastapi.testclient import TestClient

from app import jobs
from app.main import app
from app.schemas import ResearchTask
from app.tools.provider import GoClient

ROOT = Path(__file__).resolve().parents[3]


@pytest.fixture
def task_db(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_EXECUTOR", "fixture")
    monkeypatch.delenv("ZHIGU_GO_INTERNAL_URL", raising=False)
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    task = ResearchTask.model_validate(json.loads((ROOT / "handoff/examples/research-task.json").read_text()))
    conn = jobs.connect()
    conn.execute(
        "INSERT INTO tasks(task_id,payload_hash,status,payload) VALUES(?,?,?,?)",
        (task.task_id, "hash", "queued", task.model_dump_json()),
    )
    yield task, conn
    conn.close()
    jobs.CONN = None


def test_restart_lifespan_marks_abandoned_running_failed(task_db):
    task, conn = task_db
    conn.execute("UPDATE tasks SET status='running'")
    with TestClient(app):
        status = jobs.get(task.task_id)["status"]
    assert status == "failed", f"startup left abandoned task {status}"


def test_cleanup_removes_private_result(task_db):
    task, conn = task_db
    result = jobs.fixture_result(task)
    conn.execute("UPDATE tasks SET status='insufficient',result=?", (result.model_dump_json(),))
    jobs.purge_private_copies()
    row = conn.execute("SELECT payload,result FROM tasks").fetchone()
    assert not row["payload"]
    assert not row["result"], "cleanup retains arguments, citations and research result"


def test_purge_task_clears_one_copy(task_db):
    task, conn = task_db
    result = jobs.fixture_result(task)
    conn.execute("UPDATE tasks SET status='insufficient',result=?", (result.model_dump_json(),))
    jobs.purge_task(task.task_id)
    row = conn.execute("SELECT payload,result FROM tasks WHERE task_id=?", (task.task_id,)).fetchone()
    assert not row["payload"]
    assert not row["result"]


def test_canceled_queue_does_not_execute_tool(task_db, monkeypatch):
    task, conn = task_db
    monkeypatch.setenv("ZHIGU_GO_INTERNAL_URL", "http://127.0.0.1:1")
    jobs.cancel(task.task_id)
    calls = []

    class Tool:
        def invoke(self, args):
            calls.append(args)
            return {"data": {}, "evidence_ids": []}

    with patch("app.tools.bind_tools", return_value=[Tool(), Tool()]):
        jobs._run(task)
    assert not calls, "canceled task still entered execution and invoked a tool"


def test_fixture_result_matches_checked_in_contract(task_db):
    task, _ = task_db
    schema = json.loads((ROOT / "app/contracts/research-result.schema.json").read_text())
    result = jobs.fixture_result(task).model_dump(mode="json")
    errors = list(jsonschema.Draft202012Validator(schema).iter_errors(result))
    assert not errors, "; ".join(e.message for e in errors)


def test_fixture_missing_gateway_is_explicit_failure(task_db, monkeypatch):
    task, _ = task_db
    monkeypatch.delenv("ZHIGU_RESEARCH_MODE", raising=False)
    monkeypatch.delenv("ZHIGU_GO_INTERNAL_URL", raising=False)
    jobs._run(task)
    snap = jobs.get(task.task_id)
    assert snap["status"] == "failed", f"no controlled tool gateway configured, returned {snap['status']}"


def test_chinese_args_hash_keeps_utf8():
    payload = {"query": "现金流风险", "limit": 3}
    raw = json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    assert "\\u" not in raw
    assert GoClient("run", "task").args_hash(payload) == hashlib.sha256(raw.encode()).hexdigest()
