from __future__ import annotations

import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "app/scripts/research_viewpoint_manifest.py"


def load_manifest():
    spec = importlib.util.spec_from_file_location("research_viewpoint_manifest", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def test_manifest_fails_closed_when_required_test_missing(tmp_path):
    module = load_manifest()
    results = tmp_path / "results.tsv"
    results.write_text("research\tapp/server\tTestParseInputMode\tpass\n", encoding="utf-8")
    rows = module.read_results(results)
    status, _ = module.status_for("RV-02", rows)
    assert status == "missing"


def test_manifest_does_not_convert_skip_to_pass(tmp_path):
    module = load_manifest()
    results = tmp_path / "results.tsv"
    results.write_text("research\tapp/server\tTestDocumentDOCXExtraction\tskip\n", encoding="utf-8")
    rows = module.read_results(results)
    status, _ = module.status_for("RV-03", rows)
    assert status == "blocked"
