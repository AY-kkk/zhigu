import pytest
import os

os.environ.setdefault("ZHIGU_RESEARCH_EXECUTOR", "fixture")
os.environ.setdefault("ZHIGU_RESEARCH_MODE", "unit-test")


@pytest.fixture(autouse=True)
def _reset_worker_quiesce_state():
    from app import jobs
    jobs.set_quiescing(False)
    yield
    jobs.set_quiescing(False)
