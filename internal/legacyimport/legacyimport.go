// Package legacyimport copies a legacy MrRSS library into a new LiteRSS
// library, once, on the first start (spec D12). The legacy library is only
// read: it is opened read-only and copied with VACUUM INTO, and the import
// works on that snapshot. The outcome is recorded in meta under
// MetaLegacyImport; once a record exists the legacy library is never looked
// for again.
package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/syncer"
)

// MetaLegacyImport is the meta key holding the import's Report as JSON.
const MetaLegacyImport = "legacy_import"

// PathEnv names a legacy library outside the fixed locations. It is the only
// place a development build looks.
const PathEnv = "LITERSS_LEGACY_DB"

// Status values of a recorded Report.
const (
	StatusDone    = "done"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Report is what the import did, kept in meta for the import notice.
type Report struct {
	Status string `json:"status"`
	// Source is the legacy library read, empty when none was found.
	Source string `json:"source,omitempty"`
	// At is the Unix time the record was written.
	At int64 `json:"at"`
	// Reason says why the import was skipped or failed.
	Reason string `json:"reason,omitempty"`

	Articles     int `json:"articles"`
	Contents     int `json:"contents"`
	Translations int `json:"translations"`
	Summaries    int `json:"summaries"`
	Intents      int `json:"intents"`
	Settings     int `json:"settings"`
	// DroppedArticles counts legacy articles left out: no FreshRSS item id,
	// no subscription stream, or an id already taken.
	DroppedArticles int `json:"dropped_articles"`
	// Refill lists the credential settings that did not decrypt here; the
	// user has to enter them again.
	Refill []string `json:"refill,omitempty"`
}

// Outcome is what one Run did.
type Outcome int

const (
	// Recorded means a record already existed, so nothing was looked at.
	Recorded Outcome = iota
	// Finished means Run wrote a record: done, skipped or failed.
	Finished
	// Deferred means the legacy MrRSS runs; nothing was written and the
	// import is tried again later.
	Deferred
)

// Importer imports into one library.
type Importer struct {
	DB    *database.DB
	Store *settings.Store
	// DataDir holds the temporary snapshot.
	DataDir string
	// Candidates are the legacy library paths, tried in order.
	Candidates []string
	// LegacyRunning tells whether the legacy MrRSS runs; while it does the
	// import waits, since it could still change its library.
	LegacyRunning func() bool

	now  func() time.Time
	logf func(format string, args ...any)
}

// Candidates returns where to look for a legacy library: the path in
// PathEnv, then, when fixedPlaces, the legacy data directory under
// userConfigDir and, for a portable install, <exeDir>/data.
func Candidates(getenv func(string) string, fixedPlaces bool, userConfigDir, exeDir string, portable bool) []string {
	var paths []string
	if p := getenv(PathEnv); p != "" {
		paths = append(paths, p)
	}
	if !fixedPlaces {
		return paths
	}
	if userConfigDir != "" && (runtime.GOOS == "windows" || runtime.GOOS == "darwin") {
		paths = append(paths, filepath.Join(userConfigDir, "MrRSS", "rss.db"))
	}
	if portable && exeDir != "" {
		paths = append(paths, filepath.Join(exeDir, "data", "rss.db"))
	}
	return paths
}

// Run imports unless a record exists. It returns Deferred, writing nothing,
// while the legacy MrRSS runs. Any failure past finding the library is
// recorded as failed rather than returned, so the next start does not try
// again; the error is only for the record itself not being written.
func (im *Importer) Run(ctx context.Context) (Outcome, Report, error) {
	now, logf := im.clock(), im.logger()

	var raw string
	err := im.DB.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, MetaLegacyImport).Scan(&raw)
	if err == nil {
		var rep Report
		if err := json.Unmarshal([]byte(raw), &rep); err != nil {
			logf("legacy import: unreadable record %s: %v", MetaLegacyImport, err)
		}
		return Recorded, rep, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, Report{}, fmt.Errorf("legacy import: read record: %w", err)
	}

	finish := func(rep Report) (Outcome, Report, error) {
		rep.At = now().Unix()
		if rep.Status != StatusDone {
			logf("legacy import %s: %s", rep.Status, rep.Reason)
		}
		if err := writeRecord(ctx, im.DB.DB, rep); err != nil {
			return 0, rep, err
		}
		return Finished, rep, nil
	}

	fresh, err := im.libraryIsNew(ctx)
	if err != nil {
		return 0, Report{}, fmt.Errorf("legacy import: %w", err)
	}
	if !fresh {
		return finish(Report{Status: StatusSkipped, Reason: "the library already holds synced data"})
	}

	source := ""
	for _, p := range im.Candidates {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			source = p
			break
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			logf("legacy import: cannot look at %s: %v", p, err)
		}
	}
	if source == "" {
		return finish(Report{Status: StatusSkipped, Reason: "no legacy MrRSS library found"})
	}

	if im.LegacyRunning != nil && im.LegacyRunning() {
		logf("legacy import: MrRSS is running; the import waits until it quits")
		return Deferred, Report{Source: source}, nil
	}

	rep, err := im.importFrom(ctx, source, now, logf)
	if err != nil {
		return finish(Report{Status: StatusFailed, Source: source, Reason: err.Error()})
	}
	logf("legacy import done from %s: %d articles, %d bodies, %d translations, %d summaries, %d intents, %d settings, %d articles dropped, refill %v",
		source, rep.Articles, rep.Contents, rep.Translations, rep.Summaries, rep.Intents, rep.Settings, rep.DroppedArticles, rep.Refill)
	return Finished, rep, nil
}

// Gate returns cycle preceded by the import: until Run has settled it, every
// cycle first runs the import, and a deferred import skips the cycle so the
// library is not synced before the legacy data is in (spec D12). deferred is
// told each time the import waits for the legacy MrRSS to quit.
func (im *Importer) Gate(cycle func(context.Context), deferred func()) func(context.Context) {
	settled := false
	return func(ctx context.Context) {
		if !settled {
			outcome, _, err := im.Run(ctx)
			switch {
			case err != nil:
				// The record could not be written; the import is retried
				// next cycle, and syncing now would make that library "not
				// new" and skip it.
				im.logger()("%v", err)
				return
			case outcome == Deferred:
				if deferred != nil {
					deferred()
				}
				return
			}
			settled = true
		}
		cycle(ctx)
	}
}

func (im *Importer) clock() func() time.Time {
	if im.now != nil {
		return im.now
	}
	return time.Now
}

func (im *Importer) logger() func(string, ...any) {
	if im.logf != nil {
		return im.logf
	}
	return log.Printf
}

// libraryIsNew reports whether the library has never synced nor imported:
// the import only fills a library it can own whole.
func (im *Importer) libraryIsNew(ctx context.Context) (bool, error) {
	var n int
	if err := im.DB.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM articles) + (SELECT COUNT(*) FROM feeds)
		      + (SELECT COUNT(*) FROM meta WHERE key = ?)`, syncer.MetaLastSync).Scan(&n); err != nil {
		return false, fmt.Errorf("check the library is new: %w", err)
	}
	return n == 0, nil
}

func writeRecord(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, rep Report) error {
	raw, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("legacy import: encode record: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		MetaLegacyImport, string(raw)); err != nil {
		return fmt.Errorf("legacy import: write record: %w", err)
	}
	return nil
}
