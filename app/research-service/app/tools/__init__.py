from __future__ import annotations

from app.tools.calculator import calculate_metric
from app.tools.financials import get_financials
from app.tools.document import read_document_spans
from app.tools.filings import search_filings


class NamedTool:
    def __init__(self, name: str, fn=None):
        self.name = name
        self._fn = fn

    def invoke(self, args: dict):
        if self._fn is None:
            return {"ok": False}
        return self._fn(**args)


get_financials_tool = NamedTool("get_financials")
search_filings_tool = NamedTool("search_filings")
calculate_metric_tool = NamedTool("calculate_metric")
read_document_spans_tool = NamedTool("read_document_spans")
FINANCE_TOOLS = [get_financials_tool, search_filings_tool, calculate_metric_tool, read_document_spans_tool]


def bind_tools(run_id: str, task_id: str, task_token: str = "") -> list[NamedTool]:
    from app.tools.provider import GoClient

    client = GoClient(run_id, task_id, task_token)

    def _fin(metrics, periods):
        return get_financials(client, metrics, periods)

    def _fil(query, limit=5):
        return search_filings(client, query, limit)

    def _calc(operation, inputs):
        return calculate_metric(client if client.enabled() else None, operation, inputs)

    def _doc(document_id, query="", limit=5, span_ids=None):
        return read_document_spans(client, document_id, query, limit, span_ids)

    return [
        NamedTool("get_financials", _fin),
        NamedTool("search_filings", _fil),
        NamedTool("calculate_metric", _calc),
        NamedTool("read_document_spans", _doc),
    ]
