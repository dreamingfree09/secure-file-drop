"""Portable safety tests for the read-only Rorschach inventory helper."""
import json
import os
import tempfile
import unittest
from pathlib import Path

from scripts import rorschach_preflight as preflight


class PreflightTests(unittest.TestCase):
    def test_real_temporary_directory_is_reported_without_leaking_path(self):
        with tempfile.TemporaryDirectory(prefix="private-rorschach-secret-") as directory:
            result = preflight.inspect_storage([directory])
            self.assertEqual(len(result), 1)
            self.assertEqual(result[0]["label"], "storage_1")
            self.assertTrue(result[0]["accessible"])
            self.assertGreater(result[0]["capacity_bytes"], 0)
            self.assertGreaterEqual(result[0]["available_bytes"], 0)
            self.assertNotIn(directory, json.dumps(result))
            self.assertNotIn("private-rorschach-secret", json.dumps(result))

    def test_unknown_path_does_not_echo_private_directory_or_raise(self):
        with tempfile.TemporaryDirectory() as directory:
            missing = str(Path(directory) / "private-token-unavailable")
            result = preflight.inspect_storage([missing])
            self.assertEqual(result, [{"label": "storage_1", "accessible": False}])
            self.assertNotIn(missing, json.dumps(result))
            self.assertFalse(Path(missing).exists())

    def test_no_paths_means_no_storage_scan_and_no_secret_environment_echo(self):
        secret = "SFD_RORSCHACH_TEST_SECRET_SENTINEL"
        prior = os.environ.get(secret)
        os.environ[secret] = "private-secret-token"
        try:
            result = preflight.collect([])
            report = json.dumps(result)
            self.assertEqual(result["storage"], [])
            self.assertEqual(result["schema_version"], 1)
            self.assertNotIn(secret, report)
            self.assertNotIn("private-secret-token", report)
            self.assertEqual(preflight.TARGET_FILE_BYTES, 107374182400)
        finally:
            if prior is None:
                del os.environ[secret]
            else:
                os.environ[secret] = prior

    def test_multiple_directories_are_labelled_without_paths(self):
        with tempfile.TemporaryDirectory() as first, tempfile.TemporaryDirectory() as second:
            records = preflight.inspect_storage([first, second])
            self.assertEqual([item["label"] for item in records], ["storage_1", "storage_2"])
            self.assertTrue(all(item["accessible"] for item in records))
            self.assertNotIn(first, json.dumps(records))
            self.assertNotIn(second, json.dumps(records))

    def test_memory_parser_fails_closed_for_missing_or_malformed_data(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "meminfo"
            source.write_text("MemTotal: 42 kB\n", encoding="ascii")
            self.assertEqual(preflight.total_memory_bytes(str(source)), 42 * 1024)
            source.write_text("MemTotal: PRIVATE INVALID kB\n", encoding="ascii")
            self.assertIsNone(preflight.total_memory_bytes(str(source)))
            self.assertIsNone(preflight.total_memory_bytes(str(source) + ".missing"))


if __name__ == "__main__":
    unittest.main()
