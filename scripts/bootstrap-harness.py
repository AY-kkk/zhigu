#!/usr/bin/env python3
"""Verify a maintainer-only local harness checkout when a pin file exists."""
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def git(*args, cwd):
    return subprocess.check_output(["git", *args], cwd=cwd, text=True).strip()


def main():
    lock_path = ROOT / "references/repos.lock.json"
    if not lock_path.is_file():
        raise SystemExit("Local pin file is not published with this repository.")
    lock = json.loads(lock_path.read_text())
    repos = lock.get("repositories") or []
    repo = next((item for item in repos if item.get("name") == "deer-flow"), None)
    if not repo or not repo.get("url") or not repo.get("commit") or not repo.get("path"):
        raise SystemExit("Local pin file is missing the harness entry.")
    target = ROOT / repo["path"]
    if target.exists():
        if not (target / ".git").exists():
            raise SystemExit("Existing reference is not a Git checkout; preserved")
        if git("remote", "get-url", "origin", cwd=target) != repo["url"]:
            raise SystemExit("Existing origin differs; preserved")
        if git("status", "--porcelain", cwd=target):
            raise SystemExit("Existing checkout contains changes; preserved")
        if git("rev-parse", "HEAD", cwd=target) != repo["commit"]:
            raise SystemExit("Existing checkout has a different commit; preserved")
    else:
        target.mkdir(parents=True)
        git("init", cwd=target)
        git("remote", "add", "origin", repo["url"], cwd=target)
        git("fetch", "--depth=1", "origin", repo["commit"], cwd=target)
        git("checkout", "--detach", "FETCH_HEAD", cwd=target)
    if git("rev-parse", "HEAD", cwd=target) != repo["commit"]:
        raise SystemExit("Commit verification failed")
    print("Verified local harness pin")


if __name__ == "__main__":
    main()
