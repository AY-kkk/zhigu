"""Handoff-only checks: NOT application tests or investment-quality verification."""
import hashlib
import json
import subprocess
import unittest
from datetime import datetime
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker, ValidationError

ROOT = Path(__file__).resolve().parents[2]
HANDOFF = ROOT / "handoff"


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8"))


def sample(name):
    return read_json(HANDOFF / "examples" / (name + ".json"))


def validate(name, value):
    schema = read_json(HANDOFF / "contracts" / (name + ".schema.json"))
    Draft202012Validator.check_schema(schema)
    Draft202012Validator(schema, format_checker=FormatChecker()).validate(value)


def instant(value):
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timezone is mandatory")
    return parsed


def check_packet(task, result, evidence, report):
    """Minimal executable semantic probes, not the production authorization service."""
    for name, value in [
        ("research-task", task), ("research-result", result),
        ("evidence", evidence), ("report", report)
    ]:
        validate(name, value)
    if len({x["run_id"] for x in [task, result, evidence, report]}) != 1:
        raise ValueError("run mismatch")
    if task["task_id"] != result["task_id"]:
        raise ValueError("task mismatch")
    if task["instrument_id"] != evidence["instrument_id"]:
        raise ValueError("instrument mismatch")
    if len({x["mode"] for x in [task, evidence, report]}) != 1:
        raise ValueError("mode mismatch")
    if report["as_of"] != task["as_of"]:
        raise ValueError("cutoff mismatch")
    for key in ["source_policy_version", "model_config_version", "prompt_version"]:
        if task[key] != report[key]:
            raise ValueError("version mismatch")
    for key in ["published_at", "available_at"]:
        if instant(evidence[key]) > instant(task["as_of"]):
            raise ValueError("future evidence")
    if instant(evidence["available_at"]) < instant(evidence["published_at"]):
        raise ValueError("invalid publication chronology")
    if instant(evidence["retrieved_at"]) < instant(evidence["available_at"]):
        raise ValueError("invalid retrieval chronology")
    if hashlib.sha256(evidence["text"].encode("utf-8")).hexdigest() != evidence["content_hash"]:
        raise ValueError("content hash mismatch")
    allowed = {evidence["evidence_id"]}
    for container, lists in [(result, ["arguments", "counterevidence"]), (report, ["support", "challenge"])]:
        declared = set(container["evidence_ids"])
        if not declared <= allowed:
            raise ValueError("unregistered citation")
        for name in lists:
            for item in container[name]:
                if not set(item["evidence_ids"]) <= declared:
                    raise ValueError("undeclared argument citation")
    for field, limit in [("model_calls", "max_model_calls"), ("tool_calls", "max_tool_calls")]:
        if result["usage"][field] > task[limit]:
            raise ValueError("role budget exceeded")


class HandoffContracts(unittest.TestCase):
    def setUp(self):
        self.task, self.result, self.evidence, self.report = [
            sample(n) for n in ["research-task", "research-result", "evidence", "report"]
        ]

    def packet(self):
        check_packet(self.task, self.result, self.evidence, self.report)

    def test_valid_packet(self):
        self.packet()

    def test_all_schemas_and_samples(self):
        for name in ["research-task", "research-result", "evidence", "report"]:
            with self.subTest(name=name):
                validate(name, sample(name))

    def test_unknown_field_rejected(self):
        self.task["api_key"] = "must-not-be-in-payload"
        with self.assertRaises(ValidationError):
            validate("research-task", self.task)

    def test_recursive_role_rejected(self):
        self.task["role"] = "supervisor"
        with self.assertRaises(ValidationError):
            validate("research-task", self.task)

    def test_invalid_timestamp_rejected(self):
        self.evidence["published_at"] = "yesterday"
        with self.assertRaises(ValidationError):
            validate("evidence", self.evidence)

    def test_unbounded_calls_rejected(self):
        self.task["max_model_calls"] = 999
        with self.assertRaises(ValidationError):
            validate("research-task", self.task)

    def test_decimal_must_not_be_float(self):
        self.evidence["metrics"][0]["value"] = 100.0
        with self.assertRaises(ValidationError):
            validate("evidence", self.evidence)

    def test_fact_requires_citation(self):
        self.result["arguments"][0]["evidence_ids"] = []
        with self.assertRaises(ValidationError):
            validate("research-result", self.result)

    def test_incomplete_has_no_verdict(self):
        self.report["quality_status"] = "incomplete"
        with self.assertRaises(ValidationError):
            validate("report", self.report)
        self.report["verdict"] = None
        validate("report", self.report)

    def test_completed_requires_verdict(self):
        self.report["verdict"] = None
        with self.assertRaises(ValidationError):
            validate("report", self.report)

    def test_future_availability_rejected(self):
        self.evidence["available_at"] = "2026-09-18T00:00:00Z"
        with self.assertRaisesRegex(ValueError, "future"):
            self.packet()

    def test_foreign_run_rejected(self):
        self.result["run_id"] = "another_run"
        with self.assertRaisesRegex(ValueError, "run mismatch"):
            self.packet()

    def test_foreign_task_rejected(self):
        self.result["task_id"] = "another_task"
        with self.assertRaisesRegex(ValueError, "task mismatch"):
            self.packet()

    def test_unknown_citation_rejected(self):
        self.report["evidence_ids"] = ["ev_fake"]
        with self.assertRaisesRegex(ValueError, "citation"):
            self.packet()

    def test_undeclared_argument_citation_rejected(self):
        self.result["arguments"][0]["evidence_ids"] = ["ev_fake"]
        with self.assertRaisesRegex(ValueError, "citation"):
            self.packet()

    def test_content_tampering_rejected(self):
        self.evidence["text"] += " tampered"
        with self.assertRaisesRegex(ValueError, "hash"):
            self.packet()

    def test_frozen_version_mismatch_rejected(self):
        self.report["model_config_version"] = "model_v2"
        with self.assertRaisesRegex(ValueError, "version"):
            self.packet()

    def test_fixture_cannot_turn_live(self):
        self.report["mode"] = "live"
        with self.assertRaisesRegex(ValueError, "mode"):
            self.packet()

    def test_lower_task_budget_enforced(self):
        self.task["max_model_calls"] = 1
        with self.assertRaisesRegex(ValueError, "budget"):
            self.packet()

    def test_acceptance_inventory(self):
        data = read_json(HANDOFF / "acceptance-cases.json")
        cases = data["cases"]
        self.assertGreaterEqual(len(cases), 30)
        self.assertEqual(len(cases), len({x["id"] for x in cases}))
        for case in cases:
            for field in ["given", "when", "then", "implementation_task"]:
                self.assertTrue(case[field])
            self.assertEqual(case["status"], "not_implemented")

    def test_repository_pins_and_reuse_paths(self):
        lock = read_json(ROOT / "references" / "repos.lock.json")
        self.assertEqual(len(lock["repositories"]), 8)
        for repo in lock["repositories"]:
            with self.subTest(repo=repo["name"]):
                path = ROOT / repo["path"]
                def git(*args):
                    return subprocess.check_output(["git", "-C", str(path), *args], text=True).strip()
                self.assertEqual(git("rev-parse", "HEAD"), repo["commit"])
                self.assertEqual(git("remote", "get-url", "origin"), repo["url"])
                self.assertEqual(git("status", "--porcelain"), "")
                self.assertFalse((path / ".git/objects/info/alternates").exists())
                for rel in repo["reuse_files"]:
                    self.assertTrue((path / rel).is_file(), rel)


if __name__ == "__main__":
    unittest.main()
