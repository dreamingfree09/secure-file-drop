"""Synthetic validator fixtures; never loads the project's environment file."""
import pathlib
import subprocess
import tempfile

script = pathlib.Path(__file__).with_name("validate-env.sh").resolve()
values = {
    "POSTGRES_DB": "fixture", "POSTGRES_USER": "fixture",
    "POSTGRES_PASSWORD": "SyntheticFixturePassword123456",
    "DATABASE_URL": "postgres://fixture:SyntheticFixturePassword123456@localhost/fixture",
    "MINIO_ROOT_USER": "fixture", "MINIO_ROOT_PASSWORD": "SyntheticFixturePassword123456",
    "SFD_BUCKET": "fixture", "SFD_ADMIN_USER": "fixture",
    "SFD_ADMIN_PASS": "SyntheticFixturePassword123456",
    "SFD_SESSION_SECRET": "s" * 64, "SFD_DOWNLOAD_SECRET": "d" * 64,
    "SFD_MAX_UPLOAD_BYTES": "1024", "SFD_MAX_REQUEST_BYTES": "1049600",
    "SFD_PUBLIC_BASE_URL": "https://localhost:8443",
}

def run(changes):
    fixture = values | changes
    with tempfile.TemporaryDirectory(prefix="sfd-validator-") as directory:
        path = pathlib.Path(directory) / "synthetic.env"
        path.write_text("\n".join(f"{key}='{value}'" for key, value in fixture.items()) + "\n")
        result = subprocess.run(["bash", str(script), str(path)], capture_output=True, text=True)
    for key in ["POSTGRES_PASSWORD", "SFD_ADMIN_PASS", "MINIO_ROOT_PASSWORD",
                "SFD_SESSION_SECRET", "SFD_DOWNLOAD_SECRET"]:
        assert fixture[key] not in result.stdout if len(fixture[key]) > 10 else True
    return result

assert run({}).returncode == 0
warning = run({"SFD_ADMIN_PASS": "Short123"})
assert warning.returncode == 0 and "1 warning(s)" in warning.stdout
errors = run({"SFD_ADMIN_PASS": "CHANGE_ME_SENTINEL_PRIVATE_VALUE",
              "SFD_MAX_UPLOAD_BYTES": "PRIVATE_NONNUMBER_SENTINEL"})
assert errors.returncode == 1 and "2 error(s)" in errors.stdout
assert "PRIVATE_NONNUMBER_SENTINEL" not in errors.stdout
assert "Advanced Checks" in errors.stdout
print("PASS: validator success, warning continuation, accumulated errors and redacted values")
