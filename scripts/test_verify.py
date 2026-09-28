"""Regression coverage for the verification entry point, not the product."""

import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import verify


class VerifyTests(unittest.TestCase):
    @unittest.skipUnless(os.name == "nt" and shutil.which("powershell"), "Windows PowerShell wrapper test")
    def test_powershell_wrappers_forward_mode_and_exit_status_from_other_directory(self):
        with tempfile.TemporaryDirectory(prefix="verify wrapper ") as folder:
            fake = Path(folder) / "python stub.cmd"
            for status in (0, 7):
                fake.write_text(f"@echo off\necho %*\nexit /b {status}\n")
                for wrapper, mode in (("check", "check"), ("pre-release", "release")):
                    result = subprocess.run(
                        ["powershell", "-NoProfile", "-File", str(verify.ROOT / f"scripts/{wrapper}.ps1")],
                        cwd=folder, env={**os.environ, "PYTHON": str(fake)}, capture_output=True,
                    )
                    self.assertEqual(result.returncode, status, result.stderr)
                    self.assertTrue(result.stdout.decode().replace('"', '').strip().endswith(f"verify.py {mode}"))

    @unittest.skipUnless(os.name != "nt" and shutil.which("bash"), "POSIX Bash wrapper test")
    def test_bash_wrappers_forward_mode_and_exit_status_from_other_directory(self):
        with tempfile.TemporaryDirectory(prefix="verify wrapper ") as folder:
            fake = Path(folder) / "python stub"
            for status in (0, 7):
                fake.write_text(f'#!/bin/sh\nprintf "%s\\n" "$@"\nexit {status}\n')
                fake.chmod(0o700)
                for wrapper, mode in (("check", "check"), ("pre-release", "release")):
                    result = subprocess.run(
                        ["bash", str(verify.ROOT / f"scripts/{wrapper}.sh")], cwd=folder,
                        env={**os.environ, "PYTHON": str(fake)}, capture_output=True,
                    )
                    self.assertEqual(result.returncode, status, result.stderr)
                    self.assertEqual(result.stdout.splitlines()[-1], mode.encode())

    def test_builds_frontend_before_go_and_runs_each_check_once(self):
        with patch.object(verify, "check_format") as formatting, patch.object(verify, "run") as run:
            verify.check()
        formatting.assert_called_once_with()
        self.assertEqual([call.args for call in run.call_args_list], [
            ("npm", "run", "lint:check"), ("npm", "run", "test:unit"),
            ("npm", "run", "build"), ("go", "vet", "./..."),
            ("go", "test", "-timeout=5m", "./internal/..."), ("go", "build", "./..."),
        ])

    def test_failure_stops_later_commands(self):
        with patch.object(verify, "check_format"), patch.object(verify, "run") as run:
            run.side_effect = subprocess.CalledProcessError(3, "lint")
            with self.assertRaises(subprocess.CalledProcessError):
                verify.check()
        self.assertEqual(run.call_count, 1)

    def test_release_checks_once_without_requiring_commit_or_writing_modules(self):
        with patch.object(verify, "check") as check, patch.object(verify, "run") as run, patch.object(verify, "check_versions"):
            verify.release()
        check.assert_called_once_with()
        self.assertEqual([call.args for call in run.call_args_list], [
            ("go", "mod", "tidy", "-diff"), ("npm", "audit", "--audit-level=moderate"),
        ])

    def test_format_accepts_crlf_but_rejects_content_difference_without_writing(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            source = root / "example.go"
            original = b"package example\r\n"
            source.write_bytes(original)
            with patch.object(verify, "ROOT", root), patch.object(verify, "run", return_value=b"package example\n"):
                verify.check_format(["example.go"])
            with patch.object(verify, "ROOT", root), patch.object(verify, "run", return_value=b"package changed\n"):
                with self.assertRaisesRegex(RuntimeError, "Formatting differs"):
                    verify.check_format(["example.go"])
            self.assertEqual(source.read_bytes(), original)

    def test_formatter_failure_cannot_pass_as_empty_output(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "example.go").write_text("package example\n")
            with patch.object(verify, "ROOT", root), patch.object(verify, "run", side_effect=subprocess.CalledProcessError(1, "gofmt")):
                with self.assertRaises(subprocess.CalledProcessError):
                    verify.check_format(["example.go"])

    def test_explicit_missing_file_is_not_a_successful_check(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(verify, "run") as run:
            with patch.object(verify, "ROOT", Path(folder)):
                with self.assertRaisesRegex(RuntimeError, "Expected an existing Go file"):
                    verify.check_format(["missing.go"])
            run.assert_not_called()

    def test_cli_returns_failure_when_tool_is_missing(self):
        with patch.object(verify.sys, "argv", ["verify.py", "check"]), patch.object(verify, "check", side_effect=RuntimeError("missing tool")):
            self.assertEqual(verify.main(), 1)

    def test_versions_use_frontend_file_and_reject_mismatch_or_missing_constant(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "frontend").mkdir()
            (root / "internal/version").mkdir(parents=True)
            (root / "frontend/package.json").write_text(json.dumps({"version": "1.2.3"}))
            source = root / "internal/version/version.go"
            with patch.object(verify, "ROOT", root):
                source.write_text('const Version = "1.2.3"\n')
                verify.check_versions()
                for text in ('const Version = "2.0.0"\n', "package version\n"):
                    source.write_text(text)
                    with self.assertRaisesRegex(RuntimeError, "Version mismatch"):
                        verify.check_versions()


if __name__ == "__main__":
    unittest.main()
