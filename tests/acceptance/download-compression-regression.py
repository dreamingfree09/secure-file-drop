"""Run the same real-HTTP test with reviewed middleware and corrected middleware.
Go overlays never overwrite source. Only the old, source-pinned gzip defect is
expected to fail; compilation or unrelated failures are not accepted evidence.
"""
import hashlib, json, pathlib, subprocess, tempfile, urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
BASE = "cd464b8dab57236230a706b236144155ff7b0de7"
SHA256 = "ca6144494de50d17538d022b7c16a1ddb5b3bf6147ff6f6254aee8bbadeec934"
with urllib.request.urlopen("https://raw.githubusercontent.com/dreamingfree09/secure-file-drop/" + BASE + "/internal/server/compression.go", timeout=20) as response:
    raw = response.read(100_000)
assert hashlib.sha256(raw).hexdigest() == SHA256
fixture = pathlib.Path(tempfile.mkdtemp(prefix="sfd-compression-red-"))
legacy = fixture / "reviewed.go"
legacy.write_bytes(raw)
overlay = fixture / "overlay.json"
overlay.write_text(json.dumps({"Replace": {str(ROOT / "internal/server/compression.go"): str(legacy)}}), encoding="utf-8")
command = ["go", "test", "./internal/server", "-run", "^TestBinaryDownloadCompression$", "-count=1", "-v"]
red = subprocess.run(command[:2] + ["-overlay", str(overlay)] + command[2:], cwd=ROOT, capture_output=True, text=True)
assert red.returncode != 0 and '/receive-download encoding="gzip"' in red.stdout and '--- FAIL: TestBinaryDownloadCompression' in red.stdout, "expected reviewed HTTP corruption was not reproduced"
green = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
assert green.returncode == 0, green.stdout + green.stderr
print(json.dumps({"reviewed_revision": BASE, "reviewed_middleware_SHA256": SHA256, "reviewed_HTTP_regression_failed": True, "corrected_HTTP_regression_passed": True, "source_overwritten": False}))
