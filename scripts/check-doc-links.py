"""Check repository-local Markdown targets; external URLs are not fetched."""
import pathlib
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit

root = pathlib.Path(__file__).resolve().parent.parent
files = subprocess.check_output(["git", "ls-files", "*.md"], cwd=root, text=True).splitlines()
errors = []
for name in files:
    path = root / name
    for match in re.finditer(r"(?<!!)\[[^\]]*\]\(([^)]+)\)", path.read_text()):
        target = match[1].split(' "', 1)[0].strip("<>")
        parsed = urlsplit(target)
        if parsed.scheme or not parsed.path:
            continue
        dest = (path.parent / unquote(parsed.path)).resolve()
        if not dest.exists():
            errors.append(f"{name}: missing {target}")
if errors:
    print("\n".join(errors))
    sys.exit(1)
print(f"PASS: local Markdown targets in {len(files)} tracked documents; external URLs not checked")
