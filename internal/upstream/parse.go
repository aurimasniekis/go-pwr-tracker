// Package upstream fetches and parses the Power Hit Radio "last song"
// endpoint. The endpoint is undocumented; see docs/upstream.md for the
// observed contract this parser relies on.
package upstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aurimasniekis/go-pwr-tracker/internal/normalize"
)

// ErrInvalidPayload marks responses that were received but cannot be
// accepted as an observation.
var ErrInvalidPayload = errors.New("invalid upstream payload")

// Kind distinguishes music from station segments (news, information
// blocks, jingles) that the endpoint reports without an artist.
type Kind string

const (
	KindMusic   Kind = "music"
	KindSegment Kind = "segment"
)

const maxFieldLen = 512

// upstreamDateLayout is the observed layout of the "date" field, a local
// wall-clock time without an offset.
const upstreamDateLayout = "2006-01-02 15:04:05"

// Item is one parsed "now playing" response.
type Item struct {
	// UID is the upstream item identifier ("uid"). Observed to be a
	// sequential integer that stays constant while an item is on air and
	// changes for each new item, including back-to-back repeats.
	UID string
	// Artist and Title are display values (whitespace-collapsed, NFC).
	// Artist is empty for station segments.
	Artist string
	Title  string
	Kind   Kind
	// RawDate is the unparsed upstream "date" string.
	RawDate string
	// StartAt is the upstream start time converted to UTC, or nil when the
	// field is missing, unparsable, or implausible relative to ObservedAt.
	StartAt *time.Time
	// Program is the DJ/program name if present. Informational only.
	Program string
}

type payload struct {
	UID        json.RawMessage `json:"uid"`
	ArtistName *string         `json:"artistName"`
	SongTitle  *string         `json:"songTitle"`
	Song       *string         `json:"song"`
	Date       *string         `json:"date"`
	DJ         json.RawMessage `json:"dj"`
}

type djPayload struct {
	Program string `json:"program"`
	DJName  string `json:"djname"`
}

// Parse validates and parses a response body. loc is the timezone the
// upstream "date" field is expressed in; observedAt is when the response was
// received and is used to resolve DST ambiguity and reject implausible
// start times.
func Parse(body []byte, loc *time.Location, observedAt time.Time) (Item, error) {
	body = bytes.TrimSpace(body)
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")) // UTF-8 BOM
	if len(body) == 0 {
		return Item{}, fmt.Errorf("%w: empty body", ErrInvalidPayload)
	}
	if !utf8.Valid(body) {
		return Item{}, fmt.Errorf("%w: body is not valid UTF-8", ErrInvalidPayload)
	}
	if body[0] != '{' {
		return Item{}, fmt.Errorf("%w: expected a JSON object, got %q", ErrInvalidPayload, snippet(body))
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	uid, err := parseUID(p.UID)
	if err != nil {
		return Item{}, err
	}
	it := Item{UID: uid}

	if p.ArtistName != nil {
		it.Artist = normalize.Display(*p.ArtistName)
	}
	if p.SongTitle != nil {
		it.Title = normalize.Display(*p.SongTitle)
	}
	if it.Title == "" && it.Artist == "" {
		// Fall back to the combined "song" field only when both structured
		// fields are absent; it is stored as a title, never split.
		if p.Song != nil {
			it.Title = normalize.Display(*p.Song)
		}
	}
	if it.Title == "" {
		return Item{}, fmt.Errorf("%w: missing songTitle", ErrInvalidPayload)
	}
	if len(it.Title) > maxFieldLen || len(it.Artist) > maxFieldLen {
		return Item{}, fmt.Errorf("%w: artist/title longer than %d bytes", ErrInvalidPayload, maxFieldLen)
	}
	if it.Artist == "" {
		it.Kind = KindSegment
	} else {
		it.Kind = KindMusic
	}

	if p.Date != nil {
		it.RawDate = strings.TrimSpace(*p.Date)
		it.StartAt = parseStart(it.RawDate, loc, observedAt)
	}

	if len(p.DJ) > 0 && p.DJ[0] == '{' {
		var dj djPayload
		if json.Unmarshal(p.DJ, &dj) == nil {
			it.Program = normalize.Display(dj.Program)
			if it.Program == "" {
				it.Program = normalize.Display(dj.DJName)
			}
			if len(it.Program) > maxFieldLen {
				it.Program = ""
			}
		}
	}
	return it, nil
}

func parseUID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	switch raw[0] {
	case '"':
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("%w: uid: %v", ErrInvalidPayload, err)
		}
	default:
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", fmt.Errorf("%w: uid must be a string or number", ErrInvalidPayload)
		}
		s = n.String()
	}
	s = strings.TrimSpace(s)
	if len(s) > 64 {
		return "", fmt.Errorf("%w: uid too long", ErrInvalidPayload)
	}
	return s, nil
}

// Plausibility window for upstream start times relative to observation.
// Long segments (news blocks, shows) can legitimately start well before we
// see them, but a start far in the past suggests a stale or reused record.
const (
	maxStartAge    = 6 * time.Hour
	maxStartFuture = 5 * time.Minute
)

// parseStart converts a local wall-clock string to UTC. During the
// autumn DST transition a wall-clock time occurs twice; the candidate
// closest to the observation time wins. Times that fall in
// the spring-forward gap are normalized by Go and still bounded by the
// plausibility window.
func parseStart(s string, loc *time.Location, observedAt time.Time) *time.Time {
	if s == "" {
		return nil
	}
	wall, err := time.Parse(upstreamDateLayout, s)
	if err != nil {
		return nil
	}
	var best *time.Time
	for _, c := range wallCandidates(wall, loc) {
		if c.After(observedAt.Add(maxStartFuture)) || c.Before(observedAt.Add(-maxStartAge)) {
			continue
		}
		if best == nil || absDur(observedAt.Sub(c)) < absDur(observedAt.Sub(*best)) {
			c := c.UTC()
			best = &c
		}
	}
	return best
}

// wallCandidates returns every instant whose wall clock in loc equals wall.
func wallCandidates(wall time.Time, loc *time.Location) []time.Time {
	base := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, loc)
	var out []time.Time
	for _, shift := range []time.Duration{-time.Hour, 0, time.Hour} {
		c := base.Add(shift)
		y, m, d := c.Date()
		if y == wall.Year() && m == wall.Month() && d == wall.Day() &&
			c.Hour() == wall.Hour() && c.Minute() == wall.Minute() && c.Second() == wall.Second() {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = append(out, base) // nonexistent (spring-forward) time
	}
	return out
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func snippet(b []byte) string {
	const n = 40
	if len(b) > n {
		b = b[:n]
	}
	return string(b)
}
