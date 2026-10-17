package upstream

import (
	"errors"
	"os"
	"strings"
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

func TestParseRealMusicFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/music.json")
	if err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 10, 17, 16, 4, 46, 0, time.UTC)
	it, err := Parse(body, vilnius(t), observed)
	if err != nil {
		t.Fatal(err)
	}
	if it.UID != "3086824" {
		t.Errorf("UID = %q", it.UID)
	}
	if it.Artist != "David Guetta & Anne-Marie & Coi Leray" || it.Title != "Baby Don't Hurt Me" {
		t.Errorf("artist/title = %q / %q", it.Artist, it.Title)
	}
	if it.Kind != KindMusic {
		t.Errorf("Kind = %q", it.Kind)
	}
	// 19:04:04 in Vilnius (EEST, UTC+3) on 2026-10-17.
	want := time.Date(2026, 10, 17, 16, 4, 4, 0, time.UTC)
	if it.StartAt == nil || !it.StartAt.Equal(want) {
		t.Errorf("StartAt = %v, want %v", it.StartAt, want)
	}
	if it.RawDate != "2026-10-17 19:04:04" || it.Program != "POWER MUZIKA" {
		t.Errorf("RawDate/Program = %q / %q", it.RawDate, it.Program)
	}
}

func TestParseRealSegmentFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/segment.json")
	if err != nil {
		t.Fatal(err)
	}
	it, err := Parse(body, vilnius(t), time.Date(2026, 10, 17, 16, 0, 6, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	// Segments have an empty artist; the title must not be split on " - ".
	if it.Kind != KindSegment || it.Artist != "" || it.Title != "Eteryje - Informacija" {
		t.Errorf("got %+v", it)
	}
}

func TestParseRejectsInvalidPayloads(t *testing.T) {
	loc := vilnius(t)
	now := time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)
	bad := map[string]string{
		"empty":         "",
		"whitespace":    "   \n",
		"html":          "<html><body>Just a moment...</body></html>",
		"array":         `[{"songTitle":"x"}]`,
		"truncated":     `{"uid":"1","songTitle":"Abc`,
		"no title":      `{"uid":"1","artistName":"Someone"}`,
		"blank title":   `{"uid":"1","artistName":"","songTitle":"   "}`,
		"bad uid":       `{"uid":{"x":1},"songTitle":"x"}`,
		"invalid utf8":  "{\"songTitle\":\"\xff\xfe\"}",
		"title too big": `{"songTitle":"` + strings.Repeat("x", 600) + `"}`,
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(body), loc, now)
			if !errors.Is(err, ErrInvalidPayload) {
				t.Fatalf("want ErrInvalidPayload, got %v", err)
			}
		})
	}
}

func TestParsePartialPayloads(t *testing.T) {
	loc := vilnius(t)
	now := time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)

	it, err := Parse([]byte(`{"songTitle":"Only Title"}`), loc, now)
	if err != nil || it.UID != "" || it.StartAt != nil || it.Kind != KindSegment {
		t.Fatalf("title-only: %+v %v", it, err)
	}

	it, err = Parse([]byte(`{"uid":42,"artistName":"A","songTitle":"T","date":"garbage","dj":""}`), loc, now)
	if err != nil || it.UID != "42" || it.StartAt != nil || it.RawDate != "garbage" {
		t.Fatalf("numeric uid / bad date: %+v %v", it, err)
	}

	// Combined "song" is used only when structured fields are absent.
	it, err = Parse([]byte(`{"song":"A - B"}`), loc, now)
	if err != nil || it.Title != "A - B" || it.Artist != "" {
		t.Fatalf("song fallback: %+v %v", it, err)
	}
}

func TestParseUnicode(t *testing.T) {
	body := []byte("\xef\xbb\xbf" + `{"uid":"9","artistName":"  Jurgis   Didžiulis ","songTitle":"Ž\u0301emyn – Ąžuolų (Remix)","date":"2026-10-17 19:00:00"}`)
	it, err := Parse(body, vilnius(t), time.Date(2026, 10, 17, 16, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if it.Artist != "Jurgis Didžiulis" {
		t.Errorf("Artist = %q", it.Artist)
	}
	if it.Title != "Ž\u0301emyn – Ąžuolų (Remix)" {
		t.Errorf("Title = %q", it.Title)
	}
}

func TestParseStartPlausibility(t *testing.T) {
	loc := vilnius(t)
	now := time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)
	for _, date := range []string{"2026-10-17 10:00:00", "2026-10-17 19:10:00", "2025-01-01 00:00:00"} {
		it, err := Parse([]byte(`{"songTitle":"x","date":"`+date+`"}`), loc, now)
		if err != nil {
			t.Fatal(err)
		}
		if it.StartAt != nil {
			t.Errorf("date %s should be rejected as implausible, got %v", date, it.StartAt)
		}
	}
}

func TestParseStartAcrossDSTFallBack(t *testing.T) {
	loc := vilnius(t)
	// 2026-10-25 03:30 local occurs twice: 00:30Z (EEST) and 01:30Z (EET).
	first := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	second := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)
	cases := []struct {
		observed time.Time
		want     time.Time
	}{
		{first.Add(2 * time.Minute), first},
		{second.Add(2 * time.Minute), second},
	}
	for _, c := range cases {
		got := parseStart("2026-10-25 03:30:00", loc, c.observed)
		if got == nil || !got.Equal(c.want) {
			t.Errorf("observed %v: got %v, want %v", c.observed, got, c.want)
		}
	}
}

func TestParseStartAcrossDSTSpringForward(t *testing.T) {
	loc := vilnius(t)
	// 2026-03-29: 03:00 local jumps to 04:00. 04:15 EEST = 01:15Z.
	got := parseStart("2026-03-29 04:15:00", loc, time.Date(2026, 3, 29, 1, 16, 0, 0, time.UTC))
	if got == nil || !got.Equal(time.Date(2026, 3, 29, 1, 15, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
	// 02:59 EET = 00:59Z, just before the gap.
	got = parseStart("2026-03-29 02:59:00", loc, time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC))
	if got == nil || !got.Equal(time.Date(2026, 3, 29, 0, 59, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}
