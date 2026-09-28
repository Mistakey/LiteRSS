package update

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"LiteRSS/internal/update/updatetest"
)

const testRepo = "Mistakey/LiteRSS"

func TestInstallerNames(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch string
		want         []string
	}{
		{"windows", "amd64", []string{"LiteRSS-2.0.0-windows-amd64-installer.exe"}},
		{"windows", "arm64", []string{"LiteRSS-2.0.0-windows-arm64-installer.exe"}},
		{"darwin", "arm64", []string{"LiteRSS-2.0.0-darwin-universal.dmg", "LiteRSS-2.0.0-darwin-arm64.dmg"}},
		{"linux", "amd64", nil},
	} {
		if got := InstallerNames("2.0.0", tc.goos, tc.goarch); !slices.Equal(got, tc.want) {
			t.Errorf("%s/%s: %v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func testRelease(assets ...string) release {
	rel := release{TagName: "v2.0.0"}
	for _, a := range assets {
		name, url, _ := strings.Cut(a, "=")
		if url == "" {
			url = "https://github.com/" + testRepo + "/releases/download/v2.0.0/" + name
		}
		rel.Assets = append(rel.Assets, Asset{Name: name, URL: url})
	}
	return rel
}

func TestPickAssetsTakesOnlyThisRelease(t *testing.T) {
	const exe = "LiteRSS-2.0.0-windows-amd64-installer.exe"
	rel := testRelease("LiteRSS-2.0.0-windows-arm64-installer.exe", exe, "LiteRSS-2.0.0-darwin-universal.dmg", SumsName)

	installer, sums, err := pickAssets(rel, GitHubDownloads, testRepo, "windows", "amd64")
	if err != nil || installer.Name != exe || sums.Name != SumsName {
		t.Fatalf("windows/amd64: %+v %+v %v", installer, sums, err)
	}
	if installer, _, err := pickAssets(rel, GitHubDownloads, testRepo, "darwin", "arm64"); err != nil || installer.Name != "LiteRSS-2.0.0-darwin-universal.dmg" {
		t.Fatalf("darwin/arm64: %+v %v", installer, err)
	}
	if _, _, err := pickAssets(rel, GitHubDownloads, testRepo, "linux", "amd64"); !errors.Is(err, errNoInstaller) {
		t.Fatalf("linux: %v", err)
	}

	for _, url := range []string{
		"https://github.com/someone/LiteRSS/releases/download/v2.0.0/" + exe,
		"https://github.com/" + testRepo + "/releases/download/v1.0.0/" + exe,
		"https://evil.example/" + testRepo + "/releases/download/v2.0.0/" + exe,
		"http://github.com/" + testRepo + "/releases/download/v2.0.0/" + exe,
		"https://github.com/" + testRepo + "/releases/download/v2.0.0/../v2.0.0/" + exe,
	} {
		rel := testRelease(exe+"="+url, SumsName)
		if _, _, err := pickAssets(rel, GitHubDownloads, testRepo, "windows", "amd64"); !errors.Is(err, errNoInstaller) {
			t.Errorf("%s: %v, want no installer", url, err)
		}
	}
	if _, _, err := pickAssets(testRelease(exe), GitHubDownloads, testRepo, "windows", "amd64"); !errors.Is(err, errNoSums) {
		t.Fatalf("no sums: %v", err)
	}
	foreignSums := testRelease(exe, SumsName+"=https://evil.example/SHA256SUMS")
	if _, _, err := pickAssets(foreignSums, GitHubDownloads, testRepo, "windows", "amd64"); !errors.Is(err, errNoSums) {
		t.Fatalf("foreign sums: %v", err)
	}
}

func TestSumFor(t *testing.T) {
	const a = "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e"
	const b = "B591A6D40BF420404A011733CFB7B190D62C65BF0BCDA32B57B277D9AD9F146E"
	sums := a + "  one.exe\n" + b + " *two.dmg\nnothex  three\n" + a[:10] + "  four\n"
	for name, want := range map[string]string{
		"one.exe": a, "two.dmg": strings.ToLower(b), "three": "", "four": "", "five": "",
	} {
		if got := sumFor(sums, name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

func TestLocate(t *testing.T) {
	writable := func(ok ...string) func(string) bool {
		return func(dir string) bool { return slices.Contains(ok, dir) }
	}
	// Installed copies have the installer's Uninstall.exe; D:\copy does not.
	installed := func(p string) bool {
		return strings.HasSuffix(p, `\Uninstall.exe`) && !strings.HasPrefix(p, `D:\copy`)
	}
	for _, tc := range []struct {
		name string
		self Self
		want Target
	}{
		{"portable", Self{GOOS: "windows", Exe: `D:\LiteRSS\LiteRSS.exe`, Portable: true, Writable: writable(`D:\LiteRSS`)},
			Target{Why: "portable copy"}},
		{"windows per user", Self{GOOS: "windows", Exe: `C:\Users\u\AppData\Local\Programs\LiteRSS\LiteRSS.exe`,
			Writable: writable(`C:\Users\u\AppData\Local\Programs\LiteRSS`), Exists: installed},
			Target{Kind: WindowsInstaller, Dir: `C:\Users\u\AppData\Local\Programs\LiteRSS`}},
		{"windows program files", Self{GOOS: "windows", Exe: `C:\Program Files\LiteRSS\LiteRSS.exe`, Writable: writable(), Exists: installed},
			Target{Kind: WindowsInstaller, Dir: `C:\Program Files\LiteRSS`, Elevate: true}},
		{"windows copied exe", Self{GOOS: "windows", Exe: `D:\copy\LiteRSS.exe`, Writable: writable(`D:\copy`), Exists: installed},
			Target{Why: `no Uninstall.exe beside D:\copy\LiteRSS.exe`}},
		{"mac applications", Self{GOOS: "darwin", Exe: "/Applications/LiteRSS.app/Contents/MacOS/LiteRSS", Writable: writable("/Applications")},
			Target{Kind: MacBundle, App: "/Applications/LiteRSS.app"}},
		{"mac read-only folder", Self{GOOS: "darwin", Exe: "/Applications/LiteRSS.app/Contents/MacOS/LiteRSS", Writable: writable()},
			Target{Why: "cannot write /Applications"}},
		{"mac outside a bundle", Self{GOOS: "darwin", Exe: "/Users/u/bin/LiteRSS", Writable: writable("/Users/u/bin")},
			Target{Why: "executable /Users/u/bin/LiteRSS is not inside an .app bundle"}},
		{"linux", Self{GOOS: "linux", Exe: "/usr/bin/literss", Writable: writable("/usr/bin")},
			Target{Why: "linux has no in-app update"}},
	} {
		if got := Locate(tc.self); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestPlanWindowsRunsTheInstallerSilentlyOverTheRunningDirectory(t *testing.T) {
	const file = `C:\Temp\LiteRSS-update\LiteRSS-2.0.0-windows-amd64-installer.exe`
	plan := PlanInstall(Target{Kind: WindowsInstaller, Dir: `C:\Users\u\AppData\Local\Programs\LiteRSS`}, file, 42)
	want := Command{Path: file, Args: []string{"/S", `/D=C:\Users\u\AppData\Local\Programs\LiteRSS`}, Detach: true}
	if len(plan.Steps) != 1 || !commandEqual(plan.Steps[0], want) || len(plan.Cleanup) != 0 {
		t.Fatalf("plan = %+v", plan)
	}

	// A directory with spaces stays unquoted: NSIS takes /D= to the line's end.
	plan = PlanInstall(Target{Kind: WindowsInstaller, Dir: `C:\Program Files\LiteRSS`, Elevate: true}, file, 42)
	if got := plan.Steps[0].String(); got != `runas "`+strings.ReplaceAll(file, `\`, `\\`)+`" /S /D=C:\Program Files\LiteRSS` {
		t.Fatalf("elevated command = %s", got)
	}
	if !plan.Steps[0].Elevate {
		t.Fatal("an unwritable directory must elevate")
	}
	if got := PlanInstall(Target{Why: "portable copy"}, file, 42); len(got.Steps) != 0 {
		t.Fatalf("release page target planned %+v", got)
	}
}

func commandEqual(a, b Command) bool {
	return a.Path == b.Path && slices.Equal(a.Args, b.Args) && a.Elevate == b.Elevate && a.Detach == b.Detach
}

func TestPlanMacCopiesBesideTheBundleAndSwapsAfterQuit(t *testing.T) {
	const dmg = "/tmp/LiteRSS-update/LiteRSS-2.0.0-darwin-universal.dmg"
	plan := PlanInstall(Target{Kind: MacBundle, App: "/Applications/LiteRSS.app"}, dmg, 42)
	m := NewMacSwap(dmg, "/Applications/LiteRSS.app", 42)
	if m.Mount != "/tmp/LiteRSS-update/mnt" || m.Source != "/tmp/LiteRSS-update/mnt/LiteRSS.app" ||
		m.Staging != "/Applications/.LiteRSS-update.app" || m.Backup != "/Applications/.LiteRSS-old.app" {
		t.Fatalf("swap = %+v", m)
	}
	want := []Command{
		{Path: "/usr/bin/hdiutil", Args: []string{"attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", m.Mount, dmg}},
		{Path: "/bin/rm", Args: []string{"-rf", m.Staging}},
		{Path: "/usr/bin/ditto", Args: []string{m.Source, m.Staging}},
		{Path: "/bin/sh", Args: []string{"-c", m.Script()}, Detach: true},
	}
	if !slices.EqualFunc(plan.Steps, want, commandEqual) {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	cleanup := []Command{
		{Path: "/usr/bin/hdiutil", Args: []string{"detach", m.Mount, "-force"}},
		{Path: "/bin/rm", Args: []string{"-rf", m.Staging}},
	}
	if !slices.EqualFunc(plan.Cleanup, cleanup, commandEqual) {
		t.Fatalf("cleanup = %+v", plan.Cleanup)
	}
	if !strings.Contains(m.Script(), "kill -0 42") {
		t.Fatalf("script does not wait for the app:\n%s", m.Script())
	}
	if got := shellQuote("/Users/o'neil/Apps"); got != `'/Users/o'\''neil/Apps'` {
		t.Fatalf("quote = %s", got)
	}
}

// runSwap runs a swap script with the system tools replaced by echo, and
// returns its output.
func runSwap(t *testing.T, m MacSwap) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell")
	}
	m.Hdiutil, m.Open = "echo", "echo"
	out, _ := exec.Command(sh, "-c", m.Script()).CombinedOutput()
	return string(out)
}

func writeBundle(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "body"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBundle(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "body"))
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return string(data)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestMacSwapScript(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	app := dir + "/Apps/Lite RSS.app"
	dmg := dir + "/dl/LiteRSS.dmg"
	// 999999 is above the process IDs macOS hands out: already exited.
	m := NewMacSwap(dmg, app, 999999)
	writeBundle(t, app, "old")
	writeBundle(t, m.Staging, "new")
	os.MkdirAll(filepath.Dir(dmg), 0o755)
	os.WriteFile(dmg, []byte("dmg"), 0o644)

	out := runSwap(t, m)
	if got := readBundle(t, app); got != "new" {
		t.Fatalf("app holds %q after the swap; output:\n%s", got, out)
	}
	if exists(m.Staging) || exists(m.Backup) || exists(dmg) {
		t.Fatalf("left behind: staging %v, backup %v, dmg %v", exists(m.Staging), exists(m.Backup), exists(dmg))
	}
	if strings.TrimSpace(out) != app {
		t.Fatalf("opened %q, want %q", out, app)
	}

	// The new bundle cannot be moved in: the old one is put back and opened,
	// even over a backup an interrupted swap left behind.
	os.RemoveAll(m.Staging)
	writeBundle(t, app, "old")
	writeBundle(t, m.Backup, "stale")
	out = runSwap(t, m)
	if got := readBundle(t, app); got != "old" || exists(m.Backup) {
		t.Fatalf("app holds %q, backup left %v; output:\n%s", got, exists(m.Backup), out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "\n"+app) {
		t.Fatalf("the old app was not reopened: %q", out)
	}
}

// fakeRelease serves a release with installers for Windows amd64 and the
// universal DMG.
func fakeRelease(t *testing.T, r updatetest.Release) (*updatetest.Server, *httptest.Server) {
	t.Helper()
	fake := updatetest.New(testRepo)
	if r.Tag == "" {
		r.Tag = "v2.0.0"
	}
	if r.Files == nil {
		r.Files = map[string][]byte{
			"LiteRSS-2.0.0-windows-amd64-installer.exe": []byte(strings.Repeat("installer ", 100)),
			"LiteRSS-2.0.0-darwin-universal.dmg":        []byte("dmg"),
		}
	}
	fake.Publish(r)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

type testUpdater struct {
	*Updater
	mu         sync.Mutex
	ran        []Plan
	runErr     error
	quits      int
	trayFailed []Status
}

func newTestUpdater(t *testing.T, srv *httptest.Server, installs bool, target Target) *testUpdater {
	t.Helper()
	c := &Checker{Repo: testRepo, API: srv.URL, Downloads: srv.URL, Current: "1.0.0", HTTP: srv.Client()}
	tu := &testUpdater{Updater: NewUpdater(context.Background(), c)}
	tu.GOOS, tu.GOARCH, tu.Installs, tu.PID = "windows", "amd64", installs, 42
	tu.Dir = filepath.Join(t.TempDir(), "LiteRSS-update")
	tu.Locate = func() Target { return target }
	tu.Run = func(p Plan) error {
		tu.mu.Lock()
		defer tu.mu.Unlock()
		tu.ran = append(tu.ran, p)
		return tu.runErr
	}
	tu.Quit = func() { tu.mu.Lock(); tu.quits++; tu.mu.Unlock() }
	tu.TrayFailed = func(s Status) { tu.mu.Lock(); tu.trayFailed = append(tu.trayFailed, s); tu.mu.Unlock() }
	return tu
}

// wait returns the status once the update is no longer busy.
func (tu *testUpdater) wait(t *testing.T) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := tu.Status(); !st.Busy() || (st.State == StateInstalling && tu.quitCount() > 0) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("update still busy: %+v", tu.Status())
	return Status{}
}

func (tu *testUpdater) quitCount() int {
	tu.mu.Lock()
	defer tu.mu.Unlock()
	return tu.quits
}

func downloaded(t *testing.T, dir string) []string {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

var winTarget = Target{Kind: WindowsInstaller, Dir: `C:\Users\u\AppData\Local\Programs\LiteRSS`}

func TestUpdateInstallsAndQuits(t *testing.T) {
	_, srv := fakeRelease(t, updatetest.Release{})
	tu := newTestUpdater(t, srv, true, winTarget)

	if st := tu.Start(false); st.State != StateDownloading {
		t.Fatalf("Start = %+v", st)
	}
	st := tu.wait(t)
	if st.State != StateInstalling || st.Version != "2.0.0" || st.Received != st.Total || st.Total != 1000 {
		t.Fatalf("status = %+v", st)
	}
	file := filepath.Join(tu.Dir, "LiteRSS-2.0.0-windows-amd64-installer.exe")
	if len(tu.ran) != 1 || !slices.EqualFunc(tu.ran[0].Steps, []Command{{Path: file,
		Args: []string{"/S", `/D=C:\Users\u\AppData\Local\Programs\LiteRSS`}, Detach: true}}, commandEqual) {
		t.Fatalf("ran %+v", tu.ran)
	}
	if tu.quitCount() != 1 || len(tu.trayFailed) != 0 {
		t.Fatalf("quits %d, tray failures %v", tu.quitCount(), tu.trayFailed)
	}
}

func TestDevelopmentBuildStopsAfterTheChecksum(t *testing.T) {
	_, srv := fakeRelease(t, updatetest.Release{})
	tu := newTestUpdater(t, srv, false, winTarget)

	if !tu.Check(context.Background()).InApp {
		t.Fatal("a development build must walk the update for forensics")
	}
	tu.Start(false)
	st := tu.wait(t)
	if st.State != StateNotInstalled || st.Message != "开发构建不安装：安装包已下载并通过校验。" {
		t.Fatalf("status = %+v", st)
	}
	if len(tu.ran) != 0 || tu.quitCount() != 0 {
		t.Fatalf("a development build ran %+v and quit %d times", tu.ran, tu.quitCount())
	}
	if got := downloaded(t, tu.Dir); !slices.Equal(got, []string{"LiteRSS-2.0.0-windows-amd64-installer.exe"}) {
		t.Fatalf("downloaded %v", got)
	}
}

func TestUpdateFailures(t *testing.T) {
	const page = "https://github.com/" + testRepo + "/releases/tag/v2.0.0"
	for _, tc := range []struct {
		name     string
		release  updatetest.Release
		target   Target
		runErr   error
		goarch   string
		want     string
		wantPage string
	}{
		{name: "checksum mismatch", release: updatetest.Release{Corrupt: "LiteRSS-2.0.0-windows-amd64-installer.exe"},
			want: "下载的安装包校验不符，已删除，请到发布页下载。"},
		{name: "no SHA256SUMS", release: updatetest.Release{NoSums: true},
			want: "这次发布缺少校验和文件，没有下载，请到发布页下载。"},
		{name: "not listed in SHA256SUMS", release: updatetest.Release{Files: map[string][]byte{
			"LiteRSS-2.0.0-windows-amd64-installer.exe": []byte("x"), SumsName: []byte(updatetest.Sums(map[string][]byte{"other": nil}))}},
			want: "校验和文件里没有这个安装包，没有下载，请到发布页下载。"},
		{name: "no installer for this platform", goarch: "arm64",
			want: "这次发布里没有本机可用的安装包，请到发布页下载。"},
		{name: "up to date", release: updatetest.Release{Tag: "v1.0.0"}, want: "已是最新版本。",
			wantPage: "https://github.com/" + testRepo + "/releases/tag/v1.0.0"},
		{name: "portable copy", target: Target{Why: "portable copy"},
			want: "这份 LiteRSS 不能在应用内更新，请到发布页下载。"},
		{name: "UAC declined", target: Target{Kind: WindowsInstaller, Dir: `C:\Program Files\LiteRSS`, Elevate: true},
			runErr: ErrCancelled, want: "没有获得安装所需的权限，更新已取消。"},
		{name: "installer does not start", runErr: errors.New("boom"), want: "启动安装失败，请到发布页下载。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := fakeRelease(t, tc.release)
			target := tc.target
			if target == (Target{}) {
				target = winTarget
			}
			tu := newTestUpdater(t, srv, true, target)
			tu.runErr = tc.runErr
			if tc.goarch != "" {
				tu.GOARCH = tc.goarch
			}
			tu.Start(true)
			st := tu.wait(t)
			wantPage := tc.wantPage
			if wantPage == "" {
				wantPage = page
			}
			if st.State != StateFailed || st.Message != tc.want || st.ReleaseURL != wantPage {
				t.Fatalf("status = %+v, want message %q and page %s", st, tc.want, wantPage)
			}
			if tu.quitCount() != 0 {
				t.Fatal("a failed update quit LiteRSS")
			}
			if len(tu.trayFailed) != 1 || tu.trayFailed[0] != st {
				t.Fatalf("tray told %v", tu.trayFailed)
			}
			if tc.runErr == nil {
				if got := downloaded(t, tu.Dir); len(got) != 0 {
					t.Fatalf("left %v in the download directory", got)
				}
			}
		})
	}
}

func TestUpdateCheckFailureKeepsTheReleaseList(t *testing.T) {
	_, srv := fakeRelease(t, updatetest.Release{})
	tu := newTestUpdater(t, srv, true, winTarget)
	tu.Checker.Repo = "Mistakey/Nothing"
	tu.Start(false)
	st := tu.wait(t)
	if st.State != StateFailed || st.Message != "还没有发布过版本。" || st.ReleaseURL != "https://github.com/Mistakey/Nothing/releases" {
		t.Fatalf("status = %+v", st)
	}
	if len(tu.trayFailed) != 0 {
		t.Fatal("a panel update told the tray")
	}
}

func TestCheckSaysWhetherTheUpdateInstallsInApp(t *testing.T) {
	_, srv := fakeRelease(t, updatetest.Release{})
	if !newTestUpdater(t, srv, true, winTarget).Check(context.Background()).InApp {
		t.Fatal("installed copy: InApp false")
	}
	if newTestUpdater(t, srv, true, Target{Why: "portable copy"}).Check(context.Background()).InApp {
		t.Fatal("portable copy: InApp true")
	}
	_, current := fakeRelease(t, updatetest.Release{Tag: "v1.0.0"})
	if newTestUpdater(t, current, true, winTarget).Check(context.Background()).InApp {
		t.Fatal("up to date: InApp true")
	}
}

func TestUpdateReportsProgressAndRunsOnce(t *testing.T) {
	_, srv := fakeRelease(t, updatetest.Release{Rate: 2000})
	tu := newTestUpdater(t, srv, false, winTarget)
	tu.Start(false)

	deadline := time.Now().Add(10 * time.Second)
	var st Status
	for st = tu.Status(); st.State != StateDownloading || st.Received == 0; st = tu.Status() {
		if time.Now().After(deadline) || !st.Busy() {
			t.Fatalf("no progress seen: %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if st.Total != 1000 || st.Received >= st.Total {
		t.Fatalf("progress = %+v", st)
	}
	if again := tu.Start(false); again.State != StateDownloading || again.Received == 0 {
		t.Fatalf("a second Start while downloading = %+v", again)
	}
	if st := tu.wait(t); st.State != StateNotInstalled {
		t.Fatalf("status = %+v", st)
	}
}
