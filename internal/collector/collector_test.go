package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

type step struct {
	body string
	err  error
}

// scriptFetcher returns scripted responses in order, repeating the last.
type scriptFetcher struct {
	mu    sync.Mutex
	steps []step
	calls int
	block chan struct{} // if set, Fetch waits on it or ctx
}

func (f *scriptFetcher) Mode() string { return "direct" }

func (f *scriptFetcher) Fetch(ctx context.Context) ([]byte, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.steps[min(f.calls, len(f.steps)-1)]
	f.calls++
	return []byte(s.body), s.err
}

// fakeClock advances only when the test says so.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	waits  chan time.Duration
	fireCh chan time.Time
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, waits: make(chan time.Duration, 16), fireCh: make(chan time.Time)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.waits <- d
	return c.fireCh
}

// advance waits for the loop to request a timer, moves time forward by the
// requested duration and fires it.
func (c *fakeClock) advance(t *testing.T) time.Duration {
	t.Helper()
	select {
	case d := <-c.waits:
		c.mu.Lock()
		c.now = c.now.Add(d)
		now := c.now
		c.mu.Unlock()
		c.fireCh <- now
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("collector did not schedule a wait")
		return 0
	}
}

func (c *fakeClock) set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

var t0 = time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)

func song(uid, artist, title string) string {
	return fmt.Sprintf(`{"uid":%q,"artistName":%q,"songTitle":%q,"song":"","date":"2026-10-17 18:59:30"}`, uid, artist, title)
}

func opts() Options {
	loc, _ := time.LoadLocation("Europe/Vilnius")
	return Options{
		Interval: time.Minute, StartupDelay: 2 * time.Second, MaxBackoff: 15 * time.Minute,
		ContinuityGap: 5 * time.Minute, UpstreamTimezone: loc,
		ObservationRetention: 90 * 24 * time.Hour,
	}
}

func setup(t *testing.T, f upstream.Fetcher, clock Clock) (*Collector, *store.Store) {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if testing.Verbose() {
		log = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	c := New(f, s, clock, log, opts())
	c.jitter = func() float64 { return 0.5 } // multiplier exactly 1.0
	return c, s
}

func plays(t *testing.T, s *store.Store) []store.Play {
	t.Helper()
	ps, _, err := s.ListPlays(context.Background(), store.PlayFilter{IncludeSegments: true}, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func TestPollRepeatedSameTrackIsOnePlay(t *testing.T) {
	f := &scriptFetcher{steps: []step{{body: song("1", "A", "T")}}}
	clock := newFakeClock(t0)
	c, s := setup(t, f, clock)
	for i := range 4 {
		clock.set(t0.Add(time.Duration(i) * time.Minute))
		if d := c.Poll(context.Background()); d != time.Minute {
			t.Fatalf("success delay = %s", d)
		}
	}
	ps := plays(t, s)
	if len(ps) != 1 || ps[0].ObservationCount != 4 {
		t.Fatalf("plays = %+v", ps)
	}
	// 18:59:30 Vilnius = 15:59:30Z, accepted as authoritative upstream start.
	if !ps[0].StartAuthoritative || !ps[0].StartAt.Equal(t0.Add(-30*time.Second)) {
		t.Fatalf("start = %v (authoritative %v)", ps[0].StartAt, ps[0].StartAuthoritative)
	}
}

func TestBackoffSequenceAndRecovery(t *testing.T) {
	f := &scriptFetcher{steps: []step{
		{body: song("1", "A", "T")},
		{err: errors.New("connection refused")},
		{err: errors.New("connection refused")},
		{err: errors.New("connection refused")},
		{err: errors.New("connection refused")},
		{err: errors.New("connection refused")},
		{err: errors.New("connection refused")},
		{body: song("2", "B", "U")},
	}}
	clock := newFakeClock(t0)
	c, s := setup(t, f, clock)
	ctx := context.Background()

	var delays []time.Duration
	for range 8 {
		delays = append(delays, c.Poll(ctx))
	}
	want := []time.Duration{1, 1, 2, 4, 8, 15, 15, 1}
	for i, w := range want {
		if delays[i] != w*time.Minute {
			t.Errorf("delay[%d] = %s, want %dm", i, delays[i], w)
		}
	}
	st, _ := s.CollectorState(ctx)
	if st.ConsecutiveFailures != 0 || len(plays(t, s)) != 2 {
		t.Fatalf("not recovered: %+v", st)
	}
}

func TestBackoffJitterBounds(t *testing.T) {
	c := &Collector{opts: opts()}
	for _, j := range []float64{0, 0.999} {
		c.jitter = func() float64 { return j }
		c.failures = 3 // base 4m
		d := c.backoff(0)
		if d < time.Duration(float64(4*time.Minute)*0.8) || d > time.Duration(float64(4*time.Minute)*1.2) {
			t.Errorf("jitter %v: %s outside ±20%%", j, d)
		}
		c.failures = 1
		if d := c.backoff(0); d < time.Minute {
			t.Errorf("first failure retry %s faster than the poll interval", d)
		}
		c.failures = 50
		if d := c.backoff(0); d > 15*time.Minute {
			t.Errorf("backoff %s exceeds cap", d)
		}
	}
}

func TestRetryAfterIsHonoredAndBounded(t *testing.T) {
	f := &scriptFetcher{steps: []step{
		{err: &upstream.StatusError{StatusCode: 429, RetryAfter: 7 * time.Minute}},
		{err: &upstream.StatusError{StatusCode: 503, RetryAfter: 48 * time.Hour}},
	}}
	c, s := setup(t, f, newFakeClock(t0))
	if d := c.Poll(context.Background()); d != 7*time.Minute {
		t.Fatalf("Retry-After 7m → %s", d)
	}
	if d := c.Poll(context.Background()); d != time.Hour {
		t.Fatalf("Retry-After 48h → %s, want capped at 1h", d)
	}
	st, _ := s.CollectorState(context.Background())
	if st.LastError != "upstream_status: upstream returned HTTP 503" {
		t.Fatalf("last error %q", st.LastError)
	}
}

func TestInvalidPayloadKeepsLastSuccess(t *testing.T) {
	f := &scriptFetcher{steps: []step{
		{body: song("1", "A", "T")},
		{body: "<html>Just a moment...</html>"},
		{body: `{"uid":"2"}`},
	}}
	clock := newFakeClock(t0)
	c, s := setup(t, f, clock)
	ctx := context.Background()
	c.Poll(ctx)
	clock.set(t0.Add(time.Minute))
	c.Poll(ctx)
	clock.set(t0.Add(2 * time.Minute))
	c.Poll(ctx)
	st, _ := s.CollectorState(ctx)
	if st.CurrentPlay == nil || st.CurrentPlay.Track.Title != "T" || !st.LastSuccessAt.Equal(t0) {
		t.Fatalf("last success lost: %+v", st)
	}
	if st.ConsecutiveFailures != 2 || !st.LastAttemptAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("state %+v", st)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]error{
		"solver":          &upstream.SolverError{Message: "x"},
		"upstream_status": fmt.Errorf("wrapped: %w", &upstream.StatusError{StatusCode: 500}),
		"invalid_payload": fmt.Errorf("%w: bad", upstream.ErrInvalidPayload),
		"too_large":       upstream.ErrTooLarge,
		"timeout":         context.DeadlineExceeded,
		"error":           errors.New("other"),
	}
	for want, err := range cases {
		if got := classify(err); got != want {
			t.Errorf("classify(%v) = %s, want %s", err, got, want)
		}
	}
}

func TestRunLoopSchedulingAndShutdown(t *testing.T) {
	f := &scriptFetcher{steps: []step{{body: song("1", "A", "T")}, {err: errors.New("boom")}}}
	clock := newFakeClock(t0)
	c, s := setup(t, f, clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()

	if d := clock.advance(t); d != 2*time.Second {
		t.Fatalf("startup delay = %s", d)
	}
	if d := clock.advance(t); d != time.Minute {
		t.Fatalf("interval after success = %s", d)
	}
	if d := clock.advance(t); d != time.Minute {
		t.Fatalf("first failure retry = %s", d)
	}
	if d := <-clock.waits; d != 2*time.Minute {
		t.Fatalf("second failure retry = %s", d)
	}
	if c.NextAttemptAt().IsZero() {
		t.Fatal("next attempt not exposed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
	if f.calls != 3 || len(plays(t, s)) != 1 {
		t.Fatalf("calls=%d", f.calls)
	}
}

func TestShutdownDuringFetchRecordsNoFailure(t *testing.T) {
	f := &scriptFetcher{steps: []step{{body: song("1", "A", "T")}}, block: make(chan struct{})}
	c, s := setup(t, f, newFakeClock(t0))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	c.Poll(ctx)
	st, _ := s.CollectorState(context.Background())
	if st.ConsecutiveFailures != 0 || st.LastAttemptAt != nil {
		t.Fatalf("shutdown recorded as failure: %+v", st)
	}
}

func TestPruneRunsHourly(t *testing.T) {
	f := &scriptFetcher{steps: []step{{body: song("1", "A", "T")}}}
	clock := newFakeClock(t0.Add(-100 * 24 * time.Hour))
	c, s := setup(t, f, clock)
	ctx := context.Background()
	c.Poll(ctx) // ancient observation, prune runs (nothing old yet)
	clock.set(t0)
	c.Poll(ctx) // >1h later: prune deletes the ancient observation
	ps := plays(t, s)
	if len(ps) != 1 || ps[0].ObservationCount != 2 {
		t.Fatalf("play history must survive pruning: %+v", ps)
	}
	gaps, earliest, _ := s.ObservationGaps(ctx, t0.Add(-time.Hour), t0.Add(time.Hour), time.Minute, 10)
	if earliest == nil || !earliest.Equal(t0) || len(gaps) != 0 {
		t.Fatalf("old observation not pruned: earliest=%v", earliest)
	}
}
