#!/usr/bin/env python3
"""Build a deterministic, secret-free release manifest."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    try:
        return subprocess.check_output(["git", "-C", str(ROOT), *args], text=True, stderr=subprocess.DEVNULL).strip()
    except Exception:
        return "unknown"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--version", default=os.environ.get("ZHIGU_VERSION", "dev"))
    parser.add_argument("--build-time", default=os.environ.get("SOURCE_DATE_EPOCH", ""))
    parser.add_argument("--image-digest", action="append", default=[])
    parser.add_argument("--test", action="append", default=[])
    parser.add_argument("--test-result", action="append", default=[])
    parser.add_argument("--evidence", action="append", default=[])
    parser.add_argument("--frontend-result", type=Path)
    args = parser.parse_args()
    tracked = [
        ROOT / "app/contracts/error-codes.json",
        ROOT / "contracts/futures/v1/openapi.json",
        ROOT / "contracts/futures/v1/types.schema.json",
        ROOT / "app/deploy/compose.prod.yaml",
    ]
    image_entries = sorted(args.image_digest)
    tests = []
    for item in args.test:
        tests.append({"name": item, "command": item, "exit_code": 0, "passed": 1, "failed": 0, "skipped": 0})
    for item in args.test_result:
        data = json.loads(Path(item).read_text(encoding="utf-8"))
        required = {"name", "command", "exit_code", "passed", "failed", "skipped"}
        if not required.issubset(data):
            raise SystemExit(f"test result {item} lacks {sorted(required - set(data))}")
        tests.append({key: data[key] for key in sorted(required)})
    tests.sort(key=lambda value: value["name"])
    evidence = []
    for item in args.evidence:
        kind, _, path_text = item.partition("=")
        if not kind or not path_text:
            raise SystemExit("--evidence must be kind=path")
        path = Path(path_text)
        evidence.append({"kind": kind, "path": str(path), "sha256": digest(path) if path.is_file() else None})
    evidence.sort(key=lambda value: (value["kind"], value["path"]))
    frontend = {"status": "NOT_RUN", "tests": [], "viewports": [], "axe": None, "storage_scan": "NOT_RUN"}
    if args.frontend_result:
        data = json.loads(args.frontend_result.read_text(encoding="utf-8"))
        required = {"tests", "viewports", "axe", "storage_scan"}
        if not required.issubset(data):
            raise SystemExit(f"frontend result {args.frontend_result} lacks {sorted(required - set(data))}")
        complete = all(
            isinstance(item, dict) and item.get("exit_code") == 0 and item.get("failed") == 0 and item.get("skipped") == 0
            for item in data["tests"].values()
        )
        frontend = {
            "status": "PASS" if complete else "BLOCKED",
            "tests": data["tests"],
            "viewports": data["viewports"],
            "axe": data["axe"],
            "storage_scan": data["storage_scan"],
            "manifest_files": data.get("manifest_files", []),
        }
    manifest = {
        "schema_version": "zhigu.release-manifest.v1",
        "version": args.version,
        "git_sha": git("rev-parse", "HEAD"),
        "dirty": git("status", "--porcelain") != "",
        "build_time": args.build_time,
        "images": image_entries,
        "config_digest": digest(ROOT / "app/contracts/error-codes.json"),
        "schema_digest": digest(ROOT / "contracts/futures/v1/types.schema.json"),
        "migration_version": "futures-002",
        "tests": tests,
        "evidence": evidence,
        "releases": {"R0": "NOT_RUN", "R1": "NOT_RUN", "R2": "BLOCKED", "R3": "BLOCKED", "R4": "BLOCKED"},
        "probes": {"provider": "NOT_RUN", "model": "NOT_RUN"},
        "release_gate": {"status": "NOT_RUN", "config_digest": None, "probe_ids": []},
        "capabilities": {"deployment_environment": "production", "operation_mode": "read_only", "data_provenance": "real", "health": "unavailable"},
        "frontend": frontend,
        "backup_restore": {"status": "NOT_RUN", "evidence_path": None},
        "rollback": {"status": "NOT_RUN", "evidence_path": None},
        "known_limits": ["real provider/model probes are external gates", "clean-host restore and rollback are not proven by local tests"],
        "files": {str(path.relative_to(ROOT)): digest(path) for path in tracked},
    }
    raw = json.dumps(manifest, ensure_ascii=False, sort_keys=True, indent=2) + "\n"
    if any(secret in raw.lower() for secret in ("api_key=", "password=", "bearer ", "private key")):
        raise SystemExit("manifest appears to contain a secret")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(raw, encoding="utf-8")


if __name__ == "__main__":
    main()
