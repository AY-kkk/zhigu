import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

class FrontendReleaseTests(unittest.TestCase):
    def test_frontend_result_is_traceable_and_secret_free(self):
        with tempfile.TemporaryDirectory() as name:
            tmp = Path(name)
            result = tmp / 'frontend.json'
            result.write_text(json.dumps({
                'tests': {'build': {'command': 'npm run build', 'exit_code': 0, 'passed': 1, 'failed': 0, 'skipped': 0}},
                'viewports': [1440, 1024, 390, 320],
                'axe': {'critical': 0, 'serious': 0},
                'storage_scan': 'PASS',
                'manifest_files': ['assets/index.js'],
            }))
            output = tmp / 'manifest.json'
            subprocess.run([
                'python3', str(ROOT / 'scripts/release-evidence.py'),
                '--output', str(output),
                '--version', 'frontend-test',
                '--frontend-result', str(result),
            ], check=True)
            manifest = json.loads(output.read_text())
            self.assertEqual(manifest['frontend']['status'], 'PASS')
            self.assertEqual(manifest['frontend']['viewports'], [1440, 1024, 390, 320])
            self.assertEqual(manifest['frontend']['storage_scan'], 'PASS')
            self.assertNotIn('token', output.read_text().lower())

    def test_verifier_runs_all_required_frontend_gates(self):
        text = (ROOT / 'scripts/verify-frontend.sh').read_text()
        for command in ('npm run lint', 'npm run test:unit', 'npm run test:component', 'npm run test:contracts', 'npm run test:performance', 'npx playwright test'):
            self.assertIn(command, text)
