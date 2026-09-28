"""Render the app icons from the SVG sources in frontend/public/assets.

logo-<N>.svg are hand-tuned to the pixel grid and give the small ICO frames;
logo.svg gives the large frames and build/appicon.png. Small sizes are never
scaled from a large image: Windows would blur them in the tray and title bar.

Needs Playwright's chrome-headless-shell (docs/TESTING.md) and wails3.
Writes build/windows/icon.ico, build/appicon.png and build/darwin/icons.icns.
"""

import glob
import os
import shutil
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "frontend" / "public" / "assets"
LARGE_ICO_SIZES = (128, 256)


def headless_shell():
    pattern = os.path.join(
        os.environ.get("LOCALAPPDATA", ""), "ms-playwright", "chromium_headless_shell-*",
        "chrome-headless-shell-win64", "chrome-headless-shell.exe")
    found = sorted(glob.glob(pattern))
    if not found:
        sys.exit(f"chrome-headless-shell not found: {pattern}")
    return found[-1]


def render(shell, svg, size, out, workdir):
    page = Path(workdir) / f"render-{size}.html"
    page.write_text(
        "<!doctype html><style>html,body{margin:0;background:transparent}"
        f"img{{display:block}}</style><img src=\"{svg.as_uri()}\" width={size} height={size}>",
        encoding="utf-8")
    subprocess.run(
        [shell, "--hide-scrollbars", "--force-device-scale-factor=1",
         "--default-background-color=00000000", f"--window-size={size},{size}",
         f"--user-data-dir={Path(workdir) / 'profile'}", f"--screenshot={out}", page.as_uri()],
        check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def write_ico(path, frames):
    """Pack PNG frames into an ICO; Windows Vista and later read PNG entries."""
    header = struct.pack("<HHH", 0, 1, len(frames))
    entries, data = b"", b""
    offset = len(header) + 16 * len(frames)
    for size, png in frames:
        side = 0 if size >= 256 else size
        entries += struct.pack("<BBBBHHII", side, side, 0, 0, 1, 32, len(png), offset + len(data))
        data += png
    Path(path).write_bytes(header + entries + data)


def main():
    wails = shutil.which("wails3") or sys.exit("wails3 not found")
    shell = headless_shell()
    small = sorted((int(p.stem.split("-")[1]), p) for p in ASSETS.glob("logo-*.svg"))
    with tempfile.TemporaryDirectory() as tmp:
        frames = []
        for size, svg in small + [(s, ASSETS / "logo.svg") for s in LARGE_ICO_SIZES]:
            out = Path(tmp) / f"{size}.png"
            render(shell, svg, size, out, tmp)
            frames.append((size, out.read_bytes()))
        write_ico(ROOT / "build" / "windows" / "icon.ico", frames)
        render(shell, ASSETS / "logo.svg", 512, ROOT / "build" / "appicon.png", tmp)
    subprocess.run(
        [wails, "generate", "icons", "-input", "appicon.png", "-windowsfilename=",
         "-macfilename", "darwin/icons.icns"], cwd=ROOT / "build", check=True)
    print("wrote build/windows/icon.ico, build/appicon.png, build/darwin/icons.icns")


if __name__ == "__main__":
    main()
