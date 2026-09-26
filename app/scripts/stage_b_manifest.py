#!/usr/bin/env python3
"""Build the stage-B acceptance manifest (research + strategy reported separately).

Input: a TSV of executed test nodes (domain<TAB>package<TAB>test<TAB>status).
Output: manifest.json per spec/验收与接入清单.md §4 — every requirement
carries exact test nodes and counts; a single total boolean is never enough.
"""
import argparse
import datetime
import json
import os
import sys

PASS, FAIL, SKIP = "pass", "fail", "skip"

# strategy B-S-ID -> (gate, modes, [test name substrings], note)
STRATEGY_REQS = {
    "B-S-01": ("BS2", ["integration"], ["strategy-market.spec.js"], "受控 UI；真 Go+DB 链路由 B-S-07 分层覆盖"),
    "B-S-02": ("BS1", ["offline"], ["TestMigrate", "TestSplitStatements", "TestParseSchemaObjects"], "隔离真实 PostgreSQL"),
    "B-S-03": ("BS1", ["offline"], ["TestPublication", "TestValidateRecords", "TestStrategyMarketAuth", "TestStrategyMarketConsumer"], "含普通用户403与未发布404"),
    "B-S-04": ("BS3", ["offline"], ["TestCopy"], "含并发同key与删副本410"),
    "B-S-05": ("BS3", ["offline"], ["TestDraftEditorPatchCAS", "TestSaveStrategy", "TestExplain", "TestModify", "TestExistingStrategy", "TestLateModify", "TestStrategyHTTP", "TestLive"], "含局部修改保真/无模型解释不伪成功/base_version 强制"),
    "B-S-06": ("BS3", ["offline"], ["service/backtest", "TestCompile", "TestRejectUnknown", "TestWeekly", "TestEditor", "TestMarketTemplate", "TestContracts"], "v1 重放、v2/lag、连续性检查进真实引擎"),
    "B-S-07": ("BS4", ["integration"], ["strategy-market-chain", "strategy.spec.js", "TestCopy", "TestSaveStrategy"], "真实 Vue→Go→PG 链路 + Go 集成分层"),
    "B-S-08": ("BS1", ["offline"], ["TestEvidence"], "含公开 DTO 无 source_run_id 断言"),
    "B-S-09": ("BS4", ["offline"], ["service/finance", "service/market", "TestStrategyHTTP"], "研究/行情/workbench 回归"),
    "B-S-10": ("BS4", ["live"], [], "正式内容来源审核 + live 分项"),
}

# research B-ID -> (gate, modes, [test name substrings])
RESEARCH_REQS = {
    "B-01": ("G0", ["offline"], ["test_b01_scope_manifest"]),
    "B-02": ("G0", ["offline"], ["test_b02_gate_prerequisites"]),
    "B-03": ("G0", ["offline"], ["TestStageBIdentityAndRoutes"]),
    "B-04": ("G2", ["offline"], ["test_b04_runtime_provenance"]),
    "B-05": ("G5", ["integration"], ["b05", "stage-b.spec.js"]),
    "B-06": ("G2", ["offline"], ["TestStageBConfigCapabilityActivation"]),
    "B-07": ("G2", ["offline"], ["TestStageBConfigSnapshotIsolation"]),
    "B-08": ("G0", ["offline"], ["TestStageBLiveFailClosedAndOwnership"]),
    "B-09": ("G2", ["offline"], ["test_b09_contract_scope"]),
    "B-10": ("G3", ["offline"], ["TestStageBGrantScopeAndSingleExecution"]),
    "B-11": ("G3", ["offline"], ["TestStageBEvidenceRecordHandleIntegrity"]),
    "B-12": ("G2", ["offline"], ["TestStageBRequestIdempotencyAndUnknown"]),
    "B-13": ("G3", ["offline"], ["TestStageBConnectorEgressContract"]),
    "B-14": ("G2", ["offline"], ["test_b14_actual_harness_tool_roundtrip"]),
    "B-15": ("G2", ["offline"], ["test_b15_final_tools_and_middleware"]),
    "B-16": ("G2", ["offline"], ["test_b16_task_credentials_and_memory_isolation"]),
    "B-17": ("G4", ["offline"], ["test_b17_role_limits_enforced"]),
    "B-18": ("G3", ["live"], ["test_b18_live_provider_capabilities"]),
    "B-19": ("G3", ["offline"], ["TestStageBTimeAndRevisionGate"]),
    "B-20": ("G3", ["offline"], ["test_b20_source_passage_provenance"]),
    "B-21": ("G3", ["offline"], ["TestStageBDecimalAndMetricBasis"]),
    "B-22": ("G4", ["offline"], ["TestStageBWholeReportEvidenceGate"]),
    "B-23": ("G4", ["offline"], ["TestStageBRepairOnceAndRejectedDraftHidden"]),
    "B-24": ("G4", ["offline"], ["TestStageBParallelRolesAndOutcomeSemantics"]),
    "B-25": ("G4", ["offline"], ["TestStageBAtomicBudgetAndBilling"]),
    "B-26": ("G4", ["offline"], ["TestStageBCancelLeaseAndLatePublish"]),
    "B-27": ("G4", ["offline"], ["test_b27_delete_restart_and_no_replay"]),
    "B-28": ("G4", ["offline"], ["TestStageBDeadlineQueueAndConcurrency"]),
    "B-29": ("G3", ["offline"], ["TestStageBInjectionSSRFAndSecretRedaction"]),
    "B-30": ("G5", ["offline"], ["test_b30_fixed_quality_suite"]),
    "B-31": ("G5", ["live"], ["b31", "stage-b-live.spec.js"]),
    "B-32": ("G5", ["offline"], ["TestStageBQuestionsEvidenceOnly"]),
    "B-33": ("G5", ["offline"], ["test_b33_manifest_fail_closed"]),
    "B-34": ("G5", ["live"], ["test_b34_live_sample_summary"]),
    "B-35": ("G5", ["offline"], ["test_b35_handoff_completeness"]),
}

GATE_REFS = {
    "G0": ["B-01", "B-02", "B-03", "B-08"],
    "G1": ["B-18"],
    "G2": ["B-04", "B-06", "B-07", "B-09", "B-12", "B-14", "B-15", "B-16"],
    "G3": ["B-10", "B-11", "B-13", "B-19", "B-20", "B-21", "B-29"],
    "G4": ["B-17", "B-22", "B-23", "B-24", "B-25", "B-26", "B-27", "B-28"],
    "G5": ["B-05", "B-30", "B-31", "B-32", "B-33", "B-34", "B-35"],
    "BS0": [],
    "BS1": ["B-S-02", "B-S-03", "B-S-08"],
    "BS2": ["B-S-01"],
    "BS3": ["B-S-04", "B-S-05", "B-S-06"],
    "BS4": ["B-S-07", "B-S-09", "B-S-10"],
}


def load_results(path):
    rows = []
    if not os.path.exists(path):
        return rows
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            parts = line.rstrip("\n").split("\t")
            if len(parts) == 4:
                rows.append({"domain": parts[0], "package": parts[1], "test": parts[2], "status": parts[3]})
    return rows


MODE_COVER = {
    "offline": {"offline"},
    "integration": {"offline", "integration"},
    "live": {"live"},
    "all": {"offline", "integration", "live"},
}


def evidence_for(row, rel):
    pkg = row["package"]
    if "strategy-market-chain" in pkg:
        sub = f"{rel}/e2e-chain.json"
    elif pkg.startswith("app/web/e2e"):
        sub = f"{rel}/e2e-strategy.json"
    elif pkg == "research-service":
        sub = f"{rel}/research-service-pytest.log"
    elif pkg == "app/web":
        sub = f"{rel}/web-build.log"
    elif row["domain"] == "strategy":
        sub = f"{rel}/strategy/{log_slug(row['domain'], pkg)}.log"
    else:
        sub = f"{rel}/{log_slug(row['domain'], pkg)}.log"
    return sub


def assess(req_id, gate, modes, patterns, rows, mode, note="", artifacts_rel="", artifacts_dir=""):
    in_mode = any(m in MODE_COVER.get(mode, set()) for m in modes)
    matched = [r for r in rows if any(p in f"{r['package']}::{r['test']}" for p in patterns)] if patterns else []
    passed = [r for r in matched if r["status"] == PASS]
    failed = [r for r in matched if r["status"] == FAIL]
    skipped = [r for r in matched if r["status"] == SKIP]
    if not in_mode:
        status = "not_run_in_mode"
    elif failed:
        status = "failed"
    elif matched and len(passed) == len(matched):
        status = "ready_for_review"
    elif matched:
        status = "partial"
    else:
        status = "missing"
    if modes == ["live"] and not any(m in MODE_COVER.get(mode, set()) for m in ["live"]):
        status = "not_run_in_mode"
    if modes == ["live"] and mode in ("live", "all"):
        status = "blocked" if not matched else status
    logs = []
    for r in matched:
        sub = evidence_for(r, artifacts_rel)
        # evidence 链接必须指向真实存在的证据文件（R7-E3）。
        if artifacts_dir and os.path.exists(os.path.join(artifacts_dir, os.path.relpath(sub, artifacts_rel))):
            logs.append(sub)
        elif artifacts_dir and os.path.exists(os.path.join(os.path.dirname(os.path.join(artifacts_dir, "..")), sub)):
            logs.append(sub)
    logs = sorted(set(logs))
    return {
        "id": req_id,
        "domain": "strategy" if req_id.startswith("B-S-") else "research",
        "gate": gate,
        "tests": matched,
        "passed": len(passed),
        "failed": len(failed),
        "skipped": len(skipped),
        "evidence": logs,
        "review_status": status,
        "note": note,
    }


def log_slug(domain, package):
    return f"{domain}-{package}".replace("/", "_").replace(".", "_")


def gate_status(refs, by_id):
    statuses = [by_id[r]["review_status"] for r in refs if r in by_id]
    if not refs:
        return "in_progress"
    if any(s == "failed" for s in statuses):
        return "failed"
    if any(s in ("missing", "blocked") for s in statuses):
        return "blocked"
    if all(s == "ready_for_review" for s in statuses):
        return "ready_for_review"
    return "in_progress"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", required=True)
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--git-sha", required=True)
    ap.add_argument("--working-tree-hash", default="")
    ap.add_argument("--started", required=True)
    ap.add_argument("--finished", required=True)
    ap.add_argument("--runtime", default="")
    ap.add_argument("--locks", default="")
    ap.add_argument("--prereq", default="")
    ap.add_argument("--limitations", default="")
    args = ap.parse_args()

    rows = load_results(args.results)
    artifacts_dir = os.path.dirname(args.out)
    artifacts_rel = os.path.basename(artifacts_dir)
    requirements = []
    by_id = {}
    for req_id, (gate, modes, patterns, note) in STRATEGY_REQS.items():
        row = assess(req_id, gate, modes, patterns, rows, args.mode, note, artifacts_rel, artifacts_dir)
        requirements.append(row)
        by_id[req_id] = row
    for req_id, (gate, modes, patterns) in RESEARCH_REQS.items():
        row = assess(req_id, gate, modes, patterns, rows, args.mode, "", artifacts_rel, artifacts_dir)
        requirements.append(row)
        by_id[req_id] = row

    limitations = [s for s in args.limitations.split("|") if s]
    manifest = {
        "spec_version": "B-1.7",
        "git_sha": args.git_sha,
        "working_tree_hash": args.working_tree_hash,
        "runtime_versions": args.runtime,
        "lock_hashes": args.locks,
        "mode": args.mode,
        "started_at": args.started,
        "finished_at": args.finished,
        "prerequisites": [s for s in args.prereq.split("|") if s],
        "gates": {g: {"status": gate_status(refs, by_id), "requirements": refs} for g, refs in GATE_REFS.items()},
        "requirements": requirements,
        "commands": [
            "bash app/scripts/verify-stage-b.sh offline",
            "bash app/scripts/verify-stage-b.sh integration",
            "bash app/scripts/verify-stage-b.sh live",
        ],
        "limitations": limitations,
    }
    os.makedirs(os.path.dirname(args.out), exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as fh:
        json.dump(manifest, fh, ensure_ascii=False, indent=2)
    totals = {"ready_for_review": 0, "failed": 0, "missing": 0, "blocked": 0, "not_run_in_mode": 0, "partial": 0}
    for r in requirements:
        totals[r["review_status"]] = totals.get(r["review_status"], 0) + 1
    print(f"manifest written: {args.out}")
    print(f"requirement status counts: {totals}")
    # R7-E2：missing / failed / 必需空跑（partial 且零通过）映射为不完整退出码，
    # 不再允许带着缺证据的需求拿退出 0 去签收。
    incomplete = [
        r["id"]
        for r in requirements
        if r["review_status"] in ("missing", "failed")
        or (r["review_status"] == "partial" and r["passed"] == 0)
    ]
    if incomplete:
        print(f"INCOMPLETE requirements (exit 3): {incomplete}")
        sys.exit(3)


if __name__ == "__main__":
    main()
