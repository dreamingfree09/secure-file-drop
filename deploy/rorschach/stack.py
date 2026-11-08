#!/usr/bin/env python3
"""Fail-closed Compose entry point for a freshly qualified dedicated Linux VM."""

import argparse
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

MARKER = Path("/etc/sfd-rorschach-instance")
DOCKER = ["docker", "--host", "unix:///var/run/docker.sock"]
SAFE_ENV = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
REPO = Path(__file__).resolve().parents[2]
COMPOSE = Path(__file__).resolve().with_name("compose.yml")
SECRET_KEYS = (
    "R1_PG_ADMIN", "R1_PG_APP", "R1_S3_ROOT_SECRET", "R1_S3_APP_SECRET",
    "R1_SESSION_SECRET", "R1_DOWNLOAD_SECRET",
)
CERT_FILES = (
    "origin-tls/fullchain.pem", "origin-tls/key.pem", "storage-ca.pem",
    "storage-tls/rustfs_cert.pem", "storage-tls/rustfs_key.pem",
)
PROJECT_RESOURCES = {
    "volume": ("postgres", "objects"),
    "network": ("database", "objects", "application", "ingress"),
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def checked_file(path, private=True):
    require(path.resolve() == path, "Symlinks are forbidden")
    metadata = path.stat()
    require(stat.S_ISREG(metadata.st_mode) and metadata.st_uid == 0,
            "Expected an ordinary root-owned configuration file")
    require(not metadata.st_mode & (0o077 if private else 0o022),
            "Unsafe configuration file mode")
    require(metadata.st_size <= 65536, "Configuration file is too large")
    return path.read_text(encoding="ascii")


def parse_env(content):
    """The accepted format intentionally has no Compose/shell expansion."""
    result = {}
    for raw in content.splitlines():
        if not raw or raw.startswith("#"):
            continue
        key, separator, value = raw.partition("=")
        require(separator and key not in result and re.fullmatch(r"R1_[A-Z0-9_]+", key),
                "Malformed or repeated environment key")
        result[key] = value
    expected = set(SECRET_KEYS) | {
        "R1_HOST", "R1_ORIGIN_PORT", "R1_PUBLIC_PORT_SUFFIX",
        "R1_S3_ROOT_KEY", "R1_S3_APP_KEY", "R1_ADMIN_HASH",
    }
    require(result.keys() == expected, "Unexpected or missing environment key")
    require(re.fullmatch(r"(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?", result["R1_HOST"]),
            "Expected a DNS host name")
    port = result["R1_ORIGIN_PORT"]
    require(re.fullmatch(r"[1-9][0-9]{3,4}", port) and 1024 <= int(port) <= 65535,
            "Expected an unprivileged origin port")
    require(result["R1_PUBLIC_PORT_SUFFIX"] in ("", ":" + port), "Unexpected public port suffix")
    for key in SECRET_KEYS:
        require(re.fullmatch(r"[a-f0-9]{64}", result[key]), "Expected generated secret format")
    require(len({result[key] for key in SECRET_KEYS}) == len(SECRET_KEYS), "Credentials must be distinct")
    for key in ("R1_S3_ROOT_KEY", "R1_S3_APP_KEY"):
        require(re.fullmatch(r"[A-Z0-9]{20}", result[key]), "Expected generated S3 identity format")
    require(result["R1_S3_ROOT_KEY"] != result["R1_S3_APP_KEY"], "S3 identities must be distinct")
    require(re.fullmatch(r"\$2[aby]\$(?:1[2-4])\$[./A-Za-z0-9]{53}", result["R1_ADMIN_HASH"]),
            "Expected a cost 12–14 bcrypt administrator hash")
    return result


def run(command, env=SAFE_ENV, capture=False):
    return subprocess.run(
        command, cwd=REPO, env=env, check=True, text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.DEVNULL if capture else None,
    ).stdout


def verify_vm(instance):
    require(sys.platform == "linux" and os.geteuid() == 0, "Requires a dedicated Linux VM administrator")
    require(run(["systemd-detect-virt", "--vm"], capture=True).strip() in {"kvm", "qemu"},
            "Requires a dedicated Proxmox Linux VM")
    machine_id = checked_file(Path("/etc/machine-id"), private=False).strip()
    require(re.fullmatch(r"[a-f0-9]{32}", machine_id) and
            checked_file(MARKER).strip() == f"{instance} {machine_id}", "Dedicated VM marker mismatch")
    sock = Path("/var/run/docker.sock")
    require(stat.S_ISSOCK(sock.stat().st_mode), "Expected the dedicated VM's local Docker socket")
    require(run(DOCKER + ["info", "--format", "{{.OSType}} {{.OperatingSystem}}"], capture=True)
            .strip().startswith("linux "), "Expected a Linux engine")
    # Docker Desktop reports a different Linux server OS even via a Unix socket.
    system = run(DOCKER + ["info", "--format", "{{.OperatingSystem}}"], capture=True).strip()
    require("docker desktop" not in system.casefold(), "Docker Desktop is outside this deployment")


def compose_environment(instance):
    runtime = Path("/srv/sfd-rorschach") / instance
    require(runtime.resolve() == runtime and runtime.is_dir(), "Expected the owned runtime directory")
    metadata = runtime.stat()
    require(metadata.st_uid == 0 and not metadata.st_mode & 0o077, "Expected a private root-owned runtime directory")
    env = parse_env(checked_file(runtime / "secrets.env"))
    # Key files are owned by the dedicated container UID for direct mounts.
    for item in CERT_FILES:
        target = runtime / item
        require(target.resolve() == target, "TLS paths must not be symlinks")
        metadata = target.stat()
        key = item.endswith("key.pem")
        require(stat.S_ISREG(metadata.st_mode) and metadata.st_size > 0 and
                metadata.st_uid == (10001 if key else 0) and
                not metadata.st_mode & (0o077 if key else 0o022), "Unsafe TLS file ownership or mode")
    require(not run(["git", "status", "--porcelain", "--untracked-files=all"], capture=True).strip(),
            "Expected a clean reviewed checkout")
    revision = run(["git", "rev-parse", "HEAD"], capture=True).strip()
    require(re.fullmatch(r"[a-f0-9]{40}", revision) and
            checked_file(runtime / "approved-revision").strip() == revision, "Reviewed revision mismatch")
    return runtime, {**SAFE_ENV, **env, "R1_RUNTIME": str(runtime), "R1_SOURCE_REVISION": revision}


def verify_project(project, fresh):
    label = f"label=com.docker.compose.project={project}"
    containers = run(DOCKER + ["ps", "-a", "--format", "{{.Label \"com.docker.compose.service\"}}", "--filter", label],
                     capture=True).splitlines()
    expected = [] if fresh else ["backend", "postgres", "proxy", "storage"]
    require(sorted(containers) == expected, "Project container inventory does not match the requested operation")
    for kind, suffixes in PROJECT_RESOURCES.items():
        all_names = set(run(DOCKER + [kind, "ls", "--format", "{{.Name}}"], capture=True).splitlines())
        owned = set(run(DOCKER + [kind, "ls", "--format", "{{.Name}}", "--filter", label],
                        capture=True).splitlines())
        names = {f"{project}_{suffix}" for suffix in suffixes}
        require(not (owned or names & all_names) if fresh else owned == names,
                "Project storage or network identity does not match the requested operation")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("instance", help="approved 12-character lower-case hexadecimal instance ID")
    parser.add_argument("action", choices=["validate", "initialize", "start", "verify-storage", "stop", "status"])
    args = parser.parse_args()
    try:
        require(re.fullmatch(r"[a-f0-9]{12}", args.instance), "Invalid instance ID")
        verify_vm(args.instance)
        runtime, env = compose_environment(args.instance)
        import fcntl
        directory = os.open(runtime, os.O_RDONLY | os.O_DIRECTORY)
        try:
            fcntl.flock(directory, fcntl.LOCK_EX | fcntl.LOCK_NB)
            project = "sfd-rorschach-" + args.instance
            compose = DOCKER + ["compose", "--env-file", "/dev/null", "-f", str(COMPOSE), "-p", project]
            run(compose + ["config", "--quiet"], env)
            if args.action != "validate":
                verify_project(project, fresh=args.action == "initialize")
            if args.action == "initialize":
                run(compose + ["build", "backend", "provision"], env)
                run(compose + ["up", "-d", "postgres", "storage"], env)
                run(compose + ["run", "--rm", "--no-deps", "provision", "initialize"], env)
                run(compose + ["up", "-d", "--no-build", "--wait", "--wait-timeout", "180", "backend", "proxy"], env)
            elif args.action == "start":
                run(compose + ["up", "-d", "--no-build", "--no-recreate", "postgres", "storage"], env)
                run(compose + ["run", "--rm", "--no-deps", "provision", "verify"], env)
                run(compose + ["up", "-d", "--no-build", "--no-recreate", "--wait", "--wait-timeout", "180", "backend", "proxy"], env)
            elif args.action == "verify-storage":
                run(compose + ["run", "--rm", "--no-deps", "provision", "verify"], env)
            elif args.action in {"stop", "status"}:
                run(compose + ["stop" if args.action == "stop" else "ps"], env)
            print("Rorschach command completed")
        finally:
            os.close(directory)
    except (OSError, UnicodeError, ValueError, subprocess.CalledProcessError) as error:
        message = str(error) if isinstance(error, ValueError) and not isinstance(error, UnicodeError) else type(error).__name__
        print(f"STOP: {message}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
