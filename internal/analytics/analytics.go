// Package analytics implements timezone-aware range parsing, bucketing and
// period comparison. Plays are stored in UTC; every calendar notion (day,
// hour, weekday) is evaluated in the configured IANA timezone so that
// daylight-saving transitions produce 23- and 25-hour days rather than
// shifted buckets.
package analytics

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// MaxRange bounds analytics queries.
const MaxRange = 400 * 24 * time.Hour

// Range is a half-open interval [From, To) of instants.
type Range struct {
	From, To time.Time
}

func (r Range) Duration() time.Duration { return r.To.Sub(r.From) }

var ErrInvalidRange = errors.New("invalid range")

// ParseTime accepts an RFC 3339 timestamp or a calendar date (YYYY-MM-DD),
// which is interpreted as local midnight in loc.
func ParseTime(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if d, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return d, nil
	}
	return time.Time{}, fmt.Errorf("%w: %q is neither RFC 3339 nor YYYY-MM-DD", ErrInvalidRange, s)
}

// ParseRange parses optional from/to parameters. Missing values default to
// the last defaultDays local calendar days including today: [today-(n-1), tomorrow).
func ParseRange(from, to string, loc *time.Location, now time.Time, defaultDays int) (Range, error) {
	var r Range
	var err error
	if to == "" {
		r.To = StartOfDay(now, loc).AddDate(0, 0, 1)
	} else if r.To, err = ParseTime(to, loc); err != nil {
		return r, err
	}
	if from == "" {
		r.From = StartOfDay(r.To.Add(-time.Nanosecond), loc).AddDate(0, 0, -(defaultDays - 1))
	} else if r.From, err = ParseTime(from, loc); err != nil {
		return r, err
	}
	if !r.From.Before(r.To) {
		return r, fmt.Errorf("%w: from must be before to", ErrInvalidRange)
	}
	if r.Duration() > MaxRange {
		return r, fmt.Errorf("%w: range may not exceed %d days", ErrInvalidRange, int(MaxRange.Hours()/24))
	}
	return r, nil
}

// StartOfDay returns local midnight of t's calendar day in loc.
func StartOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

type Bucket string

const (
	BucketHour Bucket = "hour"
	BucketDay  Bucket = "day"
	BucketWeek Bucket = "week"
)

// Bucket limits keep series sizes reasonable.
const (
	maxHourRange = 31 * 24 * time.Hour
	maxDayRange  = MaxRange
)

// ParseBucket validates a bucket name for r, choosing a sensible default
// when empty.
func ParseBucket(s string, r Range) (Bucket, error) {
	switch Bucket(s) {
	case "":
		switch d := r.Duration(); {
		case d <= 3*24*time.Hour:
			return BucketHour, nil
		case d <= 120*24*time.Hour:
			return BucketDay, nil
		default:
			return BucketWeek, nil
		}
	case BucketHour:
		if r.Duration() > maxHourRange {
			return "", fmt.Errorf("%w: hourly buckets are limited to 31 days", ErrInvalidRange)
		}
		return BucketHour, nil
	case BucketDay:
		if r.Duration() > maxDayRange {
			return "", fmt.Errorf("%w: range too long for daily buckets", ErrInvalidRange)
		}
		return BucketDay, nil
	case BucketWeek:
		return BucketWeek, nil
	}
	return "", fmt.Errorf("%w: bucket must be hour, day or week", ErrInvalidRange)
}

// floor returns the start of the bucket containing t.
func floor(t time.Time, b Bucket, loc *time.Location) time.Time {
	lt := t.In(loc)
	switch b {
	case BucketHour:
		// Subtracting the local minutes/seconds is correct even in the
		// repeated hour of a DST fall-back, where time.Date is ambiguous.
		return t.Add(-time.Duration(lt.Minute())*time.Minute - time.Duration(lt.Second())*time.Second - time.Duration(lt.Nanosecond()))
	case BucketWeek:
		day := StartOfDay(t, loc)
		offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
		return day.AddDate(0, 0, -offset)
	default:
		return StartOfDay(t, loc)
	}
}

func next(t time.Time, b Bucket) time.Time {
	switch b {
	case BucketHour:
		return t.Add(time.Hour)
	case BucketWeek:
		return t.AddDate(0, 0, 7)
	default:
		return t.AddDate(0, 0, 1)
	}
}

// Point is one bucket of an airplay series. Start/End are rendered in the
// analytics timezone. Partial is true when the bucket is not fully inside
// the requested range, has not fully elapsed, or begins before collection
// started; its count is therefore not comparable with complete buckets.
type Point struct {
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	Plays          int       `json:"plays"`
	UncertainPlays int       `json:"uncertain_plays"`
	Partial        bool      `json:"partial"`
}

// PlayTime is a play start instant with its uncertainty flag.
type PlayTime struct {
	At        time.Time
	Uncertain bool
}

// Series buckets plays into consecutive buckets covering r. coverageStart
// is when collection began (nil if never).
func Series(plays []PlayTime, r Range, b Bucket, loc *time.Location, now time.Time, coverageStart *time.Time) []Point {
	var pts []Point
	for s := floor(r.From, b, loc); s.Before(r.To); s = next(s, b) {
		e := next(s, b)
		partial := s.Before(r.From) || e.After(r.To) || e.After(now) || coverageStart == nil || s.Before(*coverageStart)
		pts = append(pts, Point{Start: s.In(loc), End: e.In(loc), Partial: partial})
	}
	i := 0
	for _, p := range plays {
		if p.At.Before(r.From) || !p.At.Before(r.To) {
			continue
		}
		for i < len(pts) && !p.At.Before(pts[i].End) {
			i++
		}
		if i == len(pts) {
			break
		}
		pts[i].Plays++
		if p.Uncertain {
			pts[i].UncertainPlays++
		}
	}
	return pts
}

// Distribution counts plays by local hour of day and by weekday × hour.
// Weekdays are indexed Monday = 0 … Sunday = 6. On DST fall-back days the
// repeated local hour accumulates two real hours of plays, and on
// spring-forward days one local hour has none; this is inherent to
// wall-clock distributions.
type Distribution struct {
	Hours        [24]int    `json:"hours"`
	WeekdayHours [7][24]int `json:"weekday_hours"`
}

func Distribute(plays []PlayTime, loc *time.Location) Distribution {
	var d Distribution
	for _, p := range plays {
		lt := p.At.In(loc)
		h := lt.Hour()
		wd := (int(lt.Weekday()) + 6) % 7
		d.Hours[h]++
		d.WeekdayHours[wd][h]++
	}
	return d
}

// Change compares a current count with the preceding period.
type Change struct {
	Previous int `json:"previous"`
	Delta    int `json:"delta"`
	// State is one of:
	//   up, down, flat – both periods have data; Percent is set
	//   new            – previous period had zero plays; Percent is null
	//   none           – zero plays in both periods
	//   unavailable    – the previous period is not fully covered by
	//                    collection, so no comparison is made
	State   string   `json:"state"`
	Percent *float64 `json:"percent"`
}

func Compare(current, previous int, previousCovered bool) Change {
	c := Change{Previous: previous, Delta: current - previous}
	switch {
	case !previousCovered:
		c.State = "unavailable"
	case previous == 0 && current == 0:
		c.State = "none"
	case previous == 0:
		c.State = "new"
	default:
		pct := math.Round(float64(current-previous)/float64(previous)*1000) / 10
		c.Percent = &pct
		switch {
		case current > previous:
			c.State = "up"
		case current < previous:
			c.State = "down"
		default:
			c.State = "flat"
		}
	}
	return c
}

// Periods returns the comparable current period (clipped to now so an
// in-progress period is not compared with a complete one) and the
// immediately preceding period of equal length.
func Periods(r Range, now time.Time) (current, previous Range, ok bool) {
	end := r.To
	if now.Before(end) {
		end = now
	}
	if !end.After(r.From) {
		return Range{}, Range{}, false
	}
	current = Range{From: r.From, To: end}
	previous = Range{From: r.From.Add(-current.Duration()), To: r.From}
	return current, previous, true
}

// Covered reports whether collection had started by t.
func Covered(t time.Time, coverageStart *time.Time) bool {
	return coverageStart != nil && !coverageStart.After(t)
}
