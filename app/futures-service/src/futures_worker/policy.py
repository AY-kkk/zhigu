"""Local defense in depth; the Go grant gateway remains authoritative."""

ALLOWED_TOOLS = frozenset({
    "futures_get_observations",
    "futures_get_evidence",
    "futures_calculate",
})


def require_tool(name: str) -> str:
    if name not in ALLOWED_TOOLS:
        raise ValueError("FUTURES_TOOL_DENIED")
    return name


def validate_scope(*, domain: str, mode: str, owner_id: int, run_id: str) -> None:
    # P0 exposes no demo executor. Offline fixtures use test-only adapters.
    if (
        domain != "futures"
        or mode != "live"
        or type(owner_id) is not int
        or owner_id <= 0
        or not isinstance(run_id, str)
        or not run_id.strip()
    ):
        raise ValueError("FUTURES_SCOPE_DENIED")
