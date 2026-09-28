package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"LiteRSS/internal/browser"
	"LiteRSS/internal/config"
	"LiteRSS/internal/database"
	"LiteRSS/internal/desktopapi"
	"LiteRSS/internal/enrich"
	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/identity"
	"LiteRSS/internal/library"
	"LiteRSS/internal/middleware"
	"LiteRSS/internal/routes"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/shell"
	"LiteRSS/internal/syncer"
	"LiteRSS/internal/update"
	"LiteRSS/internal/utils/fileutil"
	"LiteRSS/internal/utils/httputil"
	"LiteRSS/internal/version"
	"LiteRSS/internal/webui"
)

//go:embed frontend/dist
var frontendFiles embed.FS

// systemBrowser opens links outside the WebView; the API's "open in browser"
// route (spec D13) calls it.
var systemBrowser browser.Opener

func main() {
	id := identity.Current()

	dataDir, err := fileutil.GetDataDir()
	if err != nil {
		log.Fatalf("Could not create data directory: %v", err)
	}
	logPath, err := fileutil.GetLogPath()
	if err != nil {
		log.Fatalf("Could not create log directory: %v", err)
	}

	// Keep the previous run's log as debug.log.1.
	f, err := fileutil.OpenRotatedLog(logPath)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
	} else {
		defer f.Close()
		log.SetOutput(f)
	}

	log.Printf("Starting %s %s (UniqueID %s)...", id.Name, version.Version, id.UniqueID)

	// Log portable mode status
	if fileutil.IsPortableMode() {
		log.Println("Running in PORTABLE mode")
	} else {
		log.Println("Running in NORMAL mode")
	}

	log.Printf("Data directory: %s", dataDir)

	db, err := database.Open(context.Background(), filepath.Join(dataDir, "literss.db"))
	if err != nil {
		log.Fatalf("Could not open the library: %v", err)
	}
	defer db.Close()
	store := settings.New(db.DB)
	configureProxy(store)
	autostart := applyAutostart(store)

	syncService := syncer.New(db, freshrssRemote(store))
	syncScheduler := syncer.NewScheduler(func(ctx context.Context) {
		if _, err := syncService.RunCycle(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Sync cycle failed: %v", err)
		}
	}, syncInterval(store))
	syncCtx, stopSync := context.WithCancel(context.Background())
	syncStopped := make(chan struct{})
	go func() {
		syncScheduler.Run(syncCtx)
		close(syncStopped)
	}()

	// Article pages and the model can be slow; Baidu and the update check
	// share the model's client (pitfall 8).
	outbound := httputil.CreateHTTPClient(90 * time.Second)

	panel := settings.NewPanel(store, outbound, syncScheduler.Trigger)
	if autostart != nil {
		panel.Autostart = autostart.Apply
	}
	// Set once the application and its window exist.
	desktop := &desktopShell{store: store, sync: syncScheduler}

	// The in-app update (spec D20); shellCtx ends a download on quit.
	shellCtx, stopShell := context.WithCancel(context.Background())
	updates := inAppUpdater(shellCtx, id, outbound, desktop)
	desktop.updates = updates

	// Every /api route is in routes.Table (spec D13).
	apiMux := routes.Handler(routes.Deps{
		Sync:    syncService,
		SyncNow: syncScheduler.Trigger,
		Library: library.New(db.DB),
		Intents: syncService,
		Enrich: enrich.New(db.DB, store, enrich.Clients{
			Web: httputil.CreateWebScrapingClient(30 * time.Second),
			API: outbound,
		}),
		Settings: panel,
		// Set below, once the application exists.
		Browser: &systemBrowser,
		Updates: updates,
		Window:  desktop,
	})

	// Static Files
	log.Println("Setting up static files...")
	frontendFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	// Both channels mount the same site, so they send the same CSP (spec D16).
	site := webui.Site(apiMux, frontendFS)

	// Encryption key for single instance communication (IPC between app instances).
	// This key is used to encrypt/decrypt messages between first and subsequent instances.
	// Note: This is not for sensitive data encryption - it only carries launch arguments.
	// The key is hardcoded per Wails v3 examples since the data exchanged is not sensitive
	// (just signals to bring window to front).
	var encryptionKey = [32]byte{
		0x1e, 0x1f, 0x1c, 0x1d, 0x1a, 0x1b, 0x18, 0x19,
		0x16, 0x17, 0x14, 0x15, 0x12, 0x13, 0x10, 0x11,
		0x0e, 0x0f, 0x0c, 0x0d, 0x0a, 0x0b, 0x08, 0x09,
		0x06, 0x07, 0x04, 0x05, 0x02, 0x03, 0x00, 0x01,
	}

	showMainWindow := func() {
		if desktop.window != nil {
			desktop.show()
		}
	}

	log.Println("Starting Wails v3...")

	app := application.New(application.Options{
		Name:        id.Name,
		Description: "FreshRSS 未读阅读器",
		// The window icon when the executable carries none: packaged builds
		// embed build/windows/icon.ico, a plain `go build` does not.
		Icon:     appIcon(),
		LogLevel: slog.LevelError,
		Assets: application.AssetOptions{
			Handler:    site,
			Middleware: webui.WailsMiddleware(site),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath:   identity.WebviewDataDir(dataDir),
			AdditionalBrowserArgs: id.WebviewBrowserArgs(os.Getenv(identity.WebviewDebugPortEnv)),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:      id.UniqueID,
			EncryptionKey: encryptionKey,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				log.Printf("Second instance detected, bringing window to front")
				showMainWindow()
			},
		},
	})

	systemBrowser = browser.New(app.Browser.OpenURL)

	// The window exists before the listener starts, so /api/window never
	// sees it unset.
	desktop.app = app
	desktop.window = app.Window.NewWithOptions(mainWindowOptions(id.Name, store))
	desktop.install(id.Name)

	// The listener is loopback-only. Development builds also serve the frontend
	// on it as the browser forensics channel (spec D5); installed builds do not.
	var desktopHandler http.Handler = apiMux
	if id.BrowserChannel {
		desktopHandler = site
	}
	desktopAPIServer, apiErr := desktopapi.Start(
		id.APIAddress,
		middleware.Apply(desktopHandler, middleware.Recovery()),
	)
	if apiErr != nil {
		log.Printf("Local desktop API unavailable: %v", apiErr)
	} else {
		log.Printf("Local desktop API listening on %s/api", desktopAPIServer.Origin())
		if id.BrowserChannel {
			log.Printf("Browser forensics channel on %s/", desktopAPIServer.Origin())
		}
		go func() {
			if serveErr := <-desktopAPIServer.Errors(); serveErr != nil {
				log.Printf("Local desktop API stopped unexpectedly: %v", serveErr)
			}
		}()
	}

	// The automatic update check (update_check_enabled) asks with a dialog.
	updateWatch := &update.Watch{
		Check:   updates.Check,
		Enabled: func() bool { v, err := store.Get(shellCtx, "update_check_enabled"); return err == nil && v == "true" },
		Notify:  desktop.promptUpdate,
		Delay:   time.Minute,
		Every:   24 * time.Hour,
	}
	go updateWatch.Run(shellCtx)

	// On macOS, handle dock icon click to show the window
	if runtime.GOOS == "darwin" {
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
			log.Println("Dock icon clicked, showing window")
			showMainWindow()
		})
	}

	log.Println("Window initialized, running app...")

	err = app.Run()

	log.Println("Shutting down...")
	stopShell()
	// Stop scheduling and wait out a cycle in progress, then wake every
	// status long poll, before the listener and the library close.
	stopSync()
	<-syncStopped
	syncService.Close()
	if desktopAPIServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if shutdownErr := desktopAPIServer.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("Local desktop API shutdown failed: %v", shutdownErr)
		}
		shutdownCancel()
	}

	if err != nil {
		log.Printf("Error running Wails: %v", err)
		log.Fatal(err)
	}
	log.Println("Application finished")
}

// inAppUpdater returns the update the settings panel and the tray start.
// It first clears what an earlier update downloaded. Development builds plan
// as an installed copy would, and only log the plan.
func inAppUpdater(ctx context.Context, id identity.Identity, client *http.Client, desktop *desktopShell) *update.Updater {
	u := update.NewUpdater(ctx, update.New(client))
	u.GOOS, u.GOARCH = runtime.GOOS, runtime.GOARCH
	u.Installs = id.InstallsUpdates
	u.Dir = update.DownloadDir(id.Name)
	update.CleanDownloads(u.Dir)
	u.PID = os.Getpid()
	u.Locate = func() update.Target {
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			return update.Target{Why: "executable not found: " + err.Error()}
		}
		return update.Locate(update.Self{
			GOOS:     runtime.GOOS,
			Exe:      exe,
			Portable: id.InstallsUpdates && fileutil.IsPortableMode(),
			Writable: update.DirWritable,
			Exists:   update.FileExists,
		})
	}
	u.Run = update.RunPlan
	u.Quit = desktop.quit
	u.TrayFailed = desktop.updateFailed
	return u
}

// applyAutostart makes the system's login items match startup_on_boot, so a
// build that moved or was reinstalled starts from where it now is; the
// settings panel applies later changes through the returned entry.
func applyAutostart(store *settings.Store) *shell.Autostart {
	autostart, err := shell.NewAutostart()
	if err != nil {
		log.Printf("Launch at login unavailable: %v", err)
		return nil
	}
	on, err := store.Get(context.Background(), "startup_on_boot")
	if err != nil {
		log.Printf("Launch at login not checked: %v", err)
		return autostart
	}
	if err := autostart.Apply(on == "true"); err != nil {
		log.Printf("Launch at login not applied: %v", err)
	}
	return autostart
}

// freshrssRemote returns the sync service's client source: the account in the
// settings, read at the start of every cycle and push.
func freshrssRemote(store *settings.Store) func() (syncer.Remote, error) {
	var clients freshrss.Clients
	return func() (syncer.Remote, error) {
		values, err := store.Load(context.Background())
		if err != nil {
			return nil, fmt.Errorf("read the FreshRSS account: %w", err)
		}
		serverURL, user, pass := values["freshrss_server_url"], values["freshrss_username"], values["freshrss_api_password"]
		if serverURL == "" || user == "" || pass == "" {
			return nil, errors.New("the FreshRSS account is not configured")
		}
		return clients.For(serverURL, user, pass), nil
	}
}

// configureProxy installs the stored proxy settings for every outbound
// client; the settings API installs them again when they are saved. Settings
// that cannot be installed leave the system proxy in place.
func configureProxy(store *settings.Store) {
	values, err := store.Load(context.Background())
	if err == nil {
		err = httputil.ConfigureProxyFromSettings(values)
	}
	if err != nil {
		log.Printf("Proxy settings not applied, following the system proxy: %v", err)
	}
}

// syncInterval reads freshrss_auto_sync_interval (minutes) for the wait after
// each cycle; a value below 1 falls back to the schema default.
func syncInterval(store *settings.Store) func() time.Duration {
	const key = "freshrss_auto_sync_interval"
	return func() time.Duration {
		raw, err := store.Get(context.Background(), key)
		minutes, convErr := strconv.Atoi(raw)
		if err != nil || convErr != nil || minutes < 1 {
			minutes, _ = strconv.Atoi(config.GetString(key))
		}
		return time.Duration(max(minutes, 1)) * time.Minute
	}
}
