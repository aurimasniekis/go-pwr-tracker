package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
)

// demoCatalog is obviously synthetic so demo data is never mistaken for
// real airplay.
var demoCatalog = []struct{ artist, title string }{
	{"Demo Artist Aurora", "Northern Placeholder"},
	{"Demo Artist Aurora", "Northern Placeholder (Extended Mix)"},
	{"Demo Artist Aurora & The Fixtures", "Synthetic Summer"},
	{"Demo Band Baltic", "Test Pattern"},
	{"Demo Band Baltic", "Loopback"},
	{"Demo DJ Cobalt feat. Sample Singer", "Fake Love"},
	{"Demo DJ Cobalt", "Mock Turtle"},
	{"Demo Duo Delta", "Seed Value"},
	{"Demo Duo Delta", "Deterministic"},
	{"Demo Singer Ėglė", "Žalia Šviesa"},
	{"Demo Singer Ėglė", "Ąžuolų Daina"},
	{"Demo Crew Echo", "Echo Chamber"},
	{"Demo Crew Echo", "Echo Chamber - Radio Edit"},
	{"Demo Project Foxtrot", "Fixture Fever"},
	{"Demo Project Foxtrot", "Null Pointer"},
	{"Demo Artist Gamma", "Golden Snapshot"},
	{"Demo Artist Gamma", "Race Condition"},
	{"Demo Group Helix", "Spiral"},
	{"Demo Group Helix", "Unit Test"},
	{"Demo Artist Iris", "Sandbox"},
}

func demoSeed(args []string) error {
	fs := flag.NewFlagSet("demo-seed", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path of the demo database to create (required; must be empty or new)")
	days := fs.Int("days", 30, "days of synthetic history to generate, ending now")
	seed := fs.Uint64("seed", 42, "random seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return errors.New("-db is required; demo data is never written to DATABASE_PATH implicitly")
	}
	if *days < 1 || *days > 400 {
		return errors.New("-days must be between 1 and 400")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if ps, _, err := st.ListPlays(ctx, store.PlayFilter{IncludeSegments: true}, nil, 1); err != nil {
		return err
	} else if len(ps) > 0 {
		return fmt.Errorf("%s already contains plays; demo-seed only writes to an empty database", *dbPath)
	}

	rng := rand.New(rand.NewPCG(*seed, *seed^0x9e3779b97f4a7c15))
	end := time.Now().UTC().Truncate(time.Minute)
	t := end.AddDate(0, 0, -*days)
	n := 0
	for t.Before(end) {
		var it upstream.Item
		if rng.IntN(12) == 0 {
			it = upstream.Item{Title: "Eteryje - Informacija", Kind: upstream.KindSegment}
		} else {
			// Zipf-like popularity: low indexes play more often.
			idx := int(float64(len(demoCatalog)) * rng.Float64() * rng.Float64())
			c := demoCatalog[idx]
			it = upstream.Item{Artist: c.artist, Title: c.title, Kind: upstream.KindMusic}
		}
		n++
		it.UID = fmt.Sprintf("demo-%d", n)
		start := t
		it.StartAt = &start
		length := time.Duration(150+rng.IntN(90)) * time.Second
		if it.Kind == upstream.KindSegment {
			length = 20 * time.Second
		}
		if _, err := st.RecordObservation(ctx, store.Observation{
			ObservedAt: t.Add(15 * time.Second), Item: it, FetchMode: "demo",
		}, 5*time.Minute); err != nil {
			return err
		}
		t = t.Add(length)
	}
	fmt.Fprintf(os.Stdout, "seeded %d synthetic plays over %d days into %s\n", n, *days, *dbPath)
	return nil
}
