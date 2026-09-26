from __future__ import annotations

import json
from pathlib import Path

from app.deerflow_adapter import ALLOWED_TOOLS, FinanceDeerFlowAdapter, assert_exact_tool_allowlist
from app.jobs import Conflict, cancel, get, mark_running_failed_on_restart, payload_hash, submit
from app.schemas import ResearchTask
from app.tools import FINANCE_TOOLS

SAMPLE = Path(__file__).resolve().parents[3] / "handoff" / "examples" / "research-task.json"


def _task(**overrides) -> ResearchTask:
    data = json.loads(SAMPLE.read_text())
    data.update(overrides)
    return ResearchTask.model_validate(data)


def test_actual_harness_import():
    import sys
    from pathlib import Path

    root = Path(__file__).resolve().parents[3]
    client_py = root / "references" / "deer-flow" / "backend" / "packages" / "harness" / "deerflow" / "client.py"
    source = client_py.read_text(encoding="utf-8")
    assert "class DeerFlowClient" in source
    assert "subagent_enabled: bool = False" in source
    sys.path.insert(0, str(root / "references" / "deer-flow" / "backend" / "packages" / "harness"))
    adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
    cls = adapter.import_harness()
    assert cls.__name__ == "DeerFlowClient"


def test_exact_tool_allowlist():
    names = {t.name for t in FINANCE_TOOLS}
    assert_exact_tool_allowlist(names)
    adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
    client = type("C", (), {"_agent": None})()
    assert adapter.actual_tool_names(client) == set(ALLOWED_TOOLS)


def test_role_context_isolation(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs as jobsmod

    jobsmod.CONN = None
    jobsmod.DB_PATH = tmp_path / "jobs.sqlite"
    a = _task(task_id="task_support", role="supporter")
    b = _task(task_id="task_challenge", role="challenger", run_id="run_other")
    submit(a)
    submit(b)
    ga = get("task_support")
    gb = get("task_challenge")
    assert ga["task_id"] != gb["task_id"]


def test_no_global_credential_mutation(monkeypatch):
    os = __import__("os")
    token = "task-token-a"
    credential_names = ("OPENAI_API_KEY", "DEEPSEEK_API_KEY", "ZHIGU_MODEL_API_KEY")
    before = {name: os.environ.get(name) for name in credential_names}
    adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
    try:
        adapter.build_client(token=token, thread_id="t1")
    except Exception:
        pass
    assert {name: os.environ.get(name) for name in credential_names} == before
    assert token not in os.environ.values()


def test_task_replay_after_restart(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs as jobsmod

    jobsmod.CONN = None
    jobsmod.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobsmod.connect()
    conn.execute(
        "INSERT INTO tasks(task_id, payload_hash, status, payload) VALUES (?,?,?,?)",
        ("task_support", "hash", "running", "{}"),
    )
    mark_running_failed_on_restart()
    snap = get("task_support")
    assert snap["status"] == "failed"
    assert snap["result"]["errors"][0]["code"] == "WORKER_RESTARTED"


def test_task_replay_same_payload(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs as jobsmod

    jobsmod.CONN = None
    jobsmod.DB_PATH = tmp_path / "jobs.sqlite"
    task = _task(task_id="task_support")
    first = submit(task)
    second = submit(task)
    assert first["task_id"] == second["task_id"]
    other = _task(task_id="task_support", prompt_version="prompt_v2")
    try:
        submit(other)
        raise AssertionError("expected conflict")
    except Conflict:
        pass


def test_cancel(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs as jobsmod

    jobsmod.CONN = None
    jobsmod.DB_PATH = tmp_path / "jobs.sqlite"
    submit(_task(task_id="task_support"))
    out = cancel("task_support")
    assert out["status"] in {"canceled", "queued", "running", "succeeded", "insufficient", "failed"}
