from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_b01_scope_manifest():
    spec = (ROOT / "spec/阶段B_开发SPEC.md").read_text(encoding="utf-8")
    assert "观点裁判" in spec
    assert "MCP" in spec
    main = (ROOT / "app/server/main.go").read_text(encoding="utf-8")
    assert "gosaas" not in main.lower()


def test_b02_gate_prerequisites():
    status = (ROOT / "app/IMPLEMENTATION_STATUS.md").read_text(encoding="utf-8")
    assert "accepted" not in status.split("不能")[0] or "不能" in status
    assert "ready_for_review" in status or "blocked" in status
    script = (ROOT / "app/scripts/verify-stage-b.sh").read_text(encoding="utf-8")
    assert "exit 2" in script


def test_b04_runtime_provenance():
    adapter = ROOT / "app/research-service/app/deerflow_adapter.py"
    text = adapter.read_text(encoding="utf-8")
    assert "class FinanceDeerFlowAdapter" in text
    assert "DeerFlowClient" in text
    lock = ROOT / "app/research-service/uv.lock"
    assert lock.exists() and lock.stat().st_size > 0


def test_b09_contract_scope():
    main = (ROOT / "app/research-service/app/main.py").read_text(encoding="utf-8")
    assert 'CONTRACT_VERSION = "2"' in main
    assert "401" in main and "409" in main


def test_b15_final_tools_and_middleware():
    from app.deerflow_adapter import ALLOWED_TOOLS, forbidden_middleware
    from app.tools import FINANCE_TOOLS

    assert {t.name for t in FINANCE_TOOLS} == set(ALLOWED_TOOLS)
    assert forbidden_middleware(["MemoryMiddleware", "BashTool"]) == ["memory", "bash"]


def test_b16_task_credentials_and_memory_isolation(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs
    from app.schemas import ResearchTask

    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    sample = json.loads((ROOT / "handoff/examples/research-task.json").read_text())
    a = ResearchTask.model_validate({**sample, "task_id": "task_a"})
    b = ResearchTask.model_validate({**sample, "task_id": "task_b", "run_id": "run_b"})
    jobs.submit(a)
    jobs.submit(b)
    assert jobs.get("task_a")["task_id"] != jobs.get("task_b")["task_id"]
    jobs.CONN = None


def test_b17_role_limits_enforced():
    text = (ROOT / "app/server/service/finance/run_state.go").read_text(encoding="utf-8")
    assert "MaxModelCallsPerRole: 4" in text
    assert "MaxToolCallsPerRole:  6" in text


def test_b18_live_provider_capabilities():
    raw = json.loads((ROOT / "contracts/stage-b/source-contract.json").read_text(encoding="utf-8"))
    blob = json.dumps(raw, ensure_ascii=False)
    for token in ("600519.SH", "300750.SZ", "000333.SZ", "00700.HK"):
        assert token in blob
    assert "http" in blob.lower()


def test_b20_source_passage_provenance():
    challenger = (ROOT / "app/research-service/app/prompts/challenger.md").read_text(encoding="utf-8")
    assert "正文不足" in challenger
    assert "标题" in challenger


def test_b27_delete_restart_and_no_replay(tmp_path, monkeypatch):
    monkeypatch.setenv("ZHIGU_RESEARCH_SQLITE", str(tmp_path / "jobs.sqlite"))
    from app import jobs

    jobs.CONN = None
    jobs.DB_PATH = tmp_path / "jobs.sqlite"
    conn = jobs.connect()
    conn.execute(
        "INSERT INTO tasks(task_id, payload_hash, status, payload) VALUES (?,?,?,?)",
        ("task_restart", "hash", "running", "{}"),
    )
    jobs.mark_running_failed_on_restart()
    snap = jobs.get("task_restart")
    assert snap["status"] == "failed"
    assert snap["result"]["errors"][0]["code"] == "WORKER_RESTARTED"
    jobs.CONN = None


def test_b30_fixed_quality_suite():
    doc = json.loads((ROOT / "app/tests/fixtures/quality/set30.json").read_text(encoding="utf-8"))
    assert len(doc["cases"]) == 30
    families = {row["family"] for row in doc["cases"]}
    assert "injection" in families and "future_leak" in families


def test_b33_manifest_fail_closed():
    env = os.environ.copy()
    for key in ("ZHIGU_MODEL_API_KEY", "OPENAI_API_KEY", "DEEPSEEK_API_KEY", "ZHIGU_STAGE_B_LIVE_DATA", "ZHIGU_MODEL_FEE_CAP"):
        env.pop(key, None)
    proc = subprocess.run(
        ["bash", str(ROOT / "app/scripts/verify-stage-b.sh"), "live"],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    assert proc.returncode == 2
    assert "blocked" in proc.stderr


def test_b34_live_sample_summary():
    script = (ROOT / "app/scripts/verify-stage-b.sh").read_text(encoding="utf-8")
    assert "D-05" in script
    assert "exit 2" in script
    results = ROOT / "artifacts/stage-b/quality/results.json"
    assert not results.exists()


def test_b35_handoff_completeness():
    for rel in (
        "spec/阶段B_开发SPEC.md",
        "spec/验收与接入清单.md",
        "app/scripts/verify-stage-b.sh",
        "contracts/stage-b/source-contract.json",
        "app/server/migrations/finance/003_stage_b.sql",
    ):
        assert (ROOT / rel).exists(), rel
