#!/usr/bin/env python3
"""Read-only, sanitised capacity preflight for a candidate Rorschach Linux host.

Does not authenticate, invoke network tools, execute shell commands, enumerate
existing VMs, inspect storage contents, or make filesystem changes. Never publish
raw path names, IPs, hostnames, or environment variables in CI/issue comments.
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import shutil
from pathlib import Path

GIB = 1024 ** 3
TARGET_FILE_BYTES = 100 * GIB


def total_memory_bytes(meminfo: str = "/proc/meminfo") -> int | None:
    try:
        with open(meminfo, encoding="ascii") as source:
            for line in source:
                if line.startswith("MemTotal:"):
                    fields = line.split()
                    return int(fields[1]) * 1024 if len(fields) == 3 and fields[2] == "kB" else None
    except (OSError, ValueError, IndexError):
        pass
    return None


def inspect_storage(paths: list[str]) -> list[dict[str, int | bool | str]]:
    results: list[dict[str, int | bool | str]] = []
    for number, raw_path in enumerate(paths, start=1):
        # Labels replace identifying path names, including in errors.
        result: dict[str, int | bool | str] = {"label": f"storage_{number}", "accessible": False}
        try:
            path = Path(raw_path)
            if not path.is_dir():
                results.append(result)
                continue
            stat = os.statvfs(path)
            result.update({
                "accessible": True,
                "mountpoint": os.path.ismount(path),
                "capacity_bytes": stat.f_blocks * stat.f_frsize,
                "available_bytes": stat.f_bavail * stat.f_frsize,
                # Capacity is only a hint; it does not certify ownership or
                # that production, backups and temporary parts will all fit.
                "target_file_bytes": TARGET_FILE_BYTES,
            })
        except (OSError, ValueError):
            pass
        results.append(result)
    return results


def collect(paths: list[str]) -> dict[str, object]:
    return {
        "schema_version": 1,
        "scope": "read_only_local_inventory",
        "os_family": platform.system(),
        "logical_cpus": os.cpu_count(),
        "memory_total_bytes": total_memory_bytes(),
        "proxmox_cli_available": shutil.which("pveversion") is not None,
        "docker_cli_available": shutil.which("docker") is not None,
        "storage": inspect_storage(paths),
        "warnings": [
            "This report proves neither Proxmox access nor free-space ownership.",
            "Do not run destructive tests, provision volumes, expose admin ports, or change existing storage from this report alone.",
            "Confirm private networking, free space, isolated backup/restore and storage privacy separately.",
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Safe read-only Rorschach host preflight")
    parser.add_argument(
        "--storage-path", action="append", default=[], metavar="PATH",
        help="Existing authorised data path; never printed in the report (repeatable)",
    )
    args = parser.parse_args()
    print(json.dumps(collect(args.storage_path), sort_keys=True, separators=(",", ":")))


if __name__ == "__main__":
    main()
