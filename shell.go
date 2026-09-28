package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"LiteRSS/internal/settings"
	"LiteRSS/internal/shell"
	"LiteRSS/internal/syncer"
	"LiteRSS/internal/update"
)

// The tray and window icon. Windows takes the .ico and picks the frame drawn
// for the current DPI; a single large PNG would be scaled down and blur.
//
//go:embed build/windows/icon.ico
var iconWindows []byte

//go:embed build/appicon.png
var iconMac []byte

func appIcon() []byte {
	if runtime.GOOS == "darwin" {
		return iconMac
	}
	return iconWindows
}

// desktopShell is what the shell does around the window (spec D4): the tray,
// close to tray, the saved window placement, and syncs on focus and wake.
type desktopShell struct {
	app    *application.App
	window *application.WebviewWindow
	store  *settings.Store
	sync   *syncer.Scheduler
	// updates is the in-app update the tray's dialog starts.
	updates *update.Updater
	// quitting is set by the tray's 退出, so closing the window then quits.
	quitting atomic.Bool
}

// mainWindowOptions opens the window where it was last saved, else centered.
func mainWindowOptions(title string, store *settings.Store) application.WebviewWindowOptions {
	opts := application.WebviewWindowOptions{
		Name:   "main",
		Title:  title,
		Width:  1024,
		Height: 768,
		URL:    "/",
		// No system title bar: the frontend's top bar is the title bar
		// (spec D4). Windows keeps the frame's shadow and snapping; macOS
		// keeps its traffic lights over the top bar.
		Frameless: runtime.GOOS == "windows",
		Mac:       application.MacWindow{TitleBar: application.MacTitleBarHidden},
		// Dark gray prevents a white flash on startup/close.
		BackgroundColour: application.NewRGB(30, 30, 30),
		InitialPosition:  application.WindowCentered,
	}
	w, placed, err := shell.LoadWindow(context.Background(), store)
	if err != nil {
		log.Printf("Window placement not read: %v", err)
		return opts
	}
	opts.Width, opts.Height = w.Width, w.Height
	if placed {
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = w.X, w.Y
	}
	if w.Maximized {
		opts.StartState = application.WindowStateMaximised
	}
	return opts
}

// install adds the tray and the window's hooks; call it before app.Run.
func (s *desktopShell) install(name string) {
	menu := s.app.NewMenu()
	menu.Add("显示窗口").OnClick(func(*application.Context) { s.show() })
	menu.Add("立即同步").OnClick(func(*application.Context) { s.sync.Trigger() })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { s.quit() })

	tray := s.app.SystemTray.New()
	tray.SetIcon(appIcon())
	tray.SetTooltip(name)
	tray.SetMenu(menu)
	tray.OnClick(s.show)

	s.window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		if s.closeToTray() {
			s.saveWindow()
			s.window.Hide()
			return
		}
		// Quit rather than let the window go: macOS keeps running after its
		// last window closes and would be left with nothing to show.
		s.quit()
	})
	s.window.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		s.sync.TriggerOnFocus()
	})
	s.app.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) {
		s.sync.Trigger()
	})
}

// show brings the window back from the tray, the taskbar or behind others,
// keeping it maximized if it was.
func (s *desktopShell) show() {
	s.window.Show()
	if s.window.IsMinimised() {
		s.window.UnMinimise()
	}
	s.window.Focus()
}

// Minimise, ToggleMaximise and Close are the frameless title bar's buttons,
// which the frontend reaches through /api/window (routes.Window).
func (s *desktopShell) Minimise() {
	if s.window != nil {
		s.window.Minimise()
	}
}

func (s *desktopShell) ToggleMaximise() {
	if s.window != nil {
		s.window.ToggleMaximise()
	}
}

// Close does what the × of a framed window does: the WindowClosing hook
// hides to the tray or quits.
func (s *desktopShell) Close() {
	if s.window != nil {
		s.window.Close()
	}
}

// quit saves the placement once and ends the application.
func (s *desktopShell) quit() {
	s.quitting.Store(true)
	s.saveWindow()
	s.app.Quit()
}

func (s *desktopShell) closeToTray() bool {
	v, err := s.store.Get(context.Background(), "close_to_tray")
	return err == nil && v == "true"
}

// saveWindow runs as the window is closed or hidden to the tray; a hidden
// window still reports its bounds.
func (s *desktopShell) saveWindow() {
	x, y := s.window.Position()
	w, h := s.window.Size()
	state := shell.Window{X: x, Y: y, Width: w, Height: h, Maximized: s.window.IsMaximised()}
	if err := shell.SaveWindow(context.Background(), s.store, state, s.window.IsMinimised()); err != nil {
		log.Printf("Window placement not saved: %v", err)
	}
}

// yesNo returns the labels of a two-button question. A Windows message box
// shows the system's own 是/否 and reports them by these names; other
// platforms show the labels.
func yesNo(yes, no string) (string, string) {
	if runtime.GOOS == "windows" {
		return "Yes", "No"
	}
	return yes, no
}

// promptUpdate asks whether to update to a newer version: in the app when
// it can install there (spec D20), else from the release page.
func (s *desktopShell) promptUpdate(r update.Result) {
	news := fmt.Sprintf("新版本 %s 已发布，当前是 %s。", r.LatestVersion, r.CurrentVersion)
	if r.InApp {
		yes, no := yesNo("更新", "以后")
		dialog := s.app.Dialog.Question().
			SetTitle("LiteRSS 有新版本").
			SetMessage(news + "\n\n现在更新吗？LiteRSS 会下载并安装新版本，然后重新启动。")
		dialog.AddButton(yes).SetAsDefault().OnClick(func() { s.updates.Start(true) })
		dialog.AddButton(no).SetAsCancel()
		dialog.Show()
		return
	}
	s.askReleasePage("LiteRSS 有新版本", news+"\n\n去发布页下载吗？", r.ReleaseURL)
}

// updateFailed tells of an update the tray started that did not finish.
func (s *desktopShell) updateFailed(st update.Status) {
	s.askReleasePage("LiteRSS 更新没有完成", st.Message+"\n\n去发布页下载吗？", st.ReleaseURL)
}

// askReleasePage asks whether to open the release page at url.
func (s *desktopShell) askReleasePage(title, message, url string) {
	yes, no := yesNo("去发布页", "以后")
	dialog := s.app.Dialog.Question().SetTitle(title).SetMessage(message)
	dialog.AddButton(yes).SetAsDefault().OnClick(func() {
		if err := systemBrowser.Open(url); err != nil {
			log.Printf("Release page not opened: %v", err)
		}
	})
	dialog.AddButton(no).SetAsCancel()
	dialog.Show()
}
