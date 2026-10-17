package api

import (
	"net/http"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/analytics"
	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/version"
)

const (
	defaultRangeDays = 7
	maxGapsReported  = 100
)

// CollectorStatus separates upstream freshness from application health.
type CollectorStatus struct {
	// State is one of: disabled, starting (no success yet), ok, stale
	// (last success older than stale_after), error (latest attempt failed
	// but data is not yet stale).
	State               string     `json:"state"`
	Stale               bool       `json:"stale"`
	FetchMode           string     `json:"fetch_mode"`
	PollIntervalSeconds int        `json:"poll_interval_seconds"`
	StaleAfterSeconds   int        `json:"stale_after_seconds"`
	FirstSuccessAt      *time.Time `json:"first_success_at"`
	LastAttemptAt       *time.Time `json:"last_attempt_at"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	NextAttemptAt       *time.Time `json:"next_attempt_at"`
	LastError           string     `json:"last_error,omitempty"`
	LastErrorAt         *time.Time `json:"last_error_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

func (s *Server) collectorStatus(st store.CollectorState, now time.Time) CollectorStatus {
	cs := CollectorStatus{
		FetchMode:           s.opts.FetchMode,
		PollIntervalSeconds: int(s.opts.PollInterval.Seconds()),
		StaleAfterSeconds:   int(s.opts.StaleAfter.Seconds()),
		FirstSuccessAt:      st.FirstSuccessAt,
		LastAttemptAt:       st.LastAttemptAt,
		LastSuccessAt:       st.LastSuccessAt,
		LastErrorAt:         st.LastErrorAt,
		ConsecutiveFailures: st.ConsecutiveFailures,
	}
	if st.ConsecutiveFailures > 0 {
		cs.LastError = st.LastError
	}
	if s.opts.NextAttemptAt != nil {
		if t := s.opts.NextAttemptAt(); !t.IsZero() {
			cs.NextAttemptAt = &t
		}
	}
	cs.Stale = st.LastSuccessAt == nil || now.Sub(*st.LastSuccessAt) > s.opts.StaleAfter
	switch {
	case !s.opts.CollectorEnabled:
		cs.State = "disabled"
	case st.LastSuccessAt == nil && st.ConsecutiveFailures == 0:
		cs.State = "starting"
	case cs.Stale:
		cs.State = "stale"
	case st.ConsecutiveFailures > 0:
		cs.State = "error"
	default:
		cs.State = "ok"
	}
	return cs
}

type nowPlayingResponse struct {
	// Play is the most recently observed play, retained across failures.
	Play      *store.Play     `json:"play"`
	Collector CollectorStatus `json:"collector"`
	Timezone  string          `json:"timezone"`
}

func (s *Server) nowPlaying(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.CollectorState(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nowPlayingResponse{
		Play:      st.CurrentPlay,
		Collector: s.collectorStatus(st, s.opts.Now()),
		Timezone:  s.opts.Timezone.String(),
	})
}

type statusResponse struct {
	Collector            CollectorStatus `json:"collector"`
	Timezone             string          `json:"timezone"`
	ContinuityGapSeconds int             `json:"continuity_gap_seconds"`
	SchemaVersion        int             `json:"schema_version"`
	Build                version.Info    `json:"build"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.CollectorState(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	v, err := s.store.SchemaVersion(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Collector:            s.collectorStatus(st, s.opts.Now()),
		Timezone:             s.opts.Timezone.String(),
		ContinuityGapSeconds: int(s.opts.ContinuityGap.Seconds()),
		SchemaVersion:        v,
		Build:                version.Get(),
	})
}

type playsResponse struct {
	Items      []store.Play `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}

func (s *Server) listPlays(w http.ResponseWriter, r *http.Request) {
	var f store.PlayFilter
	var err error
	limit, err := intParam(r, "limit", 25, 1, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err == nil {
		f.From, err = s.optionalTime(r, "from")
	}
	if err == nil {
		f.To, err = s.optionalTime(r, "to")
	}
	if err == nil {
		f.TrackID, err = idParam(r, "track_id")
	}
	if err == nil {
		f.ArtistID, err = idParam(r, "artist_id")
	}
	if err == nil {
		f.Query, err = queryParam(r)
	}
	if err == nil {
		f.IncludeSegments, err = boolParam(r, "include_segments")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		writeError(w, http.StatusBadRequest, "invalid_range", "from must be before to")
		return
	}
	plays, more, err := s.store.ListPlays(r.Context(), f, cursor, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := playsResponse{Items: plays}
	if more {
		last := plays[len(plays)-1]
		c := encodeCursor(store.Cursor{StartAt: last.StartAt, ID: last.ID})
		resp.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

type listResponse[T any] struct {
	Items []T `json:"items"`
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	q, err := queryParam(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	artistID, err := idParam(r, "artist_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 25, 1, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	offset, err := intParam(r, "offset", 0, 0, 10000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.store.SearchTracks(r.Context(), q, artistID, limit, offset)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[store.TrackSummary]{Items: items})
}

func (s *Server) getTrack(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.store.Track(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) listArtists(w http.ResponseWriter, r *http.Request) {
	q, err := queryParam(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 25, 1, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	offset, err := intParam(r, "offset", 0, 0, 10000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.store.SearchArtists(r.Context(), q, limit, offset)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[store.ArtistSummary]{Items: items})
}

func (s *Server) getArtist(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.store.Artist(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// --- analytics ---

type rangeInfo struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Timezone string    `json:"timezone"`
}

func (s *Server) rangeInfo(rg analytics.Range) rangeInfo {
	return rangeInfo{From: rg.From.In(s.opts.Timezone), To: rg.To.In(s.opts.Timezone), Timezone: s.opts.Timezone.String()}
}

// statsParams parses from/to/track_id/artist_id shared by analytics endpoints.
func (s *Server) statsParams(r *http.Request) (analytics.Range, store.StatsFilter, error) {
	q := r.URL.Query()
	var f store.StatsFilter
	rg, err := analytics.ParseRange(q.Get("from"), q.Get("to"), s.opts.Timezone, s.opts.Now(), defaultRangeDays)
	if err != nil {
		return rg, f, err
	}
	if f.TrackID, err = idParam(r, "track_id"); err != nil {
		return rg, f, err
	}
	f.ArtistID, err = idParam(r, "artist_id")
	return rg, f, err
}

type comparison struct {
	Current       rangeInfo        `json:"current"`
	Previous      rangeInfo        `json:"previous"`
	CurrentPlays  int              `json:"current_plays"`
	Change        analytics.Change `json:"change"`
	CurrentClosed bool             `json:"current_closed"`
}

type coverage struct {
	CollectionStartedAt *time.Time `json:"collection_started_at"`
	// ObservationsSince is the oldest retained observation; gaps before it
	// cannot be detected because observations are pruned by retention.
	ObservationsSince *time.Time      `json:"observations_since"`
	Gaps              []store.Gap     `json:"gaps"`
	GapsTruncated     bool            `json:"gaps_truncated"`
	Partial           bool            `json:"partial"`
	MinGapSeconds     int             `json:"min_gap_seconds"`
	Collector         CollectorStatus `json:"collector"`
}

type summaryResponse struct {
	Range      rangeInfo         `json:"range"`
	Totals     store.Totals      `json:"totals"`
	Comparison *comparison       `json:"comparison"`
	Coverage   coverage          `json:"coverage"`
	Semantics  map[string]string `json:"semantics"`
}

var summarySemantics = map[string]string{
	"plays":           "Distinct airplay events of music tracks whose start falls in [from, to). Station segments are excluded. Observations (poll results) are never counted.",
	"uncertain_plays": "Plays included in 'plays' that were inferred without an upstream event id after startup or a collection gap; they may duplicate or truncate an adjacent play.",
	"comparison":      "Current period is clipped to now; previous period is the immediately preceding period of equal length. State 'unavailable' means collection had not started for the whole previous period.",
}

func (s *Server) statsSummary(w http.ResponseWriter, r *http.Request) {
	rg, f, err := s.statsParams(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	now := s.opts.Now()
	totals, err := s.store.Totals(ctx, rg.From, rg.To, f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	st, err := s.store.CollectorState(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := summaryResponse{Range: s.rangeInfo(rg), Totals: totals, Semantics: summarySemantics}

	if cur, prev, ok := analytics.Periods(rg, now); ok {
		curT, err := s.store.Totals(ctx, cur.From, cur.To, f)
		if err == nil {
			var prevT store.Totals
			prevT, err = s.store.Totals(ctx, prev.From, prev.To, f)
			resp.Comparison = &comparison{
				Current: s.rangeInfo(cur), Previous: s.rangeInfo(prev), CurrentPlays: curT.Plays,
				Change:        analytics.Compare(curT.Plays, prevT.Plays, analytics.Covered(prev.From, st.FirstSuccessAt)),
				CurrentClosed: !rg.To.After(now),
			}
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}

	gaps, since, err := s.store.ObservationGaps(ctx, rg.From, rg.To, s.opts.ContinuityGap, maxGapsReported+1)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	cov := coverage{
		CollectionStartedAt: st.FirstSuccessAt, ObservationsSince: since, MinGapSeconds: int(s.opts.ContinuityGap.Seconds()),
		Collector: s.collectorStatus(st, now),
	}
	if len(gaps) > maxGapsReported {
		gaps, cov.GapsTruncated = gaps[:maxGapsReported], true
	}
	// A trailing gap (collector currently failing or stopped) has no
	// closing observation, so add it explicitly.
	end := rg.To
	if now.Before(end) {
		end = now
	}
	if st.LastSuccessAt != nil && st.LastSuccessAt.Before(end) && end.Sub(*st.LastSuccessAt) > s.opts.ContinuityGap &&
		!st.LastSuccessAt.Before(rg.From.Add(-s.opts.ContinuityGap)) && !cov.GapsTruncated {
		gaps = append(gaps, store.Gap{From: *st.LastSuccessAt, To: end})
	}
	cov.Gaps = gaps
	cov.Partial = len(gaps) > 0 || !analytics.Covered(rg.From, st.FirstSuccessAt) || rg.To.After(now)
	resp.Coverage = cov
	writeJSON(w, http.StatusOK, resp)
}

type rankedTrack struct {
	store.TrackCount
	Change *analytics.Change `json:"change"`
}

type rankedArtist struct {
	store.ArtistCount
	Change *analytics.Change `json:"change"`
}

type rankingResponse[T any] struct {
	Range rangeInfo `json:"range"`
	// Previous is the comparison period used for each item's change, or
	// null when the range lies entirely in the future.
	Previous *rangeInfo `json:"previous"`
	Items    []T        `json:"items"`
}

// previousCounts computes per-item counts for the comparison period.
func (s *Server) previousCounts(r *http.Request, rg analytics.Range, byArtist bool, ids []int64) (*rangeInfo, map[int64]int, bool, error) {
	_, prev, ok := analytics.Periods(rg, s.opts.Now())
	if !ok {
		return nil, nil, false, nil
	}
	st, err := s.store.CollectorState(r.Context())
	if err != nil {
		return nil, nil, false, err
	}
	counts, err := s.store.CountsBy(r.Context(), prev.From, prev.To, byArtist, ids)
	ri := s.rangeInfo(prev)
	return &ri, counts, analytics.Covered(prev.From, st.FirstSuccessAt), err
}

func (s *Server) topTracks(w http.ResponseWriter, r *http.Request) {
	rg, f, err := s.statsParams(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 10, 1, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	top, err := s.store.TopTracks(r.Context(), rg.From, rg.To, store.StatsFilter{ArtistID: f.ArtistID}, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ids := make([]int64, len(top))
	for i, t := range top {
		ids[i] = t.Track.ID
	}
	prevRange, prev, covered, err := s.previousCounts(r, rg, false, ids)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := rankingResponse[rankedTrack]{Range: s.rangeInfo(rg), Previous: prevRange, Items: make([]rankedTrack, len(top))}
	for i, t := range top {
		resp.Items[i].TrackCount = t
		if prev != nil {
			c := analytics.Compare(t.Plays, prev[t.Track.ID], covered)
			resp.Items[i].Change = &c
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) topArtists(w http.ResponseWriter, r *http.Request) {
	rg, _, err := s.statsParams(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 10, 1, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	top, err := s.store.TopArtists(r.Context(), rg.From, rg.To, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ids := make([]int64, len(top))
	for i, a := range top {
		ids[i] = a.Artist.ID
	}
	prevRange, prev, covered, err := s.previousCounts(r, rg, true, ids)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := rankingResponse[rankedArtist]{Range: s.rangeInfo(rg), Previous: prevRange, Items: make([]rankedArtist, len(top))}
	for i, a := range top {
		resp.Items[i].ArtistCount = a
		if prev != nil {
			c := analytics.Compare(a.Plays, prev[a.Artist.ID], covered)
			resp.Items[i].Change = &c
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) playTimes(r *http.Request, rg analytics.Range, f store.StatsFilter) ([]analytics.PlayTime, error) {
	starts, err := s.store.PlayStarts(r.Context(), rg.From, rg.To, f)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.PlayTime, len(starts))
	for i, p := range starts {
		out[i] = analytics.PlayTime{At: p.At, Uncertain: p.Uncertain}
	}
	return out, nil
}

type airplayResponse struct {
	Range  rangeInfo         `json:"range"`
	Bucket analytics.Bucket  `json:"bucket"`
	Points []analytics.Point `json:"points"`
}

func (s *Server) airplay(w http.ResponseWriter, r *http.Request) {
	rg, f, err := s.statsParams(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bucket, err := analytics.ParseBucket(r.URL.Query().Get("bucket"), rg)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	plays, err := s.playTimes(r, rg, f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	st, err := s.store.CollectorState(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	pts := analytics.Series(plays, rg, bucket, s.opts.Timezone, s.opts.Now(), st.FirstSuccessAt)
	writeJSON(w, http.StatusOK, airplayResponse{Range: s.rangeInfo(rg), Bucket: bucket, Points: pts})
}

type hourlyResponse struct {
	Range rangeInfo `json:"range"`
	Plays int       `json:"plays"`
	analytics.Distribution
}

func (s *Server) hourly(w http.ResponseWriter, r *http.Request) {
	rg, f, err := s.statsParams(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	plays, err := s.playTimes(r, rg, f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hourlyResponse{Range: s.rangeInfo(rg), Plays: len(plays), Distribution: analytics.Distribute(plays, s.opts.Timezone)})
}
