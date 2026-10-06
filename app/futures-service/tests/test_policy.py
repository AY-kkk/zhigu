import unittest

from futures_worker.policy import ALLOWED_TOOLS, require_tool, validate_scope


class PolicyTests(unittest.TestCase):
    def test_stock_and_arbitrary_tools_are_denied(self):
        for name in ("get_financials", "search_filings", "shell", "http_get", ""):
            with self.assertRaises(ValueError):
                require_tool(name)
        for name in ALLOWED_TOOLS:
            self.assertEqual(require_tool(name), name)

    def test_scope_never_inherits_stock_or_demo(self):
        validate_scope(domain="futures", mode="live", owner_id=12, run_id="frun_1")
        for patch in ({"domain": "finance"}, {"mode": "demo"}, {"owner_id": 0}, {"owner_id": True}, {"run_id": ""}):
            args = dict(domain="futures", mode="live", owner_id=12, run_id="frun_1")
            args.update(patch)
            with self.assertRaises(ValueError):
                validate_scope(**args)


if __name__ == "__main__":
    unittest.main()
