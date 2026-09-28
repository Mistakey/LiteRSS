# Development verification scripts

Choose the applicable checks using [Testing](../docs/TESTING.md#verification-by-task). Full checks are an explicit convenience, not the minimum for every edit.

## Entry points

`verify.py` is the shared implementation. It requires Python 3.8+, Git, Go, gofmt, goimports, Node/npm and installed frontend dependencies for a full check. Toolchain versions are in [Build Requirements](../docs/BUILD_REQUIREMENTS.md).

| Command | Work performed |
| --- | --- |
| `python scripts/verify.py check` / `make check` | Read-only tracked-Go formatting and frontend lint; frontend unit tests/build, Go vet/internal tests/build, each once |
| `python scripts/verify.py release` / `make release-check` | The above once, then `go mod tidy -diff`, dependency audit and frontend/Go version comparison |
| `python scripts/verify.py format [files...]` | Read-only gofmt/goimports comparison, ignoring CRLF/LF differences; explicit files or all tracked Go files |
| `python scripts/verify.py versions` | Frontend/Go version agreement only |

All modes fail on a failed command. No mode rewrites source or module files; builds and tests can create their normal artifacts/caches. Full checks do not package installers, run E2E or prove desktop visual acceptance. Newly added untracked Go files should be passed explicitly to format mode until tracked.

The compatibility wrappers `check.ps1` / `check.sh` and `pre-release.ps1` / `pre-release.sh` delegate to this implementation. They work from any directory. PowerShell defaults to `python`, Bash to `python3`; set `PYTHON` to an executable path if needed. For Make use `make check PYTHON=/path/to/python`.

Release checks can run on uncommitted patches. They test dependency consistency rather than requiring a commit. Resolve audit/module findings within the authorized scope; publishing the intended committed revision and platform acceptance are separate release responsibilities.

`make clean` removes build artifacts only when explicitly requested; it is not needed after a successful task. Rewriting format commands are separate from checks; preserve original line endings and review only the changes belonging to the task.

## Browser forensics

`browser-forensics.mjs` loads a page over the Chrome DevTools protocol and reports requests with their CSP header, console output, off-origin resources, an optional screenshot and an optional CSP probe. It needs Node 22+ and a running dev instance; steps are in [Testing](../docs/TESTING.md#浏览器取证).

## App icons

`render_icons.py` renders `build/windows/icon.ico`, `build/appicon.png` and `build/darwin/icons.icns` from the SVG sources in `frontend/public/assets`. It needs Playwright's `chrome-headless-shell` and `wails3`; run it after changing an icon SVG and commit the outputs. See [Build Requirements](../docs/BUILD_REQUIREMENTS.md#安装包与发布).

## CI and maintenance

GitHub Actions currently defines its own platform jobs in `.github/workflows/`; it does not call these wrappers. Keep the underlying commands consistent when changing verification behavior.

Prefer one portable implementation for shared behavior. Add shell wrappers only for supported entry points that need them; a platform-specific helper does not need a second implementation. Exercise command failure propagation, relevant paths and supported platforms. Document unavailable platform checks rather than treating them as passed.

Run `python -m unittest discover -s scripts -p "test_verify.py"` after changing the shared runner. These tests use isolated fixtures and fake command boundaries so they do not need network access or modify application data.
