import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

class ReleaseEvidenceTests(unittest.TestCase):
    def test_futures_contract_verifier_validates_datetime_without_optional_plugin(self):
        script = ROOT / "scripts/verify-futures-contracts.py"
        command = (
            "import runpy, sys; "
            "sys.modules['rfc3339_validator'] = None; "
            "runpy.run_path(sys.argv[1], run_name='__main__')"
        )
        result = subprocess.run(
            [sys.executable, "-c", command, str(script)],
            cwd=ROOT.parent,
            text=True,
            capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_manifest_is_deterministic_secret_free_and_traceable(self):
        script = ROOT / "scripts/release-evidence.py"
        with tempfile.TemporaryDirectory() as directory:
            first = Path(directory) / "first.json"
            second = Path(directory) / "second.json"
            env = dict(os.environ, SOURCE_DATE_EPOCH="1770000000")
            result = Path(directory) / "test-result.json"
            result.write_text('{"name":"go","command":"go test ./...","exit_code":0,"passed":10,"failed":0,"skipped":0}')
            evidence = Path(directory) / "restore.log"
            evidence.write_text("restore verified")
            args = ["python3", str(script), "--version", "test", "--image-digest", "sha256:aaa", "--test-result", str(result), "--evidence", "backup_restore=" + str(evidence)]
            subprocess.run([*args, "--output", str(first)], check=True, env=env)
            subprocess.run([*args, "--output", str(second)], check=True, env=env)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            data = json.loads(first.read_text())
            for key in ("git_sha", "config_digest", "schema_digest", "tests", "evidence", "capabilities", "backup_restore", "rollback", "releases", "probes", "known_limits"):
                self.assertIn(key, data)
            self.assertEqual(data["probes"], {"provider": "NOT_RUN", "model": "NOT_RUN"})
            self.assertEqual(data["tests"][0]["command"], "go test ./...")
            self.assertEqual(data["evidence"][0]["kind"], "backup_restore")
            self.assertIsNotNone(data["evidence"][0]["sha256"])
            self.assertNotIn("secret", first.read_text().lower())

    def test_verifier_requires_explicit_full_mode_for_external_gates(self):
        text = (ROOT / "scripts/verify-production.sh").read_text()
        self.assertIn("--static", text)
        self.assertIn("--full", text)
        self.assertIn("production-preflight.sh", text)
        self.assertIn("unittest discover -s app/tests/contract", text)
        self.assertNotIn("app.tests.contract", text)
