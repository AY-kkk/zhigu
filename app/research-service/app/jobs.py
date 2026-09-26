from __future__ import annotations

import os
from pathlib import Path
import sqlite3
import threading
from typing import Any

from app.schemas import ResearchResult, ResearchTask, ResultError, Usage

_lock = threading.Lock()
_sema = threading.Semaphore(4)
DB_PATH = Path(os.environ.get("ZHIGU_RESEARCH_SQLITE", "data/jobs.sqlite"))
CONN: sqlite3.Connection | None = None
TERMINAL = {"succeeded", "failed", "canceled", "insufficient"}


def connect() -> sqlite3.Connection:
    global CONN
    if CONN is not None:
        return CONN
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH, check_same_thread=False)
    conn.row_factory = sqlite3.Row
    conn.isolation_level = None
    conn.execute(
        """CREATE TABLE IF NOT EXISTS tasks (
        task_id TEXT PRIMARY KEY,
        payload_hash TEXT NOT NULL,
        status TEXT NOT NULL,
        result TEXT,
        payload TEXT NOT NULL,
        acked INTEGER NOT NULL DEFAULT 0
        )"""
    )
    try:
        conn.execute("ALTER TABLE tasks ADD COLUMN acked INTEGER NOT NULL DEFAULT 0")
    except sqlite3.OperationalError:
        pass
    conn.commit()
    CONN = conn
    return conn


def payload_hash(data: dict[str, Any]) -> str:
    import hashlib
    import json

    return hashlib.sha256(json.dumps(data, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()


class Conflict(Exception):
    def __init__(self, code: str):
        self.code = code


def _snapshot(row: sqlite3.Row) -> dict[str, Any]:
    import json

    result = json.loads(row["result"]) if row["result"] else None
    return {"task_id": row["task_id"], "status": row["status"], "result": result}


def submit(task: ResearchTask, task_token: str = "") -> dict[str, str]:
    import json

    body = task.model_dump(mode="json")
    h = payload_hash(body)
    conn = connect()
    with _lock:
        row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task.task_id,)).fetchone()
        if row:
            if row["payload_hash"] != h:
                raise Conflict("TASK_PAYLOAD_CONFLICT")
            return {"task_id": task.task_id, "status": row["status"]}
        conn.execute(
            "INSERT INTO tasks(task_id, payload_hash, status, payload) VALUES (?,?,?,?)",
            (task.task_id, h, "queued", json.dumps(body)),
        )
        conn.commit()
    threading.Thread(target=_run, args=(task, task_token), daemon=True).start()
    return {"task_id": task.task_id, "status": "queued"}


def get(task_id: str) -> dict[str, Any]:
    conn = connect()
    with _lock:
        row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        if not row:
            raise KeyError(task_id)
        return _snapshot(row)


def cancel(task_id: str) -> dict[str, Any]:
    conn = connect()
    with _lock:
        row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        if not row:
            raise KeyError(task_id)
        if row["status"] in TERMINAL:
            return _snapshot(row)
        conn.execute("UPDATE tasks SET status = ? WHERE task_id = ?", ("canceled", task_id))
        conn.commit()
        row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        return _snapshot(row)


def mark_running_failed_on_restart() -> None:
    import json

    conn = connect()
    with _lock:
        rows = conn.execute(
            "SELECT task_id, payload FROM tasks WHERE status IN ('running','queued')"
        ).fetchall()
        for row in rows:
            payload: dict[str, Any] = {}
            try:
                payload = json.loads(row["payload"] or "{}")
            except json.JSONDecodeError:
                payload = {}
            failed = ResearchResult(
                run_id=str(payload.get("run_id") or "unknown"),
                task_id=row["task_id"],
                status="failed",
                arguments=[],
                evidence_ids=[],
                unknowns=[],
                counterevidence=[],
                usage=Usage(model_calls=0, tool_calls=0, input_tokens=0, output_tokens=0, usage_unknown=True, simulated=False),
                errors=[ResultError(code="WORKER_RESTARTED", message="process restarted", retryable=False)],
            )
            conn.execute(
                "UPDATE tasks SET status = ?, result = ? WHERE task_id = ?",
                ("failed", failed.model_dump_json(), row["task_id"]),
            )
        conn.commit()


def purge_private_copies() -> None:
    conn = connect()
    with _lock:
        conn.execute(
            "UPDATE tasks SET payload = '', result = '' WHERE status IN ('succeeded','failed','canceled','insufficient')"
        )
        conn.commit()


def purge_task(task_id: str) -> None:
    conn = connect()
    with _lock:
        row = conn.execute("SELECT task_id FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        if not row:
            raise KeyError(task_id)
        conn.execute("UPDATE tasks SET payload = '', result = '' WHERE task_id = ?", (task_id,))
        conn.commit()


def recover_on_start() -> None:
    connect()
    mark_running_failed_on_restart()
    conn = connect()
    with _lock:
        conn.execute(
            "UPDATE tasks SET payload = '', result = '' WHERE acked = 1 AND status IN ('succeeded','failed','canceled','insufficient')"
        )
        conn.commit()


def ack_task(task_id: str) -> None:
    conn = connect()
    with _lock:
        row = conn.execute("SELECT task_id FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        if not row:
            raise KeyError(task_id)
        conn.execute("UPDATE tasks SET acked = 1 WHERE task_id = ?", (task_id,))
        conn.commit()


def _zero_usage(*, simulated: bool) -> Usage:
    return Usage(
        model_calls=0,
        tool_calls=0,
        input_tokens=0,
        output_tokens=0,
        usage_unknown=False,
        simulated=simulated,
    )


def _failed_result(task: ResearchTask, code: str, message: str) -> ResearchResult:
    return ResearchResult(
        run_id=task.run_id,
        task_id=task.task_id,
        status="failed",
        arguments=[],
        evidence_ids=[],
        unknowns=[],
        counterevidence=[],
        usage=_zero_usage(simulated=False),
        errors=[ResultError(code=code, message=message[:200], retryable=False)],
    )


def _save_result(conn: sqlite3.Connection, task_id: str, result: ResearchResult) -> None:
    with _lock:
        current = conn.execute("SELECT status FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        if current and current["status"] == "canceled":
            return
        conn.execute(
            "UPDATE tasks SET status = ?, result = ? WHERE task_id = ?",
            (result.status, result.model_dump_json(), task_id),
        )
        conn.commit()


def _run(task: ResearchTask, task_token: str = "") -> None:
    conn = connect()
    with _sema:
        with _lock:
            conn.execute(
                "UPDATE tasks SET status = ? WHERE task_id = ? AND status = ?",
                ("running", task.task_id, "queued"),
            )
            conn.commit()
            row = conn.execute("SELECT status FROM tasks WHERE task_id = ?", (task.task_id,)).fetchone()
        if not row or row["status"] != "running":
            return
        from app.deerflow_adapter import FinanceDeerFlowAdapter
        from app.tools import FINANCE_TOOLS, bind_tools

        executor = os.environ.get("ZHIGU_RESEARCH_EXECUTOR", "").strip().lower()
        mode = os.environ.get("ZHIGU_RESEARCH_MODE", "").strip().lower()
        gateway = os.environ.get("ZHIGU_GO_INTERNAL_URL", "").strip()
        adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
        if executor != "fixture":
            try:
                adapter.import_harness()
                client = adapter.build_client(token=task_token or "fixture-task-token", thread_id=task.task_id)
                tools = bind_tools(task.run_id, task.task_id, task_token)
                adapter._tools = tools

                def _get_tools(*, model_name=None, subagent_enabled=False):
                    return tools

                client._get_tools = _get_tools  # type: ignore[method-assign]
                trace = adapter.complete_tool_roundtrip(client)
            except Exception as exc:  # noqa: BLE001
                _save_result(conn, task.task_id, _failed_result(task, "HARNESS_UNAVAILABLE", str(exc)))
                return
            result = fixture_result(task)
            result.usage = Usage(
                model_calls=0,
                tool_calls=1,
                input_tokens=0,
                output_tokens=0,
                usage_unknown=False,
                simulated=False,
            )
            ids = [i for i in (trace.get("evidence_ids") or []) if i]
            if ids:
                from app.schemas import Argument

                result.evidence_ids = ids
                result.arguments = [
                    Argument(claim_type="fact", text=str(trace.get("text") or "tool result"), evidence_ids=ids)
                ]
            result.status = "insufficient"
            _save_result(conn, task.task_id, result)
            return
        result = fixture_result(task)
        if not gateway:
            if mode == "unit-test":
                _save_result(conn, task.task_id, result)
                return
            _save_result(conn, task.task_id, _failed_result(task, "MISSING_GO_GATEWAY", "ZHIGU_GO_INTERNAL_URL is required"))
            return
        try:
            tools = bind_tools(task.run_id, task.task_id, task_token)
            if task.role == "supporter":
                out = tools[0].invoke({"metrics": ["revenue"], "periods": ["2025", "2024"]})
            else:
                out = tools[1].invoke({"query": "risk counterevidence", "limit": 3})
            ids = [i for i in (out.get("evidence_ids") or []) if i]
            result.usage = Usage(
                model_calls=0,
                tool_calls=1,
                input_tokens=0,
                output_tokens=0,
                usage_unknown=False,
                simulated=False,
            )
            if ids:
                from app.schemas import Argument

                result.evidence_ids = ids
                result.arguments = [
                    Argument(claim_type="fact", text=str(out.get("data", {}).get("text", "fixture")), evidence_ids=ids)
                ]
            result.status = "insufficient"
        except Exception as exc:  # noqa: BLE001
            _save_result(conn, task.task_id, _failed_result(task, "TOOL_PATH_FAILED", str(exc)))
            return
        _save_result(conn, task.task_id, result)


def fixture_result(task: ResearchTask) -> ResearchResult:
    from app.schemas import Argument

    unknowns = ["无法取数：缺少利润、现金流、估值与市场预期资料。"]
    arguments = []
    if task.role == "supporter" and task.instrument_id == "DEMO:COMPANY":
        arguments = [
            Argument(
                claim_type="assumption",
                text="虚构测试数据中，演示公司的营业收入同比增长，尚不足以推断股价。",
                evidence_ids=[],
            )
        ]
    return ResearchResult(
        run_id=task.run_id,
        task_id=task.task_id,
        status="insufficient",
        arguments=arguments,
        evidence_ids=[],
        unknowns=unknowns,
        counterevidence=[],
        usage=_zero_usage(simulated=True),
        errors=[],
    )
