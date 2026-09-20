"""Round-3 probes: restart must not erase unread terminal results."""
from __future__ import annotations

import json
from pathlib import Path

from app import jobs
from app.schemas import ResearchTask

ROOT = Path(__file__).resolve().parents[3]


def test_restart_preserves_unacknowledged_completed_result(tmp_path, monkeypatch):
    monkeypatch.setattr(jobs, "CONN", None)
    monkeypatch.setattr(jobs, "DB_PATH", tmp_path / "jobs.sqlite")
    task = ResearchTask.model_validate(json.loads((ROOT / "handoff/examples/research-task.json").read_text()))
    conn = jobs.connect()
    try:
        result = jobs.fixture_result(task)
        conn.execute(
            "INSERT INTO tasks(task_id,payload_hash,status,payload,result) VALUES(?,?,?,?,?)",
            (task.task_id, "hash", "insufficient", task.model_dump_json(), result.model_dump_json()),
        )
        assert jobs.get(task.task_id)["result"] is not None
        jobs.recover_on_start()
        snap = jobs.get(task.task_id)
        assert snap["result"] is not None, f"unread completed result erased on restart: {snap}"
    finally:
        conn.close()
        jobs.CONN = None
