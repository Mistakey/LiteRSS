package update

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

// Kind is how this copy of LiteRSS takes an update.
type Kind int

const (
	// ReleasePage sends the user to the release page: portable copies and
	// installs whose location is unknown or cannot be replaced.
	ReleasePage Kind = iota
	// WindowsInstaller runs the NSIS installer silently over Dir.
	WindowsInstaller
	// MacBundle swaps App for the DMG's bundle after LiteRSS quits.
	MacBundle
)

// Target is where and how an update installs.
type Target struct {
	Kind Kind
	// Dir is the running executable's directory (WindowsInstaller).
	Dir string
	// Elevate runs the installer through UAC: the user cannot write Dir.
	Elevate bool
	// App is the running .app bundle (MacBundle).
	App string
	// Why says in English, for the log, why the target is ReleasePage.
	Why string
}

// Self describes the running copy for Locate.
type Self struct {
	GOOS string
	// Exe is the running executable's path.
	Exe string
	// Portable is set next to portable.txt.
	Portable bool
	// Writable reports whether the user can create files in a directory.
	Writable func(dir string) bool
	// Exists reports whether a file exists.
	Exists func(path string) bool
}

// uninstaller is what installer.nsi writes beside the executable; without it
// a Windows copy was not installed and its folder is not the installer's.
const uninstaller = "Uninstall.exe"

// Locate decides the target of the running copy (spec D20).
func Locate(s Self) Target {
	if s.Portable {
		return Target{Why: "portable copy"}
	}
	switch s.GOOS {
	case "windows":
		i := strings.LastIndexAny(s.Exe, `\/`)
		if i <= 0 {
			return Target{Why: "executable path " + s.Exe + " has no directory"}
		}
		dir := s.Exe[:i]
		if !s.Exists(s.Exe[:i+1] + uninstaller) {
			return Target{Why: "no " + uninstaller + " beside " + s.Exe}
		}
		return Target{Kind: WindowsInstaller, Dir: dir, Elevate: !s.Writable(dir)}
	case "darwin":
		// <parent>/LiteRSS.app/Contents/MacOS/LiteRSS
		macOS := path.Dir(s.Exe)
		app := path.Dir(path.Dir(macOS))
		if path.Base(macOS) != "MacOS" || path.Base(path.Dir(macOS)) != "Contents" || !strings.HasSuffix(app, ".app") {
			return Target{Why: "executable " + s.Exe + " is not inside an .app bundle"}
		}
		if parent := path.Dir(app); !s.Writable(parent) {
			return Target{Why: "cannot write " + parent}
		}
		return Target{Kind: MacBundle, App: app}
	}
	return Target{Why: s.GOOS + " has no in-app update"}
}

// FileExists is Self.Exists for the real file system.
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// DirWritable is Self.Writable for the real file system: it creates and
// removes a file in dir.
func DirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".literss-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// Command is one program an install runs.
type Command struct {
	Path string
	// Args are passed as written: Windows gets them joined by spaces and
	// unquoted, which NSIS needs for /D= (it must be last and may hold
	// spaces).
	Args []string
	// Elevate runs the command through UAC (Windows "runas").
	Elevate bool
	// Detach starts the command and leaves it running past LiteRSS; the
	// others are waited for and must succeed.
	Detach bool
}

// String is the command as it would be typed, for the log.
func (c Command) String() string {
	s := strconv.Quote(c.Path)
	if len(c.Args) > 0 {
		s += " " + strings.Join(c.Args, " ")
	}
	if c.Elevate {
		s = "runas " + s
	}
	return s
}

// Plan is the commands that install a downloaded file. Steps run in order;
// if one fails, Cleanup runs and the running copy stays as it was.
type Plan struct {
	Steps   []Command
	Cleanup []Command
}

// mountName is the directory beside the downloaded DMG that it is mounted on.
const mountName = "mnt"

// PlanInstall returns the plan that installs file (the verified installer or
// DMG) on t. pid is LiteRSS's own process, which the macOS swap waits for.
func PlanInstall(t Target, file string, pid int) Plan {
	switch t.Kind {
	case WindowsInstaller:
		return Plan{Steps: []Command{{
			Path:    file,
			Args:    []string{"/S", "/D=" + t.Dir},
			Elevate: t.Elevate,
			Detach:  true,
		}}}
	case MacBundle:
		m := NewMacSwap(file, t.App, pid)
		detach := Command{Path: m.Hdiutil, Args: []string{"detach", m.Mount, "-force"}}
		return Plan{
			Steps: []Command{
				{Path: m.Hdiutil, Args: []string{"attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", m.Mount, file}},
				{Path: "/bin/rm", Args: []string{"-rf", m.Staging}},
				// ditto keeps the bundle's signature, links and attributes.
				{Path: "/usr/bin/ditto", Args: []string{m.Source, m.Staging}},
				{Path: "/bin/sh", Args: []string{"-c", m.Script()}, Detach: true},
			},
			Cleanup: []Command{detach, {Path: "/bin/rm", Args: []string{"-rf", m.Staging}}},
		}
	}
	return Plan{}
}

// MacSwap is the macOS replacement: the new bundle is copied from the
// mounted DMG beside the running one under a temporary name, then a shell
// script, once LiteRSS has quit, swaps them by renames and restores the old
// bundle if a rename fails.
type MacSwap struct {
	DMG, Mount string
	// Source is the bundle inside the mounted DMG.
	Source string
	// App is the running bundle; Staging and Backup sit beside it.
	App, Staging, Backup string
	PID                  int
	// Hdiutil and Open are the system tools, replaceable in tests.
	Hdiutil, Open string
	// Wait is how many half seconds the script waits for PID to exit.
	Wait int
}

// NewMacSwap lays the swap out for dmg and the running app.
func NewMacSwap(dmg, app string, pid int) MacSwap {
	dir, base := path.Split(app)
	name := strings.TrimSuffix(base, ".app")
	mount := path.Join(path.Dir(dmg), mountName)
	return MacSwap{
		DMG:     dmg,
		Mount:   mount,
		Source:  mount + "/" + base,
		App:     app,
		Staging: dir + "." + name + "-update.app",
		Backup:  dir + "." + name + "-old.app",
		PID:     pid,
		Hdiutil: "/usr/bin/hdiutil",
		Open:    "/usr/bin/open",
		Wait:    120,
	}
}

// Script is the swap run after LiteRSS quits. If LiteRSS does not quit in
// time, nothing is swapped. The DMG is detached and the result, new or old,
// is opened.
func (m MacSwap) Script() string {
	q := shellQuote
	return fmt.Sprintf(`i=0
while kill -0 %[1]d 2>/dev/null; do
  i=$((i+1))
  if [ "$i" -ge %[2]d ]; then
    rm -rf %[5]s
    %[7]s detach %[4]s -force >/dev/null 2>&1
    exit 1
  fi
  sleep 0.5
done
rm -rf %[6]s
if mv %[3]s %[6]s; then
  if mv %[5]s %[3]s; then
    rm -rf %[6]s
  else
    mv %[6]s %[3]s
    rm -rf %[5]s
  fi
else
  rm -rf %[5]s
fi
%[7]s detach %[4]s -force >/dev/null 2>&1
rm -f %[8]s
%[9]s %[3]s
`, m.PID, m.Wait, q(m.App), q(m.Mount), q(m.Staging), q(m.Backup), q(m.Hdiutil), q(m.DMG), q(m.Open))
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
