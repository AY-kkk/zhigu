#!/usr/bin/env python3
import argparse
import json
from pathlib import Path

THRESHOLDS = {
    "fact_error_rate": 0.03,
    "challenge_score": 4.0,
    "citation_coverage": 0.90,
    "link_valid_rate": 0.95,
    "numeric_errors": 0,
    "fabricated_sources": 0,
}

def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))

def validate_cases(doc):
    cases = doc.get("cases", [])
    if doc.get("case_count") != 50 or len(cases) != 50:
        raise SystemExit("evaluation set must contain exactly 50 cases")
    ids = [case.get("id") for case in cases]
    if len(set(ids)) != 50:
        raise SystemExit("evaluation case ids must be unique")
    for case in cases:
        for key in ["family", "input_mode", "text", "expected_fact_status", "expected_verdict", "required_challenge_theme"]:
            if not case.get(key):
                raise SystemExit(f"case {case.get('id')} missing {key}")
    return cases

def score(cases, predictions):
    by_id = {row.get("id"): row for row in predictions.get("cases", [])}
    if set(by_id) != {case["id"] for case in cases}:
        raise SystemExit("predictions must cover exactly the evaluation case ids")
    fact_errors = 0
    challenge_scores = []
    cited = total = valid_links = total_links = numeric_errors = fabricated_sources = 0
    for case in cases:
        row = by_id[case["id"]]
        if row.get("fact_status") != case["expected_fact_status"]:
            fact_errors += 1
        score = float(row.get("challenge_score", 0))
        if score < 1 or score > 5:
            raise SystemExit(f"{case['id']} challenge_score must be 1..5")
        challenge_scores.append(score)
        cited += int(row.get("cited_claims", 0))
        total += int(row.get("total_claims", 1))
        valid_links += int(row.get("valid_links", 0))
        total_links += int(row.get("total_links", 0))
        numeric_errors += int(row.get("numeric_errors", 0))
        fabricated_sources += int(row.get("fabricated_sources", 0))
    metrics = {
        "fact_error_rate": fact_errors / len(cases),
        "challenge_score": sum(challenge_scores) / len(challenge_scores),
        "citation_coverage": cited / total if total else 0,
        "link_valid_rate": valid_links / total_links if total_links else 0,
        "numeric_errors": numeric_errors,
        "fabricated_sources": fabricated_sources,
    }
    failures = []
    for key, threshold in THRESHOLDS.items():
        if key in {"fact_error_rate", "numeric_errors", "fabricated_sources"}:
            if metrics[key] > threshold:
                failures.append(key)
        elif metrics[key] < threshold:
            failures.append(key)
    return metrics, failures

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", required=True)
    ap.add_argument("--predictions")
    ap.add_argument("--out", required=True)
    ap.add_argument("--validate-only", action="store_true")
    args = ap.parse_args()
    cases = validate_cases(load(args.input))
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    if args.validate_only or not args.predictions:
        result = {
            "schema_version": "research-viewpoint-eval-result.v1",
            "status": "ready_for_evaluation" if args.validate_only else "blocked",
            "case_count": len(cases),
            "thresholds": THRESHOLDS,
            "metrics": None,
            "failed_thresholds": [],
            "limit": "human/model predictions have not been independently supplied",
        }
        out.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps(result, ensure_ascii=False))
        raise SystemExit(0 if args.validate_only else 2)
    metrics, failures = score(cases, load(args.predictions))
    result = {
        "schema_version": "research-viewpoint-eval-result.v1",
        "status": "pass" if not failures else "fail",
        "case_count": len(cases),
        "thresholds": THRESHOLDS,
        "metrics": metrics,
        "failed_thresholds": failures,
    }
    out.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(result, ensure_ascii=False))
    raise SystemExit(0 if not failures else 1)

if __name__ == "__main__":
    main()
