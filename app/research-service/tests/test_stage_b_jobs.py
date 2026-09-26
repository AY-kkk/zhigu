from __future__ import annotations

import json
from pathlib import Path

from app import jobs
from app.schemas import ResearchTask

SAMPLE = Path(__file__).resolve().parents[3] / "handoff" / "examples" / "research-task.json"


def _task(**overrides) -> ResearchTask:
    data = json.loads(SAMPLE.read_text())
    data.update(overrides)
    return ResearchTask.model_validate(data)


def test_stage_b_jobs_submit_get_and_conflict(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    task = _task(task_id="task_support")
    first = jobs.submit(task)
    second = jobs.submit(task)
    assert first["task_id"] == second["task_id"]
    snap = jobs.get("task_support")
    assert snap["task_id"] == "task_support"
    assert snap["status"] in {"queued", "running", "succeeded", "insufficient", "failed", "canceled"}
    other = _task(task_id="task_support", prompt_version="prompt_v2")
    try:
        jobs.submit(other)
        raise AssertionError("expected conflict")
    except jobs.Conflict as exc:
        assert exc.code == "TASK_PAYLOAD_CONFLICT"
    out = jobs.cancel("task_support")
    assert out["status"] in {"canceled", "queued", "running", "succeeded", "insufficient", "failed"}
    jobs.CONN = None


def test_stage_b_jobs_restart_marks_running_failed(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobs.connect()
    conn.execute(
        "INSERT INTO tasks(task_id, payload_hash, status, payload) VALUES (?,?,?,?)",
        ("task_support", "hash", "running", "{}"),
    )
    jobs.mark_running_failed_on_restart()
    snap = jobs.get("task_support")
    assert snap["status"] == "failed"
    assert snap["result"]["errors"][0]["code"] == "WORKER_RESTARTED"
    jobs.CONN = None
