package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/normalize"
)

// ErrNotFound is returned by single-entity lookups.
var ErrNotFound = errors.New("not found")

type ArtistRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TrackRef struct {
	ID     int64      `json:"id"`
	Title  string     `json:"title"`
	Kind   string     `json:"kind"`
	Artist *ArtistRef `json:"artist"`
}

type Play struct {
	ID                 int64      `json:"id"`
	Track              TrackRef   `json:"track"`
	StartAt            time.Time  `json:"start_at"`
	StartAuthoritative bool       `json:"start_authoritative"`
	UpstreamStartAt    *time.Time `json:"upstream_start_at"`
	FirstObservedAt    time.Time  `json:"first_observed_at"`
	LastObservedAt     time.Time  `json:"last_observed_at"`
	ObservationCount   int        `json:"observation_count"`
	Reason             string     `json:"reason"`
	Uncertain          bool       `json:"uncertain"`
}

const playColumns = `p.id, p.start_at, p.start_authoritative, p.upstream_start_at, p.first_observed_at,
	p.last_observed_at, p.observation_count, p.reason, p.uncertain,
	t.id, t.title, t.kind, a.id, a.name`

const playFrom = ` FROM plays p JOIN tracks t ON t.id = p.track_id LEFT JOIN artists a ON a.id = t.artist_id`

type scanner interface{ Scan(...any) error }

func scanPlay(sc scanner) (Play, error) {
	var p Play
	var start, first, last string
	var upStart, artistName sql.NullString
	var artistID sql.NullInt64
	err := sc.Scan(&p.ID, &start, &p.StartAuthoritative, &upStart, &first, &last, &p.ObservationCount,
		&p.Reason, &p.Uncertain, &p.Track.ID, &p.Track.Title, &p.Track.Kind, &artistID, &artistName)
	if err != nil {
		return p, err
	}
	p.StartAt, p.FirstObservedAt, p.LastObservedAt = parseTS(start), parseTS(first), parseTS(last)
	p.UpstreamStartAt = parseNullTS(upStart)
	if artistID.Valid {
		p.Track.Artist = &ArtistRef{ID: artistID.Int64, Name: artistName.String}
	}
	return p, nil
}

// Cursor positions the activity feed at (StartAt, ID), exclusive.
type Cursor struct {
	StartAt time.Time
	ID      int64
}

type PlayFilter struct {
	From, To        *time.Time // half-open [From, To) on start_at
	TrackID         int64
	ArtistID        int64
	Query           string
	IncludeSegments bool
}

type where struct {
	clauses []string
	args    []any
}

func (w *where) add(clause string, args ...any) {
	w.clauses = append(w.clauses, clause)
	w.args = append(w.args, args...)
}

func (w *where) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(normalize.Key(q)) + "%"
}

func (f PlayFilter) apply(w *where) {
	if f.From != nil {
		w.add("p.start_at >= ?", ts(*f.From))
	}
	if f.To != nil {
		w.add("p.start_at < ?", ts(*f.To))
	}
	if f.TrackID != 0 {
		w.add("p.track_id = ?", f.TrackID)
	}
	if f.ArtistID != 0 {
		w.add("t.artist_id = ?", f.ArtistID)
	}
	if !f.IncludeSegments {
		w.add("t.kind = 'music'")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := likePattern(q)
		w.add(`(t.title_key LIKE ? ESCAPE '\' OR a.name_key LIKE ? ESCAPE '\')`, p, p)
	}
}

// ListPlays returns plays newest first, ordered by (start_at DESC, id DESC).
// It fetches limit+1 rows so the caller can tell whether more remain.
func (s *Store) ListPlays(ctx context.Context, f PlayFilter, after *Cursor, limit int) ([]Play, bool, error) {
	var w where
	f.apply(&w)
	if after != nil {
		at := ts(after.StartAt)
		w.add("(p.start_at < ? OR (p.start_at = ? AND p.id < ?))", at, at, after.ID)
	}
	rows, err := s.r.QueryContext(ctx, "SELECT "+playColumns+playFrom+w.sql()+
		" ORDER BY p.start_at DESC, p.id DESC LIMIT ?", append(w.args, limit+1)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	plays := make([]Play, 0, limit)
	for rows.Next() {
		p, err := scanPlay(rows)
		if err != nil {
			return nil, false, err
		}
		plays = append(plays, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(plays) > limit
	if more {
		plays = plays[:limit]
	}
	return plays, more, nil
}

// CollectorState is the persisted collector status.
type CollectorState struct {
	CurrentPlay         *Play
	FirstSuccessAt      *time.Time
	LastAttemptAt       *time.Time
	LastSuccessAt       *time.Time
	LastError           string
	LastErrorAt         *time.Time
	ConsecutiveFailures int
	FetchMode           string
}

func (s *Store) CollectorState(ctx context.Context) (CollectorState, error) {
	var st CollectorState
	var playID sql.NullInt64
	var first, attempt, success, errAt, lastErr, mode sql.NullString
	err := s.r.QueryRowContext(ctx, `SELECT current_play_id, first_success_at, last_attempt_at, last_success_at,
		last_error, last_error_at, consecutive_failures, fetch_mode FROM collector_state WHERE id = 1`).
		Scan(&playID, &first, &attempt, &success, &lastErr, &errAt, &st.ConsecutiveFailures, &mode)
	if err != nil {
		return st, err
	}
	st.FirstSuccessAt, st.LastAttemptAt, st.LastSuccessAt, st.LastErrorAt =
		parseNullTS(first), parseNullTS(attempt), parseNullTS(success), parseNullTS(errAt)
	st.LastError, st.FetchMode = lastErr.String, mode.String
	if playID.Valid {
		p, err := scanPlay(s.r.QueryRowContext(ctx, "SELECT "+playColumns+playFrom+" WHERE p.id = ?", playID.Int64))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return st, err
		}
		if err == nil {
			st.CurrentPlay = &p
		}
	}
	return st, nil
}

type TrackSummary struct {
	TrackRef
	FirstObservedAt time.Time  `json:"first_observed_at"`
	LastObservedAt  time.Time  `json:"last_observed_at"`
	TotalPlays      int        `json:"total_plays"`
	UncertainPlays  int        `json:"uncertain_plays"`
	LastPlayAt      *time.Time `json:"last_play_at"`
}

const trackSummaryColumns = `t.id, t.title, t.kind, a.id, a.name, t.first_observed_at, t.last_observed_at,
	COUNT(p.id), COALESCE(SUM(p.uncertain), 0), MAX(p.start_at)`

func scanTrackSummary(sc scanner) (TrackSummary, error) {
	var t TrackSummary
	var artistID sql.NullInt64
	var artistName, lastPlay sql.NullString
	var first, last string
	if err := sc.Scan(&t.ID, &t.Title, &t.Kind, &artistID, &artistName, &first, &last,
		&t.TotalPlays, &t.UncertainPlays, &lastPlay); err != nil {
		return t, err
	}
	t.FirstObservedAt, t.LastObservedAt, t.LastPlayAt = parseTS(first), parseTS(last), parseNullTS(lastPlay)
	if artistID.Valid {
		t.Artist = &ArtistRef{ID: artistID.Int64, Name: artistName.String}
	}
	return t, nil
}

// SearchTracks lists music tracks, most-played first, optionally filtered by
// a case-insensitive substring of the title or artist.
func (s *Store) SearchTracks(ctx context.Context, q string, artistID int64, limit, offset int) ([]TrackSummary, error) {
	w := where{}
	w.add("t.kind = 'music'")
	if q = strings.TrimSpace(q); q != "" {
		p := likePattern(q)
		w.add(`(t.title_key LIKE ? ESCAPE '\' OR a.name_key LIKE ? ESCAPE '\')`, p, p)
	}
	if artistID != 0 {
		w.add("t.artist_id = ?", artistID)
	}
	rows, err := s.r.QueryContext(ctx, "SELECT "+trackSummaryColumns+
		` FROM tracks t LEFT JOIN artists a ON a.id = t.artist_id LEFT JOIN plays p ON p.track_id = t.id`+
		w.sql()+` GROUP BY t.id ORDER BY COUNT(p.id) DESC, t.title_key, t.id LIMIT ? OFFSET ?`,
		append(w.args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackSummary{}
	for rows.Next() {
		t, err := scanTrackSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Track(ctx context.Context, id int64) (TrackSummary, error) {
	t, err := scanTrackSummary(s.r.QueryRowContext(ctx, "SELECT "+trackSummaryColumns+
		` FROM tracks t LEFT JOIN artists a ON a.id = t.artist_id LEFT JOIN plays p ON p.track_id = t.id
		WHERE t.id = ? GROUP BY t.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

type ArtistSummary struct {
	ArtistRef
	TrackCount      int        `json:"track_count"`
	TotalPlays      int        `json:"total_plays"`
	UncertainPlays  int        `json:"uncertain_plays"`
	FirstObservedAt *time.Time `json:"first_observed_at"`
	LastObservedAt  *time.Time `json:"last_observed_at"`
	LastPlayAt      *time.Time `json:"last_play_at"`
}

const artistSummaryColumns = `a.id, a.name, COUNT(DISTINCT t.id), COUNT(p.id), COALESCE(SUM(p.uncertain), 0),
	MIN(t.first_observed_at), MAX(t.last_observed_at), MAX(p.start_at)`

const artistSummaryFrom = ` FROM artists a JOIN tracks t ON t.artist_id = a.id LEFT JOIN plays p ON p.track_id = t.id`

func scanArtistSummary(sc scanner) (ArtistSummary, error) {
	var a ArtistSummary
	var first, last, lastPlay sql.NullString
	if err := sc.Scan(&a.ID, &a.Name, &a.TrackCount, &a.TotalPlays, &a.UncertainPlays, &first, &last, &lastPlay); err != nil {
		return a, err
	}
	a.FirstObservedAt, a.LastObservedAt, a.LastPlayAt = parseNullTS(first), parseNullTS(last), parseNullTS(lastPlay)
	return a, nil
}

func (s *Store) SearchArtists(ctx context.Context, q string, limit, offset int) ([]ArtistSummary, error) {
	w := where{}
	if q = strings.TrimSpace(q); q != "" {
		w.add(`a.name_key LIKE ? ESCAPE '\'`, likePattern(q))
	}
	rows, err := s.r.QueryContext(ctx, "SELECT "+artistSummaryColumns+artistSummaryFrom+w.sql()+
		` GROUP BY a.id ORDER BY COUNT(p.id) DESC, a.name_key, a.id LIMIT ? OFFSET ?`,
		append(w.args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArtistSummary{}
	for rows.Next() {
		a, err := scanArtistSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Artist(ctx context.Context, id int64) (ArtistSummary, error) {
	a, err := scanArtistSummary(s.r.QueryRowContext(ctx, "SELECT "+artistSummaryColumns+artistSummaryFrom+
		` WHERE a.id = ? GROUP BY a.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}
