package syncer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"LiteRSS/internal/freshrss"
)

// DisplayRead is the SQL for the read state the user sees, 0 or 1, of the
// article aliased a: its pending_read intent when there is one, else read
// when a pending_mark_all covers it, else the server_read mirror (spec D6).
// A mark-all deletes the unread intents it covers when written, so an item
// intent under a mark-all is either newer or agrees with it.
const DisplayRead = `COALESCE(
	(SELECT p.value FROM pending_read p WHERE p.item_id = a.item_id),
	CASE WHEN EXISTS (
		SELECT 1 FROM pending_mark_all m
		WHERE a.item_id <= m.ts
		  AND (m.stream_id = '` + freshrss.StreamReadingList + `' OR m.stream_id = a.stream_id
		       OR m.stream_id IN (SELECT ft.tag_id FROM feed_tags ft WHERE ft.stream_id = a.stream_id))
	) THEN 1 ELSE a.server_read END)`

// MetaIntentSeq is the meta key of the counter every intent write draws its
// seq from.
const MetaIntentSeq = "intent_seq"

// UndoTTL is how long a batch can be undone. Tokens live in memory only, so a
// restart ends them too (spec D7).
const UndoTTL = 60 * time.Second

// ErrUndoExpired reports an undo token that is unknown, used, or past UndoTTL.
var ErrUndoExpired = errors.New("undo token expired")

// Batch is the outcome of a batch mark-as-read.
type Batch struct {
	// Token undoes the batch within UndoTTL.
	Token string
	// Count is how many articles went from unread to read.
	Count int
}

type undoEntry struct {
	ids     []int64 // articles the batch turned from unread to read
	markAll int64   // the pending_mark_all row it wrote, 0 for none
	expires time.Time
}

// SetRead records the user marking one article read or unread. The intent
// covers every article sharing its URL, so an older copy does not surface
// as unread elsewhere.
func (s *Service) SetRead(ctx context.Context, itemID int64, read bool) error {
	value := 0
	if read {
		value = 1
	}
	err := s.writeIntents(ctx, func(tx *sql.Tx) error {
		ids, err := urlGroup(ctx, tx, []int64{itemID}, false)
		if err != nil {
			return err
		}
		return putRead(ctx, tx, ids, value)
	})
	if err != nil {
		return fmt.Errorf("set read: %w", err)
	}
	return nil
}

// MarkItemsRead marks the given articles and their URL groups read, writing
// intents only for those shown unread. It is "this and above / below": the
// ids come from the view's snapshot, never from a ts range (spec D7).
func (s *Service) MarkItemsRead(ctx context.Context, ids []int64) (Batch, error) {
	var changed []int64
	err := s.writeIntents(ctx, func(tx *sql.Tx) error {
		var err error
		if changed, err = urlGroup(ctx, tx, ids, true); err != nil {
			return err
		}
		return putRead(ctx, tx, changed, 1)
	})
	if err != nil {
		return Batch{}, fmt.Errorf("mark items read: %w", err)
	}
	return s.remember(changed, 0), nil
}

// MarkStreamRead marks a feed ("feed/<n>"), a label, or the whole reading
// list read up to ts, an item id: the fetch time of the newest item the user
// saw in that range. With ts 0 the newest local item of the stream is taken.
// It is pushed as mark-all-as-read, which leaves items crawled later unread.
func (s *Service) MarkStreamRead(ctx context.Context, stream string, ts int64) (Batch, error) {
	if !ValidStream(stream) {
		return Batch{}, fmt.Errorf("mark stream read: unknown stream %q", stream)
	}
	var (
		changed []int64
		rowID   int64
	)
	err := s.writeIntents(ctx, func(tx *sql.Tx) error {
		if ts <= 0 {
			if err := tx.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(a.item_id), 0) FROM articles a WHERE `+InStream, sql.Named("stream", stream)).Scan(&ts); err != nil {
				return fmt.Errorf("newest item: %w", err)
			}
			if ts == 0 {
				return nil
			}
		}
		var err error
		if changed, err = queryIDs(ctx, tx,
			`SELECT a.item_id FROM articles a WHERE a.item_id <= @ts AND `+InStream+` AND `+DisplayRead+` = 0`,
			sql.Named("ts", ts), sql.Named("stream", stream)); err != nil {
			return err
		}
		seq, err := nextSeq(ctx, tx)
		if err != nil {
			return err
		}
		// The mark-all goes out before item intents, so an unread intent it
		// covers would overrule it; the user's latest action is the
		// mark-all. Those articles are in changed, so an undo restores them.
		// Read intents agree with it and stay.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM pending_read WHERE value = 0 AND item_id IN (
			   SELECT a.item_id FROM articles a WHERE a.item_id <= @ts AND `+InStream+`)`,
			sql.Named("ts", ts), sql.Named("stream", stream)); err != nil {
			return fmt.Errorf("supersede item intents: %w", err)
		}
		// Copies of covered articles in other streams get read intents, as
		// any read does for its URL group.
		group, err := urlGroup(ctx, tx, changed, true)
		if err != nil {
			return err
		}
		covered := map[int64]bool{}
		for _, id := range changed {
			covered[id] = true
		}
		var copies []int64
		for _, id := range group {
			if !covered[id] {
				copies = append(copies, id)
			}
		}
		if err := putRead(ctx, tx, copies, 1); err != nil {
			return err
		}
		changed = append(changed, copies...)
		return tx.QueryRowContext(ctx,
			`INSERT INTO pending_mark_all (stream_id, ts, seq) VALUES (?, ?, ?) RETURNING id`,
			stream, ts, seq).Scan(&rowID)
	})
	if err != nil {
		return Batch{}, fmt.Errorf("mark stream read: %w", err)
	}
	return s.remember(changed, rowID), nil
}

// Undo reverts a batch: a mark-all not yet sent is deleted, and every
// article the batch turned read that would still show read gets an unread
// intent. Once the mark-all has been sent, all of them get one.
func (s *Service) Undo(ctx context.Context, token string) error {
	s.intentMu.Lock()
	entry, ok := s.undos[token]
	delete(s.undos, token)
	s.intentMu.Unlock()
	if !ok || !s.now().Before(entry.expires) {
		return ErrUndoExpired
	}
	err := s.writeIntents(ctx, func(tx *sql.Tx) error {
		ids := entry.ids
		if entry.markAll != 0 && !s.inflight[entry.markAll] {
			r, err := tx.ExecContext(ctx, `DELETE FROM pending_mark_all WHERE id = ?`, entry.markAll)
			if err != nil {
				return fmt.Errorf("delete mark-all: %w", err)
			}
			if n, err := r.RowsAffected(); err != nil {
				return fmt.Errorf("delete mark-all: %w", err)
			} else if n == 1 {
				// Never sent: what the server has unread shows unread again
				// on its own.
				idsJSON, err := jsonList(ids)
				if err != nil {
					return err
				}
				if ids, err = queryIDs(ctx, tx,
					`SELECT a.item_id FROM articles a
					 WHERE a.item_id IN (SELECT value FROM json_each(?)) AND `+DisplayRead+` = 1`, idsJSON); err != nil {
					return err
				}
			}
		}
		return putRead(ctx, tx, ids, 0)
	})
	if err != nil {
		// Nothing was written, so the batch can still be undone.
		s.intentMu.Lock()
		s.undos[token] = entry
		s.intentMu.Unlock()
		return fmt.Errorf("undo: %w", err)
	}
	return nil
}

// writeIntents runs fn in one transaction, serialized with other intent
// writes and the pusher's bookkeeping, then schedules a push and updates the
// status's pending count.
func (s *Service) writeIntents(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := s.commitIntents(ctx, fn); err != nil {
		return err
	}
	s.schedule(s.debounce)
	s.refreshPending(ctx)
	return nil
}

func (s *Service) commitIntents(ctx context.Context, fn func(*sql.Tx) error) error {
	s.intentMu.Lock()
	defer s.intentMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin intents: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit intents: %w", err)
	}
	return nil
}

// remember keeps an undo entry and returns the batch it stands for; callers
// hold no lock.
func (s *Service) remember(ids []int64, markAll int64) Batch {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	now := s.now()
	s.intentMu.Lock()
	defer s.intentMu.Unlock()
	for t, e := range s.undos {
		if !now.Before(e.expires) {
			delete(s.undos, t)
		}
	}
	s.undos[token] = undoEntry{ids: ids, markAll: markAll, expires: now.Add(UndoTTL)}
	return Batch{Token: token, Count: len(ids)}
}

// InStream is the SQL condition that the article a belongs to the stream
// bound as @stream: a feed, a label, or the reading list. The list reads use
// it too, so a view and its mark-all cover the same articles.
const InStream = `(a.stream_id = @stream OR @stream = '` + freshrss.StreamReadingList + `'
	OR a.stream_id IN (SELECT ft.stream_id FROM feed_tags ft WHERE ft.tag_id = @stream))`

// ValidStream reports whether stream names a feed ("feed/<n>"), a label, or
// the reading list.
func ValidStream(stream string) bool {
	return stream == freshrss.StreamReadingList ||
		(strings.HasPrefix(stream, "feed/") && len(stream) > len("feed/")) ||
		(strings.HasPrefix(stream, freshrss.LabelPrefix) && len(stream) > len(freshrss.LabelPrefix))
}

// urlGroup returns the library's articles among ids plus every article
// sharing a non-empty URL with one of them; with onlyUnread, only those
// shown unread.
func urlGroup(ctx context.Context, tx *sql.Tx, ids []int64, onlyUnread bool) ([]int64, error) {
	idsJSON, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	q := `SELECT a.item_id FROM articles a
	      WHERE (a.item_id IN (SELECT value FROM json_each(?))
	             OR (a.url <> '' AND a.url IN (
	                   SELECT b.url FROM articles b WHERE b.item_id IN (SELECT value FROM json_each(?)))))`
	if onlyUnread {
		q += ` AND ` + DisplayRead + ` = 0`
	}
	return queryIDs(ctx, tx, q, idsJSON, idsJSON)
}

// putRead upserts an intent for each id under one fresh seq. A rewritten
// intent starts its attempts over; its new seq keeps a push of the old value
// from deleting it.
func putRead(ctx context.Context, tx *sql.Tx, ids []int64, value int) error {
	if len(ids) == 0 {
		return nil
	}
	seq, err := nextSeq(ctx, tx)
	if err != nil {
		return err
	}
	idsJSON, err := jsonList(ids)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO pending_read (item_id, value, seq)
		 SELECT j.value, ?, ? FROM json_each(?) j WHERE j.value IN (SELECT item_id FROM articles)
		 ON CONFLICT(item_id) DO UPDATE SET
		   value = excluded.value, seq = excluded.seq, attempts = 0, last_error = ''`,
		value, seq, idsJSON); err != nil {
		return fmt.Errorf("write intents: %w", err)
	}
	return nil
}

// nextSeq draws the next value of the intent counter.
func nextSeq(ctx context.Context, tx *sql.Tx) (int64, error) {
	var seq int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, '1')
		 ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1
		 RETURNING CAST(value AS INTEGER)`, MetaIntentSeq).Scan(&seq); err != nil {
		return 0, fmt.Errorf("next intent seq: %w", err)
	}
	return seq, nil
}

func queryIDs(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query ids: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
