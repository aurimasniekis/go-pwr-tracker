package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

const gap = 5 * time.Minute

var t0 = time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func music(uid, artist, title string, start *time.Time) upstream.Item {
	return upstream.Item{UID: uid, Artist: artist, Title: title, Kind: upstream.KindMusic, StartAt: start}
}

func record(t *testing.T, s *Store, at time.Time, it upstream.Item) RecordResult {
	t.Helper()
	r, err := s.RecordObservation(context.Background(), Observation{ObservedAt: at, Item: it, FetchMode: "direct"}, gap)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func allPlays(t *testing.T, s *Store) []Play {
	t.Helper()
	ps, _, err := s.ListPlays(context.Background(), PlayFilter{IncludeSegments: true}, nil, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// oldest first is easier to assert
	for i, j := 0, len(ps)-1; i < j; i, j = i+1, j-1 {
		ps[i], ps[j] = ps[j], ps[i]
	}
	return ps
}

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.r.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mins(n int) time.Duration { return time.Duration(n) * time.Minute }

func TestOpenAppliesPragmasAndMigrations(t *testing.T) {
	s, _ := openTest(t)
	var mode string
	var fk int
	if err := s.w.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q (%v)", mode, err)
	}
	if err := s.r.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d (%v)", fk, err)
	}
	if v, err := s.SchemaVersion(context.Background()); err != nil || v != 1 {
		t.Fatalf("schema version = %d (%v)", v, err)
	}
	if _, err := s.r.Exec("INSERT INTO artists (name_key, name, created_at) VALUES ('x','x','x')"); err == nil {
		t.Fatal("read pool must be query_only")
	}
}

func TestMigrateUpgradeAndIdempotence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "m.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	record(t, s, t0, music("1", "A", "T", nil))

	// Simulate a future release adding a migration on top of existing data.
	upgraded := fstest.MapFS{}
	ms, _ := loadMigrations(migrationsFS)
	for _, m := range ms {
		upgraded["migrations/"+m.name] = &fstest.MapFile{Data: []byte(m.sql)}
	}
	upgraded["migrations/0002_test.sql"] = &fstest.MapFile{Data: []byte(`ALTER TABLE tracks ADD COLUMN note TEXT;`)}
	if err := migrate(ctx, s.w, upgraded); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if err := migrate(ctx, s.w, upgraded); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	var note *string
	if err := s.w.QueryRow("SELECT note FROM tracks").Scan(&note); err != nil {
		t.Fatalf("existing data after upgrade: %v", err)
	}
	s.Close()

	// The real binary now sees a schema newer than it knows and refuses.
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("expected refusal to open a database with a newer schema")
	}
}

func TestRepeatedPollsSameUIDAreOnePlay(t *testing.T) {
	s, _ := openTest(t)
	start := t0.Add(-30 * time.Second)
	for i := range 5 {
		record(t, s, t0.Add(mins(i)), music("100", "Leon Somov & Kaia", "Sustoja Laikas (Meile)", &start))
	}
	ps := allPlays(t, s)
	if len(ps) != 1 {
		t.Fatalf("want 1 play, got %d", len(ps))
	}
	p := ps[0]
	if p.ObservationCount != 5 || !p.StartAt.Equal(start) || !p.StartAuthoritative || p.Reason != ReasonUpstreamEvent || p.Uncertain {
		t.Fatalf("unexpected play %+v", p)
	}
	if !p.LastObservedAt.Equal(t0.Add(mins(4))) || !p.FirstObservedAt.Equal(t0) {
		t.Fatalf("observed range %v..%v", p.FirstObservedAt, p.LastObservedAt)
	}
	if count(t, s, "observations") != 5 {
		t.Fatal("each poll must be stored as an observation")
	}
}

func TestConsecutiveSameTrackWithDistinctUIDs(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, music("1", "A", "Song", nil))
	record(t, s, t0.Add(mins(1)), music("1", "A", "Song", nil))
	record(t, s, t0.Add(mins(2)), music("2", "A", "Song", nil))
	ps := allPlays(t, s)
	if len(ps) != 2 || ps[0].Track.ID != ps[1].Track.ID {
		t.Fatalf("want 2 plays of the same track, got %+v", ps)
	}
	if ps[0].StartAuthoritative {
		t.Fatal("start without upstream date must not be authoritative")
	}
}

func TestInferredABAWithoutUIDs(t *testing.T) {
	s, _ := openTest(t)
	seq := []string{"A", "A", "B", "B", "B", "A"}
	for i, title := range seq {
		record(t, s, t0.Add(mins(i)), music("", "Artist", title, nil))
	}
	ps := allPlays(t, s)
	if len(ps) != 3 {
		t.Fatalf("A→B→A must be 3 plays, got %d", len(ps))
	}
	want := []string{ReasonInitialObservation, ReasonObservedTransition, ReasonObservedTransition}
	for i, p := range ps {
		if p.Reason != want[i] {
			t.Errorf("play %d reason = %s, want %s", i, p.Reason, want[i])
		}
		if p.StartAuthoritative {
			t.Errorf("play %d: inferred start marked authoritative", i)
		}
	}
	if !ps[0].Uncertain || ps[1].Uncertain || ps[2].Uncertain {
		t.Errorf("only the initial observation should be uncertain: %v %v %v", ps[0].Uncertain, ps[1].Uncertain, ps[2].Uncertain)
	}
	if ps[1].ObservationCount != 3 {
		t.Errorf("B observation count = %d", ps[1].ObservationCount)
	}
}

func TestContinuityGapCreatesUncertainSegment(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, music("", "A", "Song", nil))
	record(t, s, t0.Add(mins(1)), music("", "A", "Song", nil))
	// Outage longer than the continuity gap; same track seen again.
	r := record(t, s, t0.Add(mins(20)), music("", "A", "Song", nil))
	if !r.NewPlay || r.Reason != ReasonContinuityGap {
		t.Fatalf("got %+v", r)
	}
	ps := allPlays(t, s)
	if len(ps) != 2 || !ps[1].Uncertain {
		t.Fatalf("want gap segment marked uncertain, got %+v", ps)
	}
	// Different track after a gap is also a gap segment, not a clean transition.
	r = record(t, s, t0.Add(mins(40)), music("", "A", "Other", nil))
	if r.Reason != ReasonContinuityGap {
		t.Fatalf("got %s", r.Reason)
	}
}

func TestUIDModeIgnoresGapsForIdentity(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, music("7", "A", "Long Mix", nil))
	r := record(t, s, t0.Add(mins(30)), music("7", "A", "Long Mix", nil))
	if r.NewPlay {
		t.Fatal("same upstream uid after a gap is still the same play")
	}
}

func TestUIDReusedForDifferentTrackFallsBack(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, music("9", "A", "One", nil))
	r := record(t, s, t0.Add(mins(1)), music("9", "B", "Two", nil))
	if !r.NewPlay || r.Reason != ReasonObservedTransition {
		t.Fatalf("got %+v", r)
	}
	if len(allPlays(t, s)) != 2 {
		t.Fatal("want 2 plays")
	}
}

func TestRestartDoesNotDuplicatePlays(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	record(t, s, t0, music("", "A", "NoUID", nil))
	record(t, s, t0.Add(mins(1)), music("50", "B", "WithUID", nil))
	s.Close()

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Same uid after restart (and a retry of the same poll) extends the play.
	record(t, s, t0.Add(mins(2)), music("50", "B", "WithUID", nil))
	record(t, s, t0.Add(mins(2)), music("50", "B", "WithUID", nil))
	if n := len(allPlays(t, s)); n != 2 {
		t.Fatalf("uid play duplicated across restart: %d plays", n)
	}

	// Fallback mode: restart within the continuity gap continues the play.
	record(t, s, t0.Add(mins(3)), music("", "C", "NoUID2", nil))
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := record(t, s, t0.Add(mins(4)), music("", "C", "NoUID2", nil))
	if r.NewPlay {
		t.Fatal("inferred play duplicated across restart")
	}
}

func TestRecordObservationIsTransactional(t *testing.T) {
	s, _ := openTest(t)
	if _, err := s.w.Exec(`CREATE TEMP TRIGGER fail_obs BEFORE INSERT ON observations
		BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := s.RecordObservation(context.Background(), Observation{ObservedAt: t0, Item: music("1", "A", "T", nil), FetchMode: "direct"}, gap)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, tbl := range []string{"artists", "tracks", "plays", "observations"} {
		if n := count(t, s, tbl); n != 0 {
			t.Errorf("%s has %d rows after rollback", tbl, n)
		}
	}
	st, _ := s.CollectorState(context.Background())
	if st.LastSuccessAt != nil || st.CurrentPlay != nil {
		t.Error("collector state changed despite rollback")
	}
}

func TestFailureKeepsLastSuccess(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	record(t, s, t0, music("1", "A", "T", nil))
	if err := s.RecordFailure(ctx, t0.Add(mins(1)), "direct", "upstream returned HTTP 503"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFailure(ctx, t0.Add(mins(3)), "direct", "timeout"); err != nil {
		t.Fatal(err)
	}
	st, err := s.CollectorState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.CurrentPlay == nil || st.CurrentPlay.Track.Title != "T" || !st.LastSuccessAt.Equal(t0) {
		t.Fatalf("last success lost: %+v", st)
	}
	if st.ConsecutiveFailures != 2 || st.LastError != "timeout" || !st.LastAttemptAt.Equal(t0.Add(mins(3))) {
		t.Fatalf("failure state: %+v", st)
	}
	record(t, s, t0.Add(mins(4)), music("1", "A", "T", nil))
	st, _ = s.CollectorState(ctx)
	if st.ConsecutiveFailures != 0 || st.LastError != "" {
		t.Fatalf("success must reset failures: %+v", st)
	}
}

func TestUnicodeIdentity(t *testing.T) {
	s, _ := openTest(t)
	r1 := record(t, s, t0, music("1", "Jurgis Didžiulis", "Ąžuolas", nil))
	r2 := record(t, s, t0.Add(mins(1)), music("2", "JURGIS DIDŽIULIS", "a\u0328z\u030cuolas", nil))
	r3 := record(t, s, t0.Add(mins(2)), music("3", "Jurgis Didžiulis", "Ąžuolas (Remix)", nil))
	if r1.TrackID != r2.TrackID {
		t.Error("case/normalization variants should be the same track")
	}
	if r1.TrackID == r3.TrackID {
		t.Error("remix must be a distinct track")
	}
	tr, err := s.Track(context.Background(), r1.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Title != "Ąžuolas" || tr.Artist.Name != "Jurgis Didžiulis" {
		t.Errorf("display values not preserved: %+v", tr)
	}
	found, _ := s.SearchTracks(context.Background(), "ąžuol", 0, 10, 0)
	if len(found) != 2 {
		t.Errorf("unicode case-insensitive search found %d", len(found))
	}
}

func TestSegmentsExcludedByDefault(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, upstream.Item{UID: "1", Title: "Eteryje - Informacija", Kind: upstream.KindSegment})
	record(t, s, t0.Add(mins(1)), music("2", "A", "T", nil))
	ps, _, _ := s.ListPlays(context.Background(), PlayFilter{}, nil, 10)
	if len(ps) != 1 || ps[0].Track.Kind != "music" {
		t.Fatalf("segments should be hidden by default: %+v", ps)
	}
	tot, _ := s.Totals(context.Background(), t0.Add(-time.Hour), t0.Add(time.Hour), StatsFilter{})
	if tot.Plays != 1 || tot.DistinctArtists != 1 {
		t.Fatalf("totals %+v", tot)
	}
}

func TestPaginationNoSkipsOrDuplicates(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	// 37 plays, several sharing identical start timestamps.
	for i := range 37 {
		start := t0.Add(time.Duration(i/3) * time.Minute)
		record(t, s, t0.Add(time.Duration(i)*time.Second), music(fmt.Sprint(i), "A", fmt.Sprint("T", i%4), &start))
	}
	seen := map[int64]bool{}
	var cur *Cursor
	var prev *Play
	pages := 0
	for {
		ps, more, err := s.ListPlays(ctx, PlayFilter{}, cur, 5)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, p := range ps {
			if seen[p.ID] {
				t.Fatalf("duplicate play %d", p.ID)
			}
			seen[p.ID] = true
			if prev != nil && (p.StartAt.After(prev.StartAt) || (p.StartAt.Equal(prev.StartAt) && p.ID >= prev.ID)) {
				t.Fatalf("ordering violated at %d", p.ID)
			}
			p := p
			prev = &p
		}
		if !more {
			break
		}
		last := ps[len(ps)-1]
		cur = &Cursor{StartAt: last.StartAt, ID: last.ID}
	}
	if len(seen) != 37 || pages != 8 {
		t.Fatalf("saw %d plays over %d pages", len(seen), pages)
	}
}

func TestRetentionPreservesPlays(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	old := t0.Add(-100 * 24 * time.Hour)
	_, err := s.RecordObservation(ctx, Observation{ObservedAt: old, Item: music("1", "A", "Old", nil), FetchMode: "direct", RawPayload: []byte(`{"x":1}`)}, gap)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordObservation(ctx, Observation{ObservedAt: t0.Add(-10 * 24 * time.Hour), Item: music("2", "A", "Mid", nil), FetchMode: "direct", RawPayload: []byte(`{"x":2}`)}, gap)
	if err != nil {
		t.Fatal(err)
	}
	record(t, s, t0, music("3", "A", "New", nil))

	res, err := s.Prune(ctx, t0, 90*24*time.Hour, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Observations != 1 || res.RawPayloads != 2 {
		t.Fatalf("prune result %+v", res)
	}
	if n := count(t, s, "observations"); n != 2 {
		t.Fatalf("observations left: %d", n)
	}
	if n := len(allPlays(t, s)); n != 3 {
		t.Fatalf("plays must survive observation pruning, have %d", n)
	}
	var raw *string
	_ = s.r.QueryRow("SELECT raw_payload FROM observations ORDER BY observed_at LIMIT 1").Scan(&raw)
	if raw != nil {
		t.Fatal("raw payload not cleared")
	}
}

func TestObservationGaps(t *testing.T) {
	s, _ := openTest(t)
	for _, m := range []int{0, 1, 2, 30, 31} {
		record(t, s, t0.Add(mins(m)), music("x", "A", "T", nil))
	}
	gaps, earliest, err := s.ObservationGaps(context.Background(), t0.Add(mins(1)), t0.Add(time.Hour), gap, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || !gaps[0].From.Equal(t0.Add(mins(2))) || !gaps[0].To.Equal(t0.Add(mins(30))) {
		t.Fatalf("gaps = %+v", gaps)
	}
	if earliest == nil || !earliest.Equal(t0) {
		t.Fatalf("earliest = %v", earliest)
	}
}

func TestTopTracksAndCounts(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	titles := []string{"X", "Y", "X", "Z", "X", "Y"}
	for i, ti := range titles {
		at := t0.Add(mins(i * 4))
		record(t, s, at, music(fmt.Sprint(i), "Art", ti, &at))
	}
	top, err := s.TopTracks(ctx, t0, t0.Add(time.Hour), StatsFilter{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0].Track.Title != "X" || top[0].Plays != 3 || top[1].Plays != 2 {
		t.Fatalf("top = %+v", top)
	}
	// Half-open range: a play exactly at `to` is excluded.
	tot, _ := s.Totals(ctx, t0, t0.Add(mins(20)), StatsFilter{})
	if tot.Plays != 5 {
		t.Fatalf("half-open totals = %d", tot.Plays)
	}
	counts, err := s.CountsBy(ctx, t0, t0.Add(time.Hour), false, []int64{top[0].Track.ID, 999})
	if err != nil || counts[top[0].Track.ID] != 3 || counts[999] != 0 {
		t.Fatalf("counts = %v %v", counts, err)
	}
	artists, _ := s.TopArtists(ctx, t0, t0.Add(time.Hour), 5)
	if len(artists) != 1 || artists[0].Plays != 6 || artists[0].DistinctTracks != 3 {
		t.Fatalf("artists = %+v", artists)
	}
}

func TestBackupTo(t *testing.T) {
	s, _ := openTest(t)
	record(t, s, t0, music("1", "A", "T", nil))
	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := s.BackupTo(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	b, err := Open(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if n := count(t, b, "plays"); n != 1 {
		t.Fatalf("backup has %d plays", n)
	}
}
