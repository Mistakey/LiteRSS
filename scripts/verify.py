"""Shared, fail-fast local verification. Never formats or tidies source files."""

import argparse
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def run(*args, cwd=ROOT, capture=False):
    executable = shutil.which(args[0])
    if not executable:
        raise RuntimeError(f"Required executable not found: {args[0]}")
    print(f"[{Path(cwd).name}] {' '.join(args)}", flush=True)
    return subprocess.run(
        [executable, *args[1:]], cwd=cwd, check=True,
        stdout=subprocess.PIPE if capture else None,
    ).stdout


def check_format(files=None):
    explicit = files is not None
    if files is None:
        output = run("git", "ls-files", "-z", "--", "*.go", capture=True)
        files = [p for p in output.decode().split("\0") if p]
    failed = []
    for name in files:
        path = (ROOT / name).resolve()
        path.relative_to(ROOT)  # Keep explicit file arguments inside the repository.
        if path.suffix != ".go" or not path.is_file():
            if explicit:
                raise RuntimeError(f"Expected an existing Go file: {name}")
            continue
        original = path.read_bytes().replace(b"\r\n", b"\n")
        for tool in ("gofmt", "goimports"):
            formatted = run(tool, str(path), capture=True).replace(b"\r\n", b"\n")
            if formatted != original:
                failed.append(f"{name}: {tool}")
    if failed:
        raise RuntimeError("Formatting differs (source unchanged):\n" + "\n".join(failed))


def check():
    check_format()
    frontend = ROOT / "frontend"
    run("npm", "run", "lint:check", cwd=frontend)
    run("npm", "run", "test:unit", cwd=frontend)
    # Go embeds frontend/dist, so always produce it before Go verification.
    run("npm", "run", "build", cwd=frontend)
    run("go", "vet", "./...")
    run("go", "test", "-timeout=5m", "./internal/...")
    run("go", "build", "./...")


def check_versions():
    frontend = json.loads((ROOT / "frontend/package.json").read_text(encoding="utf-8"))["version"]
    source = (ROOT / "internal/version/version.go").read_text(encoding="utf-8")
    match = re.search(r'\bconst\s+Version\s*=\s*"([^"]+)"', source)
    if not match or match[1] != frontend:
        raise RuntimeError(f"Version mismatch: frontend={frontend}, Go={match[1] if match else 'missing'}")
    print(f"Version consistent: {frontend}")


def release():
    check()
    # -diff reports required changes without rewriting go.mod/go.sum.
    run("go", "mod", "tidy", "-diff")
    run("npm", "audit", "--audit-level=moderate", cwd=ROOT / "frontend")
    check_versions()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("check", "release", "format", "versions"))
    parser.add_argument("files", nargs="*", help="format mode: selected Go files; omitted means tracked Go files")
    args = parser.parse_args()
    if args.files and args.mode != "format":
        parser.error("file arguments apply only to format mode")
    try:
        if args.mode == "format":
            check_format(args.files or None)
        else:
            {"check": check, "release": release, "versions": check_versions}[args.mode]()
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"Verification failed: {error}", file=sys.stderr)
        return 1
    print(f"{args.mode}: passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
