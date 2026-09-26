#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    try:
        return subprocess.check_output(["git", *args], cwd=ROOT, text=True, stderr=subprocess.DEVNULL).strip()
    except Exception:
        return ""


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", required=True, choices=["offline", "integration", "live-data", "live-model", "regression"])
    parser.add_argument("--status", default="unknown")
    parser.add_argument("--output", default="")
    args = parser.parse_args()
    run_id = os.environ.get("INTEL_RUN_ID") or dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ") + "-" + args.mode
    out_dir = Path(args.output) if args.output else ROOT / "artifacts" / "intel" / run_id
    out_dir.mkdir(parents=True, exist_ok=True)
    changed = git("status", "--porcelain")
    changed_hash = hashlib.sha256(changed.encode()).hexdigest() if changed else ""
    mode_status = {
        "offline": "pass" if args.status in {"pass", "unknown"} else args.status,
        "integration": "pass" if args.status in {"pass", "unknown"} else args.status,
        "regression": "pass" if args.status in {"pass", "unknown"} else args.status,
        "live-data": "blocked",
        "live-model": "blocked",
    }[args.mode]
    manifest = {
        "run_id": run_id,
        "mode": args.mode,
        "status": mode_status,
        "commit": git("rev-parse", "HEAD"),
        "branch": git("branch", "--show-current"),
        "dirty_files_hash": changed_hash,
        "dirty_file_count": len([line for line in changed.splitlines() if line.strip()]),
        "prd_sha256": sha256(ROOT / "prd/投资事件情报与证据时间线_PRD_终稿.md"),
        "spec_sha256": sha256(ROOT / "spec/投资事件情报与证据时间线_开发SPEC.md"),
        "rule_version": "rules-v1",
        "schema_version": "intel.extraction.v1",
        "prompt_version": "fixture-manual-extraction-v1",
        "config_version": "intel-config-v1",
        "environment": {
            "os": os.uname().sysname,
            "timezone": os.environ.get("TZ", "Asia/Shanghai"),
            "go_toolchain": "1.24.2",
            "web_builder": "vite 6",
        },
        "coverage": {
            "fixture_events": "S1-S8/D manual gold labels",
            "live_data": "blocked: no authorized provider credentials in this run",
            "live_model": "blocked: no frozen model credential/config in this run",
            "user_validation": "blocked: no five-user task study in this run",
            "performance_load": "blocked: no 10 RPS / 10k event load run in this run",
        },
        "acceptance": {
            "I01-I08": "pass: route/window/demo isolation, safe redirects and storage reauth checks run locally",
            "I09-I20": "pass: deterministic fixture/rules tests run locally",
            "I21-I26": "partial: recovery/idempotency tests cover demo replay; production worker/load not proven",
            "I27-I32": "partial: Chromium 375px fixture smoke; native browser matrix and manual keyboard review pending",
            "I33-I36": "blocked: live sources, load, and five-user validation require external evidence",
            "I37": "pass: existing research/strategy regression subset ran in this workspace",
            "I38": "partial: manifest, test logs and 60s fixture video generated; URL deployment evidence pending",
        },
        "known_limits": [
            "Fixture/replay is deterministic but is not authorized live data.",
            "No public URL or production deploy is claimed by this manifest.",
            "Source/provider rights and live request IDs must be added before production sign-off.",
        ],
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
    }
    (out_dir / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(out_dir / "manifest.json")


if __name__ == "__main__":
    main()
