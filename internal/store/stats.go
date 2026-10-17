package store

import (
	"context"
	"strings"
	"time"
)

// Analytics queries count plays (never observations) of music tracks whose
// start_at falls in the half-open UTC range [from, to). Station segments are
// excluded. Uncertain plays are included in totals and reported separately.

type StatsFilter struct {
	TrackID  int64
	ArtistID int64
}

func (f StatsFilter) where(from, to time.Time) *where {
	w := &where{}
	w.add("p.start_at >= ?", ts(from))
	w.add("p.start_at < ?", ts(to))
	w.add("t.kind = 'music'")
	if f.TrackID != 0 {
		w.add("p.track_id = ?", f.TrackID)
	}
	if f.ArtistID != 0 {
		w.add("t.artist_id = ?", f.ArtistID)
	}
	return w
}

type Totals struct {
	Plays           int `json:"plays"`
	UncertainPlays  int `json:"uncertain_plays"`
	DistinctTracks  int `json:"distinct_tracks"`
	DistinctArtists int `json:"distinct_artists"`
}

func (s *Store) Totals(ctx context.Context, from, to time.Time, f StatsFilter) (Totals, error) {
	w := f.where(from, to)
	var t Totals
	err := s.r.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(p.uncertain), 0),
		COUNT(DISTINCT p.track_id), COUNT(DISTINCT t.artist_id)
		FROM plays p JOIN tracks t ON t.id = p.track_id`+w.sql(), w.args...).
		Scan(&t.Plays, &t.UncertainPlays, &t.DistinctTracks, &t.DistinctArtists)
	return t, err
}

type TrackCount struct {
	Track          TrackRef `json:"track"`
	Plays          int      `json:"plays"`
	UncertainPlays int      `json:"uncertain_plays"`
}

func (s *Store) TopTracks(ctx context.Context, from, to time.Time, f StatsFilter, limit int) ([]TrackCount, error) {
	w := f.where(from, to)
	rows, err := s.r.QueryContext(ctx, `SELECT t.id, t.title, t.kind, a.id, a.name, COUNT(*), SUM(p.uncertain)
		FROM plays p JOIN tracks t ON t.id = p.track_id LEFT JOIN artists a ON a.id = t.artist_id`+w.sql()+
		` GROUP BY t.id ORDER BY COUNT(*) DESC, t.title_key, t.id LIMIT ?`, append(w.args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackCount{}
	for rows.Next() {
		var c TrackCount
		var aid *int64
		var aname *string
		if err := rows.Scan(&c.Track.ID, &c.Track.Title, &c.Track.Kind, &aid, &aname, &c.Plays, &c.UncertainPlays); err != nil {
			return nil, err
		}
		if aid != nil {
			c.Track.Artist = &ArtistRef{ID: *aid, Name: deref(aname)}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type ArtistCount struct {
	Artist         ArtistRef `json:"artist"`
	Plays          int       `json:"plays"`
	UncertainPlays int       `json:"uncertain_plays"`
	DistinctTracks int       `json:"distinct_tracks"`
}

func (s *Store) TopArtists(ctx context.Context, from, to time.Time, limit int) ([]ArtistCount, error) {
	w := StatsFilter{}.where(from, to)
	rows, err := s.r.QueryContext(ctx, `SELECT a.id, a.name, COUNT(*), SUM(p.uncertain), COUNT(DISTINCT t.id)
		FROM plays p JOIN tracks t ON t.id = p.track_id JOIN artists a ON a.id = t.artist_id`+w.sql()+
		` GROUP BY a.id ORDER BY COUNT(*) DESC, a.name_key, a.id LIMIT ?`, append(w.args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArtistCount{}
	for rows.Next() {
		var c ArtistCount
		if err := rows.Scan(&c.Artist.ID, &c.Artist.Name, &c.Plays, &c.UncertainPlays, &c.DistinctTracks); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountsBy returns play counts in [from, to) grouped by track id (byArtist
// false) or artist id (byArtist true), restricted to ids.
func (s *Store) CountsBy(ctx context.Context, from, to time.Time, byArtist bool, ids []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	col := "p.track_id"
	if byArtist {
		col = "t.artist_id"
	}
	w := StatsFilter{}.where(from, to)
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := w.args
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.r.QueryContext(ctx, `SELECT `+col+`, COUNT(*) FROM plays p JOIN tracks t ON t.id = p.track_id`+
		w.sql()+` AND `+col+` IN (`+ph+`) GROUP BY `+col, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// PlayStart is the minimal per-play data needed for time bucketing, which
// happens in Go so that timezone and DST rules are applied correctly.
type PlayStart struct {
	At        time.Time
	Uncertain bool
}

// MaxPlayStarts bounds memory for bucketing queries. At roughly 20 plays per
// hour this is many years of data; validated ranges never get close.
const MaxPlayStarts = 500_000

func (s *Store) PlayStarts(ctx context.Context, from, to time.Time, f StatsFilter) ([]PlayStart, error) {
	w := f.where(from, to)
	rows, err := s.r.QueryContext(ctx, `SELECT p.start_at, p.uncertain FROM plays p JOIN tracks t ON t.id = p.track_id`+
		w.sql()+` ORDER BY p.start_at LIMIT ?`, append(w.args, MaxPlayStarts)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlayStart
	for rows.Next() {
		var at string
		var ps PlayStart
		if err := rows.Scan(&at, &ps.Uncertain); err != nil {
			return nil, err
		}
		ps.At = parseTS(at)
		out = append(out, ps)
	}
	return out, rows.Err()
}

// Gap is a period with no successful observation longer than the
// configured continuity gap.
type Gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// ObservationGaps lists gaps between consecutive successful observations in
// [from, to) that exceed minGap, plus the earliest retained observation
// overall (observations older than the retention window are pruned, so
// gaps before that point cannot be determined).
func (s *Store) ObservationGaps(ctx context.Context, from, to time.Time, minGap time.Duration, limit int) ([]Gap, *time.Time, error) {
	var earliest *string
	if err := s.r.QueryRowContext(ctx, `SELECT MIN(observed_at) FROM observations`).Scan(&earliest); err != nil {
		return nil, nil, err
	}
	var earliestT *time.Time
	if earliest != nil {
		t := parseTS(*earliest)
		earliestT = &t
	}
	// Include the last observation before the range so a gap spanning the
	// range start is detected.
	rows, err := s.r.QueryContext(ctx, `
		WITH o AS (
			SELECT observed_at FROM observations WHERE observed_at >= ? AND observed_at < ?
			UNION ALL
			SELECT MAX(observed_at) FROM observations WHERE observed_at < ?
		), seq AS (
			SELECT observed_at, LAG(observed_at) OVER (ORDER BY observed_at) AS prev
			FROM o WHERE observed_at IS NOT NULL
		)
		SELECT prev, observed_at FROM seq
		WHERE prev IS NOT NULL AND (julianday(observed_at) - julianday(prev)) * 86400.0 > ?
		ORDER BY observed_at LIMIT ?`,
		ts(from), ts(to), ts(from), minGap.Seconds(), limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	gaps := []Gap{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, nil, err
		}
		gaps = append(gaps, Gap{From: parseTS(a), To: parseTS(b)})
	}
	return gaps, earliestT, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
