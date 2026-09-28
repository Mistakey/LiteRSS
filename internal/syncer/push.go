package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"LiteRSS/internal/freshrss"
)

// push sends every intent to the server: mark-all intents first, oldest
// first, then item intents grouped by value in requests of pushBatch ids. On
// success the intent is deleted, but only if its seq is still the one sent,
// so an intent rewritten while its request was in flight stays for the next
// push; the mirror takes the sent value at once, so the display does not
// fall back to a stale mirror until the next pull, which still has the last
// word. A request the server blames on its items is split in halves until the
// rejected item is found; that item spends one attempt and is dropped at
// maxAttempts. Any other failure stops the push and is returned, with every
// intent not yet confirmed kept as it was. Callers hold s.mu.
func (s *Service) push(ctx context.Context, remote Remote) error {
	if err := s.pushMarkAll(ctx, remote); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item_id, value, seq FROM pending_read ORDER BY seq, item_id`)
	if err != nil {
		return fmt.Errorf("push: read intents: %w", err)
	}
	byValue := map[int][]itemIntent{}
	for rows.Next() {
		var it itemIntent
		var value int
		if err := rows.Scan(&it.id, &value, &it.seq); err != nil {
			rows.Close()
			return fmt.Errorf("push: read intent: %w", err)
		}
		byValue[value] = append(byValue[value], it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("push: read intents: %w", err)
	}
	for _, value := range []int{1, 0} {
		intents := byValue[value]
		for len(intents) > 0 {
			n := min(s.pushBatch, len(intents))
			if err := s.pushItems(ctx, remote, value, intents[:n]); err != nil {
				return fmt.Errorf("push: %w", err)
			}
			intents = intents[n:]
		}
	}
	return nil
}

type itemIntent struct {
	id, seq int64
}

func (s *Service) pushItems(ctx context.Context, remote Remote, value int, intents []itemIntent) error {
	ids := make([]int64, len(intents))
	for i, it := range intents {
		ids[i] = it.id
	}
	var err error
	if value == 1 {
		err = remote.MarkAsRead(ctx, ids)
	} else {
		err = remote.MarkAsUnread(ctx, ids)
	}
	if err == nil {
		return s.settle(ctx, func(tx *sql.Tx) error {
			for _, it := range intents {
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM pending_read WHERE item_id = ? AND seq = ?`, it.id, it.seq); err != nil {
					return fmt.Errorf("confirm intent %d: %w", it.id, err)
				}
				if _, err := tx.ExecContext(ctx,
					`UPDATE articles SET server_read = ? WHERE item_id = ?`, value, it.id); err != nil {
					return fmt.Errorf("mirror pushed state of %d: %w", it.id, err)
				}
			}
			return nil
		})
	}
	if !rejectsItem(err) {
		return err
	}
	if len(intents) == 1 {
		it := intents[0]
		return s.settle(ctx, func(tx *sql.Tx) error {
			return s.recordRejection(ctx, tx, err, fmt.Sprintf("the read intent of item %d", it.id),
				`pending_read`, `item_id = ? AND seq = ?`, it.id, it.seq)
		})
	}
	half := len(intents) / 2
	if err := s.pushItems(ctx, remote, value, intents[:half]); err != nil {
		return err
	}
	return s.pushItems(ctx, remote, value, intents[half:])
}

// pushMarkAll sends the mark-all intents. Each is marked in flight while its
// request runs, so an undo meanwhile writes unread intents instead of
// deleting a row whose effect may already be on the server.
func (s *Service) pushMarkAll(ctx context.Context, remote Remote) error {
	type markAll struct {
		id, ts, seq int64
		stream      string
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, stream_id, ts, seq FROM pending_mark_all ORDER BY seq, id`)
	if err != nil {
		return fmt.Errorf("read mark-all intents: %w", err)
	}
	var all []markAll
	for rows.Next() {
		var m markAll
		if err := rows.Scan(&m.id, &m.stream, &m.ts, &m.seq); err != nil {
			rows.Close()
			return fmt.Errorf("read mark-all intent: %w", err)
		}
		all = append(all, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read mark-all intents: %w", err)
	}

	for _, m := range all {
		// An undo may have deleted the row since it was read.
		s.intentMu.Lock()
		var exists int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_mark_all WHERE id = ?`, m.id).Scan(&exists)
		if err == nil && exists == 1 {
			s.inflight[m.id] = true
		}
		s.intentMu.Unlock()
		if err != nil {
			return fmt.Errorf("check mark-all intent: %w", err)
		}
		if exists == 0 {
			continue
		}

		sendErr := remote.MarkAllAsRead(ctx, m.stream, m.ts)

		err = s.settle(ctx, func(tx *sql.Tx) error {
			delete(s.inflight, m.id)
			switch {
			case sendErr == nil:
				if _, err := tx.ExecContext(ctx, `DELETE FROM pending_mark_all WHERE id = ? AND seq = ?`, m.id, m.seq); err != nil {
					return fmt.Errorf("confirm mark-all intent: %w", err)
				}
				if _, err := tx.ExecContext(ctx,
					`UPDATE articles AS a SET server_read = 1 WHERE a.item_id <= @ts AND `+InStream,
					sql.Named("ts", m.ts), sql.Named("stream", m.stream)); err != nil {
					return fmt.Errorf("mirror pushed mark-all: %w", err)
				}
			case rejectsItem(sendErr):
				return s.recordRejection(ctx, tx, sendErr, "the mark-all intent of "+m.stream,
					`pending_mark_all`, `id = ? AND seq = ?`, m.id, m.seq)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if sendErr != nil && !rejectsItem(sendErr) {
			return sendErr
		}
	}
	return nil
}

// settle runs fn in one transaction under intentMu, for the pusher's writes
// after a request returns.
func (s *Service) settle(ctx context.Context, fn func(*sql.Tx) error) error {
	s.intentMu.Lock()
	defer s.intentMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settle: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settle: %w", err)
	}
	return nil
}

// recordRejection spends one attempt of the intent row in table matched by
// where, keeping cause as its last error, and drops the row at maxAttempts.
// table and where are constants of the callers, never input.
func (s *Service) recordRejection(ctx context.Context, tx *sql.Tx, cause error, what, table, where string, args ...any) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE `+table+` SET attempts = attempts + 1, last_error = ? WHERE `+where,
		append([]any{cause.Error()}, args...)...); err != nil {
		return fmt.Errorf("record rejection of %s: %w", what, err)
	}
	r, err := tx.ExecContext(ctx,
		`DELETE FROM `+table+` WHERE `+where+` AND attempts >= ?`, append(args, s.maxAttempts)...)
	if err != nil {
		return fmt.Errorf("drop %s: %w", what, err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return fmt.Errorf("drop %s: %w", what, err)
	}
	if n > 0 {
		log.Printf("sync: dropped %s after %d rejections: %v", what, s.maxAttempts, cause)
	}
	return nil
}

// rejectsItem reports whether the server refused the request because of what
// it named, which is the only failure that counts against an intent.
func rejectsItem(err error) bool {
	var apiErr *freshrss.APIError
	return errors.As(err, &apiErr) && apiErr.RejectsItem()
}

// schedule arms the push timer to fire after d, replacing a timer already
// armed: a user's write pushes after the debounce even while a retry waits.
func (s *Service) schedule(d time.Duration) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.closed {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, s.pushNow)
}

// pushed takes the outcome of a push: success clears the backoff, a failure
// doubles it within [backoffMin, backoffMax] and schedules a retry.
func (s *Service) pushed(err error) {
	if err == nil {
		s.timerMu.Lock()
		s.backoff = 0
		s.timerMu.Unlock()
		return
	}
	s.timerMu.Lock()
	s.backoff = min(max(2*s.backoff, s.backoffMin), s.backoffMax)
	d := s.backoff
	s.timerMu.Unlock()
	s.schedule(d)
}

// pushNow is the timer's push, outside any cycle.
func (s *Service) pushNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	remote, err := s.remote()
	if err == nil {
		err = s.push(s.ctx, remote)
	}
	if err != nil && s.ctx.Err() == nil {
		log.Printf("sync: %v", err)
	}
	s.pushed(err)
	if s.ctx.Err() == nil {
		s.refreshPending(s.ctx)
	}
}
