"""Regression probes for stage A review defects."""
from __future__ import annotations

import json
import os
import threading
from pathlib import Path
from unittest.mock import patch

from app import jobs
from app.deerflow_adapter import FinanceDeerFlowAdapter
from app.schemas import ResearchTask


def _load_task() -> ResearchTask:
    root = Path(__file__).resolve().parents[3]
    return ResearchTask.model_validate(json.loads((root / "handoff/examples/research-task.json").read_text()))


def _reset(conn, task: ResearchTask) -> None:
    conn.execute("DELETE FROM tasks")
    conn.execute(
        "INSERT INTO tasks(task_id,payload_hash,status,payload) VALUES(?,?,?,?)",
        (task.task_id, "hash", "queued", task.model_dump_json()),
    )


def test_harness_unavailable_is_failed(tmp_path, monkeypatch):
    monkeypatch.delenv("ZHIGU_RESEARCH_EXECUTOR", raising=False)
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobs.connect()
    task = _load_task()
    _reset(conn, task)
    with (
        patch.dict(os.environ, {"ZHIGU_GO_INTERNAL_URL": ""}, clear=False),
        patch.object(FinanceDeerFlowAdapter, "import_harness", side_effect=ImportError("harness unavailable")),
    ):
        jobs._run(task)
        r = jobs.get(task.task_id)
    assert r["status"] == "failed"
    assert r["result"]["errors"]
    assert r["result"]["errors"][0]["code"] == "HARNESS_UNAVAILABLE"
    assert r["result"]["usage"]["model_calls"] == 0
    assert r["result"]["usage"]["tool_calls"] == 0


def test_tool_path_failure_is_failed(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_EXECUTOR", "fixture")
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobs.connect()
    task = _load_task()
    _reset(conn, task)

    class BrokenTool:
        def invoke(self, args):
            raise ConnectionError("simulated Go data gateway offline")

    with (
        patch.dict(os.environ, {"ZHIGU_GO_INTERNAL_URL": "http://127.0.0.1:1"}, clear=False),
        patch("app.tools.bind_tools", return_value=[BrokenTool(), BrokenTool()]),
    ):
        jobs._run(task)
        r = jobs.get(task.task_id)
    assert r["status"] == "failed"
    assert any(e["code"] == "TOOL_PATH_FAILED" for e in r["result"]["errors"])


def test_terminal_cancel_is_not_deadlock(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_EXECUTOR", "fixture")
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobs.connect()
    task = _load_task()
    _reset(conn, task)
    jobs._run(task)
    done = {"ok": False}

    def _cancel():
        jobs.cancel(task.task_id)
        done["ok"] = True

    thread = threading.Thread(target=_cancel, daemon=True)
    thread.start()
    thread.join(timeout=1)
    assert not thread.is_alive()
    assert done["ok"]
    snap = jobs.get(task.task_id)
    assert snap["status"] in {"insufficient", "failed", "canceled", "succeeded"}
    other = _load_task()
    other = other.model_copy(update={"task_id": "task_other"})
    out = jobs.submit(other)
    assert out["task_id"] == "task_other"


def test_healthz_reports_executor(monkeypatch):
    monkeypatch.delenv("ZHIGU_RESEARCH_EXECUTOR", raising=False)
    from app.main import healthz

    out = healthz()
    assert out["executor"] == "harness"
    assert out["mode"] == "harness"
    monkeypatch.setenv("ZHIGU_RESEARCH_EXECUTOR", "fixture")
    out = healthz()
    assert out["executor"] == "fixture"
    assert out["mode"] == "fixture"


def test_role_isolation_compares_run_and_task(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_EXECUTOR", "fixture")
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    a = _load_task()
    b = _load_task().model_copy(update={"task_id": "task_challenge", "role": "challenger", "run_id": "run_other"})
    jobs.submit(a)
    jobs.submit(b)
    ga = jobs.get(a.task_id)
    gb = jobs.get(b.task_id)
    assert ga["task_id"] != gb["task_id"]
    assert a.run_id != b.run_id
