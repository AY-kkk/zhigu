#!/usr/bin/env python3
"""Check tracked publication contents without displaying matched secrets."""
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SIGNATURES = {
    "private key": re.compile(rb"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    "GitHub token": re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})\b"),
    "cloud access key": re.compile(rb"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b"),
    "provider token": re.compile(rb"\bsk-[A-Za-z0-9_-]{25,}\b"),
    # Split so this file does not contain a literal home-directory prefix.
    "local machine path": re.compile(
        re.escape(b"/" + b"Users/") + rb"[^\s/]+/" + b"|" + re.escape(b"C:\\" + b"Users\\")
    ),
}


def main():
    files = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode().split("\0")
    errors = []
    for name in filter(None, files):
        path = Path(name)
        full = ROOT / path
        if name.startswith(("app/gosaas/", "reviews/")) or any(
            p in {"node_modules", ".venv", "__pycache__", "test-results", "playwright-report"}
            for p in path.parts
        ):
            errors.append(f"{name}: local/private artifact")
        if name.startswith("references/") and len(path.parts) > 2:
            errors.append(f"{name}: upstream source checkout")
        if (path.name.startswith(".env") and path.name != ".env.example") or path.suffix in {
            ".pem", ".key", ".sqlite", ".sqlite3", ".db", ".log"
        }:
            errors.append(f"{name}: environment, credential or runtime data")
        if full.is_symlink():
            errors.append(f"{name}: unexpected symlink")
            continue
        if not full.is_file():
            errors.append(f"{name}: tracked file missing")
            continue
        if full.stat().st_size > 10 * 1024 * 1024:
            errors.append(f"{name}: file exceeds 10 MiB")
            continue
        data = full.read_bytes()
        for label, pattern in SIGNATURES.items():
            if pattern.search(data):
                errors.append(f"{name}: possible {label}; value redacted")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"Public tree checked: {len(list(filter(None, files)))} tracked files")
    return 0


if __name__ == "__main__":
    sys.exit(main())
