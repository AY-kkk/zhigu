#!/usr/bin/env python3
import argparse
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path

REQS = {
    "RV-01": ["testparseinputmode", "testparserejectsmissinginput"],
    "RV-02": ["testdocumentuploadvalidation", "testdocumenttextextractionandownerisolation"],
    "RV-03": ["testdocumentdocxextraction", "testdocumentpdfextraction"],
    "RV-04": ["testclassifyclaimitems", "testextractclaimitemskeepssourcespan", "testextractnumbers"],
    "RV-05": ["testpatchrejectschangedresearchinput", "testcreateresearchpreservesreportinput"],
    "RV-06": ["testreaddocument_spans_registers_report_only_evidence", "testdocument_tool_is_in_exact_allowlist"],
    "RV-07": ["testreaddocument_spans_registers_report_only_evidence"],
    "RV-08": ["testdocument_result_marks_report_statements_not_independent"],
    "RV-09": ["testadjudicateclaimdoesnotignoreunresolvedchallenge"],
    "RV-10": ["testreportv2rejectsmissingrequiredsection", "testreportv2rejectsunregisteredfactevidence"],
    "RV-11": ["testresearchviewpointcrossprocessfixture"],
    "RV-12": ["research-viewpoint-input.spec.js"],
    "RV-13": ["research-viewpoint-input.spec.js"],
    "RV-14": ["testresearchviewpointcrossprocessfixture"],
    "RV-15": ["testexporthtmlhasnoscriptandhasdisclaimer"],
    "RV-16": ["evaluate-research-viewpoint"],
}

def norm(value):
    return "".join(ch for ch in str(value).lower() if ch.isalnum())

REQUIRED_BY_MODE = {
    "offline": [f"RV-{i:02d}" for i in range(1, 16)],
    "integration": [f"RV-{i:02d}" for i in range(1, 16)],
    "live": [f"RV-{i:02d}" for i in range(1, 17)],
    "eval": ["RV-16"],
}

def read_results(path):
    rows = []
    for line in Path(path).read_text().splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        parts = line.split("\t")
        if len(parts) != 4:
            raise SystemExit(f"invalid results row: {line}")
        rows.append({"domain": parts[0], "package": parts[1], "test": parts[2], "status": parts[3]})
    return rows

def status_for(req, rows):
    needles = [norm(n) for n in REQS[req]]
    matched = [row for row in rows if any(n in norm(row["test"]) for n in needles)]
    if not matched:
        return "missing", []
    if any(row["status"] == "fail" for row in matched):
        return "fail", matched
    if any(row["status"] in {"skip", "blocked", "not_run_in_mode"} for row in matched):
        return "blocked", matched
    if all(row["status"] == "pass" for row in matched):
        return "pass", matched
    return "partial", matched

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", choices=sorted(REQUIRED_BY_MODE), required=True)
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()
    rows = read_results(args.results)
    requirements = {}
    for req in sorted(REQS):
        status, matched = status_for(req, rows)
        requirements[req] = {"status": status, "tests": matched}
    required = REQUIRED_BY_MODE[args.mode]
    failed = [req for req in required if requirements[req]["status"] != "pass"]
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    try:
        commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    except Exception:
        commit = "unknown"
    manifest = {
        "schema_version": "research-viewpoint-manifest.v1",
        "mode": args.mode,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "commit": commit,
        "requirements": requirements,
        "required": required,
        "failed_required": failed,
        "results_file": str(Path(args.results).resolve().relative_to(Path.cwd().resolve())) if Path(args.results).resolve().is_relative_to(Path.cwd().resolve()) else Path(args.results).name,
        "limits": [
            "fixture and mock evidence are not live evidence",
            "RV-16 evaluation is a separate human/eval gate",
            "missing or skipped required evidence makes the manifest fail closed",
        ],
    }
    (out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"failed_required": failed, "out": str(out / "manifest.json")}, ensure_ascii=False))
    raise SystemExit(1 if failed else 0)

if __name__ == "__main__":
    main()
