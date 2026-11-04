"""Verify the pinned SDK fixture; evaluate only pure IaC with denied external IO.
No npm install, Railway CLI, account, credentials or live provider is used.
"""
import base64, hashlib, io, os, pathlib, subprocess, tarfile, tempfile, urllib.request

URL = "https://registry.npmjs.org/railway/-/railway-3.12.0.tgz"
INTEGRITY = "a6th+76NE3fZ8QMTOtEIe15sPTjObWOJYx0pCHCR/rd00r++I2rZHLKTn2R1qmPAwvGdAj6vY45GDVJs0AzOXA=="
ROOT = pathlib.Path(__file__).resolve().parents[2]
with urllib.request.urlopen(URL, timeout=30) as response:
    raw = response.read(5_000_001)
assert len(raw) <= 5_000_000 and base64.b64encode(hashlib.sha512(raw).digest()).decode() == INTEGRITY
archive = tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz")
member = archive.getmember("package/dist/iac/index.cjs")
assert member.isfile() and member.size < 1_000_000
source = archive.extractfile(member).read()
# New fixture only. Retain it on success/failure; never extract archive paths.
fixture = pathlib.Path(tempfile.mkdtemp(prefix="sfd-railway-iac-")) / "sdk.cjs"
with fixture.open("xb") as output:
    output.write(source)
env = dict(os.environ, SFD_RAILWAY_IAC_FIXTURE=str(fixture))
subprocess.run(["node", "--test", "tests/railway/config.test.cjs"], cwd=ROOT, env=env, check=True)
print("SDK source verified: railway3.12.0 sha512; no provider/CLI execution")
