// Package collector runs the polling loop: fetch → parse → persist, with
// bounded exponential backoff on failure. A single goroutine performs every
// poll, so polls never overlap.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

// Clock abstracts time so the loop can be tested without sleeping.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// RealClock is the wall clock.
var RealClock Clock = realClock{}

// Store is the subset of *store.Store the collector needs.
type Store interface {
	RecordObservation(ctx context.Context, o store.Observation, continuityGap time.Duration) (store.RecordResult, error)
	RecordFailure(ctx context.Context, at time.Time, fetchMode, msg string) error
	Prune(ctx context.Context, now time.Time, observationRetention, rawRetention time.Duration) (store.PruneResult, error)
}

type Options struct {
	Interval             time.Duration
	StartupDelay         time.Duration
	MaxBackoff           time.Duration
	ContinuityGap        time.Duration
	UpstreamTimezone     *time.Location
	StoreRawPayloads     bool
	ObservationRetention time.Duration
	RawPayloadRetention  time.Duration
}

// Retry-After values are honored up to this bound so a misconfigured
// upstream cannot park the collector for days.
const maxRetryAfter = time.Hour

const pruneEvery = time.Hour

type Collector struct {
	fetcher upstream.Fetcher
	store   Store
	clock   Clock
	log     *slog.Logger
	opts    Options
	// jitter returns a value in [0, 1); injectable for tests.
	jitter func() float64

	failures  int
	lastPrune time.Time
	nextAt    atomic.Int64 // unix nanos of the next scheduled attempt
}

func New(f upstream.Fetcher, s Store, clock Clock, log *slog.Logger, opts Options) *Collector {
	if opts.UpstreamTimezone == nil {
		opts.UpstreamTimezone = time.UTC
	}
	return &Collector{fetcher: f, store: s, clock: clock, log: log, opts: opts, jitter: rand.Float64}
}

// NextAttemptAt reports when the next poll is scheduled (zero if unknown).
func (c *Collector) NextAttemptAt() time.Time {
	n := c.nextAt.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// Run polls until ctx is cancelled. The first attempt happens after
// StartupDelay.
func (c *Collector) Run(ctx context.Context) {
	delay := c.opts.StartupDelay
	c.log.Info("collector started", "mode", c.fetcher.Mode(), "interval", c.opts.Interval.String())
	for {
		c.nextAt.Store(c.clock.Now().Add(delay).UnixNano())
		select {
		case <-ctx.Done():
			c.log.Info("collector stopped")
			return
		case <-c.clock.After(delay):
		}
		delay = c.Poll(ctx)
	}
}

// Poll performs one collection attempt and returns the delay until the
// next one.
func (c *Collector) Poll(ctx context.Context) time.Duration {
	started := c.clock.Now()
	body, err := c.fetcher.Fetch(ctx)
	if ctx.Err() != nil {
		return 0 // shutting down; do not record a failure
	}
	if err != nil {
		return c.fail(ctx, started, err)
	}
	item, err := upstream.Parse(body, c.opts.UpstreamTimezone, started)
	if err != nil {
		return c.fail(ctx, started, err)
	}
	obs := store.Observation{ObservedAt: started, Item: item, FetchMode: c.fetcher.Mode()}
	if c.opts.StoreRawPayloads {
		obs.RawPayload = body
	}
	res, err := c.store.RecordObservation(ctx, obs, c.opts.ContinuityGap)
	if err != nil {
		return c.fail(ctx, started, err)
	}
	if c.failures > 0 {
		c.log.Info("collection recovered", "after_failures", c.failures)
	}
	c.failures = 0
	attrs := []any{"track_id", res.TrackID, "play_id", res.PlayID, "uid", item.UID, "kind", item.Kind}
	if res.NewPlay {
		c.log.Info("new play", append(attrs, "reason", res.Reason, "artist", item.Artist, "title", item.Title)...)
	} else {
		c.log.Debug("observation", attrs...)
	}
	c.maybePrune(ctx, started)
	return c.opts.Interval
}

func (c *Collector) fail(ctx context.Context, at time.Time, err error) time.Duration {
	c.failures++
	kind := classify(err)
	delay := c.backoff(retryAfter(err))
	c.log.Warn("collection failed", "error", err.Error(), "error_kind", kind,
		"consecutive_failures", c.failures, "retry_in", delay.Round(time.Second).String())
	if rerr := c.store.RecordFailure(ctx, at, c.fetcher.Mode(), kind+": "+err.Error()); rerr != nil && ctx.Err() == nil {
		c.log.Error("record failure", "error", rerr.Error())
	}
	return delay
}

// backoff returns the delay after c.failures consecutive failures:
// Interval × 2^(failures-1), ±20% jitter, capped at MaxBackoff, and never
// shorter than the regular interval or a Retry-After hint.
func (c *Collector) backoff(retryAfter time.Duration) time.Duration {
	exp := math.Min(float64(c.failures-1), 16)
	d := time.Duration(float64(c.opts.Interval) * math.Pow(2, exp))
	if d > c.opts.MaxBackoff || d <= 0 {
		d = c.opts.MaxBackoff
	}
	d = time.Duration(float64(d) * (0.8 + 0.4*c.jitter()))
	d = min(d, c.opts.MaxBackoff)
	d = max(d, c.opts.Interval)
	if retryAfter > d {
		d = min(retryAfter, maxRetryAfter)
	}
	return d
}

func retryAfter(err error) time.Duration {
	var se *upstream.StatusError
	if errors.As(err, &se) {
		return se.RetryAfter
	}
	return 0
}

func classify(err error) string {
	var se *upstream.StatusError
	var sol *upstream.SolverError
	var ne net.Error
	switch {
	case errors.As(err, &sol):
		return "solver"
	case errors.As(err, &se):
		return "upstream_status"
	case errors.Is(err, upstream.ErrInvalidPayload):
		return "invalid_payload"
	case errors.Is(err, upstream.ErrTooLarge):
		return "too_large"
	case errors.As(err, &ne) && ne.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &ne):
		return "network"
	default:
		return "error"
	}
}

func (c *Collector) maybePrune(ctx context.Context, now time.Time) {
	if !c.lastPrune.IsZero() && now.Sub(c.lastPrune) < pruneEvery {
		return
	}
	c.lastPrune = now
	if c.opts.ObservationRetention == 0 && c.opts.RawPayloadRetention == 0 {
		return
	}
	res, err := c.store.Prune(ctx, now, c.opts.ObservationRetention, c.opts.RawPayloadRetention)
	if err != nil {
		c.log.Error("retention prune failed", "error", err.Error())
		return
	}
	if res.Observations > 0 || res.RawPayloads > 0 {
		c.log.Info("retention prune", "observations_deleted", res.Observations, "raw_payloads_cleared", res.RawPayloads)
	}
}
