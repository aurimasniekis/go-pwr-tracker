// Package config loads and validates runtime configuration from environment
// variables. Every variable is documented in .env.example and README.md.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Fetch modes. There is deliberately no automatic fallback between them.
const (
	FetchModeDirect       = "direct"
	FetchModeFlareSolverr = "flaresolverr"
)

// DefaultUpstreamURL is the Power Hit Radio "last song" endpoint.
const DefaultUpstreamURL = "https://powerhitradio.tv3.lt/Pwr/lastSong/"

type Config struct {
	ListenAddr   string
	DatabasePath string
	LogLevel     slog.Level

	UpstreamURL      string
	UpstreamTimezone *time.Location
	UserAgent        string
	PollInterval     time.Duration
	StartupDelay     time.Duration
	HTTPTimeout      time.Duration
	MaxBackoff       time.Duration
	MaxResponseBytes int64
	StaleAfter       time.Duration
	ContinuityGap    time.Duration

	FetchMode           string
	FlareSolverrURL     string
	FlareSolverrTimeout time.Duration

	AnalyticsTimezone *time.Location

	ObservationRetention time.Duration
	StoreRawPayloads     bool
	RawPayloadRetention  time.Duration

	CollectorEnabled bool
}

// Load reads configuration from the given lookup function (normally
// os.LookupEnv). All problems are collected and returned together so an
// operator can fix them in one pass.
func Load(lookup func(string) (string, bool)) (*Config, error) {
	p := parser{lookup: lookup}
	c := &Config{
		ListenAddr:   p.str("LISTEN_ADDR", ":8080"),
		DatabasePath: p.str("DATABASE_PATH", "/data/pwr-tracker.db"),
		LogLevel:     p.level("LOG_LEVEL", slog.LevelInfo),

		UpstreamURL:      p.str("UPSTREAM_URL", DefaultUpstreamURL),
		UpstreamTimezone: p.location("UPSTREAM_TIMEZONE", "Europe/Vilnius"),
		UserAgent:        p.str("USER_AGENT", ""),
		PollInterval:     p.duration("POLL_INTERVAL", 60*time.Second),
		StartupDelay:     p.duration("STARTUP_DELAY", 2*time.Second),
		HTTPTimeout:      p.duration("HTTP_TIMEOUT", 15*time.Second),
		MaxBackoff:       p.duration("MAX_BACKOFF", 15*time.Minute),
		MaxResponseBytes: p.int64("MAX_RESPONSE_BYTES", 64*1024),
		StaleAfter:       p.duration("STALE_AFTER", 0),
		ContinuityGap:    p.duration("CONTINUITY_GAP", 0),

		FetchMode:           strings.ToLower(p.str("FETCH_MODE", FetchModeDirect)),
		FlareSolverrURL:     p.str("FLARESOLVERR_URL", ""),
		FlareSolverrTimeout: p.duration("FLARESOLVERR_TIMEOUT", 60*time.Second),

		AnalyticsTimezone: p.location("ANALYTICS_TIMEZONE", "Europe/Vilnius"),

		ObservationRetention: p.duration("OBSERVATION_RETENTION", 90*24*time.Hour),
		StoreRawPayloads:     p.bool("STORE_RAW_PAYLOADS", false),
		RawPayloadRetention:  p.duration("RAW_PAYLOAD_RETENTION", 7*24*time.Hour),

		CollectorEnabled: p.bool("COLLECTOR_ENABLED", true),
	}
	if c.StaleAfter == 0 {
		c.StaleAfter = 3 * c.PollInterval
	}
	if c.ContinuityGap == 0 {
		c.ContinuityGap = 5 * c.PollInterval
	}
	c.validate(&p)
	if len(p.errs) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %w", errors.Join(p.errs...))
	}
	return c, nil
}

// FromEnv loads configuration from the process environment.
func FromEnv() (*Config, error) { return Load(os.LookupEnv) }

func (c *Config) validate(p *parser) {
	if c.DatabasePath == "" {
		p.fail("DATABASE_PATH must not be empty")
	}
	if u, err := url.Parse(c.UpstreamURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.fail("UPSTREAM_URL must be an absolute http(s) URL, got %q", c.UpstreamURL)
	}
	if c.PollInterval < time.Second {
		p.fail("POLL_INTERVAL must be at least 1s, got %s", c.PollInterval)
	}
	if c.HTTPTimeout <= 0 || c.HTTPTimeout > 5*time.Minute {
		p.fail("HTTP_TIMEOUT must be between 1ns and 5m, got %s", c.HTTPTimeout)
	}
	if c.MaxBackoff < c.PollInterval {
		p.fail("MAX_BACKOFF (%s) must be >= POLL_INTERVAL (%s)", c.MaxBackoff, c.PollInterval)
	}
	if c.MaxResponseBytes < 1024 || c.MaxResponseBytes > 10<<20 {
		p.fail("MAX_RESPONSE_BYTES must be between 1024 and 10485760, got %d", c.MaxResponseBytes)
	}
	if c.StartupDelay < 0 {
		p.fail("STARTUP_DELAY must not be negative")
	}
	if c.StaleAfter < c.PollInterval {
		p.fail("STALE_AFTER (%s) must be >= POLL_INTERVAL (%s)", c.StaleAfter, c.PollInterval)
	}
	if c.ContinuityGap < c.PollInterval {
		p.fail("CONTINUITY_GAP (%s) must be >= POLL_INTERVAL (%s)", c.ContinuityGap, c.PollInterval)
	}
	switch c.FetchMode {
	case FetchModeDirect:
	case FetchModeFlareSolverr:
		if u, err := url.Parse(c.FlareSolverrURL); c.FlareSolverrURL == "" || err != nil || u.Host == "" {
			p.fail("FLARESOLVERR_URL must be set to the FlareSolverr base URL (e.g. http://flaresolverr:8191) when FETCH_MODE=flaresolverr")
		}
		if c.FlareSolverrTimeout < time.Second {
			p.fail("FLARESOLVERR_TIMEOUT must be at least 1s, got %s", c.FlareSolverrTimeout)
		}
	default:
		p.fail("FETCH_MODE must be %q or %q, got %q", FetchModeDirect, FetchModeFlareSolverr, c.FetchMode)
	}
	if c.ObservationRetention < 0 {
		p.fail("OBSERVATION_RETENTION must not be negative (0 keeps observations forever)")
	}
	if c.RawPayloadRetention < 0 {
		p.fail("RAW_PAYLOAD_RETENTION must not be negative")
	}
}

type parser struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (p *parser) fail(format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf(format, args...))
}

func (p *parser) raw(key string) (string, bool) {
	v, ok := p.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (p *parser) str(key, def string) string {
	if v, ok := p.raw(key); ok {
		return v
	}
	return def
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.fail("%s: %q is not a duration (examples: 60s, 5m, 2160h)", key, v)
		return def
	}
	return d
}

func (p *parser) int64(key string, def int64) int64 {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		p.fail("%s: %q is not an integer", key, v)
		return def
	}
	return n
}

func (p *parser) bool(key string, def bool) bool {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.fail("%s: %q is not a boolean (use true or false)", key, v)
		return def
	}
	return b
}

func (p *parser) level(key string, def slog.Level) slog.Level {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		p.fail("%s: %q is not a log level (debug, info, warn, error)", key, v)
		return def
	}
	return l
}

func (p *parser) location(key, def string) *time.Location {
	name := p.str(key, def)
	loc, err := time.LoadLocation(name)
	if err != nil {
		p.fail("%s: unknown IANA time zone %q", key, name)
		return time.UTC
	}
	return loc
}
