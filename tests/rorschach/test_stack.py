import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "deploy" / "rorschach" / "stack.py"
SPEC = importlib.util.spec_from_file_location("rorschach_stack", SOURCE)
stack = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(stack)


def synthetic_env():
    return {
        "R1_HOST": "sfd.fixture.test",
        "R1_ORIGIN_PORT": "18443",
        "R1_PUBLIC_PORT_SUFFIX": ":18443",
        "R1_S3_ROOT_KEY": "A" * 20,
        "R1_S3_APP_KEY": "B" * 20,
        "R1_ADMIN_HASH": "$2b$12$" + "." * 53,
        **{key: str(number) * 64 for number, key in enumerate(stack.SECRET_KEYS, 1)},
    }


def render_env(config):
    return "\n".join(f"{key}={value}" for key, value in config.items())


class StackTests(unittest.TestCase):
    def test_strict_format(self):
        values = synthetic_env()
        self.assertEqual(stack.parse_env(render_env(values)), values)
        for key, value in (
            ("R1_HOST", "test.example\nCOMPOSE_FILE=attacker"),
            ("R1_HOST", "https://test.example"),
            ("R1_ORIGIN_PORT", "443"),
            ("R1_PUBLIC_PORT_SUFFIX", ":18081"),
            ("R1_S3_APP_KEY", values["R1_S3_ROOT_KEY"]),
            ("R1_PG_APP", values["R1_PG_ADMIN"]),
            ("R1_PG_APP", "$HOME"),
            ("R1_ADMIN_HASH", "plaintext"),
            ("R1_RUNTIME", "/var/lib/docker"),
        ):
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                stack.parse_env(render_env({**values, key: value}))
        with self.assertRaises(ValueError):
            stack.parse_env(render_env(values) + "\nR1_HOST=other.fixture.test")
        qualified = {**values, "R1_RESUMABLE_MAX_BYTES": "1073741824",
                     "R1_RESUMABLE_RESERVED_BYTES": "2147483648",
                     "R1_MANAGED_LINKS_ENABLED": "true"}
        self.assertEqual(stack.parse_env(render_env(qualified)), qualified)
        registration = {**qualified, "R1_REGISTRATION_INVITATIONS_ENABLED": "true"}
        self.assertEqual(stack.parse_env(render_env(registration)), registration)
        with self.assertRaises(ValueError):
            stack.parse_env(render_env({**registration, "R1_MANAGED_LINKS_ENABLED": "false"}))
        with self.assertRaises(ValueError):
            stack.parse_env(render_env({**qualified, "R1_REGISTRATION_INVITATIONS_ENABLED": "yes"}))
        for invalid in ("yes", "TRUE", "1"):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                stack.parse_env(render_env({**qualified, "R1_MANAGED_LINKS_ENABLED": invalid}))
        with self.assertRaises(ValueError):
            stack.parse_env(render_env({**values, "R1_MANAGED_LINKS_ENABLED": "true"}))

    def test_project_refuses_unowned_preexisting_volume(self):
        with patch.object(stack, "run", side_effect=["", "sfd-rorschach-123_objects\n", ""]):
            with self.assertRaisesRegex(ValueError, "storage or network identity"):
                stack.verify_project("sfd-rorschach-123", fresh=True)

    def test_cannot_restart_an_incomplete_project(self):
        with patch.object(stack, "run", return_value="backend\npostgres\nstorage\n"):
            with self.assertRaisesRegex(ValueError, "container inventory"):
                stack.verify_project("sfd-rorschach-123", fresh=False)

    def test_fresh_inventory(self):
        with patch.object(stack, "run", side_effect=["", "unrelated\n", "", "bridge\nhost\nnone\n", ""]):
            stack.verify_project("sfd-rorschach-123", fresh=True)

    def test_effective_compose_does_not_expose_sensitive_services(self):
        standalone = shutil.which("docker-compose")
        compose = [standalone] if standalone else ["docker", "compose"]
        with tempfile.TemporaryDirectory(prefix="sfd-rorschach-render-") as directory:
            env = {**os.environ, **synthetic_env(), "R1_RUNTIME": directory, "R1_SOURCE_REVISION": "a" * 40}
            result = subprocess.run(
                [*compose, "--env-file", os.devnull, "-p", "sfd-rorschach-fixture",
                 "-f", str(stack.COMPOSE), "--profile", "initialize", "config", "--format", "json"],
                env=env, text=True, capture_output=True, check=True,
            )
        config = json.loads(result.stdout)
        services = config["services"]
        for name, service in services.items():
            with self.subTest(service=name):
                self.assertEqual(service["read_only"], True)
                self.assertIn("ALL", service["cap_drop"])
                self.assertIn("no-new-privileges:true", service["security_opt"])
                self.assertNotIn("privileged", service)
                self.assertNotIn("container_name", service)
                self.assertNotIn("network_mode", service)
                self.assertFalse(any("docker.sock" in volume["source"] for volume in service.get("volumes", [])))
                if name != "proxy":
                    self.assertNotIn("ports", service)
        self.assertEqual(services["proxy"]["ports"][0]["host_ip"], "127.0.0.1")
        self.assertEqual(len(services["proxy"]["ports"]), 1)
        for name in ("database", "objects", "application"):
            self.assertEqual(config["networks"][name]["internal"], True)
        backend = services["backend"]["environment"]
        self.assertEqual(backend["SFD_S3_ENDPOINT"], "https://storage:9000")
        self.assertEqual(backend["SFD_S3_PRIVACY_MODE"], "bucket-policy")
        self.assertEqual(backend["SFD_ADMIN_PASS"].replace("$$", "$"), synthetic_env()["R1_ADMIN_HASH"])
        self.assertEqual(backend["SFD_BACKUP_ENABLED"], "false")
        self.assertEqual(backend["SFD_CLEANUP_ENABLED"], "false")
        self.assertFalse(any("ROOT_" in key or "RAILWAY" in key or "RESEND" in key for key in backend))
        self.assertEqual(set(config["volumes"]), {"postgres", "objects"})
