package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/normalize"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

// Reasons a play segment was created.
const (
	ReasonUpstreamEvent      = "upstream_event"
	ReasonObservedTransition = "observed_transition"
	ReasonInitialObservation = "initial_observation"
	ReasonContinuityGap      = "continuity_gap"
)

// Observation is one successful poll ready to be persisted.
type Observation struct {
	ObservedAt time.Time
	Item       upstream.Item
	FetchMode  string
	RawPayload []byte // nil unless raw payload storage is enabled
}

// RecordResult describes what RecordObservation did.
type RecordResult struct {
	ObservationID int64
	PlayID        int64
	TrackID       int64
	NewPlay       bool
	Reason        string // set when NewPlay
}

// RecordObservation persists an observation and creates or extends the
// corresponding play in a single transaction. continuityGap bounds how far
// apart two observations of the same track may be and still be treated as
// one inferred play when no upstream uid is available.
//
// Play identity:
//   - With an upstream uid, the uid is the identity. Re-observing it (from a
//     later poll, a retry, or after a restart) extends the same play;
//     a new uid is a new play even if the track is the same.
//   - Without one, consecutive observations of the same track within the
//     continuity gap form one play. A different track, or the same track
//     after a gap, starts a new play.
func (s *Store) RecordObservation(ctx context.Context, o Observation, continuityGap time.Duration) (RecordResult, error) {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return RecordResult{}, err
	}
	defer tx.Rollback()

	at := ts(o.ObservedAt)
	trackID, err := upsertTrack(ctx, tx, o.Item, at)
	if err != nil {
		return RecordResult{}, err
	}

	res := RecordResult{TrackID: trackID}
	cur, err := currentPlay(ctx, tx)
	if err != nil {
		return RecordResult{}, err
	}

	var extend *playRow
	uid := o.Item.UID
	if uid != "" {
		p, err := playByUID(ctx, tx, uid)
		if err != nil {
			return RecordResult{}, err
		}
		if p != nil && p.trackID != trackID {
			// The uid no longer identifies the same track. Do not trust it
			// for this observation; fall back to transition inference.
			uid = ""
		} else {
			extend = p
		}
	}
	if uid == "" && cur != nil && cur.trackID == trackID && cur.uid == "" &&
		o.ObservedAt.Sub(cur.lastObserved) <= continuityGap {
		extend = cur
	}

	if extend != nil {
		last := at
		if o.ObservedAt.Before(extend.lastObserved) {
			last = ts(extend.lastObserved)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE plays SET last_observed_at = ?, observation_count = observation_count + 1 WHERE id = ?`,
			last, extend.id); err != nil {
			return RecordResult{}, err
		}
		res.PlayID = extend.id
	} else {
		var reason string
		switch {
		case uid != "":
			reason = ReasonUpstreamEvent
		case cur == nil:
			reason = ReasonInitialObservation
		case o.ObservedAt.Sub(cur.lastObserved) > continuityGap:
			reason = ReasonContinuityGap
		default:
			reason = ReasonObservedTransition
		}
		start, authoritative := at, false
		var upstreamStart sql.NullString
		if uid != "" && o.Item.StartAt != nil {
			upstreamStart = nullTS(o.Item.StartAt)
			start, authoritative = upstreamStart.String, true
		}
		// Without an upstream event, the first play after startup or after a
		// collection gap may be a continuation of something already counted
		// or may have started long before it was seen.
		uncertain := reason == ReasonInitialObservation || reason == ReasonContinuityGap
		r, err := tx.ExecContext(ctx, `INSERT INTO plays
			(track_id, upstream_uid, upstream_start_at, start_at, start_authoritative,
			 first_observed_at, last_observed_at, observation_count, reason, uncertain)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			trackID, nullStr(uid), upstreamStart, start, authoritative, at, at, reason, uncertain)
		if err != nil {
			return RecordResult{}, fmt.Errorf("insert play: %w", err)
		}
		res.PlayID, _ = r.LastInsertId()
		res.NewPlay, res.Reason = true, reason
	}

	var raw sql.NullString
	if o.RawPayload != nil {
		raw = sql.NullString{String: string(o.RawPayload), Valid: true}
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO observations
		(observed_at, track_id, play_id, upstream_uid, upstream_date, program, fetch_mode, raw_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		at, trackID, res.PlayID, nullStr(o.Item.UID), nullStr(o.Item.RawDate), nullStr(o.Item.Program), o.FetchMode, raw)
	if err != nil {
		return RecordResult{}, fmt.Errorf("insert observation: %w", err)
	}
	res.ObservationID, _ = r.LastInsertId()

	if _, err := tx.ExecContext(ctx, `UPDATE collector_state SET
		current_play_id = ?, last_attempt_at = ?, last_success_at = ?,
		first_success_at = COALESCE(first_success_at, ?),
		consecutive_failures = 0, last_error = NULL, fetch_mode = ?
		WHERE id = 1`, res.PlayID, at, at, at, o.FetchMode); err != nil {
		return RecordResult{}, err
	}
	return res, tx.Commit()
}

func upsertTrack(ctx context.Context, tx *sql.Tx, it upstream.Item, at string) (int64, error) {
	var artistID sql.NullInt64
	if it.Artist != "" {
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO artists (name_key, name, created_at) VALUES (?, ?, ?)
			ON CONFLICT (name_key) DO UPDATE SET name_key = excluded.name_key
			RETURNING id`, normalize.Key(it.Artist), it.Artist, at).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("upsert artist: %w", err)
		}
		artistID = sql.NullInt64{Int64: id, Valid: true}
	}
	kind := string(it.Kind)
	if kind == "" {
		kind = string(upstream.KindMusic)
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO tracks
		(identity_key, artist_id, title, title_key, kind, first_observed_at, last_observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (identity_key) DO UPDATE SET
			last_observed_at = MAX(last_observed_at, excluded.last_observed_at),
			first_observed_at = MIN(first_observed_at, excluded.first_observed_at)
		RETURNING id`,
		normalize.TrackKey(it.Artist, it.Title), artistID, it.Title, normalize.Key(it.Title), kind, at, at).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert track: %w", err)
	}
	return id, nil
}

type playRow struct {
	id           int64
	trackID      int64
	uid          string
	lastObserved time.Time
}

func scanPlayRow(row *sql.Row) (*playRow, error) {
	var p playRow
	var uid sql.NullString
	var last string
	if err := row.Scan(&p.id, &p.trackID, &uid, &last); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.uid, p.lastObserved = uid.String, parseTS(last)
	return &p, nil
}

func currentPlay(ctx context.Context, tx *sql.Tx) (*playRow, error) {
	return scanPlayRow(tx.QueryRowContext(ctx, `SELECT p.id, p.track_id, p.upstream_uid, p.last_observed_at
		FROM collector_state s JOIN plays p ON p.id = s.current_play_id WHERE s.id = 1`))
}

func playByUID(ctx context.Context, tx *sql.Tx, uid string) (*playRow, error) {
	return scanPlayRow(tx.QueryRowContext(ctx,
		`SELECT id, track_id, upstream_uid, last_observed_at FROM plays WHERE upstream_uid = ?`, uid))
}

// RecordFailure records a failed collection attempt. The last successful
// now-playing value is left untouched so it can be served as stale.
func (s *Store) RecordFailure(ctx context.Context, at time.Time, fetchMode, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.w.ExecContext(ctx, `UPDATE collector_state SET
		last_attempt_at = ?, last_error = ?, last_error_at = ?,
		consecutive_failures = consecutive_failures + 1, fetch_mode = ?
		WHERE id = 1`, ts(at), msg, ts(at), fetchMode)
	return err
}

// PruneResult reports how many rows retention removed or cleared.
type PruneResult struct {
	Observations int64
	RawPayloads  int64
}

// Prune deletes observations older than observationRetention and clears raw
// payloads older than rawRetention. Zero disables the respective rule.
// Plays, tracks and artists are never deleted: analytics history is kept
// independently of observation retention.
func (s *Store) Prune(ctx context.Context, now time.Time, observationRetention, rawRetention time.Duration) (PruneResult, error) {
	var res PruneResult
	if rawRetention > 0 {
		r, err := s.w.ExecContext(ctx, `UPDATE observations SET raw_payload = NULL
			WHERE raw_payload IS NOT NULL AND observed_at < ?`, ts(now.Add(-rawRetention)))
		if err != nil {
			return res, err
		}
		res.RawPayloads, _ = r.RowsAffected()
	}
	if observationRetention > 0 {
		r, err := s.w.ExecContext(ctx, `DELETE FROM observations WHERE observed_at < ?`, ts(now.Add(-observationRetention)))
		if err != nil {
			return res, err
		}
		res.Observations, _ = r.RowsAffected()
	}
	return res, nil
}
