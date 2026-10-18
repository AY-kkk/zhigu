from __future__ import annotations

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def fake_bin(directory: Path, name: str, body: str) -> None:
    path = directory / name
    path.write_text("#!/usr/bin/env bash\nset -euo pipefail\n" + body)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


class OperationalScriptTests(unittest.TestCase):
    def test_performance_harness_reports_unavailable_target_as_blocked(self):
        with tempfile.TemporaryDirectory() as tmp_name:
            env = os.environ | {"ZHIGU_PUBLIC_ORIGIN": "http://127.0.0.1:1"}
            result = subprocess.run(
                ["bash", str(ROOT / "scripts/run-performance.sh"), tmp_name],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn("NOT RUN: performance target unavailable", result.stderr)

    def test_backup_quiesces_before_dump_encrypts_offhost_and_releases(self):
        with tempfile.TemporaryDirectory() as tmp_name:
            tmp = Path(tmp_name)
            bin_dir = tmp / "bin"
            bin_dir.mkdir()
            log = tmp / "order.log"
            fake_bin(bin_dir, "curl", f'''
echo "curl $*" >> "{log}"
echo '{{"acknowledged":true,"persisted":true}}'
''')
            fake_bin(bin_dir, "pg_dump", f'''
echo "pg_dump $*" >> "{log}"
for arg in "$@"; do
  if [[ "$arg" == --file ]]; then next=1; elif [[ "${{next:-0}}" == 1 ]]; then : > "$arg"; exit 0; fi
done
exit 2
''')
            key = tmp / "backup.key"
            key.write_text("test-only-backup-key")
            backup_root = tmp / "backups"
            backup_root.mkdir()
            for index in range(9):
                (backup_root / f"old-{index}").mkdir()
            offhost = tmp / "offhost"
            env = os.environ | {
                "PATH": f"{bin_dir}:{os.environ['PATH']}",
                "BACKUP_ROOT": str(backup_root),
                "BACKUP_KEY_FILE": str(key),
                "BACKUP_OFFHOST_DIR": str(offhost),
                "FUTURES_SERVICE_TOKEN": "futures-test-token",
                "ZHIGU_INTERNAL_TOKEN": "research-test-token",
                "DATABASE_URL": "postgres://test",
                "ZHIGU_RESEARCH_SQLITE": str(tmp / "jobs.sqlite"),
            }
            result = subprocess.run(["bash", str(ROOT / "scripts/backup.sh")], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            events = log.read_text()
            self.assertLess(events.index("/internal/ops/v1/backup/begin"), events.index("pg_dump "))
            self.assertLess(events.index("pg_dump "), events.index('/internal/ops/v1/backup/end'))
            archives = list(backup_root.glob("*/backup.age"))
            self.assertEqual(len(archives), 1)
            self.assertTrue((offhost / archives[0].name).is_file())
            self.assertLessEqual(len([p for p in backup_root.iterdir() if p.is_dir()]), 8)

    def test_watchdog_restarts_and_alerts_once_on_health_failure(self):
        with tempfile.TemporaryDirectory() as tmp_name:
            tmp = Path(tmp_name)
            bin_dir = tmp / "bin"
            bin_dir.mkdir()
            log = tmp / "watchdog.log"
            fake_bin(bin_dir, "curl", 'exit 22')
            fake_bin(bin_dir, "docker", f'echo "docker $*" >> "{log}"')
            fake_bin(bin_dir, "alert-sink", f'cat >/dev/null; echo alert >> "{log}"')
            env = os.environ | {"PATH": f"{bin_dir}:{os.environ['PATH']}", "ALERT_COMMAND": "alert-sink"}
            result = subprocess.run(["bash", str(ROOT / "scripts/maintenance-watchdog.sh"), "--once"], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            text = log.read_text()
            self.assertIn("alert", text)
            self.assertEqual(text.count("docker compose"), 1)

    def test_restore_verifies_hash_and_fresh_database(self):
        with tempfile.TemporaryDirectory() as tmp_name:
            tmp = Path(tmp_name)
            work = tmp / "package"
            work.mkdir()
            (work / "postgresql.dump").write_text("dump")
            import sqlite3
            con = sqlite3.connect(work / "research.sqlite")
            con.execute("CREATE TABLE tasks(id TEXT PRIMARY KEY)")
            con.commit(); con.close()
            (work / "futures-documents.tar").write_bytes(b"")
            with open(work / "SHA256SUMS", "w") as output:
                subprocess.run(["sha256sum", "postgresql.dump", "research.sqlite", "futures-documents.tar"], cwd=work, check=True, stdout=output)
            (work / "manifest.json").write_text("{}")
            archive = tmp / "backup.age"
            subprocess.run(["tar", "-C", str(work), "-cf", str(archive), "postgresql.dump", "research.sqlite", "futures-documents.tar", "SHA256SUMS", "manifest.json"], check=True)
            bin_dir = tmp / "bin"; bin_dir.mkdir()
            restore_log = tmp / "restore.log"
            fake_bin(bin_dir, "pg_restore", f'echo pg_restore >> "{restore_log}"')
            key = tmp / "key"; key.write_text("restore-test-key")
            env = os.environ | {"PATH": f"{bin_dir}:{os.environ['PATH']}", "BACKUP_KEY_FILE": str(key), "PGDATABASE": "restore_test"}
            result = subprocess.run(["bash", str(ROOT / "scripts/restore-verify.sh"), str(archive), str(tmp / "restore-work")], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("pg_restore", restore_log.read_text())
            self.assertIn("research tables=1", result.stdout)
