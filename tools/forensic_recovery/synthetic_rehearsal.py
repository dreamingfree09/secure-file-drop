#!/usr/bin/env python3
"""Synthetic-only recovery rehearsal. No real-image input option exists."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import resource
import stat
import struct
import subprocess
import sys
import time

spec = importlib.util.spec_from_file_location("fixtures", Path(__file__).with_name("fixtures.py"))
fixtures = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixtures)

def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def regular_owned(path, root):
    path = Path(path)
    assert path.is_absolute() and path.resolve().is_relative_to(root)
    info = path.lstat()
    assert stat.S_ISREG(info.st_mode) and info.st_uid == os.getuid()
    assert not path.is_symlink()
    assert info.st_size <= 64 * 1024 * 1024
    return path

def parse_header(path):
    """Read known synthetic headers; validate CRC and choose highest sequence."""
    with path.open("rb") as stream:
        assert stream.read(8) == b"vhdxfile"
        headers = []
        for offset in [65536, 131072]:
            stream.seek(offset)
            data = bytearray(stream.read(4096))
            expected = struct.unpack_from("<I", data, 4)[0]
            struct.pack_into("<I", data, 4, 0)
            assert data[:4] == b"head" and fixtures.crc32c(data) == expected
            headers.append((struct.unpack_from("<Q", data, 8)[0], bytes(data[48:64])))
    return max(headers)[1]

def journal_accepted(result):
    text = (result.stdout + result.stderr).lower()
    failures = [b"invalid checksum", b"checksum error", b"corrupt", b"aborted",
                b"failed", b"error", b"cancel"]
    return result.returncode in (0, 1) and not any(word in text for word in failures)

class Rehearsal:
    def __init__(self, root, qemu, manifest):
        self.root = root
        self.start = time.monotonic()
        self.commands = []
        self.checks = []
        self.images = []
        self.qemu = qemu.resolve()
        prefix = self.qemu.parent.parent.parent
        self.env = {"PATH": "/usr/sbin:/usr/bin:/bin", "LANG": "C",
                    "LD_LIBRARY_PATH": str(prefix / "usr/lib/x86_64-linux-gnu")}
        self.manifest = json.loads(manifest.read_text())
        for name, expected in self.manifest["files_sha256"].items():
            path = Path(name.replace("$QEMU_PREFIX", str(prefix)))
            assert sha(path) == expected, "Tool or dependency identity changed"
        assert sha(self.qemu) == "634320b91165669917123e8e79cce1c4d00cee0a4aa4d662d7c0a8186479b3fb"

    def check(self, name, condition):
        assert condition, name
        self.checks.append(name)

    def command(self, label, args):
        remaining = 600 - (time.monotonic() - self.start)
        assert remaining > 0
        result = subprocess.run([str(x) for x in args], env=self.env,
                                capture_output=True, timeout=min(90, remaining))
        assert len(result.stdout) + len(result.stderr) < 65536
        allocated = sum(p.stat().st_blocks * 512 for p in self.root.rglob("*") if p.is_file() and not p.is_symlink())
        assert allocated <= 1024**3, "Synthetic aggregate allocation limit"
        self.commands.append({"label": label, "arguments": [str(x).replace(str(self.root), "$RUN") for x in args],
                              "exit": result.returncode,
                              "stdout_text": ("" if label == "read synthetic marker" else result.stdout.decode(errors="replace")[:4096]),
                              "stderr_text": result.stderr.decode(errors="replace")[:4096],
                              "stdout_sha256": hashlib.sha256(result.stdout).hexdigest(),
                              "stderr_sha256": hashlib.sha256(result.stderr).hexdigest()})
        return result

    def copy(self, source, destination):
        regular_owned(source, self.root)
        assert not destination.exists() and not destination.is_symlink()
        with destination.open("xb") as target:
            target.write(source.read_bytes())
        regular_owned(destination, self.root)
        assert sha(source) == sha(destination)

    def make_ext(self, label, committed=True, corrupt=False):
        folder = self.root / label
        folder.mkdir(mode=0o700)
        tree = folder / "tree"
        tree.mkdir()
        (tree / "marker.bin").write_bytes(b"A" * 4096)
        raw = folder / "journal-parent.ext4"
        with raw.open("xb") as stream:
            stream.truncate(fixtures.RAW_SIZE)
        made = self.command(label + ": mkfs", ["/usr/sbin/mke2fs", "-q", "-t", "ext4", "-F",
                                               "-b", "4096", "-d", tree, raw])
        self.check(label + ": synthetic mkfs", made.returncode == 0)
        mapping = self.command(label + ": bmap", ["/usr/sbin/debugfs", "-R", "bmap /marker.bin 0", raw])
        block = mapping.stdout.decode().strip()
        assert re.fullmatch("[0-9]+", block)
        transaction = folder / "transaction.bin"
        transaction.write_bytes(b"B" * 4096)
        commands = folder / "journal.commands"
        commands.write_text('journal_open -c -v 3\njournal_write ' +
                            ('' if committed else '-c ') + '-b ' + block + ' "' +
                            str(transaction) + '"\njournal_close\n')
        written = self.command(label + ": journal fixture", ["/usr/sbin/debugfs", "-w", "-f", commands, raw])
        self.check(label + ": journal fixture command", written.returncode == 0)
        if not committed:
            # debugfs does not mark RECOVER for an uncommitted test transaction.
            # Mark the crash state through libext2fs so superblock checksums stay valid.
            dirty = self.command(label + ": mark synthetic crash state",
                                 ["/usr/sbin/debugfs", "-w", "-R", "feature needs_recovery", raw])
            self.check(label + ": synthetic dirty-state mark", dirty.returncode == 0)
        if corrupt:
            data = bytearray(raw.read_bytes())
            offset = data.find(b"B" * 4096)
            assert offset >= 0
            data[offset+128] ^= 1
            raw.write_bytes(data)  # Only this newly created synthetic fixture.
        self.check(label + ": RECOVER before replay",
                   bool(int.from_bytes(raw.read_bytes()[1120:1124], "little") & 4))
        self.check(label + ": marker A before replay", self.marker(raw) == b"A" * 4096)
        raw.chmod(0o400)
        return raw

    def marker(self, path):
        regular_owned(path, self.root)
        result = self.command("read synthetic marker", ["/usr/sbin/debugfs", "-R", "cat /marker.bin", path])
        return result.stdout

    def outer_guard(self, parent, label):
        before = sha(parent)
        guid = parse_header(parent)
        result = self.command(label + ": readonly container", [self.qemu, "check", "-f", "vhdx", parent])
        self.check(label + ": readonly probe preserves source", sha(parent) == before)
        needs_replay = result.returncode != 0 and b"log that needs to be replayed" in result.stderr
        # Nonzero LogGuid plus no positively recognized active log is STOP.
        return guid != bytes(16) and needs_replay

    def pipeline(self, label, committed, corrupt_journal=False):
        raw = self.make_ext(label, committed, corrupt_journal)
        folder = raw.parent
        parent = folder / "pending-parent.vhdx"
        fixtures.make_vhdx(parent, raw.read_bytes())
        parent.chmod(0o400)
        parent_hash = sha(parent)
        self.check(label + ": pending VHDX guard", self.outer_guard(parent, label))
        # On-disk BAT says zero for the first payload block until outer replay.
        with parent.open("rb") as stream:
            stream.seek(2 * fixtures.MIB)
            self.check(label + ": unreplayed first BAT block is ZERO",
                       struct.unpack("<Q", stream.read(8))[0] == 2)
        child = folder / "container-replay-child.vhdx"
        self.copy(parent, child)
        before = sha(child)
        result = self.command(label + ": outer replay", [self.qemu, "check", "-r", "all", "-f", "vhdx", child])
        self.check(label + ": outer replay exit", result.returncode == 0)
        self.check(label + ": outer replay changes child only", sha(child) != before and sha(parent) == parent_hash)
        self.check(label + ": outer LogGuid cleared", parse_header(child) == bytes(16))
        child.chmod(0o400)
        after_outer = sha(child)
        derived = folder / "derived-journal-child.ext4"
        result = self.command(label + ": bounded 32MiB raw", [self.qemu, "convert", "-f", "vhdx", "-O", "raw", child, derived])
        self.check(label + ": derived raw matches original synthetic guest", result.returncode == 0 and sha(derived) == sha(raw))
        before_inner = sha(derived)
        result = self.command(label + ": journal only", ["/usr/sbin/e2fsck", "-p", "-E", "journal_only", derived])
        marker = self.marker(derived)
        if corrupt_journal:
            self.check(label + ": corrupt committed journal not accepted", not journal_accepted(result))
        else:
            self.check(label + ": journal replay exit and diagnostics", journal_accepted(result))
            self.check(label + ": replayed transaction outcome", marker == (b"B" if committed else b"A") * 4096)
            self.check(label + ": RECOVER cleared",
                       not int.from_bytes(derived.read_bytes()[1120:1124], "little") & 4)
            validation = self.command(label + ": readonly fs validation", ["/usr/sbin/e2fsck", "-f", "-n", derived])
            self.check(label + ": readonly validation clean", validation.returncode == 0)
        self.check(label + ": protected synthetic parents unchanged", sha(parent) == parent_hash and sha(child) == after_outer)
        self.images.append({"case": label, "untouched_parent_sha256": parent_hash,
                            "initial_child_sha256": before, "after_outer_sha256": after_outer,
                            "before_inner_sha256": before_inner, "after_inner_sha256": sha(derived),
                            "journal_command_exit": result.returncode,
                            "journal_diagnostics": (result.stdout+result.stderr).decode(errors="replace")[:4096],
                            "marker_sha256": hashlib.sha256(marker).hexdigest()})

    def invalid_log(self):
        raw = self.make_ext("invalid-log-fixture")
        parent = raw.parent / "invalid-log-parent.vhdx"
        fixtures.make_vhdx(parent, raw.read_bytes(), corrupt_log=True)
        parent.chmod(0o400)
        self.check("invalid VHDX log: fails positive active-log guard",
                   not self.outer_guard(parent, "invalid log"))
        self.check("invalid VHDX log: no mutable child created",
                   not (raw.parent / "container-replay-child.vhdx").exists())

    def upstream_fixture(self, sample):
        # Only this exact bounded public upstream fixture is accepted.
        assert sample.stat().st_size == 31457280
        assert sha(sample) == "511daba998dba208ffc57a7814194d5dd3afb7c314731b904ff1682e3fb4951a"
        folder = self.root / "upstream-070"
        folder.mkdir(mode=0o700)
        parent = folder / "public-synthetic-parent.vhdx"
        with parent.open("xb") as stream:
            stream.write(sample.read_bytes())
        parent.chmod(0o400)
        before = sha(parent)
        self.check("upstream070: pending log guard", self.outer_guard(parent, "upstream070"))
        child = folder / "public-synthetic-child.vhdx"
        self.copy(parent, child)
        replay = self.command("upstream070: replay", [self.qemu, "check", "-r", "all", "-f", "vhdx", child])
        self.check("upstream070: replay exit", replay.returncode == 0)
        io = self.qemu.with_name("qemu-io")
        pattern = self.command("upstream070: bounded pattern read",
                               [io, "-r", "-f", "vhdx", "-c", "read -P 0xa5 0 18M", child])
        self.check("upstream070: actual expected 18MiB data", pattern.returncode == 0)
        self.check("upstream070: public fixture parent preserved", sha(parent) == before and sha(sample) == before)

    def path_guard(self):
        outside = self.root / "outside.bin"
        outside.write_bytes(b"synthetic preservation marker")
        link = self.root / "symlink.vhdx"
        link.symlink_to(outside)
        try:
            regular_owned(link, self.root)
        except AssertionError:
            self.check("symlink refused before tool invocation", True)
        else:
            raise AssertionError("symlink accepted")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--new-synthetic-root", required=True, type=Path)
    parser.add_argument("--qemu-img", required=True, type=Path)
    parser.add_argument("--upstream-synthetic-fixture", type=Path)
    args = parser.parse_args()
    root = args.new_synthetic_root.absolute()
    assert os.geteuid() != 0, "No privileged rehearsal"
    assert not root.exists() and not root.is_symlink()
    assert not any(x in str(root).lower() for x in ["docker_data", "sealed-cold", "working-copy", "cold-recovery"])
    assert '"' not in str(root) and "\n" not in str(root)
    os.umask(0o077)
    root.mkdir(mode=0o700)
    resource.setrlimit(resource.RLIMIT_FSIZE, (128*1024*1024,128*1024*1024))
    resource.setrlimit(resource.RLIMIT_AS, (2*1024**3,2*1024**3))
    resource.setrlimit(resource.RLIMIT_CPU, (600,600))
    rehearsal = Rehearsal(root, args.qemu_img, Path(__file__).with_name("toolchain.json"))
    try:
        rehearsal.pipeline("committed", True)
        rehearsal.pipeline("uncommitted", False)
        rehearsal.pipeline("bad-journal-checksum", True, True)
        rehearsal.invalid_log()
        rehearsal.path_guard()
        if args.upstream_synthetic_fixture:
            rehearsal.upstream_fixture(args.upstream_synthetic_fixture)
        result = {"success": True, "synthetic_only": True, "real_images_opened": False,
                  "mount_nbd_privilege_used": False, "checks": rehearsal.checks,
                  "images": rehearsal.images, "commands": rehearsal.commands,
                  "elapsed_seconds": round(time.monotonic()-rehearsal.start,3),
                  "maximum_individual_image_bytes": 36*fixtures.MIB,
                  "file_size_limit_bytes":128*1024*1024,
                  "fixture_generator_sha256":sha(Path(__file__).with_name("fixtures.py")),
                  "runner_sha256":sha(Path(__file__)),
                  "toolchain_manifest_sha256":sha(Path(__file__).with_name("toolchain.json"))}
    except Exception as error:
        result = {"success":False,"synthetic_only":True,"error_type":type(error).__name__,
                  "message":str(error)[:250],"checks":rehearsal.checks,
                  "commands":rehearsal.commands,"partial_outputs_retained":True}
    with (root/"result.json").open("x") as target:
        json.dump(result,target,indent=2)
    print(json.dumps({k:v for k,v in result.items() if k not in ["commands","checks"]},indent=2))
    print("executed assertions:",len(rehearsal.checks))
    return 0 if result["success"] else 1

if __name__ == "__main__":
    sys.exit(main())
