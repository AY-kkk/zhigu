from __future__ import annotations

from app.tools import bind_tools
from app.tools.document import read_document_spans


class FakeClient:
    def __init__(self):
        self.calls = []

    def grant(self, tool_name, args):
        self.calls.append(("grant", tool_name, args))
        return "grant_doc"

    def data_query(self, grant_id, operation, params):
        self.calls.append(("query", grant_id, operation, params))
        return {
            "records": [
                {
                    "record_id": "prec_1",
                    "record": {
                        "title": "report.txt",
                        "locator": "page 1, paragraph 0",
                        "text": "研报认为现金流会改善。",
                        "source_kind": "user_report",
                    },
                }
            ],
            "quality_status": "reported_only",
            "warnings": ["user_report"],
        }

    def register_evidence(self, grant_id, record_ids):
        self.calls.append(("register", grant_id, record_ids))
        return ["ev_doc_1"]

    def complete(self, grant_id, status, output_hash):
        self.calls.append(("complete", grant_id, status, output_hash))


def test_read_document_spans_registers_report_only_evidence():
    client = FakeClient()
    out = read_document_spans(client, "doc_1", query="现金流", limit=5)
    assert out["evidence_ids"] == ["ev_doc_1"]
    assert out["verification_status"] == "reported_only"
    assert out["spans"][0]["text"] == "研报认为现金流会改善。"
    assert [row[0] for row in client.calls] == ["grant", "query", "register", "complete"]


def test_document_tool_is_in_exact_allowlist():
    tools = bind_tools("run_1", "task_1", "token")
    assert {tool.name for tool in tools} == {
        "get_financials",
        "search_filings",
        "calculate_metric",
        "read_document_spans",
    }


def test_document_result_marks_report_statements_not_independent():
    from app.jobs import document_result
    from app.schemas import ResearchTask

    task = ResearchTask(
        schema_version="1.0",
        run_id="run_1",
        task_id="task_1",
        role="challenger",
        claim={"text": "研报观点", "horizon": "2026-01-01/2026-12-31", "items": []},
        instrument_id="DEMO:COMPANY",
        as_of="2026-01-01T00:00:00Z",
        mode="fixture",
        source_policy_version="s1",
        model_config_version="m1",
        prompt_version="p1",
        max_model_calls=1,
        max_tool_calls=1,
        deadline_at="2026-01-01T00:01:00Z",
        document_id="doc_1",
        input_mode="report_only",
    )
    result = document_result(
        task,
        {
            "evidence_ids": ["ev_doc_1"],
            "spans": [{"text": "现金流会改善。", "verification_status": "reported_only"}],
        },
    )
    assert result.counterevidence[0].verification_status == "reported_only"
    assert result.arguments == []
