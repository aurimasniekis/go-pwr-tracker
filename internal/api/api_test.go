package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

var t0 = time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)

type env struct {
	t     *testing.T
	store *store.Store
	srv   *httptest.Server
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	loc, _ := time.LoadLocation("Europe/Vilnius")
	e := &env{t: t, store: s, now: t0}
	ui := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>pwr</title>")},
		"assets/app.js": {Data: []byte(strings.Repeat("console.log(1);", 200))},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}
	a := New(s, Options{
		Timezone: loc, PollInterval: time.Minute, StaleAfter: 3 * time.Minute, ContinuityGap: 5 * time.Minute,
		FetchMode: "direct", CollectorEnabled: true, Now: func() time.Time { return e.now }, UI: ui,
	})
	e.srv = httptest.NewServer(a.Handler())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) record(at time.Time, uid, artist, title string) {
	e.t.Helper()
	start := at
	_, err := e.store.RecordObservation(context.Background(), store.Observation{
		ObservedAt: at, FetchMode: "direct",
		Item: upstream.Item{UID: uid, Artist: artist, Title: title, Kind: upstream.KindMusic, StartAt: &start},
	}, 5*time.Minute)
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) get(path string, out any) int {
	e.t.Helper()
	resp, err := http.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			e.t.Fatalf("GET %s: decode %q: %v", path, body, err)
		}
	}
	return resp.StatusCode
}

func TestHealthAndReadiness(t *testing.T) {
	e := newEnv(t)
	if c := e.get("/healthz", nil); c != 200 {
		t.Fatalf("healthz %d", c)
	}
	var r map[string]string
	if c := e.get("/readyz", &r); c != 200 || r["status"] != "ready" {
		t.Fatalf("readyz %d %v", c, r)
	}
	// Readiness ignores upstream state.
	_ = e.store.RecordFailure(context.Background(), t0, "direct", "down")
	if c := e.get("/readyz", nil); c != 200 {
		t.Fatalf("readyz during upstream outage %d", c)
	}
}

func TestNowPlayingStates(t *testing.T) {
	e := newEnv(t)
	var np nowPlayingResponse
	e.get("/api/v1/now-playing", &np)
	if np.Play != nil || np.Collector.State != "starting" || !np.Collector.Stale || np.Timezone != "Europe/Vilnius" {
		t.Fatalf("empty: %+v", np)
	}

	e.record(t0, "1", "A", "Song")
	e.get("/api/v1/now-playing", &np)
	if np.Play == nil || np.Play.Track.Title != "Song" || np.Collector.State != "ok" || np.Collector.Stale {
		t.Fatalf("fresh: %+v", np)
	}

	_ = e.store.RecordFailure(context.Background(), t0.Add(time.Minute), "direct", "upstream_status: upstream returned HTTP 503")
	e.now = t0.Add(90 * time.Second)
	e.get("/api/v1/now-playing", &np)
	if np.Collector.State != "error" || np.Collector.Stale || np.Collector.LastError == "" || np.Play == nil {
		t.Fatalf("error but fresh: %+v", np.Collector)
	}

	e.now = t0.Add(10 * time.Minute)
	e.get("/api/v1/now-playing", &np)
	if np.Collector.State != "stale" || !np.Collector.Stale || np.Play == nil || np.Play.Track.Title != "Song" {
		t.Fatalf("stale must keep last value: %+v", np)
	}
}

func TestPlaysPaginationThroughAPI(t *testing.T) {
	e := newEnv(t)
	for i := range 23 {
		e.record(t0.Add(time.Duration(i/2)*time.Minute), fmt.Sprint(i), "A", fmt.Sprint("T", i))
	}
	seen := map[int64]bool{}
	path := "/api/v1/plays?limit=4"
	for path != "" {
		var resp playsResponse
		if c := e.get(path, &resp); c != 200 {
			t.Fatalf("status %d", c)
		}
		for _, p := range resp.Items {
			if seen[p.ID] {
				t.Fatalf("duplicate %d", p.ID)
			}
			seen[p.ID] = true
		}
		path = ""
		if resp.NextCursor != nil {
			path = "/api/v1/plays?limit=4&cursor=" + url.QueryEscape(*resp.NextCursor)
		}
	}
	if len(seen) != 23 {
		t.Fatalf("saw %d", len(seen))
	}
}

func TestPlaysFilters(t *testing.T) {
	e := newEnv(t)
	e.record(t0, "1", "Alpha", "One")
	e.record(t0.Add(time.Hour), "2", "Beta", "Two")
	e.record(t0.Add(2*time.Hour), "3", "Alpha", "Three")

	var resp playsResponse
	e.get("/api/v1/plays?q=alph", &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("q filter: %d", len(resp.Items))
	}
	from := url.QueryEscape(t0.Add(30 * time.Minute).Format(time.RFC3339))
	to := url.QueryEscape(t0.Add(2 * time.Hour).Format(time.RFC3339))
	e.get("/api/v1/plays?from="+from+"&to="+to, &resp)
	if len(resp.Items) != 1 || resp.Items[0].Track.Title != "Two" {
		t.Fatalf("half-open range: %+v", resp.Items)
	}
	artistID := resp.Items[0].Track.Artist.ID
	e.get(fmt.Sprintf("/api/v1/plays?artist_id=%d", artistID), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("artist filter: %d", len(resp.Items))
	}
}

func TestValidationErrors(t *testing.T) {
	e := newEnv(t)
	cases := map[string]string{
		"/api/v1/plays?limit=0":                               "invalid_parameter",
		"/api/v1/plays?limit=1000":                            "invalid_parameter",
		"/api/v1/plays?cursor=!!!":                            "invalid_parameter",
		"/api/v1/plays?cursor=Zm9v":                           "invalid_parameter",
		"/api/v1/plays?from=tomorrow":                         "invalid_range",
		"/api/v1/plays?track_id=-1":                           "invalid_parameter",
		"/api/v1/stats/summary?from=2026-10-10&to=2026-10-01": "invalid_range",
		"/api/v1/stats/summary?from=2020-01-01&to=2026-01-01": "invalid_range",
		"/api/v1/stats/airplay?bucket=minute":                 "invalid_range",
		"/api/v1/tracks?q=" + strings.Repeat("x", 101):        "invalid_parameter",
	}
	for path, code := range cases {
		var er apiError
		if c := e.get(path, &er); c != 400 || er.Error.Code != code {
			t.Errorf("%s: %d %+v", path, c, er)
		}
	}
	var er apiError
	if c := e.get("/api/v1/tracks/999", &er); c != 404 || er.Error.Code != "not_found" {
		t.Errorf("missing track: %d %+v", c, er)
	}
}

func TestUnknownAPIRoutesAreJSON404NotSPA(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/api/v1/nope", "/api/v2/plays", "/api/"} {
		resp, err := http.Get(e.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Errorf("%s: %d %s", p, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	resp, _ := http.Post(e.srv.URL+"/api/v1/plays", "application/json", nil)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("POST must not be accepted: %d", resp.StatusCode)
	}
}

func TestSPAFallbackAndAssets(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/", "/activity", "/tracks/12?from=2026-10-01"} {
		resp, err := http.Get(e.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(b), "<title>pwr</title>") || resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: %d %q", p, resp.StatusCode, b)
		}
	}
	resp, _ := http.Get(e.srv.URL + "/assets/missing.js")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("missing asset: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, _ = http.DefaultTransport.RoundTrip(req)
	resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("asset headers: %v", resp.Header)
	}
}

func TestSummaryComparisonStates(t *testing.T) {
	e := newEnv(t)
	loc, _ := time.LoadLocation("Europe/Vilnius")
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, loc) }

	// Collection starts Oct 10. Range Oct 12–13 compares with Oct 11 (covered, 0 plays).
	e.record(day(10), "0", "A", "X")
	e.record(day(12), "1", "A", "X")
	e.record(day(12).Add(time.Hour), "2", "B", "Y")
	e.now = day(20)

	var s summaryResponse
	if c := e.get("/api/v1/stats/summary?from=2026-10-12&to=2026-10-13", &s); c != 200 {
		t.Fatalf("status %d", c)
	}
	if s.Totals.Plays != 2 || s.Totals.DistinctArtists != 2 || s.Comparison == nil || s.Comparison.Change.State != "new" || s.Comparison.Change.Percent != nil {
		t.Fatalf("new: %+v %+v", s.Totals, s.Comparison)
	}
	if !s.Comparison.Previous.From.Equal(time.Date(2026, 10, 11, 0, 0, 0, 0, loc)) {
		t.Fatalf("previous period %+v", s.Comparison.Previous)
	}
	if s.Range.Timezone != "Europe/Vilnius" || !strings.HasSuffix(s.Range.From.Format(time.RFC3339), "+03:00") {
		t.Fatalf("range %+v", s.Range)
	}
	// Previous period before collection started → unavailable.
	e.get("/api/v1/stats/summary?from=2026-10-10&to=2026-10-12", &s)
	if s.Comparison.Change.State != "unavailable" {
		t.Fatalf("unavailable: %+v", s.Comparison)
	}
	// Nothing in either period → none.
	e.get("/api/v1/stats/summary?from=2026-10-15&to=2026-10-16", &s)
	if s.Comparison.Change.State != "none" || s.Totals.Plays != 0 {
		t.Fatalf("none: %+v", s.Comparison)
	}
	// The gap between Oct 10 and Oct 12 observations is reported.
	e.get("/api/v1/stats/summary?from=2026-10-11&to=2026-10-13", &s)
	if len(s.Coverage.Gaps) == 0 || !s.Coverage.Partial {
		t.Fatalf("coverage %+v", s.Coverage)
	}
}

func TestTopTracksAirplayHourly(t *testing.T) {
	e := newEnv(t)
	loc, _ := time.LoadLocation("Europe/Vilnius")
	base := time.Date(2026, 10, 16, 8, 0, 0, 0, loc)
	e.record(base.Add(-48*time.Hour), "z", "C", "Z") // collection starts
	e.record(base.Add(-24*time.Hour), "0", "A", "X") // previous day
	e.record(base, "1", "A", "X")
	e.record(base.Add(time.Hour), "2", "A", "X")
	e.record(base.Add(2*time.Hour), "3", "B", "Y")
	e.now = time.Date(2026, 10, 16, 20, 0, 0, 0, loc)

	var top rankingResponse[rankedTrack]
	e.get("/api/v1/stats/top-tracks?from=2026-10-16&to=2026-10-17", &top)
	if len(top.Items) != 2 || top.Items[0].Plays != 2 || top.Items[0].Change == nil {
		t.Fatalf("top %+v", top)
	}
	// Current period is clipped to now (20h); previous = the 20h before.
	if top.Items[0].Change.State != "up" || *top.Items[0].Change.Percent != 100 || top.Items[1].Change.State != "new" {
		t.Fatalf("changes %+v %+v", top.Items[0].Change, top.Items[1].Change)
	}

	var ap airplayResponse
	e.get("/api/v1/stats/airplay?from=2026-10-16&to=2026-10-17&bucket=hour", &ap)
	if len(ap.Points) != 24 || ap.Points[8].Plays != 1 || ap.Points[9].Plays != 1 || ap.Points[10].Plays != 1 {
		t.Fatalf("airplay %+v", ap.Points[7:11])
	}
	if ap.Points[8].Partial || !ap.Points[21].Partial {
		t.Fatalf("partial flags wrong: 08h=%v 21h=%v", ap.Points[8].Partial, ap.Points[21].Partial)
	}

	var h hourlyResponse
	e.get(fmt.Sprintf("/api/v1/stats/hourly?from=2026-10-15&to=2026-10-17&track_id=%d", top.Items[0].Track.ID), &h)
	if h.Plays != 3 || h.Hours[8] != 2 || h.Hours[9] != 1 || h.WeekdayHours[4][8] != 1 { // Oct 16 2026 is Friday
		t.Fatalf("hourly %+v", h)
	}

	var tracks listResponse[store.TrackSummary]
	e.get("/api/v1/tracks?q=x", &tracks)
	if len(tracks.Items) != 1 || tracks.Items[0].TotalPlays != 3 {
		t.Fatalf("tracks %+v", tracks)
	}
	var artist store.ArtistSummary
	if c := e.get(fmt.Sprintf("/api/v1/artists/%d", tracks.Items[0].Artist.ID), &artist); c != 200 || artist.TotalPlays != 3 {
		t.Fatalf("artist %d %+v", c, artist)
	}
}

func TestStatusEndpoint(t *testing.T) {
	e := newEnv(t)
	var st statusResponse
	if c := e.get("/api/v1/status", &st); c != 200 || st.SchemaVersion != 1 || st.Collector.PollIntervalSeconds != 60 || st.ContinuityGapSeconds != 300 {
		t.Fatalf("%d %+v", c, st)
	}
}
