#!/usr/bin/env python3
"""Fixed-tool HTTP load runner for the production resource budget."""
from __future__ import annotations
import argparse, json, statistics, time, urllib.error, urllib.request
from pathlib import Path

def percentile(values, ratio):
    if not values:
        return None
    ordered = sorted(values)
    index = min(len(ordered) - 1, max(0, int(round((len(ordered) - 1) * ratio))))
    return ordered[index]

def run(url, rps, duration, warmup):
    deadline = time.monotonic() + warmup
    while time.monotonic() < deadline:
        request(url)
        time.sleep(max(0, 1 / rps))
    latencies, errors = [], 0
    deadline = time.monotonic() + duration
    interval = 1 / rps
    while time.monotonic() < deadline:
        started = time.perf_counter()
        try:
            request(url)
            latencies.append((time.perf_counter() - started) * 1000)
        except Exception:
            errors += 1
        time.sleep(max(0, interval - (time.perf_counter() - started)))
    count = len(latencies) + errors
    return {"requests": count, "errors": errors, "error_rate": errors / count if count else 1.0,
            "p50_ms": percentile(latencies, .50), "p95_ms": percentile(latencies, .95),
            "max_ms": max(latencies) if latencies else None}

def request(url):
    with urllib.request.urlopen(url, timeout=10) as response:
        if response.status != 200:
            raise RuntimeError(response.status)
        response.read()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--rps", type=float, required=True)
    parser.add_argument("--duration", type=float, required=True)
    parser.add_argument("--warmup", type=float, default=120)
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--max-p95-ms", type=float, required=True)
    parser.add_argument("--max-error-rate", type=float, default=.001)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    results = [run(args.url, args.rps, args.duration, args.warmup) for _ in range(args.runs)]
    passed = all((item["p95_ms"] or 1e99) <= args.max_p95_ms and item["error_rate"] <= args.max_error_rate for item in results)
    payload = {"schema_version": "zhigu.performance.v1", "command": " ".join(__import__("sys").argv), "runs": results, "pass": passed}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n")
    raise SystemExit(0 if passed else 1)

if __name__ == "__main__":
    main()
