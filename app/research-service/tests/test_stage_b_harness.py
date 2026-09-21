from __future__ import annotations

import sys
from pathlib import Path

from app.deerflow_adapter import ALLOWED_TOOLS, FinanceDeerFlowAdapter, assert_exact_tool_allowlist
from app.tools import FINANCE_TOOLS

ROOT = Path(__file__).resolve().parents[3]
PROMPTS = Path(__file__).resolve().parents[1] / "app" / "prompts"
THREE_STATEMENT_MARKERS = (
    "revenue",
    "operating_cash_flow_net",
    "total_assets",
    "get_financials",
    "search_filings",
    "calculate_metric",
)


def test_stage_b_three_tools_only():
    names = {t.name for t in FINANCE_TOOLS}
    assert_exact_tool_allowlist(names)
    assert names == set(ALLOWED_TOOLS)
    adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
    client = type("C", (), {"_agent": None})()
    assert adapter.actual_tool_names(client) == set(ALLOWED_TOOLS)


def test_stage_b_role_prompts_cover_three_statements():
    supporter = (PROMPTS / "supporter.md").read_text(encoding="utf-8")
    challenger = (PROMPTS / "challenger.md").read_text(encoding="utf-8")
    for text in (supporter, challenger):
        for marker in THREE_STATEMENT_MARKERS:
            assert marker in text, marker
        assert "MCP" in text
        assert "子 Agent" in text


def test_stage_b_actual_harness_import():
    client_py = ROOT / "references" / "deer-flow" / "backend" / "packages" / "harness" / "deerflow" / "client.py"
    source = client_py.read_text(encoding="utf-8")
    assert "class DeerFlowClient" in source
    assert "subagent_enabled: bool = False" in source
    sys.path.insert(0, str(ROOT / "references" / "deer-flow" / "backend" / "packages" / "harness"))
    adapter = FinanceDeerFlowAdapter(tools=FINANCE_TOOLS)
    cls = adapter.import_harness()
    assert cls.__name__ == "DeerFlowClient"


def test_stage_b_python_has_no_business_db_credentials():
    jobs_src = (Path(__file__).resolve().parents[1] / "app" / "jobs.py").read_text(encoding="utf-8")
    main_src = (Path(__file__).resolve().parents[1] / "app" / "main.py").read_text(encoding="utf-8")
    joined = jobs_src + "\n" + main_src
    assert "finance_users" not in joined
    assert "DATABASE_URL" not in joined
    assert "ZHIGU_RESEARCH_SQLITE" in jobs_src
