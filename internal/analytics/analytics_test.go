package analytics

import (
	"errors"
	"testing"
	"time"
)

func vilnius(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Vilnius")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParseRangeDefaultsAndDates(t *testing.T) {
	loc := vilnius(t)
	now := time.Date(2026, 10, 17, 22, 30, 0, 0, time.UTC) // 01:30 on Oct 18 local
	r, err := ParseRange("", "", loc, now, 7)
	if err != nil {
		t.Fatal(err)
	}
	// Today locally is Oct 18 → [Oct 12 00:00, Oct 19 00:00) local (EEST, +3).
	if !r.From.Equal(time.Date(2026, 10, 11, 21, 0, 0, 0, time.UTC)) || !r.To.Equal(time.Date(2026, 10, 18, 21, 0, 0, 0, time.UTC)) {
		t.Fatalf("default range %v .. %v", r.From.UTC(), r.To.UTC())
	}

	r, err = ParseRange("2026-10-01", "2026-11-01", loc, now, 7)
	if err != nil {
		t.Fatal(err)
	}
	// From is in EEST (+3), to in EET (+2).
	if !r.From.Equal(time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC)) || !r.To.Equal(time.Date(2026, 10, 31, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("date range %v .. %v", r.From.UTC(), r.To.UTC())
	}

	// 12:00+02:00 is the same instant as 10:00Z, so the range is empty.
	_, err = ParseRange("2026-10-17T10:00:00Z", "2026-10-17T12:00:00+02:00", loc, now, 7)
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("empty range must be rejected, got %v", err)
	}
	for _, bad := range [][2]string{{"yesterday", ""}, {"2026-10-10", "2026-10-01"}, {"2024-01-01", "2026-01-01"}} {
		if _, err := ParseRange(bad[0], bad[1], loc, now, 7); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("%v: expected ErrInvalidRange, got %v", bad, err)
		}
	}
}

func TestDailySeriesAcrossDSTFallBack(t *testing.T) {
	loc := vilnius(t)
	r, _ := ParseRange("2026-10-24", "2026-10-27", loc, time.Now(), 7)
	pts := Series(nil, r, BucketDay, loc, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), &r.From)
	if len(pts) != 3 {
		t.Fatalf("want 3 days, got %d", len(pts))
	}
	// Oct 25 has 25 hours in Vilnius.
	if d := pts[1].End.Sub(pts[1].Start); d != 25*time.Hour {
		t.Fatalf("fall-back day length = %s", d)
	}
	for _, p := range pts {
		if p.Start.In(loc).Hour() != 0 || p.Partial {
			t.Errorf("bucket %v not aligned to local midnight or partial=%v", p.Start, p.Partial)
		}
	}
}

func TestHourlySeriesAcrossDSTFallBack(t *testing.T) {
	loc := vilnius(t)
	day, _ := ParseRange("2026-10-25", "2026-10-26", loc, time.Now(), 1)
	// One play in each instance of the repeated 03:xx hour.
	plays := []PlayTime{
		{At: time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)}, // 03:30 EEST
		{At: time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)}, // 03:30 EET
	}
	pts := Series(plays, day, BucketHour, loc, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), &day.From)
	if len(pts) != 25 {
		t.Fatalf("want 25 hourly buckets, got %d", len(pts))
	}
	if pts[3].Plays != 1 || pts[4].Plays != 1 || pts[3].Start.Hour() != 3 || pts[4].Start.Hour() != 3 {
		t.Fatalf("repeated hour buckets: %+v / %+v", pts[3], pts[4])
	}
	_, off3 := pts[3].Start.Zone()
	_, off4 := pts[4].Start.Zone()
	if off3 != 3*3600 || off4 != 2*3600 {
		t.Fatalf("offsets %d / %d", off3, off4)
	}

	d := Distribute(plays, loc)
	if d.Hours[3] != 2 || d.WeekdayHours[6][3] != 2 { // Sunday
		t.Fatalf("distribution %+v", d.Hours)
	}
}

func TestSpringForwardDayHas23Hours(t *testing.T) {
	loc := vilnius(t)
	r, _ := ParseRange("2026-03-29", "2026-03-30", loc, time.Now(), 1)
	pts := Series(nil, r, BucketHour, loc, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), &r.From)
	if len(pts) != 23 {
		t.Fatalf("want 23 hourly buckets, got %d", len(pts))
	}
}

func TestSeriesBoundariesAndPartial(t *testing.T) {
	loc := vilnius(t)
	r, _ := ParseRange("2026-10-10", "2026-10-13", loc, time.Now(), 1)
	plays := []PlayTime{
		{At: r.From.Add(-time.Second)}, // before range
		{At: r.From},                   // first instant: included
		{At: r.From.Add(24*time.Hour - time.Nanosecond)},  // end of day 1
		{At: r.From.Add(24 * time.Hour), Uncertain: true}, // start of day 2
		{At: r.To}, // excluded (half-open)
	}
	now := r.From.Add(36 * time.Hour) // midway through day 2
	coverage := r.From.Add(time.Hour)
	pts := Series(plays, r, BucketDay, loc, now, &coverage)
	if pts[0].Plays != 2 || pts[1].Plays != 1 || pts[1].UncertainPlays != 1 || pts[2].Plays != 0 {
		t.Fatalf("counts %+v", pts)
	}
	if !pts[0].Partial || !pts[1].Partial || !pts[2].Partial {
		t.Fatalf("day 1 starts before coverage, day 2 in progress, day 3 future: %+v", pts)
	}
	coverage = r.From
	pts = Series(plays, r, BucketDay, loc, now, &coverage)
	if pts[0].Partial {
		t.Fatal("complete, covered day must not be partial")
	}
}

func TestWeekBucketsStartMonday(t *testing.T) {
	loc := vilnius(t)
	r, _ := ParseRange("2026-10-14", "2026-10-20", loc, time.Now(), 1) // Wed..Mon
	pts := Series(nil, r, BucketWeek, loc, time.Now(), nil)
	if len(pts) != 2 || pts[0].Start.Weekday() != time.Monday || pts[0].Start.Day() != 12 || !pts[0].Partial {
		t.Fatalf("weeks %+v", pts)
	}
}

func TestCompare(t *testing.T) {
	pct := func(c Change) float64 {
		if c.Percent == nil {
			return -999
		}
		return *c.Percent
	}
	cases := []struct {
		cur, prev int
		covered   bool
		state     string
		pct       float64
	}{
		{10, 5, true, "up", 100},
		{5, 10, true, "down", -50},
		{7, 7, true, "flat", 0},
		{3, 0, true, "new", -999},
		{0, 0, true, "none", -999},
		{0, 4, true, "down", -100},
		{5, 0, false, "unavailable", -999},
		{1, 3, true, "down", -66.7},
	}
	for _, c := range cases {
		got := Compare(c.cur, c.prev, c.covered)
		if got.State != c.state || pct(got) != c.pct || got.Delta != c.cur-c.prev {
			t.Errorf("Compare(%d,%d,%v) = %+v (pct %v)", c.cur, c.prev, c.covered, got, pct(got))
		}
	}
}

func TestPeriodsClipToNow(t *testing.T) {
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	r := Range{From: from, To: from.Add(7 * 24 * time.Hour)}
	cur, prev, ok := Periods(r, from.Add(2*24*time.Hour))
	if !ok || cur.Duration() != 48*time.Hour || !prev.To.Equal(from) || prev.Duration() != 48*time.Hour {
		t.Fatalf("cur %+v prev %+v", cur, prev)
	}
	if _, _, ok := Periods(r, from.Add(-time.Hour)); ok {
		t.Fatal("future range has no comparable period")
	}
}

func TestParseBucket(t *testing.T) {
	day := Range{From: time.Unix(0, 0), To: time.Unix(0, 0).Add(24 * time.Hour)}
	year := Range{From: time.Unix(0, 0), To: time.Unix(0, 0).Add(365 * 24 * time.Hour)}
	if b, _ := ParseBucket("", day); b != BucketHour {
		t.Errorf("default for a day = %s", b)
	}
	if b, _ := ParseBucket("", year); b != BucketWeek {
		t.Errorf("default for a year = %s", b)
	}
	if _, err := ParseBucket("hour", year); !errors.Is(err, ErrInvalidRange) {
		t.Error("hourly over a year must be rejected")
	}
	if _, err := ParseBucket("minute", day); !errors.Is(err, ErrInvalidRange) {
		t.Error("unknown bucket must be rejected")
	}
}
