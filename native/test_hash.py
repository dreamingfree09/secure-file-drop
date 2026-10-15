"""Known SHA-256 vectors and failure exit codes for the real native CLI."""
import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile

tool = str(pathlib.Path(sys.argv[1]).resolve())
vectors = [b"", b"abc", b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq",
           b"a" * 1000000, bytes(range(256)) * 513]
with tempfile.TemporaryDirectory(prefix="sfd-native-vectors-") as temp:
    path = pathlib.Path(temp) / "fixture.bin"
    for data in vectors:
        path.write_bytes(data)
        result = subprocess.run([tool, str(path)], check=True, capture_output=True, text=True)
        actual = json.loads(result.stdout)
        assert actual == {"algorithm": "sha256", "hash": hashlib.sha256(data).hexdigest(), "bytes": len(data)}, actual
    assert subprocess.run([tool], capture_output=True).returncode == 1
    assert subprocess.run([tool, str(path.parent / "missing")], capture_output=True).returncode == 2
    assert subprocess.run([tool, str(path.parent)], capture_output=True).returncode == 2
print("PASS: 5 native SHA-256 vectors, byte counts, usage and I/O failures")
