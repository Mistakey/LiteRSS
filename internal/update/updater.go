package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is a step of the in-app update (spec D20).
type State string

const (
	StateIdle        State = "idle"
	StateDownloading State = "downloading"
	StateVerifying   State = "verifying"
	// StateInstalling: the installer has started and LiteRSS is quitting.
	StateInstalling State = "installing"
	// StateNotInstalled ends a development build's update: downloaded and
	// verified, the installer is not started.
	StateNotInstalled State = "not_installed"
	StateFailed       State = "failed"
)

// Status is the update's progress. Version is the release being installed;
// Received and Total count the installer's bytes (Total 0 when unknown).
// A failed update has Message, the Chinese reason, and ReleaseURL, the page
// to download from by hand.
type Status struct {
	State      State  `json:"state"`
	Version    string `json:"version"`
	Received   int64  `json:"received"`
	Total      int64  `json:"total"`
	Message    string `json:"message"`
	ReleaseURL string `json:"release_url"`
}

// Busy reports whether an update is under way.
func (s Status) Busy() bool {
	return s.State == StateDownloading || s.State == StateVerifying || s.State == StateInstalling
}

// Updater runs the in-app update: one at a time, shared by the settings
// panel and the tray's dialog.
type Updater struct {
	Checker *Checker
	GOOS    string
	GOARCH  string
	// Installs lets the updater start the installer; development builds
	// stop at StateNotInstalled and only log the plan.
	Installs bool
	// Locate finds the running copy's target, asked at every check and start.
	Locate func() Target
	// Dir is where installers are downloaded; the next start clears it.
	Dir string
	// PID is LiteRSS's own process.
	PID int
	// Run runs a plan's commands (RunPlan).
	Run func(Plan) error
	// Quit ends LiteRSS once the installer has started, not to the tray.
	Quit func()
	// TrayFailed tells the user of a failed update the tray started.
	TrayFailed func(Status)
	// DownloadTimeout bounds one installer download.
	DownloadTimeout time.Duration

	// ctx ends downloads when LiteRSS quits.
	ctx context.Context

	mu     sync.Mutex
	status Status
}

// NewUpdater returns an updater whose downloads end with ctx.
func NewUpdater(ctx context.Context, c *Checker) *Updater {
	return &Updater{Checker: c, ctx: ctx, DownloadTimeout: 30 * time.Minute, status: Status{State: StateIdle}}
}

// Check is (*Checker).Check that also says whether the update installs
// from the app.
func (u *Updater) Check(ctx context.Context) Result {
	res := u.Checker.Check(ctx)
	res.InApp = res.UpdateAvailable && u.inApp(u.Locate())
	return res
}

func (u *Updater) inApp(t Target) bool {
	return !u.Installs || t.Kind != ReleasePage
}

// Status returns the update's progress.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Start begins an update unless one is under way, and returns the status.
// fromTray sends a failure to TrayFailed.
func (u *Updater) Start(fromTray bool) Status {
	u.mu.Lock()
	if u.status.Busy() {
		defer u.mu.Unlock()
		return u.status
	}
	u.status = Status{State: StateDownloading}
	st := u.status
	u.mu.Unlock()
	log.Printf("Update started (from tray: %v)", fromTray)

	go u.run(fromTray)
	return st
}

func (u *Updater) set(f func(*Status)) {
	u.mu.Lock()
	f(&u.status)
	u.mu.Unlock()
}

// failure is a failed update: msg is shown, err logged.
type failure struct {
	msg string
	err error
}

func (f *failure) Error() string { return f.msg }

func fail(msg string, err error) error { return &failure{msg: msg, err: err} }

func (u *Updater) run(fromTray bool) {
	releaseURL, err := u.update(u.ctx)
	if err == nil {
		return
	}
	var f *failure
	if !errors.As(err, &f) {
		f = &failure{msg: "更新失败。", err: err}
	}
	log.Printf("Update failed: %s (%v)", f.msg, f.err)
	st := Status{}
	u.set(func(s *Status) {
		*s = Status{State: StateFailed, Version: s.Version, Message: f.msg, ReleaseURL: releaseURL}
		st = *s
	})
	// An update cut short by LiteRSS quitting is not worth a dialog.
	if fromTray && u.TrayFailed != nil && u.ctx.Err() == nil {
		u.TrayFailed(st)
	}
}

// update downloads, verifies and installs the latest release. releaseURL is
// the page to download from by hand: the release's once known, else the
// repository's release list.
func (u *Updater) update(ctx context.Context) (releaseURL string, err error) {
	releaseURL = u.Checker.ReleasesPage()
	res, rel := u.Checker.latest(ctx)
	if res.Message != "" {
		return releaseURL, fail(res.Message, nil)
	}
	releaseURL = res.ReleaseURL
	if !res.UpdateAvailable {
		return releaseURL, fail("已是最新版本。", nil)
	}
	u.set(func(s *Status) { s.Version = res.LatestVersion })

	target := u.Locate()
	if !u.inApp(target) {
		return releaseURL, fail("这份 LiteRSS 不能在应用内更新，请到发布页下载。", errors.New(target.Why))
	}
	installer, sums, err := pickAssets(rel, u.Checker.Downloads, u.Checker.Repo, u.GOOS, u.GOARCH)
	switch {
	case errors.Is(err, errNoInstaller):
		return releaseURL, fail("这次发布里没有本机可用的安装包，请到发布页下载。", err)
	case errors.Is(err, errNoSums):
		return releaseURL, fail("这次发布缺少校验和文件，没有下载，请到发布页下载。", err)
	}

	list, err := u.fetchSums(ctx, sums)
	if err != nil {
		return releaseURL, err
	}
	want := sumFor(list, installer.Name)
	if want == "" {
		return releaseURL, fail("校验和文件里没有这个安装包，没有下载，请到发布页下载。", nil)
	}

	u.set(func(s *Status) { s.Total = installer.Size })
	file, got, err := u.fetchInstaller(ctx, installer)
	if err != nil {
		return releaseURL, err
	}
	u.set(func(s *Status) { s.State = StateVerifying })
	if got != want {
		os.Remove(file)
		return releaseURL, fail("下载的安装包校验不符，已删除，请到发布页下载。",
			fmt.Errorf("%s: SHA-256 %s, SHA256SUMS lists %s", installer.Name, got, want))
	}

	plan := PlanInstall(target, file, u.PID)
	if !u.Installs {
		for _, c := range plan.Steps {
			log.Printf("Update: development build, not running %s", c)
		}
		if len(plan.Steps) == 0 {
			log.Printf("Update: development build; an installed build would send the user to the release page: %s", target.Why)
		}
		u.set(func(s *Status) {
			s.State = StateNotInstalled
			s.Message = "开发构建不安装：安装包已下载并通过校验。"
		})
		return releaseURL, nil
	}

	u.set(func(s *Status) { s.State = StateInstalling })
	for _, c := range plan.Steps {
		log.Printf("Update: running %s", c)
	}
	if err := u.Run(plan); err != nil {
		if errors.Is(err, ErrCancelled) {
			return releaseURL, fail("没有获得安装所需的权限，更新已取消。", err)
		}
		return releaseURL, fail("启动安装失败，请到发布页下载。", err)
	}
	u.Quit()
	return releaseURL, nil
}

// maxSums bounds the checksum list.
const maxSums = 64 << 10

// fetchSums reads the checksum list, bounded like a release check.
func (u *Updater) fetchSums(ctx context.Context, a Asset) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	resp, err := u.get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSums))
	if err != nil {
		return "", fail("下载校验和文件失败，请检查网络或代理设置。", err)
	}
	return string(data), nil
}

// fetchInstaller saves the installer in Dir and returns its path and
// SHA-256. A partial file is removed.
func (u *Updater) fetchInstaller(ctx context.Context, a Asset) (file, sum string, err error) {
	ctx, cancel := context.WithTimeout(ctx, u.DownloadTimeout)
	defer cancel()
	resp, err := u.get(ctx, a.URL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if a.Size <= 0 && resp.ContentLength > 0 {
		u.set(func(s *Status) { s.Total = resp.ContentLength })
	}

	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return "", "", fail("保存安装包失败。", err)
	}
	file = filepath.Join(u.Dir, filepath.Base(a.Name))
	out, err := os.Create(file)
	if err != nil {
		return "", "", fail("保存安装包失败。", err)
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h, progress{u}), resp.Body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(file)
		return "", "", fail("下载安装包中断，请检查网络或代理设置。", err)
	}
	return file, hex.EncodeToString(h.Sum(nil)), nil
}

// get fetches a release asset. The shared client's timeout is meant for API
// calls, so downloads use a copy without it, on the same transport and so the
// same proxy (pitfall 8); ctx bounds them instead.
func (u *Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fail("下载失败。", err)
	}
	client := *u.Checker.HTTP
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return nil, fail("连不上 GitHub，请检查网络或代理设置。", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fail(fmt.Sprintf("下载失败，GitHub 返回错误 %d，请稍后再试。", resp.StatusCode),
			fmt.Errorf("GET %s: %d", url, resp.StatusCode))
	}
	return resp, nil
}

// progress counts downloaded bytes into the status.
type progress struct{ u *Updater }

func (p progress) Write(b []byte) (int, error) {
	p.u.set(func(s *Status) { s.Received += int64(len(b)) })
	return len(b), nil
}

// ErrCancelled is what Run returns when the user declined elevation.
var ErrCancelled = errors.New("the user cancelled the elevation prompt")

// DownloadDir is where an identity named name downloads installers: a
// directory of its own under the system temp directory.
func DownloadDir(name string) string {
	return filepath.Join(os.TempDir(), name+"-update")
}

// CleanDownloads removes what an earlier update left in dir.
func CleanDownloads(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("Update downloads not cleared: %v", err)
	}
}
